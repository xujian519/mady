package component

import (
	"strings"
	"testing"
)

func TestViewportRenderNoClipping(t *testing.T) {
	v := NewViewport(10)
	v.SetContent([]string{"a", "b", "c"})
	lines := v.Render(10)
	if len(lines) != 3 {
		t.Fatalf("rendered %d lines, want 3", len(lines))
	}
	if !strings.Contains(lines[0], "a") {
		t.Fatalf("expected first line to contain 'a', got %q", lines[0])
	}
}

func TestViewportRenderClipsToTail(t *testing.T) {
	v := NewViewport(3)
	content := []string{"1", "2", "3", "4", "5"}
	v.SetContent(content)
	lines := v.Render(10)
	if len(lines) != 3 {
		t.Fatalf("rendered %d lines, want 3", len(lines))
	}
	if !strings.Contains(lines[0], "3") {
		t.Fatalf("expected first visible line 3, got %q", lines[0])
	}
	if !strings.Contains(lines[2], "5") {
		t.Fatalf("expected last visible line 5, got %q", lines[2])
	}
}

func TestViewportScrollBy(t *testing.T) {
	v := NewViewport(3)
	v.SetContent([]string{"1", "2", "3", "4", "5"})
	v.ScrollBy(2)
	lines := v.Render(10)
	if !strings.Contains(lines[0], "1") {
		t.Fatalf("expected first visible line 1 after scroll up, got %q", lines[0])
	}
	if !strings.Contains(lines[2], "3") {
		t.Fatalf("expected last visible line 3, got %q", lines[2])
	}
	if v.Following() {
		t.Fatal("scroll-by should disable follow-tail")
	}
}

func TestViewportScrollDown(t *testing.T) {
	v := NewViewport(3)
	v.SetContent([]string{"1", "2", "3", "4", "5"})
	v.ScrollBy(2)
	v.ScrollBy(-1)
	lines := v.Render(10)
	if !strings.Contains(lines[0], "2") {
		t.Fatalf("expected first visible line 2, got %q", lines[0])
	}
}

func TestViewportScrollToClamp(t *testing.T) {
	v := NewViewport(3)
	v.SetContent([]string{"1", "2", "3", "4", "5"})
	v.ScrollTo(100)
	if v.Offset() != 2 {
		t.Fatalf("offset = %d, want 2 (clamped)", v.Offset())
	}
}

func TestViewportFollowTail(t *testing.T) {
	v := NewViewport(3)
	v.SetContent([]string{"1", "2", "3", "4", "5"})
	v.ScrollBy(2)
	v.FollowTail()
	if !v.Following() {
		t.Fatal("expected follow-tail after FollowTail")
	}
	lines := v.Render(10)
	if !strings.Contains(lines[2], "5") {
		t.Fatalf("expected tail visible, got %q", lines[2])
	}
}

func TestViewportIndicator(t *testing.T) {
	v := NewViewport(3)
	v.SetContent([]string{"1", "2", "3", "4", "5"})
	v.SetIndicator(true)
	v.ScrollBy(2)
	lines := v.Render(10)
	if len(lines) != 3 {
		t.Fatalf("rendered %d lines, want 3", len(lines))
	}
	if !strings.Contains(lines[0], "more lines") {
		t.Fatalf("expected indicator, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "1") {
		t.Fatalf("expected indicator to consume one visible row, got %q", lines[1])
	}
}

func TestViewportSetMaxRows(t *testing.T) {
	v := NewViewport(10)
	v.SetContent([]string{"1", "2", "3", "4", "5"})
	v.SetMaxRows(2)
	lines := v.Render(10)
	if len(lines) != 2 {
		t.Fatalf("rendered %d lines, want 2", len(lines))
	}
	if !strings.Contains(lines[1], "5") {
		t.Fatalf("expected tail visible after resize, got %q", lines[1])
	}
}

func TestViewportPadToWidth(t *testing.T) {
	v := NewViewport(3)
	v.SetContent([]string{"hi"})
	lines := v.Render(10)
	if len(lines[0]) != 10 {
		t.Fatalf("padded width = %d, want 10", len(lines[0]))
	}
}

func TestViewportIndicatorCustomFn(t *testing.T) {
	v := NewViewport(3)
	v.SetContent([]string{"1", "2", "3", "4", "5"})
	v.SetIndicator(true)
	v.SetIndicatorFn(func(s string) string { return "[" + s + "]" })
	v.ScrollBy(2)
	lines := v.Render(10)
	if !strings.Contains(lines[0], "[") {
		t.Fatalf("expected custom indicator brackets, got %q", lines[0])
	}
}

func TestViewportAppendContent(t *testing.T) {
	v := NewViewport(3)
	v.SetContent([]string{"1", "2", "3"})
	v.SetContent([]string{"1", "2", "3", "4", "5"})
	lines := v.Render(10)
	if !strings.Contains(lines[2], "5") {
		t.Fatalf("expected tail to follow new content, got %q", lines[2])
	}
}
