package domains

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/psychological"
)

// UnifiedAgentOption 是 UnifiedAgentConfig 的可选配置（向后兼容的 variadic 扩展）。
type UnifiedAgentOption func(*unifiedAgentOptions)

type unifiedAgentOptions struct {
	gatewayModifier func(*agentcore.Gateway)
}

// WithGatewayModifier 允许装配层定制 Gateway（如挂接 plantask 的
// OnHighComplexity 自动进入规划态回调；bootstrap 层注入）。
func WithGatewayModifier(m func(*agentcore.Gateway)) UnifiedAgentOption {
	return func(o *unifiedAgentOptions) { o.gatewayModifier = m }
}

// UnifiedAgentConfig 构建合并后的统一 Agent 配置。
//
// 融合了原 Chat Agent（对话/情感陪伴）、Assistant Agent（工具执行）
// 和 Router（领域路由）三者的能力。用户面对的唯一智能体入口，
// 内部通过 Invisible Handoff 委派专利/法律专业任务。
//
// toolExt 是调用方已装配好的工具扩展（含文件/网络/视觉等标准能力），
// 通过被动注入模式传入，域层不再主动创建工具。
// patentToolExt 和 legalToolExt 是 Handoff 子 Agent 的独立工具扩展。
func UnifiedAgentConfig(base agentcore.Config, toolExt agentcore.Extension, patentToolExt agentcore.Extension, legalToolExt agentcore.Extension, opts ...UnifiedAgentOption) agentcore.Config {
	var o unifiedAgentOptions
	for _, opt := range opts {
		opt(&o)
	}
	cfg := base
	cfg.Name = "mady-agent"

	// 统一场景需要足够轮次：对话 + 工具链式调用。
	if cfg.MaxTurns == 0 || cfg.MaxTurns > 100 {
		cfg.MaxTurns = 100
	}

	cfg.SystemPrompt = strings.Join([]string{
		"你是 Mady（中观智能体），用户的所有对话和任务都经过你。",
		"用简体中文回复，语气自然友好，像同事而不是客服。",
		"",
		"【能力范围】",
		"- 日常对话、情感交流和倾听陪伴",
		"- 信息检索与网页搜索（使用 web_search / web_fetch 工具）",
		"- 学术论文检索（使用 scholar_search 工具）",
		"- 代码生成、阅读和修改（使用 read / write_file / edit 工具）",
		"- 文件操作和项目管理（使用 ls / glob / grep / find 工具）",
		"- 内容创作、数据整理和导出",
		"",
		"【专业任务路由】",
		"当用户提出专利或法律领域问题，使用 transfer_to_* 工具委派给领域专家：",
		"- transfer_to_patent → 专利代理与知识产权分析（专利检索、权利要求分析、新颖性比对）",
		"- transfer_to_legal → 法律咨询与研究（法条检索、判例检索、法律分析）",
		"委派完成后直接向用户呈现结果，不需要解释切换过程。",
		"",
		"【工具使用原则】",
		"使用工具前先简要说明你要做什么，执行完给出结构化结果。",
		"不确定的专业问题建议用户咨询相关专业人士。",
	}, "\n")

	// DoomLoop: 死循环检测器。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, defaultDoomLoopHook())

	// Gateway (PilotDeck 风格统一决策入口): 一次分类同时驱动
	//   - 推理 effort/budget（原 ReasoningRouter 职责）
	//   - 策略 hint 注入到系统消息（原 ReasoningStrategyRouter 职责）
	//   - 模型回退链选择（FallbackRouter，候选链由调用方按需配置）
	//   - token 预算评估与 blocking 钳制（TokenBudgetManager）
	// 替换此前单独注册的 ReasoningStrategyRouter，消除每轮两次 Classify。
	// 接入契约：注册 Gateway 后不得再单独注册 ReasoningRouter /
	// ReasoningStrategyRouter / FallbackRouter，否则会重复分类与重复健康计数。
	gateway := newDefaultGateway(cfg)
	if o.gatewayModifier != nil {
		o.gatewayModifier(gateway)
	}
	cfg.FallbackRouter = gateway.Fallback // 供 callModelWithFallback 使用
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, gateway)

	// Guardrail: LevelLight — 统一使用轻量护栏。
	// 安全防护未来通过人机协作和 plan 模式替代。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewIFaceLifecycleHook(guardrails.New(
			guardrails.WithLevel(guardrails.LevelLight),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		)),
	)

	// 法条引用核验 Gate（LevelStandard）：R1 存在性 + R2 交叉匹配，
	// 命中疑点追加存疑提示。统一 Agent 作为用户入口层也需要引用核验，
	// 防止直接回答法条相关问题时不经过 Handoff 子 Agent。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewIFaceLifecycleHook(guardrails.NewCitationGate(guardrails.WithCitationGateLevel(guardrails.LevelStandard))),
	)

	// 被动注入：调用方已装配好的工具扩展。
	// 安全策略由入口层（TUI/serve/ACP）注入的 PermissionExtension 管理。
	// TUI：PermissionExtension(ProjectAgentPolicy, TUIChannelApprover) → Ask
	// serve：PermissionExtension(DenyPolicy, AlwaysDenyApprover)    → Deny
	cfg.Extensions = append(cfg.Extensions, toolExt, psychological.NewExtension(
		ChatPsychConfig(),
	))

	// 注册专业领域 Handoff（Patent/Legal），标记为不可见。
	// 传入专利和法律的独立工具扩展用于子 Agent 配置。
	cfg.Handoffs = ProfessionalHandoffConfigs(base, patentToolExt, legalToolExt)
	for i := range cfg.Handoffs {
		cfg.Handoffs[i].Invisible = true
	}

	return cfg
}
