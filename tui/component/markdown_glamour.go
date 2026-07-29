package component

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/muesli/termenv"

	"github.com/xujian519/mady/tui/core"
	apitheme "github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// Glamour rendering backend
//
// This file provides an alternative markdown rendering backend using the
// glamour library (https://github.com/charmbracelet/glamour). It replaces
// the custom regex-based markdown parser with glamour's Goldmark-based
// renderer, providing better handling of edge cases (nested formatting,
// tables, code block syntax highlighting, etc.).
//
// The component-level Markdown type and BlockCache are unaffected — they
// continue to call renderMarkdown, which now delegates to glamour when
// available.
// ---------------------------------------------------------------------------

var (
	// cachedGlamourRenderer caches the most recently used glamour TermRenderer.
	// Invalidated when the width or theme changes.
	cachedGlamourRenderer *glamour.TermRenderer
	cachedGlamourWidth    int
)

// renderWithGlamour renders markdown source to ANSI lines using glamour.
// Returns the lines split by newline. On glamour failure, falls back to
// returning the raw source split into lines (no ANSI styling).
//
// Post-processing: because glamour's word wrapper does not handle CJK
// double-width characters correctly, we apply core.WrapAnsi as a safety
// net on any output line whose visible width exceeds the target width.
// This keeps CJK text readable while still using glamour's superior
// markdown parsing, syntax highlighting, and formatting.
func renderWithGlamour(src string, width int64) []string {
	if width < 10 {
		width = 80
	}
	renderer, err := buildGlamourRenderer(int(width))
	if err != nil {
		return strings.Split(src, "\n")
	}
	out, err := renderer.Render(src)
	if err != nil {
		return strings.Split(src, "\n")
	}
	// Remove trailing newline glamour always appends.
	out = strings.TrimSuffix(out, "\n")
	if out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")

	// Post-process: re-wrap any lines whose visible width exceeds the
	// target width. Glamour doesn't account for CJK double-width, so long
	// CJK strings may exceed the width bounds.
	var result []string
	for _, ln := range lines {
		if core.VisibleWidth(ln) > width {
			for _, w := range core.WrapAnsi(ln, width) {
				result = append(result, core.PadToWidth(w, width))
			}
		} else {
			result = append(result, ln)
		}
	}
	return result
}

// buildGlamourRenderer creates or returns a cached glamour TermRenderer for
// the given width. Uses the built-in "dark" style to match Mady's dark
// default theme. The cache is invalidated on width or theme changes.
func buildGlamourRenderer(width int) (*glamour.TermRenderer, error) {
	// Check cache hit.
	if cachedGlamourRenderer != nil && cachedGlamourWidth == width {
		return cachedGlamourRenderer, nil
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithColorProfile(termenv.TrueColor),
		glamour.WithStandardStyle("dark"),
	)
	if err != nil {
		return nil, err
	}

	cachedGlamourRenderer = r
	cachedGlamourWidth = width
	return r, nil
}

// InvalidateGlamourCache clears the cached renderer, forcing recreation on
// the next render call. Call this when the theme changes.
func InvalidateGlamourCache() {
	cachedGlamourRenderer = nil
}

// Register palette change listener to invalidate glamour cache automatically.
func init() {
	apitheme.SetOnSemanticThemeChange(func() {
		InvalidateGlamourCache()
	})
}
