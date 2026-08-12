package evaluate

import (
	"strings"
)

// Metric scores a single prediction against a reference answer, returning a
// value in [0,1] where 1 is best. Implementations must be deterministic for a
// given (prediction, reference) pair so that results are reproducible.
type Metric interface {
	// Name is the metric identifier used in reports and aggregate maps.
	Name() string
	// Compute returns the score in [0,1].
	Compute(prediction, reference string) float64
}

// CitationAwareMetric is a Metric that can accept per-case required citations.
type CitationAwareMetric interface {
	Metric
	// WithCitations returns a new metric instance that uses the given per-case
	// required citations instead of any default set at construction time.
	WithCitations(citations []string) Metric
}

// MetricFunc implements Metric via a function field, useful for inline metric
// definitions or test stubs.
type MetricFunc struct {
	MetricName string
	Run        func(prediction, reference string) float64
}

// Name returns the metric identifier.
func (m MetricFunc) Name() string { return m.MetricName }

// Compute delegates to the Run function field.
func (m MetricFunc) Compute(p, r string) float64 { return m.Run(p, r) }

// ============================================================================
// ExactMatch
// ============================================================================

// ExactMatch scores 1 when prediction equals reference (after optional
// case-folding and whitespace trimming), 0 otherwise.
type ExactMatch struct {
	CaseSensitive bool
}

// Name returns "exact_match".
func (m ExactMatch) Name() string { return "exact_match" }

// Compute returns 1 when prediction equals reference (after optional
// case-folding and whitespace trimming), 0 otherwise.
func (m ExactMatch) Compute(prediction, reference string) float64 {
	p := strings.TrimSpace(prediction)
	r := strings.TrimSpace(reference)
	if !m.CaseSensitive {
		p = strings.ToLower(p)
		r = strings.ToLower(r)
	}
	if p == r {
		return 1
	}
	return 0
}

// ============================================================================
// F1Score (token-level)
// ============================================================================

// F1Score computes token-level precision, recall, and their harmonic mean.
// Tokenization is rune-based (single-character tokens) so it works for both
// Chinese and English text without an external tokenizer.
type F1Score struct{}

// Name returns "f1".
func (F1Score) Name() string { return "f1" }

// Compute returns the token-level F1 score between prediction and reference.
func (F1Score) Compute(prediction, reference string) float64 {
	predTokens := tokenize(prediction)
	refTokens := tokenize(reference)
	if len(predTokens) == 0 && len(refTokens) == 0 {
		return 1
	}
	if len(predTokens) == 0 || len(refTokens) == 0 {
		return 0
	}

	refCounts := make(map[string]int, len(refTokens))
	for _, t := range refTokens {
		refCounts[t]++
	}

	var overlap int
	predCounts := make(map[string]int, len(predTokens))
	for _, t := range predTokens {
		predCounts[t]++
	}
	for t, pc := range predCounts {
		if rc := refCounts[t]; rc < pc {
			overlap += rc
		} else {
			overlap += pc
		}
	}
	if overlap == 0 {
		return 0
	}
	precision := float64(overlap) / float64(len(predTokens))
	recall := float64(overlap) / float64(len(refTokens))
	return 2 * precision * recall / (precision + recall)
}

// ============================================================================
// KeywordRecall
// ============================================================================

// KeywordRecall measures what fraction of the reference's keywords appear in
// the prediction. Keywords are extracted from the reference via [ExtractKeywords]
// unless an explicit keyword set is provided.
type KeywordRecall struct {
	// Keywords, when non-empty, overrides automatic extraction.
	Keywords []string
}

// Name returns "keyword_recall".
func (m KeywordRecall) Name() string { return "keyword_recall" }

// Compute returns the fraction of keywords present in the prediction.
func (m KeywordRecall) Compute(prediction, reference string) float64 {
	keywords := m.Keywords
	if len(keywords) == 0 {
		keywords = ExtractKeywords(reference)
	}
	if len(keywords) == 0 {
		return 1
	}
	lower := strings.ToLower(prediction)
	hit := 0
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			hit++
		}
	}
	return float64(hit) / float64(len(keywords))
}

// ============================================================================
