// Package fuzzy provides string normalization and substring matching
// utilities used by TUI autocomplete components.
//
// This is a minimal subset of the top-level mady/fuzzy package, copied here
// to avoid a cross-module dependency when the TUI module is published as
// an independent Go sub-module.
package fuzzy

import (
	"strings"
	"unicode"
)

// NormalizeForMatch normalizes text for fuzzy comparison.
func NormalizeForMatch(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	lines := strings.Split(normalizeChars(s), "\n")
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimRightFunc(line, unicode.IsSpace))
	}
	return b.String()
}

func normalizeChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\u2018', '\u2019', '\u201A', '\u201B':
			b.WriteByte('\'')
		case '\u201C', '\u201D', '\u201E', '\u201F':
			b.WriteByte('"')
		case '\u2013', '\u2014', '\u2015':
			b.WriteByte('-')
		case '\u00A0', '\u2000', '\u2001', '\u2002', '\u2003', '\u2004',
			'\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A',
			'\u202F', '\u205F', '\u3000':
			b.WriteByte(' ')
		case '\r':
			// skip
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Find attempts to locate search within content using exact then normalized matching.
// The returned start and end are byte offsets into the original content string,
// suitable for content[start:end] slicing.
func Find(content, search string) (start int64, end int64, found bool) {
	if idx := strings.Index(content, search); idx >= 0 {
		return int64(idx), int64(idx + len(search)), true
	}

	normContent := NormalizeForMatch(content)
	normSearch := NormalizeForMatch(search)
	if normSearch == "" {
		return 0, 0, false
	}

	idx := strings.Index(normContent, normSearch)
	if idx < 0 {
		return 0, 0, false
	}

	origStart := mapNormalizedOffset(content, normContent, idx)
	origEnd := mapNormalizedOffset(content, normContent, idx+len(normSearch))

	return int64(origStart), int64(origEnd), true
}

// mapNormalizedOffset converts a byte offset in the normalized string back to
// the corresponding byte offset in the original string.
func mapNormalizedOffset(original, normalized string, normOffset int) int {
	oi := 0
	ni := 0
	origRunes := []rune(original)
	normRunes := []rune(normalized)

	origIdx := 0
	normIdx := 0

	for origIdx < len(origRunes) && normIdx < len(normRunes) {
		if ni >= normOffset {
			break
		}

		origR := origRunes[origIdx]
		normR := normRunes[normIdx]

		origRuneLen := len(string(origR))
		normRuneLen := len(string(normR))

		if origR == '\r' {
			oi += origRuneLen
			origIdx++
			continue
		}

		_ = normR
		oi += origRuneLen
		ni += normRuneLen
		origIdx++
		normIdx++
	}

	return oi
}
