package component

import (
	"strings"
	"sync"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestTextSetters(t *testing.T) {
	txt := NewText("initial")
	txt.SetText("updated")
	if txt.GetText() != "updated" {
		t.Fatalf("expected 'updated', got %q", txt.GetText())
	}
	txt.SetPadding(2, 1)
	if txt.paddingX != 2 || txt.paddingY != 1 {
		t.Fatalf("unexpected padding %d,%d", txt.paddingX, txt.paddingY)
	}
	txt.SetPadding(-1, -3)
	if txt.paddingX != 0 || txt.paddingY != 0 {
		t.Fatalf("expected negative padding clamped to 0, got %d,%d", txt.paddingX, txt.paddingY)
	}
	txt.SetBgFn(func(s string) string { return s })
	txt.Render(10)
	txt.Update(core.KeyMsg{Data: "x"}) // no-op
}

func TestTextRenderCacheHit(t *testing.T) {
	txt := NewText("hello world")
	first := txt.Render(8)
	second := txt.Render(8) // cache hit — same slice
	if len(first) != len(second) {
		t.Fatalf("cache mismatch: %d vs %d", len(first), len(second))
	}
	// Invalidate forces a fresh render.
	txt.Invalidate()
	third := txt.Render(8)
	if len(third) == 0 {
		t.Fatal("expected lines after Invalidate")
	}
	// Different width bypasses the cache.
	fourth := txt.Render(16)
	if len(fourth) != len(first) && len(fourth) == 0 {
		t.Fatal("expected re-render at new width")
	}
}

func TestTextRenderPaddingAndBg(t *testing.T) {
	var mu sync.Mutex
	bgCalls := 0
	txt := NewText("abc")
	txt.SetPadding(2, 1)
	txt.SetBgFn(func(s string) string {
		mu.Lock()
		bgCalls++
		mu.Unlock()
		return s
	})
	lines := txt.Render(10)
	// padY top + 1 wrapped line + padY bottom = 3
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w != 10 {
			t.Fatalf("line width %d != 10 (line=%q)", w, ln)
		}
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Fatalf("expected 2-space padding, got %q", lines[1])
	}
	mu.Lock()
	defer mu.Unlock()
	if bgCalls == 0 {
		t.Fatal("expected bgFn called")
	}
}

func TestTextRenderWrapWidthOne(t *testing.T) {
	txt := NewText("abc")
	lines := txt.Render(1)
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w != 1 {
			t.Fatalf("line width %d != 1 (line=%q)", w, ln)
		}
	}
}

func TestTruncatedTextSetters(t *testing.T) {
	tt := NewTruncatedText("old")
	tt.SetText("new")
	tt.SetPadding(1, 2)
	if tt.paddingX != 1 || tt.paddingY != 2 {
		t.Fatalf("unexpected padding %d,%d", tt.paddingX, tt.paddingY)
	}
	tt.SetPadding(-5, 0)
	if tt.paddingX != 0 {
		t.Fatalf("expected negative padding clamped, got %d", tt.paddingX)
	}
	tt.Invalidate() // no-op
	tt.Update(core.KeyMsg{Data: "x"})
}

func TestTruncatedTextRender(t *testing.T) {
	tt := NewTruncatedText("the quick brown fox")
	lines := tt.Render(10)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if w := core.VisibleWidth(lines[0]); w != 10 {
		t.Fatalf("line width %d != 10 (line=%q)", w, lines[0])
	}
	if !strings.Contains(lines[0], "…") {
		t.Fatalf("expected ellipsis, got %q", lines[0])
	}
	// Short text not truncated.
	tt.SetText("hi")
	lines = tt.Render(10)
	if strings.Contains(lines[0], "…") {
		t.Fatalf("unexpected ellipsis, got %q", lines[0])
	}
}

func TestTruncatedTextNoEllipsis(t *testing.T) {
	tt := NewTruncatedText(strings.Repeat("x", 50))
	tt.SetEllipsis("")
	lines := tt.Render(10)
	if strings.Contains(lines[0], "…") {
		t.Fatalf("unexpected ellipsis, got %q", lines[0])
	}
	if w := core.VisibleWidth(lines[0]); w != 10 {
		t.Fatalf("line width %d != 10 (line=%q)", w, lines[0])
	}
}

func TestTruncatedTextPadding(t *testing.T) {
	tt := NewTruncatedText("abc")
	tt.SetPadding(1, 1)
	lines := tt.Render(8)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines with padY=1, got %d", len(lines))
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w != 8 {
			t.Fatalf("line width %d != 8 (line=%q)", w, ln)
		}
	}
	if !strings.HasPrefix(lines[1], " ") {
		t.Fatalf("expected leading padding, got %q", lines[1])
	}
}

func TestSpacerSetRowsAndClamp(t *testing.T) {
	s := NewSpacer(3)
	s.SetRows(5)
	if len(s.Render(10)) != 5 {
		t.Fatalf("expected 5 rows after SetRows, got %d", len(s.Render(10)))
	}
	s.SetRows(-2) // clamped to 0
	if len(s.Render(10)) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(s.Render(10)))
	}
	s.Invalidate() // no-op
	s.Update(core.KeyMsg{Data: "x"})
}

func TestSpacerMinimumOneRow(t *testing.T) {
	s := NewSpacer(0)
	if len(s.Render(4)) != 1 {
		t.Fatalf("expected minimum 1 row, got %d", len(s.Render(4)))
	}
}
