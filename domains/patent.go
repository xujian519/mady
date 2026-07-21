package domains

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/permission"
	"github.com/xujian519/mady/domains/doctmpl"
	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/retrieval/domain"
	"github.com/xujian519/mady/tools"
	"github.com/xujian519/mady/workflows/patent"
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

// injectDocTemplateTools 向 Agent 配置注册文档模板相关工具。
func injectDocTemplateTools(cfg *agentcore.Config) {
	if globalTemplateStore != nil {
		cfg.Tools = append(cfg.Tools,
			doctmpl.NewListDocTemplatesTool(globalTemplateStore),
			doctmpl.NewRenderDocTemplateTool(globalTemplateStore),
		)
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
		"五步工作法：",
		"1. 发现事实 — 了解发明内容、技术领域、申请人需求",
		"2. 获取规则 — 使用 web_search / web_fetch 检索相关专利法规、审查指南、现有技术；使用 scholar_search 检索学术论文（现有技术）；使用 search_knowledge / search_laws 检索本地知识库中的法律法规和案例",
		"3. 规划 — 制定检索策略或申请方案",
		"4. 执行 — 使用 patent_lookup 查询专利元数据、进行专利检索、分析权利要求、生成文书",
		"5. 检查 — 验证检索完整性、分析准确性",
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
	toolExt := tools.NewExtension(tools.ExtensionConfig{
		WorkingDir:     workingDir,
		SandboxEnabled: true,
		Vision: &tools.VisionToolConfig{
			Provider: base.Provider,
			Model:    base.Model,
		},
		WebSearch:  &tools.WebSearchToolConfig{},
		WebFetch:   &tools.WebFetchToolConfig{},
		PatentTool: tools.PatentToolConfigDefaults(),
		DisableTools: []string{
			tools.ToolBash, tools.ToolGitStatus, tools.ToolGitDiff, tools.ToolGitLog,
			tools.ToolBrowser, tools.ToolExecuteCode,
		},
		MaxBytes: 100 * 1024,
		ExtraTools: []*agentcore.Tool{
			patent.NewPatentNoveltyTool(patent.WithRetriever(globalPatentRetriever)),
			patent.NewOAResponseTool(),
		},
	})
	cfg.Extensions = append(cfg.Extensions, toolExt)

	injectDraftingTool(&cfg)
	injectDocTemplateTools(&cfg)

	// Chunked context engine for long patent documents.
	cfg.Engine = "chunked"

	// DoomLoop: 死循环检测器，监控工具调用循环、重复文本、空结果等异常。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, defaultDoomLoopHook())

	// ReasoningStrategy: 专利分析通常需要结构化分析或验证式推理，
	// 因此注入策略提示，根据问题复杂度自动选择合适推理方式。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewReasoningStrategyRouter(
			agentcore.NewDefaultClassifier(),
			agentcore.NewDefaultStrategySelector(),
		),
	)

	// 法条引用核验 Gate（P2b Strict）：命中疑点追加存疑提示 +
	// citation_verify 留痕 + SuppressPersist（未人工复核不入库）。
	// 知识源与留痕 store 由装配侧注入（citation_wiring.go）。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, newCitationGate(DomainPatent, ""))

	// Guardrail: LevelStrict with patent disclaimer + approval gate.
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		guardrails.New(
			guardrails.WithLevel(guardrails.LevelStrict),
			guardrails.WithDisclaimer(guardrails.DisclaimerPatent),
			guardrails.WithRiskKeywords(guardrails.RiskKeywordsFor("patent")),
			guardrails.WithApproval(guardrails.ApprovalKeywordsFor("patent")),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		),
	)

	// Human approval gate for critical decisions.
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		NewApprovalGate(ApprovalConfig{
			RequireApprovalFor: guardrails.ApprovalKeywordsFor("patent"),
		}),
	)

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
	toolExt := tools.NewExtension(tools.ExtensionConfig{
		WorkingDir:     rec.RootPath,
		EnabledTools:   []string{"read", "write_file", "edit", "grep", "find", "glob", "ls"},
		SandboxEnabled: true,
		Vision: &tools.VisionToolConfig{
			Provider: base.Provider,
			Model:    base.Model,
		},
		MaxBytes: 100 * 1024,
	})
	cfg.Extensions = append(cfg.Extensions, toolExt)

	injectDraftingTool(&cfg)
	injectDocTemplateTools(&cfg)

	// Chunked context engine for long patent/legal documents.
	if base.Engine == "" {
		cfg.Engine = "chunked"
	}

	// 法条引用核验 Gate（P1b）：案件答案同样纳入引用核验。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		guardrails.NewCitationGate(guardrails.WithCitationGateLevel(guardrails.LevelStandard)),
	)

	// LevelStrict 护栏 + 人工审批门
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		guardrails.New(
			guardrails.WithLevel(guardrails.LevelStrict),
			guardrails.WithDisclaimer(guardrails.DisclaimerPatent),
			guardrails.WithRiskKeywords(guardrails.RiskKeywordsFor("patent")),
			guardrails.WithApproval(guardrails.ApprovalKeywordsFor("patent")),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		),
	)
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		NewApprovalGate(ApprovalConfig{
			RequireApprovalFor: guardrails.ApprovalKeywordsFor("patent"),
		}),
	)

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
