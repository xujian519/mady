package core

import (
	"os"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"golang.org/x/text/width"
)

// ---------------------------------------------------------------------------
// East-Asian Width aware visible width measurement.
//
// Supports:
//   - ASCII (width 1)
//   - Control characters (width 0)
//   - Combining marks (width 0)
//   - East-Asian Wide / Fullwidth (width 2)
//   - East-Asian Ambiguous characters (width 2 in CJK mode, 1 otherwise)
//   - Emoji basic ranges (width 2)
//   - Variation selectors / ZWJ (width 0)
//   - ANSI CSI/OSC escape sequences (width 0, transparently stripped)
//
// Ambiguous-width characters (e.g. em dash, ellipsis, middle dot) are
// measured according to the active CJK mode. The mode defaults to true when
// LANG/LC_CTYPE indicates a CJK locale, or when MADY_CJK_WIDTH=1.
// ---------------------------------------------------------------------------

var cjkMode atomic.Bool

func init() {
	if os.Getenv("MADY_CJK_WIDTH") == "1" {
		cjkMode.Store(true)
		return
	}
	for _, v := range []string{os.Getenv("LANG"), os.Getenv("LC_CTYPE")} {
		v = strings.ToLower(v)
		if strings.HasPrefix(v, "zh") || strings.HasPrefix(v, "ja") || strings.HasPrefix(v, "ko") {
			cjkMode.Store(true)
			return
		}
	}
}

// SetCJKMode toggles whether ambiguous-width characters are treated as 2 cells.
// Call this before rendering if the terminal is known to use a CJK font.
func SetCJKMode(cjk bool) { cjkMode.Store(cjk) }

// IsCJKMode reports whether ambiguous-width characters are currently counted as 2 cells.
func IsCJKMode() bool { return cjkMode.Load() }

// RuneWidth returns the cell width of a single rune (0, 1, or 2).
func RuneWidth(r rune) int64 {
	if r == 0 {
		return 0
	}
	if r < 0x20 || (r >= 0x7F && r < 0xA0) {
		return 0
	}
	if isZeroWidth(r) {
		return 0
	}

	// Use the standard East Asian Width table. Ambiguous characters get their
	// width from the active CJK mode; this is the main source of overflow bugs
	// when the code assumed width 1 but the terminal rendered them as 2.
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	case width.EastAsianAmbiguous:
		if cjkMode.Load() {
			return 2
		}
		return 1
	}

	// Fallback to the legacy table for ranges not classified above (e.g. some
	// historic CJK extensions or private-use emoji sequences).
	if isWide(r) {
		return 2
	}
	return 1
}

func isZeroWidth(r rune) bool {
	// Combining diacritical marks & variation selectors & ZWJ.
	switch {
	case r >= 0x0300 && r <= 0x036F:
		return true
	case r >= 0x0483 && r <= 0x0489:
		return true
	case r >= 0x0591 && r <= 0x05BD:
		return true
	case r == 0x05BF, r == 0x05C1, r == 0x05C2, r == 0x05C4, r == 0x05C5, r == 0x05C7:
		return true
	case r >= 0x0610 && r <= 0x061A:
		return true
	case r >= 0x064B && r <= 0x065F:
		return true
	case r == 0x0670:
		return true
	case r >= 0x06D6 && r <= 0x06DC:
		return true
	case r >= 0x06DF && r <= 0x06E4:
		return true
	case r >= 0x06E7 && r <= 0x06E8:
		return true
	case r >= 0x06EA && r <= 0x06ED:
		return true
	case r == 0x200B, r == 0x200C, r == 0x200D, r == 0x200E, r == 0x200F:
		return true
	case r >= 0x202A && r <= 0x202E:
		return true
	case r == 0x2060, r == 0x2061, r == 0x2062, r == 0x2063, r == 0x2064:
		return true
	case r == 0x2066, r == 0x2067, r == 0x2068, r == 0x2069:
		return true
	case r >= 0xFE00 && r <= 0xFE0F:
		return true
	case r == 0xFEFF:
		return true
	case r >= 0xE0100 && r <= 0xE01EF:
		return true
	}
	return false
}

// isWide reports whether the rune occupies 2 terminal cells.
// Ranges are the subset of UAX #11 W/F relevant to modern CJK + emoji.
func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F: // Hangul Jamo
		return true
	case r >= 0x2329 && r <= 0x232A:
		return true
	case r >= 0x2E80 && r <= 0x303E: // CJK Radicals, Kangxi, etc.
		return true
	case r >= 0x3041 && r <= 0x33FF: // Hiragana, Katakana, CJK Symbols, Bopomofo, Hangul Compat Jamo
		return true
	case r >= 0x3400 && r <= 0x4DBF: // CJK Ext A
		return true
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	case r >= 0xA000 && r <= 0xA4CF: // Yi
		return true
	case r >= 0xAC00 && r <= 0xD7A3: // Hangul Syllables
		return true
	case r >= 0xF900 && r <= 0xFAFF: // CJK Compat Ideographs
		return true
	case r >= 0xFE30 && r <= 0xFE4F: // CJK Compat Forms
		return true
	case r >= 0xFF00 && r <= 0xFF60: // Fullwidth Forms
		return true
	case r >= 0xFFE0 && r <= 0xFFE6:
		return true
	case r >= 0x1F300 && r <= 0x1F64F: // Misc Symbols + Emoticons
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // Transport
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental Symbols
		return true
	case r >= 0x1FA70 && r <= 0x1FAFF: // Symbols & Pictographs Ext A
		return true
	case r >= 0x20000 && r <= 0x2FFFD: // CJK Ext B..F
		return true
	case r >= 0x30000 && r <= 0x3FFFD: // CJK Ext G
		return true
	}
	return false
}

// VisibleWidth returns the number of terminal cells a string occupies,
// ignoring ANSI escape sequences and counting East-Asian wide runes as 2.
func VisibleWidth(s string) int64 {
	var w int64
	i := 0
	for i < len(s) {
		c := s[i]
		if c == 0x1B { // ESC
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
		w += RuneWidth(r)
		i += size
	}
	return w
}

// SkipAnsiSeq returns the number of bytes consumed by an ANSI escape
// starting at s[i], or 0 if s[i] is not the start of an escape sequence.
//
// Recognizes:
//
//	CSI:  ESC '[' ... final byte in 0x40..0x7E
//	OSC:  ESC ']' ... (BEL | ESC '\\')
//	APC:  ESC '_' ... (BEL | ESC '\\')  — used for CURSOR_MARKER
//	DCS:  ESC 'P' ... (BEL | ESC '\\')
//	PM:   ESC '^' ... (BEL | ESC '\\')
//	SS2/SS3: ESC 'N'/'O' + 1 byte
//	Two-byte: ESC <byte>
func SkipAnsiSeq(s string, i int) int {
	if i >= len(s) || s[i] != 0x1B {
		return 0
	}
	if i+1 >= len(s) {
		return 0
	}
	switch s[i+1] {
	case '[':
		j := i + 2
		for j < len(s) {
			b := s[j]
			if b >= 0x40 && b <= 0x7E {
				return j - i + 1
			}
			j++
		}
		return 0 // incomplete CSI — caller treats as pending
	case ']', '_', 'P', '^':
		j := i + 2
		for j < len(s) {
			if s[j] == 0x07 {
				return j - i + 1
			}
			if s[j] == 0x1B && j+1 < len(s) && s[j+1] == '\\' {
				return j - i + 2
			}
			j++
		}
		return 0 // incomplete string-terminator sequence
	case 'N', 'O':
		if i+2 < len(s) {
			return 3
		}
		return 0
	default:
		return 2
	}
}

// TruncateToWidth truncates s to at most maxWidth terminal cells, preserving
// ANSI codes and appending ellipsis ("…") if truncation occurred.
// Pass "" for ellipsis to disable.
func TruncateToWidth(s string, maxWidth int64, ellipsis string) string {
	if maxWidth <= 0 {
		return ""
	}
	total := VisibleWidth(s)
	if total <= maxWidth {
		return s
	}
	ellW := VisibleWidth(ellipsis)
	budget := maxWidth - ellW
	if budget < 0 {
		budget = 0
	}

	var b strings.Builder
	b.Grow(len(s))
	var used int64
	i := 0
	openStyles := false

	for i < len(s) {
		c := s[i]
		if c == 0x1B {
			adv := SkipAnsiSeq(s, i)
			if adv > 0 {
				b.WriteString(s[i : i+adv])
				openStyles = true
				if i+1 < len(s) && s[i+1] == '[' && adv >= 3 && s[i+adv-1] == 'm' &&
					(adv == 3 || s[i+2] == '0') {
					openStyles = false
				}
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
		if used+rw > budget {
			break
		}
		b.WriteString(s[i : i+size])
		used += rw
		i += size
	}

	if openStyles {
		b.WriteString(ansiReset)
	}
	b.WriteString(ellipsis)
	return b.String()
}

// PadToWidth right-pads s with spaces until its visible width equals width.
// If s is already wider it is returned unchanged.
func PadToWidth(s string, width int64) string {
	vw := VisibleWidth(s)
	if vw >= width {
		return s
	}
	return s + strings.Repeat(" ", int(width-vw))
}

// SliceByColumn returns the substring covering visible columns [start, end).
// ANSI escapes inside the range are preserved; a reset is appended if the
// slice starts or ends inside a styled region.
//
// A wide (2-cell) rune that begins before start but overlaps the range is
// included rather than dropped: dropping it would silently delete visible
// characters whenever a caller slices at a mid-glyph column (which CJK wrap
// and mouse selection can legitimately do).
func SliceByColumn(s string, start, end int64) string {
	if end <= start {
		return ""
	}
	var b strings.Builder
	var col int64
	i := 0
	openStyles := false

	for i < len(s) {
		c := s[i]
		if c == 0x1B {
			adv := SkipAnsiSeq(s, i)
			if adv > 0 {
				if col >= start && col < end {
					b.WriteString(s[i : i+adv])
				}
				openStyles = true
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
		if col+rw > end {
			break
		}
		if col+rw > start {
			b.WriteString(s[i : i+size])
		}
		col += rw
		i += size
	}
	if openStyles {
		b.WriteString(ansiReset)
	}
	return b.String()
}

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

func wrapOneLine(line string, width int64) []string {
	if VisibleWidth(line) <= width {
		return []string{line}
	}
	var out []string
	cur := line
	for VisibleWidth(cur) > width {
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
			return width
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

// ansiReset is kept local to avoid importing from style.go which uses a lowercase name.
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
