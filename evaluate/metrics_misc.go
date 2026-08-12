package evaluate

import (
	"regexp"
	"strings"
)

// LengthScore rewards predictions whose rune length falls within an acceptable
// band. This discourages both terse non-answers and rambling outputs. The score
// is triangular: 1 inside [Min, Ideal], linearly decaying toward 0 outside.
type LengthScore struct {
	Min   int // minimum acceptable length (runes)
	Ideal int // length at which the score is 1.0
	Max   int // maximum acceptable length (runes)
}

// Name returns "length_score".
func (m LengthScore) Name() string { return "length_score" }

// Compute returns a triangular score based on rune length within [Min, Max].
func (m LengthScore) Compute(prediction, _ string) float64 {
	n := runeLen(prediction)
	minVal := m.Min
	if minVal <= 0 {
		minVal = 50
	}
	ideal := m.Ideal
	if ideal <= 0 {
		ideal = 500
	}
	maxVal := m.Max
	if maxVal <= 0 {
		maxVal = 3000
	}
	if n < minVal {
		return float64(n) / float64(minVal)
	}
	if n > maxVal {
		excess := n - maxVal
		decayWindow := maxVal / 2
		if decayWindow <= 0 {
			return 0
		}
		score := 1 - float64(excess)/float64(decayWindow)
		if score < 0 {
			return 0
		}
		return score
	}
	if n <= ideal {
		return float64(n-minVal) / float64(ideal-minVal)
	}
	return float64(maxVal-n) / float64(maxVal-ideal)
}

// ============================================================================
// Helpers
// ============================================================================

// tokenize splits text into single-rune tokens (lowercased), skipping
// whitespace and common punctuation. This is a deliberately simple tokenizer
// that works adequately for both Chinese and English F1 computation.
func tokenize(s string) []string {
	var tokens []string
	for _, r := range strings.ToLower(s) {
		if isSkipRune(r) {
			continue
		}
		tokens = append(tokens, string(r))
	}
	return tokens
}

func isSkipRune(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r':
		return true
	case ',', '.', '!', '?', ';', ':', '"', '\'', '`':
		return true
	case '\uff0c', '\u3002', '\uff01', '\uff1f', '\uff1b', '\uff1a', '\u201c', '\u201d', '\u2018', '\u2019', '\u3001':
		return true
	case '(', ')', '[', ']', '{', '}', '\uff08', '\uff09', '\u3010', '\u3011':
		return true
	}
	return false
}

// ExtractKeywords pulls salient terms from a reference string. It splits on
// delimiters and keeps tokens of at least 2 runes, deduplicating the result.
// This is a heuristic fallback for KeywordRecall when no explicit keyword set
// is available.
func ExtractKeywords(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ',' || r == '，' ||
			r == ';' || r == '；' || r == '|' || r == '、' || r == '。'
	})
	seen := make(map[string]bool, len(fields))
	var result []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if runeLen(f) < 2 || seen[f] {
			continue
		}
		seen[f] = true
		result = append(result, f)
	}
	return result
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// EvidenceGroundedness measures what fraction of evidence IDs cited in the
// prediction are valid (i.e. appear in the known evidence set). This catches
// hallucinated citations — a core risk in patent novelty assessment where a
// conclusion may reference a non-existent prior-art document.
//
// A prediction that cites no evidence scores 0 (ungrounded), not 1: the metric
// rewards evidence-backed conclusions. A prediction citing only valid IDs scores 1.
// (对齐 docs/specs/design-prior-art-retrieval-stage.md 第四节.)
type EvidenceGroundedness struct {
	// ValidEvidence is the set of known-valid evidence IDs (docIDs) available
	// to the model. Citations outside this set are treated as hallucinated.
	ValidEvidence []string
}

// Name returns "evidence_groundedness".
func (m EvidenceGroundedness) Name() string { return "evidence_groundedness" }

// WithCitations returns a new EvidenceGroundedness using the per-case valid
// evidence IDs. This lets the same metric instance adapt to each case's
// retrieved evidence set.
func (m EvidenceGroundedness) WithCitations(citations []string) Metric {
	m.ValidEvidence = citations
	return m
}

// Compute extracts cited evidence IDs from the prediction and returns the
// fraction that are valid. Returns 0 when no citations are present (ungrounded)
// and 1 when all citations are valid.
func (m EvidenceGroundedness) Compute(prediction, _ string) float64 {
	cited := extractCitedIDs(prediction)
	if len(cited) == 0 {
		return 0 // no evidence cited → ungrounded
	}
	if len(m.ValidEvidence) == 0 {
		return 0 // no valid evidence set available → cannot ground
	}
	validSet := make(map[string]bool, len(m.ValidEvidence))
	for _, e := range m.ValidEvidence {
		validSet[e] = true
	}
	valid := 0
	for c := range cited {
		if validSet[c] {
			valid++
		}
	}
	return float64(valid) / float64(len(cited))
}

// RuleComplianceCompleteness measures what fraction of confirmed rules are
// actually referenced in the prediction. This catches the "confirmed but
// ignored" gap: a rule may pass human review yet never surface in the final
// output, indicating the workflow skipped a required check.
// (对齐 docs/specs/design-rule-acquisition-stage.md 第五节.)
type RuleComplianceCompleteness struct {
	// Required is the set of confirmed rule IDs (e.g. NOV-001, A22.2) that
	// must be referenced in the prediction.
	Required []string
}

// Name returns "rule_compliance_completeness".
func (m RuleComplianceCompleteness) Name() string { return "rule_compliance_completeness" }

// WithCitations returns a new RuleComplianceCompleteness using the per-case
// confirmed rule set.
func (m RuleComplianceCompleteness) WithCitations(citations []string) Metric {
	m.Required = citations
	return m
}

// Compute returns the fraction of required rule IDs found in the prediction.
// Returns 1 when Required is empty (no rules to check → trivially complete).
func (m RuleComplianceCompleteness) Compute(prediction, _ string) float64 {
	if len(m.Required) == 0 {
		return 1
	}
	lowerPred := strings.ToLower(prediction)
	hit := 0
	for _, r := range m.Required {
		if strings.Contains(lowerPred, strings.ToLower(r)) {
			hit++
		}
	}
	return float64(hit) / float64(len(m.Required))
}

// extractCitedIDs pulls bracketed/doc-id-style identifiers from text.
// Recognizes patterns like "[CN001]", "doc_id: CN001", "CN001" bare tokens
// that match typical evidence-ID formats (alphanumeric with optional prefix).
var citedIDPattern = regexp.MustCompile(`(?:doc_id[:\s]*|\[)([A-Za-z0-9_\-]{3,})(?:\])?`)

func extractCitedIDs(text string) map[string]bool {
	matches := citedIDPattern.FindAllStringSubmatch(text, -1)
	out := make(map[string]bool, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			out[m[1]] = true
		}
	}
	return out
}
