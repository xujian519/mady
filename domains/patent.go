package domains

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/tools"
)

// PatentAgentConfig builds the patent domain Agent configuration.
func PatentAgentConfig(base agentcore.Config) agentcore.Config {
	cfg := base
	cfg.Name = "patent-agent"

	cfg.SystemPrompt = strings.Join([]string{
		"你是 Mady 的专利代理与知识产权分析模块。",
		"用简体中文回复，专业严谨。",
		"",
		"五步工作法：",
		"1. 发现事实 — 了解发明内容、技术领域、申请人需求",
		"2. 获取规则 — 检索相关专利法规、审查指南、现有技术",
		"3. 规划 — 制定检索策略或申请方案",
		"4. 执行 — 进行专利检索、分析权利要求、生成文书",
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
		DisableTools: []string{
			tools.ToolBash, tools.ToolGitStatus, tools.ToolGitDiff, tools.ToolGitLog,
			tools.ToolBrowser, tools.ToolExecuteCode, tools.ToolComputerUse,
			tools.ToolProcess,
		},
		MaxBytes: 100 * 1024,
	})
	cfg.Extensions = append(cfg.Extensions, toolExt)

	// Chunked context engine for long patent documents.
	cfg.Engine = "chunked"

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

	// Chunked context engine for long patent/legal documents.
	if base.Engine == "" {
		cfg.Engine = "chunked"
	}

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
