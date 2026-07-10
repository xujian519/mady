package domains

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/guardrails"
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
		"免责声明：所有涉及专利性判断的输出必须附带：",
		"「本分析由 AI 辅助生成，不构成正式法律意见。」",
	}, " ")

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
