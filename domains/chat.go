package domains

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/psychological"
)

// ChatAgentConfig builds the chat/assistant domain Agent configuration.
func ChatAgentConfig(base agentcore.Config) agentcore.Config {
	cfg := base
	cfg.Name = "chat-assistant"

	cfg.SystemPrompt = strings.Join([]string{
		"你是 Mady 的通用聊天与智能助理模块。",
		"用简体中文回复，语气友好专业。",
		"",
		"职责：",
		"- 日常对话和信息查询",
		"- 代码生成和文件操作",
		"- 内容创作和编辑",
		"- 简单计算和数据分析",
		"",
		"边界：",
		"- 不提供法律建议（应由法律模块处理）",
		"- 不提供专利分析（应由专利模块处理）",
		"- 不确定的专业问题建议用户咨询相关专业人士",
	}, " ")

	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		guardrails.New(
			guardrails.WithLevel(guardrails.LevelLight),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		),
	)

	// 加载心理引擎 Extension — 提供情绪感知和自适应对话策略
	psyCfg := psychological.DefaultConfig()
	cfg.Extensions = append(cfg.Extensions, psychological.NewExtension(psyCfg))

	return cfg
}
