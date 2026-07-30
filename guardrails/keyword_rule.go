package guardrails

import (
	"fmt"
	"strings"
)

// KeywordRule adapts the existing keyword-based guardrail (levels.go) to
// the Rule interface. It checks for blocked phrases, risk keywords, and
// approval keywords as individual rule checks.
//
// This allows the existing keyword guard to coexist with the new rule
// pipeline without code duplication.
type KeywordRule struct {
	// BlockedPhrases are strings that, if present, cause the output
	// to be blocked entirely.
	BlockedPhrases []string

	// RiskKeywords trigger disclaimer injection.
	RiskKeywords []string

	// Disclaimer is injected when RiskKeywords are found.
	Disclaimer string

	// ApprovalKeywords mark content for human review.
	ApprovalKeywords []string
}

// NewKeywordRule creates a rule from keyword lists.
func NewKeywordRule(blocked, risk, approval []string, disclaimer string) *KeywordRule {
	return &KeywordRule{
		BlockedPhrases:   blocked,
		RiskKeywords:     risk,
		ApprovalKeywords: approval,
		Disclaimer:       disclaimer,
	}
}

// Name returns the rule identifier.
func (r *KeywordRule) Name() string { return "keyword-block" }

// Check implements Rule. It checks in priority order:
//  1. Blocked phrases → ActionBlock
//  2. Risk keywords → ActionInject
//  3. Approval keywords → ActionAlert
func (r *KeywordRule) Check(content string, metadata map[string]any) RuleResult {
	// Step 1: Blocked phrases
	for _, phrase := range r.BlockedPhrases {
		if strings.Contains(content, phrase) {
			return RuleResult{
				Passed:   false,
				Severity: SeverityError,
				Action:   ActionBlock,
				Message:  "抱歉，该回复因内容安全原因被拦截。",
			}
		}
	}

	// Step 2: Risk keywords → inject disclaimer
	if len(r.RiskKeywords) > 0 {
		for _, kw := range r.RiskKeywords {
			if strings.Contains(content, kw) {
				disclaimer := r.Disclaimer
				if disclaimer == "" {
					disclaimer = "⚠️ 本回复由 AI 生成，仅供参考，不构成专业建议。"
				}
				// Avoid duplicate injection
				if strings.Contains(content, disclaimer) {
					return RuleResult{Passed: true}
				}
				return RuleResult{
					Passed:   false,
					Severity: SeverityWarning,
					Action:   ActionInject,
					Message:  disclaimer,
				}
			}
		}
	}

	// Step 3: Approval keywords
	if len(r.ApprovalKeywords) > 0 {
		for _, kw := range r.ApprovalKeywords {
			if strings.Contains(content, kw) {
				return RuleResult{
					Passed:   false,
					Severity: SeverityWarning,
					Action:   ActionAlert,
					Message:  fmt.Sprintf("输出包含审批关键词「%s」,建议人工复核。", kw),
				}
			}
		}
	}

	return RuleResult{Passed: true}
}
