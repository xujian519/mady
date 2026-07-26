package core

import "strings"

// SanitizeRawContent strips potentially dangerous terminal escape sequences
// from raw (non-cell-structured) content before writing to stdout.
//
// When the cell parser encounters escape sequences it cannot represent in
// the Cell grid (OSC, DCS, APC, non-SGR CSI), it marks the row as "Raw"
// and the renderer writes the content verbatim. This creates an injection
// vector: LLM-generated text containing OSC 8 hyperlinks, OSC 0 title
// changes, DCS device queries, or DEC private-mode switches (e.g. alt-screen
// ?1049h) would pass through unfiltered.
//
// SanitizeRawContent uses a strict whitelist: only SGR sequences (ESC[...m)
// and CursorMarker are preserved. All other escape sequences are stripped.
// Plain text (including lone ESC bytes from incomplete sequences) is passed
// through unchanged except for the stripped ESC bytes.
func SanitizeRawContent(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1B { // ESC
			adv := SkipAnsiSeq(s, i)
			if adv == 0 {
				// Incomplete escape sequence at end of input — strip the
				// lone ESC byte to prevent terminal confusion.
				i++
				continue
			}
			seq := s[i : i+adv]
			if isAllowedRawEscape(seq) {
				b.WriteString(seq)
			}
			// else: strip the sequence entirely
			i += adv
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// isAllowedRawEscape reports whether an escape sequence is safe to emit in
// raw content. Only SGR (color/style) and CursorMarker pass the whitelist.
func isAllowedRawEscape(seq string) bool {
	if isSGR(seq) {
		return true
	}
	return seq == CursorMarker
}

// isSGR reports whether seq is a complete SGR escape sequence (ESC[...m).
// It checks the three structural invariants directly rather than delegating
// to the substring-oriented isSGRSequence helper, which expects a position
// offset within a larger string.
func isSGR(seq string) bool {
	return len(seq) >= 3 && seq[0] == 0x1B && seq[1] == '[' && seq[len(seq)-1] == 'm'
}
