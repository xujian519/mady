package guardrails

import (
	"fmt"
	"strings"
)

// ConsistencyRule checks output for internal contradictions.
// It detects pairs of statements that directly contradict each other,
// e.g., "技术方案为A" appearing near "技术方案不含A".
//
// This is a heuristic check; it catches obvious contradictions but may
// miss subtle ones. For comprehensive consistency checking, pair with
// LLM-based verification.
type ConsistencyRule struct {
	// Pairs lists contradiction pairs as (affirmative, negation).
	// Default pairs cover common Chinese patent/legal patterns.
	Pairs [][2]string

	// MaxWindow is the character window within which contradictions
	// are considered significant. Default: 2000.
	MaxWindow int

	// Action to take on finding contradictions. Default: ActionAlert.
	OnContradiction Action
}

// defaultContradictionPairs are common contradictory patterns in Chinese.
var defaultContradictionPairs = [][2]string{
	{"技术方案为", "技术方案不含"},
	{"属于", "不属于"},
	{"具有", "不具有"},
	{"包括", "不包括"},
	{"满足", "不满足"},
	{"符合", "不符合"},
	{"构成侵权", "不构成侵权"},
	{"具备新颖性", "不具备新颖性"},
	{"具备创造性", "不具备创造性"},
	{"公开充分", "公开不充分"},
}

// NewConsistencyRule creates a ConsistencyRule with defaults.
func NewConsistencyRule() *ConsistencyRule {
	return &ConsistencyRule{
		Pairs:           defaultContradictionPairs,
		MaxWindow:       2000,
		OnContradiction: ActionAlert,
	}
}

// Name returns the rule identifier.
func (r *ConsistencyRule) Name() string { return "consistency" }

// Check implements Rule.
func (r *ConsistencyRule) Check(content string, metadata map[string]any) RuleResult {
	contradictions := r.findContradictions(content)
	if len(contradictions) == 0 {
		return RuleResult{Passed: true}
	}

	msg := "检测到输出中存在内部矛盾：\n"
	for _, c := range contradictions {
		msg += fmt.Sprintf("  • %s\n", c)
	}
	msg += "建议复核以上矛盾点。"

	return RuleResult{
		Passed:   false,
		Severity: SeverityWarning,
		Action:   r.OnContradiction,
		Message:  msg,
	}
}

// findContradictions scans for pairs where both the affirmative and negative
// form appear within the configured window.
func (r *ConsistencyRule) findContradictions(content string) []string {
	window := r.MaxWindow
	if window <= 0 {
		window = 2000
	}

	var contradictions []string
	for _, pair := range r.Pairs {
		affIdx := strings.Index(content, pair[0])
		negIdx := strings.Index(content, pair[1])

		if affIdx >= 0 && negIdx >= 0 {
			dist := affIdx - negIdx
			if dist < 0 {
				dist = -dist
			}
			if dist <= window {
				contradictions = append(contradictions,
					fmt.Sprintf("「%s」与「%s」同时出现（相距%d字符）",
						pair[0], pair[1], dist))
			}
		}
	}
	return contradictions
}
