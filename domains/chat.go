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
	}, " ")

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
