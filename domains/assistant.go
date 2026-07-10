package domains

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/psychological"
	"github.com/xujian519/mady/tools"
)

// AssistantAgentConfig 构建助理领域 Agent 配置。
// 助理 Agent 配备工具集（web_search、web_fetch、read、write_file 等），
// 用于代码生成、文件操作、数据分析、网页搜索等任务执行。
// 使用 LevelStandard 护栏，跨域路由由 RouterConfig 统一管理。
func AssistantAgentConfig(base agentcore.Config) agentcore.Config {
	cfg := base
	cfg.Name = "assistant-agent"

	// 助理场景需要足够轮次完成工具链式调用。
	// Agent 内部默认值为 20，此处确保不会因意外传入过低值而截断工具工作流。
	if cfg.MaxTurns == 0 || cfg.MaxTurns < 20 {
		cfg.MaxTurns = 20
	}

	cfg.SystemPrompt = strings.Join([]string{
		"你是 Mady 的通用智能助理模块。用简体中文回复，友好专业。",
		"",
		"职责：",
		"- 信息检索与网页搜索（使用 web_search / web_fetch 工具）",
		"- 代码生成、阅读和修改（使用 read / write_file / edit 工具）",
		"- 文件操作和项目管理（使用 ls / glob / grep / find 工具）",
		"- 内容创作和编辑",
		"- 数据整理和导出",
		"",
		"使用工具前，先简要说明你要做什么，执行完给出结构化结果。",
		"",
		"输出格式：完成任务后，用以下 JSON 格式返回结果（便于 Chat Agent 解释给用户）：",
		`{"action":"做了什么","result":"结果摘要","success":true}`,
		"- action: 你做了什么操作",
		"- result: 结果的简洁摘要",
		"- success: 是否成功完成",
		"",
		"边界：",
		"- 不提供法律建议（应由 legal-advisor 处理）",
		"- 不提供专利分析（应由 patent-agent 处理）",
		"- 不确定的专业问题建议用户咨询相关专业人士",
	}, "\n")

	// Guardrail: LevelStandard with assistant disclaimer.
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		guardrails.New(
			guardrails.WithLevel(guardrails.LevelStandard),
			guardrails.WithDisclaimer(guardrails.DisclaimerAssistant),
			guardrails.WithRiskKeywords(guardrails.RiskKeywordsFor("assistant")),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		),
	)

	// Tools extension — core capability of assistant agent.
	// Disable tools not relevant to patent/lawyer workflows (bash, git,
	// browser, code execution, etc.) to keep the tool surface minimal.
	toolExt := tools.NewExtension(tools.ExtensionConfig{
		WorkingDir: "./workspace",
		WebSearch:  &tools.WebSearchToolConfig{},
		WebFetch:   &tools.WebFetchToolConfig{},
		MaxBytes:   100 * 1024,
		MaxLines:   5000,
		DisableTools: []string{
			tools.ToolBash, tools.ToolGitStatus, tools.ToolGitDiff, tools.ToolGitLog,
			tools.ToolBrowser, tools.ToolExecuteCode, tools.ToolComputerUse,
			tools.ToolProcess,
		},
	})
	cfg.Extensions = append(cfg.Extensions, toolExt)

	// 心理引擎 — 轻量模式，不做认知扭曲诊断
	cfg.Extensions = append(cfg.Extensions, psychological.NewExtension(AssistantPsychConfig()))

	// 注意：跨域路由由 Router Agent 通过 RouterConfig 统一管理，
	// AssistantAgentConfig 仅定义助理 Agent 自身行为，不处理跨域 Handoff。
	// defines its own behavior, not cross-domain routing.

	return cfg
}
