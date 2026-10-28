package domains

import (
	"log/slog"
	"path/filepath"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/checker"
	figuregenerate "github.com/xujian519/mady/domains/figure/generate"
	"github.com/xujian519/mady/domains/lawverify"
	"github.com/xujian519/mady/domains/rules"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/pkg/util"
)

// injectDomainSharedExtensions 追加领域 Agent 装配共用的扩展段落：
// 五步工作流 / 文档模板 / 提示词工具追加到 Tools，规则 / 写作 / 知识库 /
// 证据判断扩展追加到 Extensions。
//
// PatentAgentConfig / LegalAgentConfig / BuildProjectAgent 三处装配块此前
// 逐行重复，抽取后统一为 patent 的追加顺序。Tools 段顺序与各装配块原序
// 一致；Extensions 段中 legal 的 Rules/Writing 相对原序交换——两者无同名
// 工具、无 LifecycleHook（仅 ToolProvider / SystemPromptProvider），
// 注册顺序不影响行为。
// globalLegalKnowledgeExt 是法律知识检索扩展的全局实例，由
// SetupLegalKnowledgeExtension 在启动期注入。专利与法律域共享：专利分析同样
// 需要司法解释与裁判案例检索。
var globalLegalKnowledgeExt agentcore.Extension

// SetupLegalKnowledgeExtension 在启动期注入法律知识检索扩展
// （legal_knowledge_search / legal_knowledge_install）。
// ext 为 nil 时静默跳过；必须在任何 Agent 创建前调用。
func SetupLegalKnowledgeExtension(ext agentcore.Extension) {
	globalLegalKnowledgeExt = ext
}

// injectFigureTool 追加附图工具组（生成 / 落版 / 跨图核验）。
// 产物路径限定在项目目录（缺省为工作区）内；工作目录都未配置时不注入。
// 跨图核验只读、不依赖工作目录，因此始终注入。
func injectFigureTool(cfg *agentcore.Config) {
	cfg.Tools = append(cfg.Tools, figuregenerate.NewConsistencyTool())
	workDir := cfg.ProjectDir
	if workDir == "" {
		workDir = cfg.WorkspaceDir
	}
	if workDir == "" {
		return
	}
	figureCfg := figuregenerate.ToolConfig{WorkingDir: workDir}
	cfg.Tools = append(cfg.Tools, figuregenerate.NewTool(figureCfg), figuregenerate.NewSubmissionTool(figureCfg))
}

func injectDomainSharedExtensions(cfg *agentcore.Config) {
	injectDraftingTool(cfg)
	injectDocTemplateTools(cfg)
	injectPromptTools(cfg)
	injectWritingTools(cfg)
	injectRulesTools(cfg)
	injectFigureTool(cfg)
	if globalKnowledgeExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalKnowledgeExt)
	}
	if globalEvidenceExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalEvidenceExt)
	}
	if globalLegalKnowledgeExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalLegalKnowledgeExt)
	}
	injectLawVerifyTool(cfg)
	injectRuleOutputGate(cfg)
}

// injectRuleOutputGate 追加规则输出门禁：工具结果命中确定性规则时，
// warn 级追加合规提示，block/review 级标注不可作为交付依据。
//
// 规则来自随包资产而非 $MADY_HOME：门禁是安全能力，不因部署目录缺失而静默失效。
// 结构门禁（StructuralGate）需要声明「哪个交付工具的哪个入参承载制品文本」，
// 交付工具在 W2 才就绪，届时与本装配一起配置 entries。
func injectRuleOutputGate(cfg *agentcore.Config) {
	engine, err := rules.NewBundledEngine()
	if err != nil {
		slog.Error("规则输出门禁未启用：加载随包门禁规则失败", "error", err)
		return
	}
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, rules.NewOutputGate(engine))
}

// injectLawVerifyTool 追加法条核验工具。
//
// 专利与法律域都会引用法条，故放在共享段落；骨架路径由工具自行探测
// （$MADY_HOME/law-skeleton/ 或仓库 data/law/skeleton/），缺失时降级为仅精校索引。
func injectLawVerifyTool(cfg *agentcore.Config) {
	cfg.Extensions = append(cfg.Extensions, lawverify.NewExtension(lawverify.Config{}))
}

// appendDraftingExtensions 追加撰写类扩展段落：权利要求撰写（claimdrafting）、
// Checker 复核、说明书撰写（specdrafting）、Orchestration 编排。
// PatentAgentConfig 与 BuildProjectAgent 两处装配块此前逐行重复。
func appendDraftingExtensions(cfg *agentcore.Config) {
	if globalClaimDraftingExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalClaimDraftingExt)
	}
	cfg.Extensions = append(cfg.Extensions, checker.NewExtension(nil))
	if globalSpecDraftingExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalSpecDraftingExt)
	}
	cfg.Extensions = append(cfg.Extensions, &OrchestrationExtension{})
}

// appendResultOffload 追加工具结果落盘段落：超过阈值的大工具结果（文件读取、
// 检索、浏览器抓取等）在持久化前替换为"头尾摘要 + 落盘句柄"，完整内容保存
// 到 $MADY_HOME/offload/（内容寻址，幂等）。落盘失败时保留原结果——best-effort
// 降级，绝不把成功的工具调用变成失败。
//
// 复用 agentcore.ToolResultBudget（PilotDeck ToolResultBudget 移植）；此前仅有
// 库与测试、无运行时接线。挂在 appendGatewayLifecycle 内与 DoomLoop 同覆盖面
// （Patent/Legal/Unified/Project 四处装配块）。
func appendResultOffload(cfg *agentcore.Config) {
	home, err := util.MadyHome()
	if err != nil {
		// MadyHome 不可解析时跳过接线：无落盘只是退回纯上下文内处理，
		// 不阻断 Agent 启动——但必须留痕，避免保护静默失效无从排查。
		slog.Warn("工具结果落盘未启用：MadyHome 解析失败", "error", err)
		return
	}
	offloadDir := filepath.Join(home, "offload")
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewToolResultBudget(agentcore.ToolResultBudgetConfig{
			RootDir: offloadDir,
		}))
	// 配套回读通道：模型判断摘要中段有需要时取回全文（根目录内校验，
	// 不依赖 WorkingDir 沙箱配置）。
	cfg.Tools = append(cfg.Tools, agentcore.NewOffloadReadTool(offloadDir))
}

// appendGatewayLifecycle 追加 DoomLoop 死循环检测 + 工具结果落盘 + Gateway
// 统一决策入口段落：注册默认 DoomLoop hook，注册工具结果落盘 hook，构建
// Gateway，将 gateway.Fallback 挂到 cfg.FallbackRouter 并注册到 Lifecycle。
//
// Patent/Legal/Unified/Project 四处装配块重复；modify 供 UnifiedAgentConfig
// 挂接 gatewayModifier（nil 时跳过）。接入契约：注册 Gateway 后不得再单独
// 注册 ReasoningRouter / ReasoningStrategyRouter / FallbackRouter。
func appendGatewayLifecycle(cfg *agentcore.Config, modify func(*agentcore.Gateway)) {
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, defaultDoomLoopHook())
	appendResultOffload(cfg)
	gateway := newDefaultGateway(*cfg)
	if modify != nil {
		modify(gateway)
	}
	cfg.FallbackRouter = gateway.Fallback
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, gateway)
}

// appendStandardCitationGate 追加法条引用核验 Gate（LevelStandard）：
// R1 存在性 + R2 交叉匹配，命中疑点追加存疑提示（P1b 行为）。
// Legal/Unified/Project 三处装配块重复；Patent 使用带 S2 知识源与
// citation_verify 留痕的 newCitationGate（P2b），不在此列。
func appendStandardCitationGate(cfg *agentcore.Config) {
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewIFaceLifecycleHook(guardrails.NewCitationGate(
			guardrails.WithCitationGateLevel(guardrails.LevelStandard),
		)),
	)
}

// appendStrictGuardrails 追加 LevelStrict 护栏 + 人工审批门段落。
// Patent/Legal/Project 三处装配块重复。domain 驱动风险关键词与审批关键词
// （guardrails.RiskKeywordsFor / ApprovalKeywordsFor），disclaimer 为领域
// 免责声明文本；withDQ 为 true 时护栏与审批门共享 DeferredPersistQueue——
// 被抑制的消息在人工批复时提交或丢弃，不静默丢失。
func appendStrictGuardrails(cfg *agentcore.Config, domain, disclaimer string, withDQ bool) {
	opts := []guardrails.Option{
		guardrails.WithLevel(guardrails.LevelStrict),
		guardrails.WithDisclaimer(disclaimer),
		guardrails.WithRiskKeywords(guardrails.RiskKeywordsFor(domain)),
		guardrails.WithApproval(guardrails.ApprovalKeywordsFor(domain)),
		guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
	}
	var dq *guardrails.DeferredPersistQueue
	if withDQ {
		dq = guardrails.NewDeferredPersistQueue()
		opts = append(opts, guardrails.WithDeferredQueue(dq))
	}
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewIFaceLifecycleHook(guardrails.New(opts...)),
	)

	approvalOpts := make([]func(*ApprovalGate), 0, 1)
	if withDQ {
		approvalOpts = append(approvalOpts, WithDeferredPersist(&DeferredPersistFuncs{
			CommitFn: func() {
				for _, idx := range dq.Pending() {
					dq.Commit(idx)
				}
			},
			DiscardFn: func() {
				for _, idx := range dq.Pending() {
					dq.Discard(idx)
				}
			},
		}))
	}
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		NewApprovalGate(ApprovalConfig{
			RequireApprovalFor: guardrails.ApprovalKeywordsFor(domain),
		}, approvalOpts...),
	)
}
