package domains

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/permission"
	"github.com/xujian519/mady/agentcore/worker"
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/domains/checker"
	"github.com/xujian519/mady/domains/claimdrafting"
	"github.com/xujian519/mady/domains/doctmpl"
	"github.com/xujian519/mady/domains/enablement"
	"github.com/xujian519/mady/domains/infringement"
	"github.com/xujian519/mady/domains/inventiveness"
	"github.com/xujian519/mady/domains/novelty"
	"github.com/xujian519/mady/domains/provisions"
	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/domains/rules"
	"github.com/xujian519/mady/domains/specdrafting"
	"github.com/xujian519/mady/domains/workflows/design"
	"github.com/xujian519/mady/domains/workflows/patent"
	"github.com/xujian519/mady/domains/writing"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/psychological"
	"github.com/xujian519/mady/retrieval/domain"
	"github.com/xujian519/mady/tools"
)

// globalDraftingRunner 是 FiveStepRunner 的全局实例，由 SetupPatentDraftingEngine
// 在启动期一次性注入，PatentAgentConfig 从中读取并注册为工具。
// 使用全局而非参数传递的原因是 PatentAgentConfig 签名受 domainFactoryMap
// 约束（func(agentcore.Config) agentcore.Config），无法添加额外参数。
//
// 并发安全：setupFrameworkContext 是单线程的，写入后所有读取都是并发安全的。
var globalDraftingRunner *reasoning.FiveStepRunner

// SetupPatentDraftingEngine 在启动期注入五步推理引擎实例，
// 使 PatentAgentConfig 可以将 run_five_step_workflow 工具注册到所有
// Patent Agent 实例中（包括 Router Handoff 创建的子 Agent）。
//
// retriever 和 llm 均可为 nil——retriever 为 nil 时 Stage ② 跳过；
// llm 为 nil 时降级为 noop 节点（仅回显步骤描述，不做 LLM 分析）。
// 必须在任何 Agent 创建前调用。
func SetupPatentDraftingEngine(retriever *reasoning.MultiSourceRetriever, llm reasoning.LlmClient) {
	globalDraftingRunner = reasoning.NewWorkflowRunner(
		"patent-agent", reasoning.CaseDrafting, "", retriever, llm,
	)
}

// injectDraftingTool 向 Agent 配置注册 run_five_step_workflow 工具。
// 当 globalDraftingRunner 未配置（nil）时静默跳过，不影响现有行为。
func injectDraftingTool(cfg *agentcore.Config) {
	if globalDraftingRunner != nil {
		cfg.Tools = append(cfg.Tools, reasoning.AsWorkflowTool(globalDraftingRunner))
	}
}

// globalTemplateStore 是 TemplateStore 的全局实例，由 SetupDocTemplateStore
// 在启动期注入。遵循与 globalDraftingRunner 一致的模式。
var globalTemplateStore *doctmpl.TemplateStore

// globalPatentRetriever 是专利领域检索器的全局实例，由 SetupPatentRetriever
// 在启动期注入。PatentAgentConfig 构造 analyze_patent_novelty 工具时传入，
// 使 search 节点能进行真实现有技术检索。
var globalPatentRetriever domain.DomainRetriever

// globalKnowledgeExt 是知识库扩展的全局实例，由 SetupKnowledgeExtension
// 在启动期注入。PatentAgentConfig 和 LegalAgentConfig 将其注册到 Extensions 中，
// 使子 Agent 拥有 search_knowledge / search_laws 工具。
// 遵循与 globalDraftingRunner 一致的全局注入模式（签名受 domainFactoryMap 约束）。
var globalKnowledgeExt agentcore.Extension

// SetupKnowledgeExtension 在启动期注入知识库扩展实例，
// 使 PatentAgentConfig 和 LegalAgentConfig 可以将 search_knowledge /
// search_laws 工具注册到所有 Agent 实例中（包括 Handoff 子 Agent）。
// ext 为 nil 时静默跳过，不影响现有行为。
// 必须在任何 Agent 创建前调用。
func SetupKnowledgeExtension(ext agentcore.Extension) {
	globalKnowledgeExt = ext
}

// globalInfringementKR 是侵权分析知识检索器的全局实例，由
// SetupInfringementKnowledgeRetriever 在启动期注入（通过 framework 层适配）。
// 检索器提供审查指南、类案和法律条文的检索能力，增强侵权分析的准确性。
// 当知识库不可用时为 nil，侵权分析降级为纯 LLM 知识。
var globalInfringementKR infringement.KnowledgeRetriever

// SetupInfringementKnowledgeRetriever 在启动期注入侵权分析知识检索器实例。
// kr 由 pkg/framework.NewInfringementKnowledgeRetriever 从知识库扩展适配生成。
// kr 为 nil 时静默跳过，不影响现有行为。
// 必须在任何 Agent 创建前调用。
func SetupInfringementKnowledgeRetriever(kr infringement.KnowledgeRetriever) {
	globalInfringementKR = kr
}

// globalEvidenceExt 是证据判断扩展的全局实例，由 SetupEvidenceExtension
// 在启动期注入。PatentAgentConfig 和 LegalAgentConfig 将其注册到 Extensions 中，
// 使 Agent 拥有证据三性审查、举证责任查询、证明标准评估等工具。
var globalEvidenceExt agentcore.Extension

// SetupEvidenceExtension 在启动期注入证据判断扩展实例，
// 使 PatentAgentConfig 和 LegalAgentConfig 可以将证据判断工具注册到 Agent 中。
// ext 为 nil 时静默跳过，不影响现有行为。
// 必须在任何 Agent 创建前调用。
func SetupEvidenceExtension(ext agentcore.Extension) {
	globalEvidenceExt = ext
}

// globalClaimDraftingExt 是权利要求撰写扩展的全局实例，由 SetupClaimDraftingExtension
// 在启动期注入。PatentAgentConfig 将其注册到 Extensions 中，使 Patent Agent
// 拥有 draft_claims 工具（五步法 builder + 规则引擎校验 + 可选 LLM 增强）。
var globalClaimDraftingExt agentcore.Extension

// globalSpecDraftingExt 是说明书撰写扩展的全局实例，由 SetupSpecDraftingExtension
// 在启动期注入。PatentAgentConfig 将其注册到 Extensions 中，使 Patent Agent
// 拥有 draft_specification / validate_specification 工具（12 节点 Pregel 图引擎
// + 16 条校验规则 + 评分器）。
var globalSpecDraftingExt agentcore.Extension

// globalRuleExt 是规则引擎扩展的全局实例，由 SetupRulesExtension 在启动期注入。
// PatentAgentConfig / LegalAgentConfig 将其注册到 Extensions 中，使 Agent 拥有 query_rules 工具。
var globalRuleExt agentcore.Extension

// globalWritingExt 是写作模式扩展的全局实例，由 SetupWritingExtension 在启动期注入。
// PatentAgentConfig / LegalAgentConfig 将其注册到 Extensions 中，使 Agent 拥有 query_writing_patterns 工具。
var globalWritingExt agentcore.Extension

// SetupClaimDraftingExtension 在启动期构造并注入权利要求撰写扩展。
// provider 非 nil 时启用 LLM 增强（通过 ProviderAdapter 桥接）。
// 必须在任何 Agent 创建前调用。
func SetupClaimDraftingExtension(provider agentcore.Provider, model string) {
	engine := claimdrafting.NewRuleEngine()
	claimdrafting.RegisterDefaultRules(engine)
	ext, err := claimdrafting.NewExtension(engine)
	if err != nil {
		slog.Error("claimdrafting: 创建扩展失败", "err", err)
		return
	}
	if provider != nil {
		adapter := claimdrafting.NewProviderAdapter(provider, model)
		// Pass a default builder so drafter's fallback path doesn't NPE.
		ext.SetDrafter(claimdrafting.NewLLMDrafter(adapter, claimdrafting.NewClaimBuilder("", ""), engine))
	}
	globalClaimDraftingExt = ext
}

// SetupSpecDraftingExtension 在启动期构造并注入说明书撰写扩展。
// provider 非 nil 时启用 12 节点 Pregel 图引擎（LLM 逐节点撰写）。
// 必须在任何 Agent 创建前调用。
func SetupSpecDraftingExtension(provider agentcore.Provider) {
	engine := specdrafting.NewRuleEngine()
	specdrafting.RegisterDefaultRules(engine)
	ext := specdrafting.NewExtension(engine)
	if provider != nil {
		ext.SetDrafter(specdrafting.NewLLMDrafter(provider, nil))
	}
	globalSpecDraftingExt = ext
}

// SetupPatentRetriever 在启动期注入专利领域检索器实例，
// 使 PatentAgentConfig 可以将检索能力注入 analyze_patent_novelty 工具。
// retriever 可为 nil——nil 时 search 节点返回占位结果，保持向后兼容。
func SetupPatentRetriever(r domain.DomainRetriever) {
	globalPatentRetriever = r
}

// GetPatentRetriever 返回已注入的全局专利检索器，供 CLI/Server 等入口复用。
func GetPatentRetriever() domain.DomainRetriever {
	return globalPatentRetriever
}

// SetupDocTemplateStore 在启动期注入模板仓库实例，使 PatentAgentConfig 和
// LegalAgentConfig 可以将文档模板工具注册到所有 Agent 实例中。
// 必须在任何 Agent 创建前调用。
func SetupDocTemplateStore(store *doctmpl.TemplateStore) {
	globalTemplateStore = store
}

// SetupRulesExtension 在启动期注入规则引擎扩展实例。
// engine 为 nil 时静默跳过，不影响现有行为。
func SetupRulesExtension(engine *rules.Engine) {
	if engine != nil {
		globalRuleExt = rules.NewExtension(engine)
	}
}

// SetupWritingExtension 在启动期注入写作模式扩展实例。
// store 为 nil 时静默跳过。
func SetupWritingExtension(store *writing.PatternStore) {
	if store != nil {
		globalWritingExt = writing.NewExtension(store)
	}
}

// injectDocTemplateTools 向 Agent 配置注册文档模板相关工具。
func injectDocTemplateTools(cfg *agentcore.Config) {
	if globalTemplateStore != nil {
		cfg.Tools = append(cfg.Tools,
			doctmpl.NewListDocTemplatesTool(globalTemplateStore),
			doctmpl.NewRenderDocTemplateTool(globalTemplateStore),
		)
	}
}

// injectRulesTools 向 Agent 配置注册 query_rules 工具。
func injectRulesTools(cfg *agentcore.Config) {
	if globalRuleExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalRuleExt)
	}
}

// injectWritingTools 向 Agent 配置注册 query_writing_patterns 和 list_writing_patterns 工具。
func injectWritingTools(cfg *agentcore.Config) {
	if globalWritingExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalWritingExt)
	}
}

// PatentAgentConfig builds the patent domain Agent configuration.
func PatentAgentConfig(base agentcore.Config) agentcore.Config {
	cfg := base
	cfg.Name = "patent-agent"

	cfg.SystemPrompt = strings.Join([]string{
		"你是 Mady 的专利代理与知识产权分析模块。",
		"用简体中文回复，专业严谨。",
		styleInjection("patent"),
		"",
		"## 工具链优先原则",
		"以下专利事务有专用编排工具链，必须优先调用 run_orchestration（而非逐个调用子工具或手写）：",
		"- 答复审查意见 → run_orchestration(case_type=\"oa_response\")",
		"- 驳回复审 → run_orchestration(case_type=\"re_examination\")",
		"- 无效宣告分析 → run_orchestration(case_type=\"invalidation\")",
		"- 撰写专利全文 → run_orchestration(case_type=\"patent_drafting\")",
		"",
		"run_orchestration 内部自动串联：法规检索 → 驳回分析 → 条件分支调用专业工具 → 文档起草 → 合规检查。",
		"各专业工具包括：draft_claims（权利要求书）、draft_specification（说明书）、analyze_enablement（26.3）、analyze_inventiveness（创造性）、analyze_patent_novelty（新颖性）、validate_amendment（A33修改检查）、draft_oa_response（答复书）、analyze_slop（套话检查）等。",
		"",
		"## 五步工作法（降级方案）",
		"当无匹配的编排工具链时，使用通用五步工作法：",
		"1. 发现事实 — 了解发明内容、技术领域、申请人需求",
		"2. 获取规则 — 使用 web_search / web_fetch 检索相关专利法规、审查指南、现有技术；使用 scholar_search 检索学术论文（现有技术）；使用 search_knowledge / search_laws 检索本地知识库中的法律法规和案例",
		"3. 规划 — 制定检索策略或申请方案",
		"4. 执行 — 使用 patent_lookup 查询专利元数据；撰写权利要求时，必须调用 draft_claims 工具（禁止手写）；撰写说明书时，必须调用 draft_specification 工具（禁止手写）；分析专利性时调用对应分析工具",
		"5. 检查 — 验证检索完整性、分析准确性",
		"",
		"## 条款智能体（专业分析委派）",
		"对专利法各条款的专项分析，可使用 transfer_to_provision-* 工具委派给对应的条款智能体：",
		"- transfer_to_provision-novelty → 新颖性分析（A22.2）",
		"- transfer_to_provision-inventiveness → 创造性分析（A22.3）",
		"- transfer_to_provision-utility → 实用性分析（A22.4）",
		"- transfer_to_provision-eligibility → 保护客体分析（A25/A5）",
		"- transfer_to_provision-disclosure → 充分公开分析（A26.3）",
		"- transfer_to_provision-claims-clarity → 清楚支持分析（A26.4）",
		"- transfer_to_provision-amendment → 修改超范围分析（A33）",
		"- transfer_to_provision-prior-art → 现有技术认定",
		"- transfer_to_provision-drafting-claims → 权利要求书撰写",
		"对跨条款的推理步骤，可使用 transfer_to_reasoning-* 委派给对应的推理模式。",
		"委派完成后直接使用结果，不需要解释切换过程。",
		"",
		"## 专利编排器",
		"对需要完整流程编排的专利分析任务（从条款分析到质量复核），可使用 transfer_to_patent-orchestrator 委派给专利编排器。",
		"编排器会自动识别涉及的法条、委派条款智能体、运行检查器复核，并输出综合报告。",
		"",
		"涉及专利性判断的输出附以下声明：",
		"「本分析由 AI 辅助生成，不构成正式法律意见。」",
		"",
		"输出格式：完成任务后，用以下 JSON 格式返回结果（便于 Chat Agent 解释给用户）：",
		`{"action":"做了什么","result":"结果摘要","success":true}`,
		"- action: 你做了什么操作",
		"- result: 结果的简洁摘要",
		"- success: 是否成功完成",
	}, "\n")

	// Tools extension — patent agent needs file tools for document analysis.
	// WorkingDir 从 base.ProjectDir 透传（用户当前项目文件夹），
	// 回退到 base.WorkspaceDir（~/.mady/workspace）。
	workingDir := base.ProjectDir
	if workingDir == "" {
		workingDir = base.WorkspaceDir
	}
	allowRead, allowWrite := BuildSandboxAllowLists()

	// infringement tool returns (tool, error) — capture error before slice literal.
	// globalInfringementKR 由 framework.Setup → SetupInfringementKnowledgeRetriever 注入。
	infTool, infErr := infringement.NewInfringementTool(base.Provider, nil, globalInfringementKR)
	if infErr != nil {
		slog.Warn("patent: infringement tool not available, skipping", "err", infErr)
	}

	toolExt := tools.NewExtension(tools.ExtensionConfig{
		WorkingDir:     workingDir,
		SandboxEnabled: true,
		AllowRead:      allowRead,
		AllowWrite:     allowWrite,
		Vision: &tools.VisionToolConfig{
			Provider: base.Provider,
			Model:    base.Model,
		},
		WebSearch:  &tools.WebSearchToolConfig{},
		WebFetch:   &tools.WebFetchToolConfig{},
		PatentTool: tools.PatentToolConfigDefaults(),
		Pandoc:     tools.PandocToolConfigDefaults(),
		DisableTools: []string{
			tools.ToolBash, tools.ToolGitStatus, tools.ToolGitDiff, tools.ToolGitLog,
			tools.ToolBrowser, tools.ToolExecuteCode,
		},
		MaxBytes: 100 * 1024,
		ExtraTools: []*agentcore.Tool{
			patent.NewPatentNoveltyTool(patent.WithRetriever(globalPatentRetriever)),
			patent.NewOAResponseTool(patent.WithOAProvider(base.Provider)),
			patent.NewDebateTool(),
			patent.NewInvalidationTool(patent.WithInvRetriever(globalPatentRetriever)),
			infTool,
			patent.NewReexaminationTool(),
			enablement.NewEnablementTool(enablement.WithProvider(base.Provider)),
			inventiveness.NewInventivenessTool(inventiveness.WithProvider(base.Provider)),
			novelty.NewNoveltyTool(novelty.WithProvider(base.Provider)),
			design.NewDesignInvalidationTool(),
			disclosure.NewDisclosureTool(base.Provider),
			tools.NewPatentEvalTool(nil),
			provisions.NewResolveDomainWorkersTool(""),
		},
	})
	cfg.Extensions = append(cfg.Extensions, toolExt,
		// 心理引擎 — 专利领域：VAD/OCC 语气调整 + 认知扭曲诊断（专利分析需要完整心理评估）。
		psychological.NewExtension(PatentPsychConfig()),
	)

	injectDraftingTool(&cfg)
	injectDocTemplateTools(&cfg)
	injectPromptTools(&cfg)
	injectWritingTools(&cfg)
	injectRulesTools(&cfg)

	// 知识库扩展：为专利 Agent 提供 search_knowledge / search_laws 工具，
	// 使其能检索本地知识库中的法律法规、司法解释和案例。
	// 之前仅在会话级 extendConfig 注入到顶层 Chat Agent，Handoff 子 Agent 无法获得。
	if globalKnowledgeExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalKnowledgeExt)
	}

	// 证据判断扩展：使专利 Agent 拥有证据三性审查、举证责任查询、证明标准评估等工具。
	if globalEvidenceExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalEvidenceExt)
	}

	// 权利要求书撰写扩展（claimdrafting）：五步法 builder + 六类规则校验 + 可选 LLM 增强。
	if globalClaimDraftingExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalClaimDraftingExt)
	}

	// Checker 复核扩展：提供 suggest_checkers / run_checker_review 工具。
	cfg.Extensions = append(cfg.Extensions, checker.NewExtension(nil))

	// 说明书撰写扩展（specdrafting）：12 节点 Pregel 图引擎 + 16 条校验规则 + 评分器，
	// 替代旧的 workflows/patent.NewSpecificationTool（简单 workflow 版）。
	if globalSpecDraftingExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalSpecDraftingExt)
	}

	// Orchestration 扩展：使 PatentAgent 拥有 run_orchestration 工具。
	// 此扩展在 Init 阶段捕获 Agent 引用并注册工具（见 orchestration_extension.go），
	// 确保 handoff 子 Agent 也能调用 run_orchestration 编排链。
	cfg.Extensions = append(cfg.Extensions, &OrchestrationExtension{})

	// Chunked context engine for long patent documents.
	cfg.Engine = "chunked"

	// DoomLoop: 死循环检测器，监控工具调用循环、重复文本、空结果等异常。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, defaultDoomLoopHook())

	// ReasoningStrategy: 专利分析通常需要结构化分析或验证式推理，
	// 因此注入策略提示，根据问题复杂度自动选择合适推理方式。
	// Gateway 统一决策入口：一次分类驱动 effort/策略/模型回退/预算钳制。
	patentGateway := newDefaultGateway(cfg)
	cfg.FallbackRouter = patentGateway.Fallback
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, patentGateway)

	// 法条引用核验 Gate（P2b Strict）：命中疑点追加存疑提示 +
	// citation_verify 留痕 + SuppressPersist（未人工复核不入库）。
	// 知识源与留痕 store 由装配侧注入（citation_wiring.go）。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, newCitationGate(DomainPatent, ""))

	// Guardrail: LevelStrict with patent disclaimer + approval gate.
	// Create a shared DeferredPersistQueue so suppressed messages are
	// not silently dropped — they are either committed (on approval) or
	// discarded (on rejection) when the human responds.
	patentDQ := guardrails.NewDeferredPersistQueue()
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewIFaceLifecycleHook(guardrails.New(
			guardrails.WithLevel(guardrails.LevelStrict),
			guardrails.WithDisclaimer(guardrails.DisclaimerPatent),
			guardrails.WithRiskKeywords(guardrails.RiskKeywordsFor("patent")),
			guardrails.WithApproval(guardrails.ApprovalKeywordsFor("patent")),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
			guardrails.WithDeferredQueue(patentDQ),
		)),
	)

	// Human approval gate for critical decisions.
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		NewApprovalGate(ApprovalConfig{
			RequireApprovalFor: guardrails.ApprovalKeywordsFor("patent"),
		},
			WithDeferredPersist(&DeferredPersistFuncs{
				CommitFn: func() {
					for _, idx := range patentDQ.Pending() {
						patentDQ.Commit(idx)
					}
				},
				DiscardFn: func() {
					for _, idx := range patentDQ.Pending() {
						patentDQ.Discard(idx)
					}
				},
			}),
		),
	)

	// Worker-driven tool registration (MADY_WORKER_ENABLED=1 gate).
	// 将 DefaultWorkers 中 Reasoning + Domain + Checker 层的 Worker 注册为 Agent 工具。
	// 默认使用 LLM 模式（需要 Provider）；Work 层（写作类）暂由专用 drafting extension 覆盖。
	if base.Provider != nil {
		llmClient := reasoning.NewLlmClientFromProvider(base.Provider, base.Model)
		if llmClient != nil {
			// 适配 reasoning.LlmClient → worker 的 LLM 函数签名
			llmFn := func(ctx context.Context, prompt string) (string, error) {
				return llmClient.Chat(ctx, []reasoning.LlmMessage{
					{Role: "user", Content: prompt},
				})
			}
			cfg.Extensions = append(cfg.Extensions,
				worker.RegisterDefaultLLMWorkers(llmFn),
			)
		}
	}

	// 条款智能体 Handoff 注册：从 Manifest 加载 Tier A 条款智能体和
	// Tier B 推理模式，注册为 PatentAgent 内不可见的 Handoff 子 Agent。
	// 专利 Agent 可通过 transfer_to_provision-* 工具将专业条款分析任务
	// 委派给对应条款智能体。
	manifest := provisions.LoadManifestOrDefault("")
	provisions.RegisterProvisionHandoffsFromManifest(&cfg, manifest)

	// 专利编排器 Handoff 注册（对用户可见）：专利业务总调度入口。
	// 编排器整合了意图识别 → 条款委派 → 质量复核的完整流程。
	// 用户可以通过 transfer_to_patent-orchestrator 发起端到端的专利分析任务。
	cfg.Handoffs = append(cfg.Handoffs, provisions.OrchestratorHandoffConfig(manifest, base))

	return cfg
}

// BuildProjectAgent 为案件动态构建专利/法律 Agent。
//
// 这是 v2 设计的关键工厂函数——每个案件获得独立的 Agent 实例，
// WorkingDir 设为案件真实文件夹（RootPath），System Prompt 注入案件元数据。
// 不同于 PatentAgentConfig 的静态配置，此函数生成的 Agent 具备案件感知能力。
func BuildProjectAgent(rec ProjectRecord, base agentcore.Config) agentcore.Config {
	cfg := base
	cfg.Name = fmt.Sprintf("patent-agent-%s", rec.ProjectID)

	// 动态 System Prompt：注入案件上下文
	cfg.SystemPrompt = buildProjectSystemPrompt(rec)
	cfg.ProjectDir = rec.RootPath

	// 权限门控：写入工具需确认，只读工具自动放行。
	// 如果 TUI 已注入带交互式 Approver 的 PermissionExtension，此处跳过。
	if !hasExtensionNamed(cfg.Extensions, permission.ExtensionName) {
		cfg.Extensions = append(cfg.Extensions,
			permission.NewExtension(permission.ProjectAgentPolicy(), nil))
	}

	// 动态 WorkingDir = 案件真实文件夹，沙箱约束在此边界内
	allowRead, allowWrite := BuildSandboxAllowLists()
	toolExt := tools.NewExtension(tools.ExtensionConfig{
		WorkingDir:     rec.RootPath,
		EnabledTools:   []string{"read", "write_file", "edit", "grep", "find", "glob", "ls"},
		SandboxEnabled: true,
		AllowRead:      allowRead,
		AllowWrite:     allowWrite,
		Vision: &tools.VisionToolConfig{
			Provider: base.Provider,
			Model:    base.Model,
		},
		Pandoc:   tools.PandocToolConfigDefaults(),
		MaxBytes: 100 * 1024,
	})
	cfg.Extensions = append(cfg.Extensions, toolExt)

	injectDraftingTool(&cfg)
	injectDocTemplateTools(&cfg)
	injectPromptTools(&cfg)
	injectWritingTools(&cfg)
	injectRulesTools(&cfg)

	// 知识库扩展：使项目级 Agent 具备 search_knowledge / search_laws 工具，
	// 能检索本地知识库中的法律法规、司法解释和案例。
	if globalKnowledgeExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalKnowledgeExt)
	}

	// 证据判断扩展：使专利 Agent 拥有证据三性审查、举证责任查询、证明标准评估等工具。
	if globalEvidenceExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalEvidenceExt)
	}

	// 权利要求书撰写扩展（claimdrafting）：五步法 builder + 六类规则校验 + 可选 LLM 增强。
	if globalClaimDraftingExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalClaimDraftingExt)
	}

	// Checker 复核扩展：提供 suggest_checkers / run_checker_review 工具。
	cfg.Extensions = append(cfg.Extensions, checker.NewExtension(nil))

	// 说明书撰写扩展（specdrafting）：12 节点 Pregel 图引擎 + 16 条校验规则 + 评分器，
	// 替代旧的 workflows/patent.NewSpecificationTool（简单 workflow 版）。
	if globalSpecDraftingExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalSpecDraftingExt)
	}

	// Orchestration 扩展：使 PatentAgent 拥有 run_orchestration 工具。
	// 此扩展在 Init 阶段捕获 Agent 引用并注册工具（见 orchestration_extension.go），
	// 确保 handoff 子 Agent 也能调用 run_orchestration 编排链。
	cfg.Extensions = append(cfg.Extensions, &OrchestrationExtension{})

	// Chunked context engine for long patent/legal documents.
	if base.Engine == "" {
		cfg.Engine = "chunked"
	}

	// DoomLoop: 死循环检测器，监控工具调用循环、重复文本、空结果等异常。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, defaultDoomLoopHook())

	// ReasoningStrategy: 项目级别 Agent 同样需要结构化推理策略。
	// Gateway 统一决策入口：一次分类驱动 effort/策略/模型回退/预算钳制。
	projectGateway := newDefaultGateway(cfg)
	cfg.FallbackRouter = projectGateway.Fallback
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, projectGateway)

	// 法条引用核验 Gate（P1b）：案件答案同样纳入引用核验。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewIFaceLifecycleHook(guardrails.NewCitationGate(guardrails.WithCitationGateLevel(guardrails.LevelStandard))),
	)

	// LevelStrict 护栏 + 人工审批门
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewIFaceLifecycleHook(guardrails.New(
			guardrails.WithLevel(guardrails.LevelStrict),
			guardrails.WithDisclaimer(guardrails.DisclaimerPatent),
			guardrails.WithRiskKeywords(guardrails.RiskKeywordsFor("patent")),
			guardrails.WithApproval(guardrails.ApprovalKeywordsFor("patent")),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		)),
	)
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		NewApprovalGate(ApprovalConfig{
			RequireApprovalFor: guardrails.ApprovalKeywordsFor("patent"),
		}),
	)

	// 条款智能体 Handoff 注册：使项目 Agent 也拥有条款智能体委派能力。
	manifest := provisions.LoadManifestOrDefault("")
	provisions.RegisterProvisionHandoffsFromManifest(&cfg, manifest)

	return cfg
}

// hasExtensionNamed reports whether cfg.Extensions already contains an
// extension with the given name. Used by BuildProjectAgent to avoid
// overwriting a PermissionExtension injected by the TUI layer.
func hasExtensionNamed(exts []agentcore.Extension, name string) bool {
	for _, ext := range exts {
		if ext.Name() == name {
			return true
		}
	}
	return false
}

// buildProjectSystemPrompt 构造含案件上下文的 System Prompt。
func buildProjectSystemPrompt(rec ProjectRecord) string {
	var b strings.Builder

	b.WriteString("你是 Mady 的智能助理，正在处理案件：")
	if rec.Alias != "" {
		b.WriteString(rec.Alias)
	}
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "案件目录：%s\n", rec.RootPath)
	fmt.Fprintf(&b, "领域：%s\n", rec.Domain)
	b.WriteString("\n")

	b.WriteString("五步工作法：\n")
	b.WriteString("1. 发现事实 — 了解案件内容、技术领域、需求\n")
	b.WriteString("2. 获取规则 — 检索相关法规、审查指南、现有技术\n")
	b.WriteString("3. 规划 — 制定检索策略或分析方案\n")
	b.WriteString("4. 执行 — 进行检索、分析权利要求、生成文书\n")
	b.WriteString("5. 检查 — 验证检索完整性、分析准确性\n")
	b.WriteString("\n")

	b.WriteString("涉及专业判断的输出附以下声明：\n")
	b.WriteString("「本分析由 AI 辅助生成，不构成正式专业意见。」\n")
	b.WriteString("\n")

	b.WriteString("输出格式：完成任务后，用以下 JSON 格式返回结果：\n")
	b.WriteString(`{"action":"做了什么","result":"结果摘要","success":true}`)
	b.WriteString("\n- action: 你做了什么操作\n")
	b.WriteString("- result: 结果的简洁摘要\n")
	b.WriteString("- success: 是否成功完成\n")
	b.WriteString("\n")

	b.WriteString("注意：\n")
	b.WriteString("- 文件操作被限制在案件目录内\n")
	b.WriteString("- 涉及法定期限的判断需明确标注 deadline\n")

	return b.String()
}
