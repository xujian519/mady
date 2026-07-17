package compiler

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/workflows/patent"
)

// CandidateToCheckRule converts a RuleCandidate into a patent.CheckRule for
// registration into the deterministic rule engine.
//
// Mapping (best-effort, human review required for structured fields):
//
//	c.DraftRuleText  → CheckRule.Message (user-visible failure message)
//	c.Description    → CheckRule.Name + Description
//	c.TriggerPattern → CheckRule.RequiredElements (keyword patterns)
//	c.Samples        → CheckRule.Severity (more samples → higher severity)
//
// Fields requiring human annotation (left empty):
//   - CheckType (domain expert must classify)
//   - Level (default LevelQuality, safest non-blocking)
//   - StepElements / Dimensions / RequiredAspects (type-specific)
//
// The caller MUST review and fill in the structured fields before registering
// the rule into production. This bridge provides a reasonable default skeleton
// but does not automate legal domain classification.
func CandidateToCheckRule(c RuleCandidate) patent.CheckRule {
	// Derive keywords from strategy preconditions + guidance fragments.
	keywords := extractKeywordsFromCandidate(c)

	// Severity based on sample reliability.
	severity := patent.SeverityMinor
	if c.Samples >= 10 && c.SuccessRate >= 0.9 {
		severity = patent.SeverityMajor
	}
	if c.Samples >= 20 && c.SuccessRate >= 0.95 {
		severity = patent.SeverityCritical
	}

	return patent.CheckRule{
		ID:               c.ID,
		Name:             c.Description,
		Description:      fmt.Sprintf("从策略 %s 蒸馏（%d 样本，成功率 %.0f%%）", c.StrategyID, c.Samples, c.SuccessRate*100),
		Level:            patent.LevelQuality, // safest non-blocking default
		Severity:         severity,
		Message:          c.DraftRuleText,
		CheckType:        "",                    // requires human classification
		RequiredElements: keywords,
		FixSuggestion:    c.Guidance,
	}
}

// extractKeywordsFromCandidate derives keyword patterns from the candidate's
// description and guidance fields. It splits on common delimiters and filters
// to maintain a reasonable keyword set for rule matching.
func extractKeywordsFromCandidate(c RuleCandidate) []string {
	seen := make(map[string]bool)
	var kw []string

	add := func(s string) {
		s = strings.TrimSpace(s)
		if len(s) < 2 || seen[s] {
			return
		}
		seen[s] = true
		kw = append(kw, s)
	}

	// Split Description and Guidance on common Chinese/English delimiters.
	for _, source := range []string{c.Description, c.Guidance} {
		for _, part := range strings.FieldsFunc(source, func(r rune) bool {
			return r == '，' || r == '、' || r == '；' || r == '。' || r == ',' || r == ';' || r == '.'
		}) {
			part = strings.TrimSpace(part)
			if len([]rune(part)) >= 2 && len([]rune(part)) <= 20 {
				add(part)
			}
		}
	}

	return kw
}

// NewRuleRegistrar creates a RuleRegistrar callback that converts candidates
// via CandidateToCheckRule and registers them into the given RuleEngine.
// Each registration is logged to the returned log slice (caller-owned).
func NewRuleRegistrar(engine *patent.RuleEngine) (registrar RuleRegistrar, logs *[]string) {
	logEntries := &[]string{}
	return func(c RuleCandidate) error {
		rule := CandidateToCheckRule(c)
		engine.RegisterRule(rule)
		entry := fmt.Sprintf("注册规则 %s: %s (来源策略 %s, 成功率 %.0f%%, %d 样本)",
			rule.ID, rule.Name, c.StrategyID, c.SuccessRate*100, c.Samples)
		*logEntries = append(*logEntries, entry)
		return nil
	}, logEntries
}
