package component

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestViewportAccessors(t *testing.T) {
	v := NewViewport(5)
	content := []string{"a", "b", "c"}
	v.SetContent(content)
	if got := v.Content(); len(got) != 3 || got[1] != "b" {
		t.Fatalf("unexpected Content: %v", got)
	}
	// Snapshot semantics: mutating the returned slice must not affect the viewport.
	got := v.Content()
	got[0] = "MUTATED"
	if v.Content()[0] != "a" {
		t.Fatal("Content must return a copy")
	}
	if v.MaxRows() != 5 {
		t.Fatalf("expected MaxRows 5, got %d", v.MaxRows())
	}
	if v.Total() != 3 {
		t.Fatalf("expected Total 3, got %d", v.Total())
	}
	if !v.Following() {
		t.Fatal("expected following by default")
	}
	v.Invalidate() // no-op
}

func TestViewportSetMaxRowsClamps(t *testing.T) {
	v := NewViewport(2)
	v.SetContent([]string{"1", "2", "3", "4", "5"})
	v.ScrollBy(99) // clamp to max offset 3
	if v.Offset() != 3 {
		t.Fatalf("expected offset clamped to 3, got %d", v.Offset())
	}
	v.SetMaxRows(10) // total <= maxRows -> offset 0, follow on
	if v.Offset() != 0 {
		t.Fatalf("expected offset reset to 0, got %d", v.Offset())
	}
	if !v.Following() {
		t.Fatal("expected following after re-clamp")
	}
}

func TestViewportScrollByOffset(t *testing.T) {
	v := NewViewport(2)
	v.SetContent([]string{"1", "2", "3", "4"})
	v.ScrollBy(1)
	if v.Offset() != 1 {
		t.Fatalf("expected offset 1, got %d", v.Offset())
	}
	if v.Following() {
		t.Fatal("expected follow disabled after scroll")
	}
	v.ScrollBy(-1)
	if v.Offset() != 0 {
		t.Fatalf("expected offset 0, got %d", v.Offset())
	}
	// Negative offset clamps to 0 and re-enables follow.
	v.ScrollBy(-5)
	if v.Offset() != 0 || !v.Following() {
		t.Fatalf("expected offset 0 + following, got %d %v", v.Offset(), v.Following())
	}
	// ScrollBy(0) keeps follow state.
	v.ScrollBy(0)
	if !v.Following() {
		t.Fatal("expected follow unchanged by ScrollBy(0)")
	}
}

func TestViewportScrollToAndFollowTail(t *testing.T) {
	v := NewViewport(2)
	v.SetContent([]string{"1", "2", "3", "4"})
	v.ScrollTo(10)
	if v.Offset() != 2 {
		t.Fatalf("expected offset clamped to 2, got %d", v.Offset())
	}
	if v.Following() {
		t.Fatal("expected follow disabled by ScrollTo")
	}
	v.FollowTail()
	if v.Offset() != 0 || !v.Following() {
		t.Fatalf("expected tail + following, got %d %v", v.Offset(), v.Following())
	}
}

func TestViewportScrollbarConfig(t *testing.T) {
	v := NewViewport(3)
	v.SetScrollbarConfig(ScrollbarConfig{}) // zero -> width 1, default thumb
	sb := v.ScrollbarConfig()
	if sb.Width != 1 || sb.ThumbSymbol != '▐' {
		t.Fatalf("expected defaulted config, got %+v", sb)
	}
	v.SetScrollbarConfig(ScrollbarConfig{Mode: ScrollbarAlways, Width: 2, ThumbSymbol: 'x'})
	sb = v.ScrollbarConfig()
	if sb.Mode != ScrollbarAlways || sb.Width != 2 || sb.ThumbSymbol != 'x' {
		t.Fatalf("unexpected config %+v", sb)
	}
}

func TestViewportScrollbarEnabled(t *testing.T) {
	v := NewViewport(3)
	v.SetScrollbarEnabled(true)
	if v.ScrollbarConfig().Mode != ScrollbarAuto {
		t.Fatalf("expected auto mode, got %+v", v.ScrollbarConfig())
	}
	v.SetScrollbarEnabled(false)
	if v.ScrollbarConfig().Mode != ScrollbarNever {
		t.Fatalf("expected never mode, got %+v", v.ScrollbarConfig())
	}
}

func TestViewportIndicatorOffsets(t *testing.T) {
	v := NewViewport(2)
	v.SetContent([]string{"1", "2", "3", "4", "5"})
	v.SetIndicator(true)
	v.ScrollBy(1)
	lines := v.Render(10)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "1 more lines") {
		t.Fatalf("expected indicator line, got %q", lines[0])
	}
	// With 0 offset the indicator must not appear.
	v.FollowTail()
	lines = v.Render(10)
	if strings.Contains(lines[0], "more lines") {
		t.Fatalf("expected no indicator at tail, got %q", lines[0])
	}
	// Disabled indicator.
	v.ScrollBy(1)
	v.SetIndicator(false)
	lines = v.Render(10)
	if strings.Contains(lines[0], "more lines") {
		t.Fatalf("expected no indicator when disabled, got %q", lines[0])
	}
}

func TestViewportIndicatorFn(t *testing.T) {
	v := NewViewport(2)
	v.SetContent([]string{"1", "2", "3"})
	v.SetIndicator(true)
	v.ScrollBy(1)
	v.SetIndicatorFn(func(s string) string { return "[CUSTOM]" + s })
	lines := v.Render(10)
	if !strings.HasPrefix(lines[0], "[CUSTOM]") {
		t.Fatalf("expected custom indicator fn, got %q", lines[0])
	}
	// nil restores default.
	v.SetIndicatorFn(nil)
	lines = v.Render(10)
	if strings.HasPrefix(lines[0], "[CUSTOM]") {
		t.Fatalf("expected default indicator fn restored, got %q", lines[0])
	}
}

func TestViewportRenderSmallContent(t *testing.T) {
	v := NewViewport(5)
	v.SetContent([]string{"a", "b"})
	lines := v.Render(10)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w != 10 {
			t.Fatalf("line width %d != 10 (line=%q)", w, ln)
		}
	}
}

func TestViewportRenderWithScrollbar(t *testing.T) {
	v := NewViewport(3)
	v.SetContent([]string{"1", "2", "3", "4", "5", "6", "7"})
	v.SetScrollbarEnabled(true) // auto mode; content exceeds viewport -> scrollbar
	lines := v.Render(10)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w != 10 {
			t.Fatalf("line width %d != 10 (line=%q)", w, ln)
		}
	}
	// Scroll up so follow is off; thumb follows dims.
	v.ScrollBy(2)
	lines = v.Render(10)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

func TestViewportRenderScrollbarNever(t *testing.T) {
	v := NewViewport(3)
	v.SetContent([]string{"1", "2", "3", "4", "5", "6", "7"})
	v.SetScrollbarConfig(ScrollbarConfig{Mode: ScrollbarNever})
	lines := v.Render(10)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w != 10 {
			t.Fatalf("line width %d != 10 (line=%q)", w, ln)
		}
	}
}

func TestViewportRenderScrollbarAlwaysSmallContent(t *testing.T) {
	// Always mode still only renders the scrollbar when content overflows.
	v := NewViewport(5)
	v.SetContent([]string{"a", "b"})
	v.SetScrollbarConfig(ScrollbarConfig{Mode: ScrollbarAlways, Width: 2})
	lines := v.Render(10)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w != 10 {
			t.Fatalf("line width %d != 10 (line=%q)", w, ln)
		}
	}
}

func TestViewportRenderWidthOne(t *testing.T) {
	// Content taller than the viewport goes through the truncating path.
	v := NewViewport(2)
	v.SetContent([]string{"long content line", "x", "y"})
	lines := v.Render(1)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w > 1 {
			t.Fatalf("line width %d > 1 (line=%q)", w, ln)
		}
	}
}

func TestViewportRenderLongLinesTruncated(t *testing.T) {
	v := NewViewport(2)
	long := strings.Repeat("x", 100)
	v.SetContent([]string{long, "b", "c"}) // 3 rows > maxRows 2 -> truncation path
	v.SetScrollbarEnabled(true)
	lines := v.Render(12)
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w > 12 {
			t.Fatalf("line width %d > 12 (line=%q)", w, ln)
		}
	}
}

func TestViewportRenderMaxRowsZero(t *testing.T) {
	v := NewViewport(0)
	v.SetContent([]string{"a", "b"})
	lines := v.Render(10)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines for maxRows 0, got %d", len(lines))
	}
}

func TestViewportScrollByZeroKeepsFollow(t *testing.T) {
	v := NewViewport(2)
	v.SetContent([]string{"1", "2", "3", "4"})
	v.ScrollBy(0)
	if v.Offset() != 0 || !v.Following() {
		t.Fatalf("expected untouched state, got %d %v", v.Offset(), v.Following())
	}
}

func TestViewportClampLockedNegativeOffset(t *testing.T) {
	v := NewViewport(2)
	v.SetContent([]string{"1", "2", "3", "4"})
	v.mu.Lock()
	v.offset = -5
	v.follow = false
	v.clampLocked()
	v.mu.Unlock()
	if v.Offset() != 0 || !v.Following() {
		t.Fatalf("expected offset 0 + following, got %d %v", v.Offset(), v.Following())
	}
}
