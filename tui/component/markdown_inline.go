package component

import (
	"regexp"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Inline formatting
// ---------------------------------------------------------------------------

var (
	reInlineBold   = regexp.MustCompile(`\*\*([^*]+)\*\*|__([^_]+)__`)
	reInlineItalic = regexp.MustCompile(`\*([^*\s][^*]*[^*\s]|[^*\s])\*|_([^_\s][^_]*[^_\s]|[^_\s])_`)
	reInlineCode   = regexp.MustCompile("`([^`]+)`")
	reInlineStrike = regexp.MustCompile(`~~([^~]+)~~`)
	reInlineLink   = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

// ---------------------------------------------------------------------------
// Inline formatting
// ---------------------------------------------------------------------------

// renderInline applies inline formatting to a single line of markdown.
//
// Order matters: code spans are extracted FIRST and replaced with sentinels
// so the emphasis regexes below cannot match asterisks/underscores inside
// code (P2-18 — previously `a*b*c` got its `b` double-emphasized and `**`
// could pair across a code span). Asterisks adjacent to digits are also
// guarded so inline math like `2*3*4=24` is not italicized (P2-17).
func renderInline(s string, t MarkdownTheme) string {
	// Phase 1: extract code spans into sentinel placeholders.
	var codes []string
	s = reInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.Trim(m, "`")
		codes = append(codes, t.CodeInlineFn(inner))
		return "\x00" + strconv.Itoa(len(codes)-1) + "\x00"
	})

	// Phase 2: guard asterisks adjacent to digits (inline math).
	var mathGuards []string
	s = guardMathAsterisks(s, &mathGuards)

	// Phase 3: emphasis on the remaining (guarded) text.
	s = reInlineBold.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.Trim(m, "*_")
		return t.StrongFn(inner)
	})
	s = reInlineStrike.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.Trim(m, "~")
		return t.StrikeFn(inner)
	})
	s = reInlineItalic.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.Trim(m, "*_")
		return t.EmphasisFn(inner)
	})
	s = reInlineLink.ReplaceAllStringFunc(s, func(m string) string {
		sub := reInlineLink.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		return t.LinkLabelFn(sub[1]) + " " + t.LinkURLFn("("+sub[2]+")")
	})

	// Phase 4: restore the guarded asterisks, then the code spans.
	for _, g := range mathGuards {
		s = strings.Replace(s, g, "*", 1)
	}
	for i, c := range codes {
		s = strings.Replace(s, "\x00"+strconv.Itoa(i)+"\x00", c, 1)
	}
	return s
}

// guardMathAsterisks replaces every '*' that has a digit immediately before
// AND after it (e.g. the middle asterisk of `2*3`, `2*3*4`) with a \x01N\x01
// sentinel, so inline math survives the italic pass untouched (P2-17). A
// one-sided digit test is NOT enough: `**2**` / `*2*` would lose their
// opening/closing asterisk and the emphasis would be destroyed, so only
// both-sides-digit asterisks are guarded. The sentinels are restored by the
// caller.
func guardMathAsterisks(s string, guards *[]string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevDigit := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '*' {
			nextDigit := i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9'
			if prevDigit && nextDigit {
				g := "\x01" + strconv.Itoa(len(*guards)) + "\x01"
				*guards = append(*guards, g)
				b.WriteString(g)
				prevDigit = false
				continue
			}
		}
		b.WriteByte(c)
		prevDigit = c >= '0' && c <= '9'
	}
	return b.String()
}
