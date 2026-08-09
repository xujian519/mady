package component

import (
	"strings"
	"unicode/utf8"

	core "github.com/xujian519/mady/tui/core"
)

// ---------------------------------------------------------------------------
// Bridge from the top-level fuzzy package into TUI constructs.
//
// The fuzzy package (fuzzy/fuzzy.go) specialises in whitespace-tolerant
// substring search — useful for letting the user paste a snippet and locate
// its exact position inside a larger document, even if whitespace differs.
//
// This file exposes that capability as TUI primitives:
//   - NormalizeForMatch: passthrough so components can normalise inputs.
//   - SubstringFuzzyMatch: reports whether query appears inside candidate.
//   - SubstringFuzzyFilter: filters candidates that contain query.
//   - FuzzyContentProvider: AutocompleteProvider that searches inside a
//     corpus of content (e.g. chat history, file body) and surfaces the
//     matched excerpt.
// ---------------------------------------------------------------------------

// NormalizeForMatch re-exports core.NormalizeForMatch so TUI consumers
// don't have to import the core package directly.
func NormalizeForMatch(s string) string { return core.NormalizeForMatch(s) }

// SubstringFuzzyMatch reports whether `query` appears inside `candidate`
// after normalising both sides (trailing whitespace collapsed, smart
// punctuation replaced with ASCII, etc.). Returns the matched byte range
// and ok=true on success.
func SubstringFuzzyMatch(candidate, query string) (start, end int64, ok bool) {
	return core.Find(candidate, query)
}

// SubstringFuzzyFilter filters candidates that contain `query` under the
// normalised substring rules, preserving original order.
func SubstringFuzzyFilter(query string, candidates []string) []string {
	if query == "" {
		out := make([]string, len(candidates))
		copy(out, candidates)
		return out
	}
	var out []string
	for _, c := range candidates {
		if _, _, ok := core.Find(c, query); ok {
			out = append(out, c)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// FuzzyContentProvider — Autocomplete provider that searches a corpus.
// ---------------------------------------------------------------------------

// ContentEntry describes one searchable document.
type ContentEntry struct {
	// Key is the label used for display.
	Key string
	// Body is the searchable text.
	Body string
	// Tag is opaque metadata surfaced in the Suggestion.
	Tag any
}

// FuzzyContentProvider implements AutocompleteProvider by searching bodies.
type FuzzyContentProvider struct {
	TriggerStr string
	Entries    []ContentEntry
	// MaxPreview caps the excerpt length; 0 = 48 characters.
	MaxPreview int64
}

// Trigger returns the configured prefix (default "#").
func (p *FuzzyContentProvider) Trigger() string {
	if p.TriggerStr == "" {
		return "#"
	}
	return p.TriggerStr
}

// Complete scans every entry for a normalised substring match.
func (p *FuzzyContentProvider) Complete(token string) []core.Suggestion {
	if token == "" {
		out := make([]core.Suggestion, 0, len(p.Entries))
		for _, e := range p.Entries {
			out = append(out, core.Suggestion{
				Label:       e.Key,
				InsertText:  e.Key,
				Description: previewText(e.Body, p.previewLen()),
				Tag:         e.Tag,
			})
		}
		return out
	}
	var out []core.Suggestion
	for _, e := range p.Entries {
		start, end, ok := core.Find(e.Body, token)
		if !ok {
			continue
		}
		excerpt := makeExcerpt(e.Body, start, end, p.previewLen())
		out = append(out, core.Suggestion{
			Label:       e.Key,
			InsertText:  e.Key,
			Description: excerpt,
			Tag:         e.Tag,
		})
	}
	return out
}

func (p *FuzzyContentProvider) previewLen() int64 {
	if p.MaxPreview > 0 {
		return p.MaxPreview
	}
	return 48
}

func previewText(s string, maxLen int64) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\t", " ")
	if int64(len(s)) <= maxLen {
		return s
	}
	// Truncate on a rune boundary — byte slicing can cut a multi-byte
	// UTF-8 sequence in half and render a replacement character (P2-24).
	runes := []rune(s)
	if int64(len(runes)) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}

func makeExcerpt(body string, start, end, maxLen int64) string {
	if end-start > maxLen {
		end = start + maxLen
	}
	b := int64(len(body))
	lo := start - maxLen/2
	if lo < 0 {
		lo = 0
	}
	hi := end + maxLen/2
	if hi > b {
		hi = b
	}
	// Align the slice bounds to UTF-8 rune boundaries: core.Find returns
	// byte offsets that are themselves on boundaries, but the ±maxLen/2
	// expansion can land mid-rune, producing invalid UTF-8 in the excerpt
	// (P2-24).
	lo = prevRuneBoundary(body, lo)
	hi = nextRuneBoundary(body, hi)
	out := body[lo:hi]
	out = strings.ReplaceAll(strings.ReplaceAll(out, "\n", " "), "\t", " ")
	if lo > 0 {
		out = "…" + out
	}
	if hi < b {
		out += "…"
	}
	return out
}

// prevRuneBoundary returns the largest index ≤ i that is a UTF-8 rune
// boundary (i itself when already on one).
func prevRuneBoundary(s string, i int64) int64 {
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}

// nextRuneBoundary returns the smallest index ≥ i that is a UTF-8 rune
// boundary (i itself when already on one).
func nextRuneBoundary(s string, i int64) int64 {
	for i < int64(len(s)) && !utf8.RuneStart(s[i]) {
		i++
	}
	return i
}
