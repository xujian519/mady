package guardrails

import (
	"strings"
)

// ConfidenceCheckRule verifies that conclusive statements include
// appropriate confidence annotations. This is important for AI-generated
// analysis where users need to understand the reliability of the output.
//
// The rule scans for conclusive phrases (e.g., "综上所述", "因此", "最终结论")
// that lack nearby confidence markers (e.g., "置信度", "confidence", "通常").
type ConfidenceCheckRule struct {
	// ConclusivePhrases are patterns that indicate a final conclusion.
	ConclusivePhrases []string

	// ConfidenceMarkers are phrases that indicate uncertainty/confidence.
	ConfidenceMarkers []string

	// MaxDistance is the character distance within which a confidence
	// marker must appear after a conclusive phrase. Default: 200.
	MaxDistance int

	// MissingAnnotationText is injected when confidence is missing.
	MissingAnnotationText string
}

// defaultConclusivePhrases for Chinese patent/legal output.
var defaultConclusivePhrases = []string{
	"综上所述",
	"因此，",
	"最终结论",
	"综上，",
	"本发明的",
}

// defaultConfidenceMarkers for Chinese output.
var defaultConfidenceMarkers = []string{
	"置信度",
	"通常",
	"大概率",
	"建议人工",
	"仅供参考",
	"不构成",
	"confidence",
	"注意：",
}

// NewConfidenceCheckRule creates a rule with sensible defaults.
func NewConfidenceCheckRule() *ConfidenceCheckRule {
	return &ConfidenceCheckRule{
		ConclusivePhrases:     defaultConclusivePhrases,
		ConfidenceMarkers:     defaultConfidenceMarkers,
		MaxDistance:           200,
		MissingAnnotationText: "请注意：以上分析结论为 AI 生成，置信度较低，建议人工复核。",
	}
}

// Name returns the rule identifier.
func (r *ConfidenceCheckRule) Name() string { return "confidence-check" }

// Check implements Rule.
func (r *ConfidenceCheckRule) Check(content string, metadata map[string]any) RuleResult {
	if len(r.ConclusivePhrases) == 0 {
		return RuleResult{Passed: true}
	}

	maxDist := r.MaxDistance
	if maxDist <= 0 {
		maxDist = 200
	}

	lower := strings.ToLower(content)
	var missingFor []string

	for _, phrase := range r.ConclusivePhrases {
		phraseLower := strings.ToLower(phrase)
		idx := strings.Index(lower, phraseLower)
		if idx < 0 {
			continue
		}

		// Check if a confidence marker appears within MaxDistance after the phrase
		after := lower[idx+len(phraseLower):]
		if len(after) > maxDist {
			after = after[:maxDist]
		}

		hasMarker := false
		for _, marker := range r.ConfidenceMarkers {
			if strings.Contains(after, strings.ToLower(marker)) {
				hasMarker = true
				break
			}
		}

		if !hasMarker {
			missingFor = append(missingFor, phrase)
		}
	}

	if len(missingFor) == 0 {
		return RuleResult{Passed: true}
	}

	injectText := r.MissingAnnotationText
	if injectText == "" {
		injectText = "请注意：以上结论为 AI 生成，仅供参考。"
	}

	return RuleResult{
		Passed:   false,
		Severity: SeverityWarning,
		Action:   ActionInject,
		Message:  injectText,
	}
}
