package guardrails

// Pre-defined disclaimer templates for different professional domains.
// Each disclaimer is designed to be appended to AI-generated output that
// contains risk-triggering content.

const (
	// DisclaimerPatent — patent agent / IP domain disclaimer.
	DisclaimerPatent = "本分析由 AI 辅助生成，不构成正式法律意见。专利申请和专利性判断应由具备资质的专利代理人或专利律师确认。"

	// DisclaimerLegal — legal domain disclaimer.
	DisclaimerLegal = "本分析由 AI 辅助生成，不构成正式法律意见。法律判断和决策应由具备执业资格的律师确认。"

	// DisclaimerGeneric — generic professional disclaimer.
	DisclaimerGeneric = "本回复由 AI 辅助生成，仅供参考，不构成专业建议。如有疑问，请咨询相关领域的专业人士。"
)

// DisclaimerFor returns the appropriate disclaimer for a domain string.
func DisclaimerFor(domain string) string {
	switch domain {
	case "patent":
		return DisclaimerPatent
	case "legal":
		return DisclaimerLegal
	default:
		return DisclaimerGeneric
	}
}

// RiskKeywordsFor returns risk keywords appropriate for a domain.
// These keywords trigger disclaimer injection when found in output.
func RiskKeywordsFor(domain string) []string {
	switch domain {
	case "patent":
		return []string{
			"侵权", "无效", "驳回", "不授权", "专利性", "自由实施",
			"新颖性结论", "创造性结论",
		}
	case "legal":
		return []string{
			"应判决", "应裁定", "构成犯罪", "不构成犯罪",
			"胜诉", "败诉", "法律意见", "诉讼策略",
		}
	default:
		return nil
	}
}

// ApprovalKeywordsFor returns keywords that trigger human approval for a domain.
func ApprovalKeywordsFor(domain string) []string {
	switch domain {
	case "patent":
		return []string{"专利结论", "侵权判断", "有效性结论", "最终建议"}
	case "legal":
		return []string{"法律意见", "诉讼策略", "判决预测", "最终建议"}
	default:
		return nil
	}
}
