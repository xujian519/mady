package domains

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/guardrails"
)

// LegalAgentConfig builds the legal domain Agent configuration.
func LegalAgentConfig(base agentcore.Config) agentcore.Config {
	cfg := base
	cfg.Name = "legal-advisor"

	cfg.SystemPrompt = strings.Join([]string{
		"你是 Mady 的法律咨询与研究模块。",
		"用简体中文回复，专业严谨。",
		"",
		"五步工作法：",
		"1. 发现事实 — 了解案件背景、当事人信息、法律诉求",
		"2. 获取规则 — 检索相关法律法规、司法解释、指导性案例",
		"3. 规划 — 确定法律分析框架和论证逻辑",
		"4. 执行 — 进行法条匹配、判例比对、法律推理",
		"5. 检查 — 验证法条引用准确性、判例相关性、论证完整性",
		"",
		"免责声明：所有涉及法律判断的输出必须附带：",
		"「本分析由 AI 辅助生成，不构成正式法律意见。」",
	}, " ")

	// Chunked context engine for long legal documents.
	cfg.Engine = "chunked"

	// Guardrail: LevelStrict with legal disclaimer + approval gate.
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		guardrails.New(
			guardrails.WithLevel(guardrails.LevelStrict),
			guardrails.WithDisclaimer(guardrails.DisclaimerLegal),
			guardrails.WithRiskKeywords(guardrails.RiskKeywordsFor("legal")),
			guardrails.WithApproval(guardrails.ApprovalKeywordsFor("legal")),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		),
	)

	// Human approval gate for critical decisions.
	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		NewApprovalGate(ApprovalConfig{
			RequireApprovalFor: guardrails.ApprovalKeywordsFor("legal"),
		}),
	)

	return cfg
}
