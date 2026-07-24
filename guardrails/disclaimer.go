package guardrails

import "github.com/xujian519/mady/pkg/i18n"

// Pre-defined disclaimer templates for different professional domains.
// Each disclaimer is designed to be appended to AI-generated output that
// contains risk-triggering content.
//
// For new code, prefer Disclaimer() / ShortDisclaimer() which select text by guardrail Level rather than by domain string.
const (
	// DisclaimerPatent — patent agent / IP domain disclaimer.
	DisclaimerPatent = "本分析由 AI 辅助生成，不构成正式法律意见。专利申请和专利性判断应由具备资质的专利代理人或专利律师确认。"

	// DisclaimerLegal — legal domain disclaimer.
	DisclaimerLegal = "本分析由 AI 辅助生成，不构成正式法律意见。法律判断和决策应由具备执业资格的律师确认。"

	// DisclaimerGeneric — generic professional disclaimer.
	DisclaimerGeneric = "本回复由 AI 辅助生成，仅供参考，不构成专业建议。如有疑问，请咨询相关领域的专业人士。"

	// DisclaimerAssistant — assistant domain disclaimer for task execution outputs.
	DisclaimerAssistant = "本结果由 AI 辅助生成，请在使用前进行人工审核。涉及专业领域（专利、法律）的判断请咨询对应专业人士。"
)

// Disclaimer 返回给定护栏等级的免责声明文本。
func Disclaimer(level Level) string {
	switch level {
	case LevelStrict:
		return i18n.T("guardrail.disclaimer.strict")
	case LevelStandard:
		return i18n.T("guardrail.disclaimer.standard")
	case LevelLight:
		return i18n.T("guardrail.disclaimer.light")
	default:
		return i18n.T("guardrail.disclaimer.standard")
	}
}

// ShortDisclaimer 返回给定护栏等级的简短免责声明文本。
func ShortDisclaimer(level Level) string {
	switch level {
	case LevelStrict:
		return i18n.T("guardrail.disclaimer.strict_short")
	case LevelStandard:
		return i18n.T("guardrail.disclaimer.standard_short")
	case LevelLight:
		return i18n.T("guardrail.disclaimer.light_short")
	default:
		return i18n.T("guardrail.disclaimer.standard_short")
	}
}

// LevelTag 返回给定护栏等级的审查标签。
func LevelTag(level Level) string {
	switch level {
	case LevelStrict:
		return i18n.T("guardrail.level_tag.strict")
	case LevelStandard:
		return i18n.T("guardrail.level_tag.standard")
	case LevelLight:
		return i18n.T("guardrail.level_tag.light")
	default:
		return i18n.T("guardrail.level_tag.standard")
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
	case "assistant":
		return []string{
			"生成法律文书", "自动提交", "发送给专利局",
			"正式申请", "官方提交",
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
	case "assistant":
		return []string{"正式提交", "发送给官方", "最终版本确认"}
	default:
		return nil
	}
}
