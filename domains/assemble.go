package domains

import (
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/checker"
	"github.com/xujian519/mady/guardrails"
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
func injectDomainSharedExtensions(cfg *agentcore.Config) {
	injectDraftingTool(cfg)
	injectDocTemplateTools(cfg)
	injectPromptTools(cfg)
	injectWritingTools(cfg)
	injectRulesTools(cfg)
	if globalKnowledgeExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalKnowledgeExt)
	}
	if globalEvidenceExt != nil {
		cfg.Extensions = append(cfg.Extensions, globalEvidenceExt)
	}
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

// appendGatewayLifecycle 追加 DoomLoop 死循环检测 + Gateway 统一决策入口段落：
// 注册默认 DoomLoop hook，构建 Gateway，将 gateway.Fallback 挂到
// cfg.FallbackRouter 并注册到 Lifecycle。
//
// Patent/Legal/Unified/Project 四处装配块重复；modify 供 UnifiedAgentConfig
// 挂接 gatewayModifier（nil 时跳过）。接入契约：注册 Gateway 后不得再单独
// 注册 ReasoningRouter / ReasoningStrategyRouter / FallbackRouter。
func appendGatewayLifecycle(cfg *agentcore.Config, modify func(*agentcore.Gateway)) {
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, defaultDoomLoopHook())
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
