package patent

// ReasoningPatternRules returns check rules derived from the 18 standardized
// reasoning patterns. Each pattern encodes a canonical reasoning template from
// the patent re-examination knowledge base. Rules use PathElements for step-
// based verification and span creativity, novelty, claims, and other categories.
func ReasoningPatternRules() []CheckRule {
	patterns := AllPatterns()
	var rules []CheckRule
	for _, p := range patterns {
		rules = append(rules, p.CheckRules...)
	}
	return rules
}
