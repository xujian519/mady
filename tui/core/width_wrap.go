package core

import (
	"strings"
	"unicode/utf8"
)

// WrapAnsi hard-wraps text to at most width cells per line, preserving ANSI
// styles across line breaks (each wrapped line reopens the active SGR state
// and appends a reset).
//
// This is a minimal word-break implementation: it breaks on whitespace when
// possible, otherwise breaks in the middle of a word/glyph.
func WrapAnsi(text string, width int64) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		lines = append(lines, wrapOneLine(para, width)...)
	}
	return lines
}

// maxWrapIterations caps the number of break iterations per input line. Each
// iteration must consume at least one glyph (findBreakColumn now returns a
// glyph boundary even for wide runes in narrow widths), so this bound is never
// reached on well-formed input — it exists purely so a future regression in
// the break-column logic degrades to an over-wide line instead of hanging the
// TUI event loop.
const maxWrapIterations = 4096

func wrapOneLine(line string, width int64) []string {
	if VisibleWidth(line) <= width {
		return []string{line}
	}
	var out []string
	cur := line
	for iter := 0; VisibleWidth(cur) > width; iter++ {
		if iter >= maxWrapIterations {
			// Defensive: emit the remainder as-is rather than loop forever.
			out = append(out, cur)
			return out
		}
		// Try to break on the last whitespace that still fits.
		breakAt := findBreakColumn(cur, width)
		left := SliceByColumn(cur, 0, breakAt)
		right := SliceByColumn(cur, breakAt, VisibleWidth(cur))
		out = append(out, strings.TrimRight(left, " \t"))
		cur = strings.TrimLeft(right, " \t")
		if cur == "" {
			return out
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// isCJKPunctuation reports whether r is a full-width punctuation at which
// a Chinese/Japanese/Korean line may preferentially break.
func isCJKPunctuation(r rune) bool {
	switch r {
	case '，', '。', '；', '：', '！', '？', '、',
		'（', '）', '「', '」', '『', '』', '【', '】', '《', '》', '〈', '〉',
		'—', '…', '·':
		return true
	}
	return false
}

// findBreakColumn returns the column at which to wrap the string: the column
// right after the last whitespace that fits within width. Falls back to the
// last soft-break boundary (path punctuation like '/', '-', '_', '.', ':' and
// full-width CJK punctuation) and finally to the last complete-glyph boundary.
// Returning a mid-glyph column would make the SliceByColumn calls in wrapOneLine
// drop that boundary rune entirely — visible as missing characters at the end
// of every wrapped CJK line.
func findBreakColumn(s string, width int64) int64 {
	var col int64
	var lastWS int64 = -1
	var lastSoft int64 = -1
	i := 0
	for i < len(s) {
		c := s[i]
		if c == 0x1B {
			adv := SkipAnsiSeq(s, i)
			if adv > 0 {
				i += adv
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			i++
			continue
		}
		rw := RuneWidth(r)
		if col+rw > width {
			if lastWS > 0 {
				return lastWS
			}
			if lastSoft > 0 {
				return lastSoft
			}
			if col > 0 {
				return col // last complete-glyph boundary
			}
			// col == 0: the first glyph is wider than width itself (e.g. a
			// 2-cell CJK rune in a 1-column window). Return the glyph's full
			// width: slicing at `width` would cut the wide rune and produce
			// an empty left slice, so wrapOneLine would never make progress
			// and hang. The wrapped line overflows by design — a wide glyph
			// cannot fit, but it must still be emitted whole.
			return col + rw
		}
		switch r {
		case ' ', '\t':
			lastWS = col + rw
		case '/', '-', '_', '.':
			lastSoft = col + rw
		}
		if isCJKPunctuation(r) {
			lastSoft = col + rw
		}
		col += rw
		i += size
	}
	return col
}

// WrapAnsiCJK wraps text with CJK-aware line breaking. It prefers breaking
// after full-width punctuation and, when safe, avoids leaving a punctuation
// character alone at the start of a wrapped line.
func WrapAnsiCJK(text string, width int64) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		lines = append(lines, wrapOneLineCJK(para, width)...)
	}
	return lines
}

func wrapOneLineCJK(line string, width int64) []string {
	if VisibleWidth(line) <= width {
		return []string{line}
	}
	out := WrapAnsi(line, width)
	// Defensive pass: if a wrapped line starts with a CJK punctuation and the
	// previous line has room, move the punctuation up so it doesn't dangle at
	// the beginning of a line. Skip lines containing ANSI escapes to avoid
	// splitting style sequences.
	for i := 1; i < len(out); i++ {
		if strings.Contains(out[i], "\x1b") {
			continue
		}
		r, size := utf8.DecodeRuneInString(out[i])
		if !isCJKPunctuation(r) {
			continue
		}
		if VisibleWidth(out[i-1])+RuneWidth(r) > width {
			continue
		}
		out[i-1] = out[i-1] + string(r)
		out[i] = out[i][size:]
	}
	return out
}

// ansiReset is the SGR reset sequence, kept local to core (theme/style.go has its own).
const ansiReset = "\x1b[0m"

func StripAnsi(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1B {
			adv := SkipAnsiSeq(s, i)
			if adv > 0 {
				i += adv
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
