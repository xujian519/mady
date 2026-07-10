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

// MetricFunc adapts a plain function into a Metric.
type MetricFunc struct {
	MetricName string
	Run        func(prediction, reference string) float64
}

func (m MetricFunc) Name() string      { return m.MetricName }
func (m MetricFunc) Compute(p, r string) float64 { return m.Run(p, r) }

// ============================================================================
// ExactMatch
// ============================================================================

// ExactMatch scores 1 when prediction equals reference (after optional
// case-folding and whitespace trimming), 0 otherwise.
type ExactMatch struct {
	CaseSensitive bool
}

func (m ExactMatch) Name() string { return "exact_match" }

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

func (F1Score) Name() string { return "f1" }

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

func (m KeywordRecall) Name() string { return "keyword_recall" }

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
// CitationCompleteness
// ============================================================================

// CitationCompleteness measures what fraction of required citation identifiers
// appear in the prediction. This is essential for legal/patent workflows where
// every conclusion must trace back to specific source documents.
type CitationCompleteness struct {
	// Required is the set of citation identifiers (docIDs, article numbers,
	// etc.) that must appear in the prediction.
	Required []string
}

func (m CitationCompleteness) Name() string { return "citation_completeness" }

func (m CitationCompleteness) Compute(prediction, _ string) float64 {
	if len(m.Required) == 0 {
		return 1
	}
	lower := strings.ToLower(prediction)
	hit := 0
	for _, c := range m.Required {
		if strings.Contains(lower, strings.ToLower(c)) {
			hit++
		}
	}
	return float64(hit) / float64(len(m.Required))
}

// ============================================================================
// LengthScore
// ============================================================================

// LengthScore rewards predictions whose rune length falls within an acceptable
// band. This discourages both terse non-answers and rambling outputs. The score
// is triangular: 1 inside [Min, Ideal], linearly decaying toward 0 outside.
type LengthScore struct {
	Min    int // minimum acceptable length (runes)
	Ideal  int // length at which the score is 1.0
	Max    int // maximum acceptable length (runes)
}

// DefaultLengthScore returns a LengthScore tuned for paragraph-length answers.
func DefaultLengthScore() LengthScore {
	return LengthScore{Min: 50, Ideal: 500, Max: 3000}
}

func (m LengthScore) Name() string { return "length_score" }

func (m LengthScore) Compute(prediction, _ string) float64 {
	n := runeLen(prediction)
	min := m.Min
	if min <= 0 {
		min = 50
	}
	ideal := m.Ideal
	if ideal <= 0 {
		ideal = 500
	}
	max := m.Max
	if max <= 0 {
		max = 3000
	}
	if n < min {
		return float64(n) / float64(min)
	}
	if n > max {
		if max <= 0 {
			return 0
		}
		excess := n - max
		decayWindow := max / 2
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
		return float64(n-min) / float64(ideal-min)
	}
	return float64(max-n) / float64(max-ideal)
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
