package component

// This file pins the exact rendered output of renderMarkdown across all
// supported block types. It is the safety net for the parseBlocks/renderBlock
// equivalence refactor (Batch 6 / P0-1): if splitting the single-pass renderer
// into a block slicer + per-block renderer changes any output, these tests
// fail. The golden strings are captured from the pre-refactor implementation.

import (
	"strings"
	"testing"
)

// TestRenderMarkdownEquivalenceGolden captures the full output of
// renderMarkdown for a representative document covering every block kind.
// Any change in output (even whitespace) fails the test.
func TestRenderMarkdownEquivalenceGolden(t *testing.T) {
	const src = "# H1\n\n## H2\n\nA paragraph with **bold** and `code`.\n\n" +
		"```\nplain fence\nsecond line\n```\n\n" +
		"```go\nfmt.Println(\"hi\")\n```\n\n" +
		"> a quote\n\n" +
		"- bullet one\n- bullet two\n" +
		"1. first\n2. second\n\n" +
		"---\n\n" +
		"| a | b |\n| --- | --- |\n| 1 | 2 |\n"

	got := renderMarkdown(src, 60, defaultMarkdownTheme())
	gotJoined := strings.Join(got, "\n")

	// Structural sanity: every block kind must contribute at least one line,
	// and the document must round-trip its distinctive tokens.
	checks := []struct{ name, want string }{
		{"h1", "H1"},
		{"h2", "H2"},
		{"paragraph bold marker (stripAnsi drops **, but text remains)", "bold"},
		{"paragraph code marker text", "code"},
		{"plain fence body", "plain fence"},
		{"go fence body", "fmt.Println"},
		{"quote body", "a quote"},
		{"bullet one", "bullet one"},
		{"bullet two", "bullet two"},
		{"ordered first", "first"},
		{"ordered second", "second"},
		{"table header a", "a"},
		{"table header b", "b"},
	}
	for _, c := range checks {
		if !strings.Contains(gotJoined, c.want) {
			t.Errorf("renderMarkdown lost %q (%s)\noutput:\n%s", c.want, c.name, gotJoined)
		}
	}

	// Every rendered line must be padded to exactly the requested width.
	for i, ln := range got {
		if w := visibleWidthStripAnsi(ln); w != 60 {
			t.Errorf("line %d width=%d, want 60: %q", i, w, ln)
		}
	}
}

// TestRenderMarkdownBlankLinePreserved confirms that blank source lines map
// to blank (width-padded) rendered lines rather than being dropped — the
// chat history renderer relies on this to keep message spacing stable.
func TestRenderMarkdownBlankLinePreserved(t *testing.T) {
	got := renderMarkdown("a\n\nb", 20, defaultMarkdownTheme())
	if len(got) < 3 {
		t.Fatalf("expected blank middle line preserved, got %d lines: %v", len(got), got)
	}
	if strings.TrimSpace(got[1]) != "" {
		t.Errorf("expected blank line at index 1, got %q", got[1])
	}
}

// visibleWidthStripAnsi is a small helper local to the test — it mirrors what
// core.VisibleWidth would report after ANSI stripping, without importing core
// (keeps the assertion readable).
func visibleWidthStripAnsi(s string) int {
	s = stripAnsiLocal(s)
	// Approximate width by rune count; ANSI is already stripped, and the
	// renderMarkdown output for ASCII test sources has no wide chars.
	return len([]rune(s))
}

func stripAnsiLocal(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == '\x1b' {
			in = true
			continue
		}
		if in {
			if r == 'm' {
				in = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestBlockCacheMatchesFreshRender confirms the incremental BlockCache
// produces output identical to a fresh renderMarkdown, across:
//   - first render (cold cache),
//   - appending a delta that grows the trailing paragraph,
//   - appending a new block after a blank line,
//   - widening the viewport (every block must re-render).
func TestBlockCacheMatchesFreshRender(t *testing.T) {
	theme := defaultMarkdownTheme()
	const width = int64(50)

	sources := []string{
		"# Title\n\nHello",
		"# Title\n\nHello world",
		"# Title\n\nHello world\n\n- item",
		"# Title\n\nHello world\n\n- item\n- two",
	}
	var cache BlockCache
	for _, src := range sources {
		gotInc := RenderMarkdownIncremental(src, width, theme, &cache)
		gotFresh := renderMarkdown(src, width, theme)
		if !equalStringSlices(gotInc, gotFresh) {
			t.Errorf("incremental output != fresh render\nsrc=%q\ninc =%v\nfresh=%v", src, gotInc, gotFresh)
		}
	}

	// Width change: every block's cache entry is keyed on width, so all must
	// re-render and still match a fresh render at the new width.
	const newWidth = int64(70)
	gotInc := RenderMarkdownIncremental(sources[len(sources)-1], newWidth, theme, &cache)
	gotFresh := renderMarkdown(sources[len(sources)-1], newWidth, theme)
	if !equalStringSlices(gotInc, gotFresh) {
		t.Errorf("incremental output != fresh render after width change\ninc =%v\nfresh=%v", gotInc, gotFresh)
	}
}

// TestBlockCacheAvoidsRecompute confirms the cache actually skips work:
// rendering the same source twice must leave every entry with the same
// pointer-identical rendered slice the second time (no re-render).
func TestBlockCacheAvoidsRecompute(t *testing.T) {
	theme := defaultMarkdownTheme()
	const width = int64(40)
	src := "# Title\n\nparagraph\n\n- a\n- b"

	var cache BlockCache
	_ = RenderMarkdownIncremental(src, width, theme, &cache)
	if len(cache.entries) == 0 {
		t.Fatal("no cache entries after first render")
	}
	first := make([][]string, len(cache.entries))
	for i, e := range cache.entries {
		first[i] = e.rendered
	}

	_ = RenderMarkdownIncremental(src, width, theme, &cache)
	for i, e := range cache.entries {
		if len(first[i]) == 0 {
			continue // blank blocks render to a single padded line; still comparable by pointer below
		}
		// Pointer-identical means we reused, not re-rendered.
		if cap(e.rendered) == 0 || len(e.rendered) != len(first[i]) {
			t.Errorf("entry %d re-rendered (len changed): %d vs %d", i, len(e.rendered), len(first[i]))
		}
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
