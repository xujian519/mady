package guardrails

import (
	"fmt"
	"strings"
)

// FormatCheckRule verifies that output contains required sections for
// the target domain. This is especially useful for patent and legal
// documents that have mandated structures.
//
// Configuration can be loaded from YAML (see rule_config.go) or
// constructed programmatically.
type FormatCheckRule struct {
	// RequiredSections lists section headings that must appear.
	// Matching is case-insensitive substring check.
	RequiredSections []string

	// MinSectionCount is the minimum number of required sections
	// that must be present. Default: len(RequiredSections).
	MinSectionCount int

	// Domain restricts this rule to specific domains.
	// Empty means all domains.
	Domain string

	// MissingSectionAction is the action when sections are missing.
	// Default: ActionAlert (warn but allow).
	MissingSectionAction Action
}

// NewFormatCheckRule creates a rule with the given required sections.
func NewFormatCheckRule(sections ...string) *FormatCheckRule {
	return &FormatCheckRule{
		RequiredSections:    sections,
		MissingSectionAction: ActionAlert,
	}
}

// PatentFormatCheck returns a pre-configured rule for patent documents.
func PatentFormatCheck() *FormatCheckRule {
	return &FormatCheckRule{
		RequiredSections: []string{
			"技术领域",
			"背景技术",
			"发明内容",
			"具体实施方式",
		},
		MinSectionCount:     3, // at least 3 of 4 required
		Domain:              "patent",
		MissingSectionAction: ActionAlert,
	}
}

// LegalFormatCheck returns a pre-configured rule for legal documents.
func LegalFormatCheck() *FormatCheckRule {
	return &FormatCheckRule{
		RequiredSections: []string{
			"案件事实",
			"法律依据",
			"分析",
			"结论",
		},
		MinSectionCount:     3,
		Domain:              "legal",
		MissingSectionAction: ActionAlert,
	}
}

// Name returns the rule identifier.
func (r *FormatCheckRule) Name() string { return "format-check" }

// Check implements Rule.
func (r *FormatCheckRule) Check(content string, metadata map[string]any) RuleResult {
	// Check domain restriction
	if r.Domain != "" {
		if domain, ok := metadata["domain"].(string); ok && domain != r.Domain {
			return RuleResult{Passed: true}
		}
	}

	lower := strings.ToLower(content)
	minRequired := r.MinSectionCount
	if minRequired <= 0 {
		minRequired = len(r.RequiredSections)
	}

	var missing []string
	found := 0
	for _, section := range r.RequiredSections {
		if strings.Contains(lower, strings.ToLower(section)) {
			found++
		} else {
			missing = append(missing, section)
		}
	}

	if found >= minRequired {
		return RuleResult{Passed: true}
	}

	msg := fmt.Sprintf("输出缺少以下必要章节（已找到%d/%d个）：\n", found, len(r.RequiredSections))
	for _, m := range missing {
		msg += fmt.Sprintf("  • %s\n", m)
	}

	return RuleResult{
		Passed:   false,
		Severity: SeverityWarning,
		Action:   r.MissingSectionAction,
		Message:  msg,
	}
}
