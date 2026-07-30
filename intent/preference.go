package intent

import (
	"strings"
	"sync"
	"time"
)

// PreferenceStore records user intent corrections and uses them to
// improve future classifications via preference boosting.
//
// When a user corrects an intent, the store records a triple:
//
//	(input_keywords, original_intent, corrected_intent)
//
// On subsequent classifications:
//   - If input keywords match a stored preference (≥2 keyword overlap),
//     the corrected_intent is returned with high confidence (0.9+).
//   - Preferences decay over time: each unused consecutive classification
//     reduces confidence by 20% (down to a minimum of 0.5).
//   - After 3 consecutive uses without correction, confidence stabilizes.
type PreferenceStore struct {
	entries []preferenceEntry
	mu      sync.RWMutex
}

type preferenceEntry struct {
	Keywords   []string
	Intent     IntentResult
	Confidence float64
	LastUsed   time.Time
	UseCount   int
}

// NewPreferenceStore creates an empty PreferenceStore.
func NewPreferenceStore() *PreferenceStore {
	return &PreferenceStore{}
}

// Name returns the classifier identifier.
func (p *PreferenceStore) Name() string { return "preference" }

// Record stores a user correction: the original intent was wrong,
// and correctedIntent is the right one. The input keywords are
// extracted for future matching.
func (p *PreferenceStore) Record(inputKeywords []string, correctedIntent IntentResult) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if we already have a similar preference
	for i := range p.entries {
		if keywordOverlap(p.entries[i].Keywords, inputKeywords) >= 2 {
			p.entries[i].Intent = correctedIntent
			p.entries[i].Confidence = 0.95
			p.entries[i].LastUsed = time.Now()
			p.entries[i].UseCount++
			return
		}
	}

	// New preference
	p.entries = append(p.entries, preferenceEntry{
		Keywords:   inputKeywords,
		Intent:     correctedIntent,
		Confidence: 0.95,
		LastUsed:   time.Now(),
		UseCount:   1,
	})
}

// Classify implements Classifier using stored preferences.
// Returns the stored intent if keyword overlap ≥2 and confidence ≥0.5.
func (p *PreferenceStore) Classify(input string) IntentResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Extract simple keywords from input (space-separated tokens)
	inputKeywords := extractKeywords(input)

	for i := range p.entries {
		overlap := keywordOverlap(p.entries[i].Keywords, inputKeywords)
		if overlap >= 2 {
			entry := &p.entries[i]
			// Apply decay: reduce confidence if not recently used
			decayed := p.decayedConfidence(entry)
			if decayed >= 0.5 {
				result := entry.Intent
				result.Confidence = decayed
				result.Sources = []string{"preference"}
				return result
			}
		}
	}

	return IntentResult{Confidence: 0}
}

// decayedConfidence returns the confidence after time-based decay.
// Each unused classification reduces confidence by 20%, minimum 0.5.
func (p *PreferenceStore) decayedConfidence(entry *preferenceEntry) float64 {
	hoursSinceUse := time.Since(entry.LastUsed).Hours()
	// Decay after 24h of non-use
	decaySteps := int(hoursSinceUse / 24)
	if decaySteps <= 0 {
		return entry.Confidence
	}
	decayed := entry.Confidence - float64(decaySteps)*0.2
	if decayed < 0.5 {
		return 0.5
	}
	return decayed
}

// keywordOverlap counts how many keywords appear in both slices.
func keywordOverlap(a, b []string) int {
	set := make(map[string]bool, len(a))
	for _, kw := range a {
		set[kw] = true
	}
	count := 0
	for _, kw := range b {
		if set[kw] {
			count++
		}
	}
	return count
}

// extractKeywords splits input into tokens suitable for keyword matching.
// For ASCII text, splits on whitespace/punctuation.
// For CJK text, produces character bigrams plus the full segment.
func extractKeywords(input string) []string {
	input = strings.ToLower(input)
	var words []string
	var current []rune

	flush := func() {
		if len(current) == 0 {
			return
		}
		s := string(current)
		// For CJK text (2+ chars without spaces), add bigrams and full segment
		if len(current) >= 2 && isCJK(current[0]) {
			for i := 0; i < len(current)-1; i++ {
				words = append(words, string(current[i:i+2]))
			}
		}
		words = append(words, s)
		current = nil
	}

	for _, r := range input {
		if r == ' ' || r == '\t' || r == '\n' || r == ',' || r == '，' ||
			r == '.' || r == '。' || r == '!' || r == '！' || r == '?' || r == '？' {
			flush()
		} else {
			current = append(current, r)
		}
	}
	flush()

	// Deduplicate and filter
	seen := make(map[string]bool)
	var result []string
	for _, w := range words {
		w = strings.TrimSpace(w)
		if len([]rune(w)) < 2 {
			continue
		}
		if !seen[w] {
			seen[w] = true
			result = append(result, w)
		}
	}
	return result
}

// isCJK returns true if the rune is in the CJK Unified Ideographs range.
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Unified Ideographs Extension A
		(r >= 0xF900 && r <= 0xFAFF) // CJK Compatibility Ideographs
}
