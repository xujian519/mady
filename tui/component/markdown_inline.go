package component

import (
	"regexp"
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

func renderInline(s string, t MarkdownTheme) string {
	s = reInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.Trim(m, "`")
		return t.CodeInlineFn(inner)
	})
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
	return s
}
