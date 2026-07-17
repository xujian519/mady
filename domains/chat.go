package domains

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/psychological"
)

// ChatAgentConfig builds the chat domain Agent configuration.
//
// Chat agent focuses on pure conversation and emotional support. It uses
// lightweight guardrails (LevelLight) and psychological extension for
// emotion-aware tone adaptation without diagnostic output. Task execution
// requests are handed off to the assistant agent via HandoffDelegate.
func ChatAgentConfig(base agentcore.Config) agentcore.Config {
	cfg := base
	cfg.Name = "chat-agent"

	// 聊天场景轮次上限 8（零值兜底 + 截断过高值）。
	// 显式传入 1-7 的较小值不会被覆盖 —— 仅当值为 0（未设置）
	// 或大于 8 时才会被修正。
	if cfg.MaxTurns == 0 || cfg.MaxTurns > 8 {
		cfg.MaxTurns = 8
	}

	cfg.SystemPrompt = strings.Join([]string{
		"你是 Mady 的日常聊天与情感陪伴模块。",
		"用简体中文回复，语气自然友好，像同事而不是客服。",
		"",
		"职责：",
		"- 日常对话和情感交流",
		"- 倾听和陪伴",
		"- 简单的信息查询",
		"",
		"边界：",
		"- 不执行代码生成、文件操作等任务（由 assistant-agent 处理）",
		"- 不提供法律建议（应由 legal-advisor 处理）",
		"- 不提供专利分析（应由 patent-agent 处理）",
		"- 不确定的专业问题建议用户咨询相关专业人士",
	}, "\n")

	// DoomLoop: 死循环检测器。
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, defaultDoomLoopHook())

	// ReasoningStrategy: 根据问题复杂度动态调整推理 effort/budget。
	// 聊天场景关闭 StrategyHintInjection：不在系统提示中注入策略提示（如
	// "请按步骤逐步推理"），避免对简单聊天产生不自然的引导。
	chatSelector := agentcore.NewDefaultStrategySelector()
	chatSelector.StrategyHintInjection = false
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		agentcore.NewReasoningStrategyRouter(
			agentcore.NewDefaultClassifier(),
			chatSelector,
		),
	)

	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		guardrails.New(
			guardrails.WithLevel(guardrails.LevelLight),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		),
	)

	// 心理引擎 — 轻量模式：VAD/OCC 语气调整，不做认知扭曲诊断
	psyCfg := ChatPsychConfig()
	cfg.Extensions = append(cfg.Extensions, psychological.NewExtension(psyCfg))

	// 注意：跨域路由由 Router Agent 通过 RouterConfig 统一管理，
	// ChatAgentConfig 仅定义聊天 Agent 自身行为，不处理跨域 Handoff。

	return cfg
}

// IntegratedChatConfig 构建融合了意图识别与路由能力的统一 Chat Agent。
//
// 这是方案 A 的核心：Chat Agent 作为用户唯一面对的统一界面，
// 内部通过 Invisible Handoff 无缝委派专业任务到对应的领域 Agent，
// 用户不感知切换过程。
//
// 与 ChatAgentConfig 的区别：
// - SystemPrompt 包含路由指令
// - 注册了 transfer_to_assistant/patent/legal 工具（Invisible Handoff）
// - 内部调用 ChatAgentConfig 获取基础配置后覆盖 SystemPrompt 和 Handoffs
func IntegratedChatConfig(base agentcore.Config) agentcore.Config {
	cfg := ChatAgentConfig(base)

	// 使用融合路由能力的 SystemPrompt 替代纯聊天提示词
	cfg.SystemPrompt = strings.Join([]string{
		"你是 Mady（中观智能体）的统一智能助手 —— 用户的所有对话都经过你。",
		"用简体中文回复，语气自然友好，像同事而不是客服。",
		"你的职责包括日常对话和专业任务路由两方面：",
		"",
		"【日常对话】",
		"- 日常对话和情感交流",
		"- 倾听和陪伴",
		"- 简单的信息查询",
		"",
		"【专业任务路由】",
		"当用户提出专业领域问题，使用 transfer_to_* 工具将任务委派给领域专家处理。",
		"委派完成后，直接向用户呈现最终结果，不需要解释「切换」过程：",
		"- transfer_to_assistant → 通用智能助理（代码生成、文件操作、网页搜索、数据分析等工具密集型任务）",
		"- transfer_to_patent → 专利代理与知识产权分析（专利检索、权利要求分析、新颖性比对）",
		"- transfer_to_legal → 法律咨询与研究（法条检索、判例检索、法律分析）",
		"",
		"路由规则：",
		"- 日常聊天、问候、情感交流、简单问题 → 直接回答即可",
		"- 需要工具执行的任务（搜索、代码、文件操作） → transfer_to_assistant",
		"- 专利相关问题 → transfer_to_patent",
		"- 法律相关问题 → transfer_to_legal",
		"- 不确定分类时，先用日常对话方式回应再引导",
	}, "\n")

	// 注册专业领域 Handoff（Assistant/Patent/Legal），标记为不可见
	cfg.Handoffs = ProfessionalHandoffConfigs(base)
	for i := range cfg.Handoffs {
		cfg.Handoffs[i].Invisible = true
	}

	return cfg
}
