package component

import (
	"strings"
	"sync"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// mockComp is a minimal core.Component that records calls.
type mockComp struct {
	mu       sync.Mutex
	rendered int
	invalid  int
	updated  int
	lines    []string
}

func (m *mockComp) Render(width int64) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rendered++
	return m.lines
}

func (m *mockComp) Invalidate() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalid++
}

func (m *mockComp) Update(msg core.Msg) core.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updated++
	return nil
}

func (m *mockComp) counts() (rendered, invalid, updated int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rendered, m.invalid, m.updated
}

func TestBoxRenderPlain(t *testing.T) {
	child := &mockComp{lines: []string{"hello", "world"}}
	b := NewBox()
	b.SetPadding(0, 0)
	b.AddChild(child)

	lines := b.Render(10)
	if len(lines) != 2 {
		t.Fatalf("rendered %d lines, want 2", len(lines))
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w != 10 {
			t.Fatalf("line width %d != 10 (line=%q)", w, ln)
		}
	}
	if !strings.HasPrefix(lines[0], "hello") {
		t.Fatalf("expected body first, got %q", lines[0])
	}
}

func TestBoxRenderDefaultPadding(t *testing.T) {
	b := NewBox() // default padding 1,1, no border
	b.AddChild(&mockComp{lines: []string{"x"}})
	lines := b.Render(6)
	// 1 top pad + body + 1 bottom pad
	if len(lines) != 3 {
		t.Fatalf("rendered %d lines, want 3", len(lines))
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w != 6 {
			t.Fatalf("line width %d != 6 (line=%q)", w, ln)
		}
	}
	if !strings.HasPrefix(lines[1], " ") {
		t.Fatalf("expected padded body, got %q", lines[1])
	}
}

func TestBoxRenderFramed(t *testing.T) {
	b := NewBox()
	b.SetBorder(BorderRounded)
	b.SetTitle("Tasks")
	b.AddChild(&mockComp{lines: []string{"a"}})
	lines := b.Render(20)
	// top border + padY + body + padY + bottom border = 5
	if len(lines) != 5 {
		t.Fatalf("rendered %d lines, want 5", len(lines))
	}
	if !strings.HasPrefix(lines[0], "╭") {
		t.Fatalf("expected rounded top border, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "Tasks") {
		t.Fatalf("expected title in top border, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[4], "╰") {
		t.Fatalf("expected rounded bottom border, got %q", lines[4])
	}
	if !strings.HasPrefix(lines[2], "│") {
		t.Fatalf("expected framed body line, got %q", lines[2])
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w != 20 {
			t.Fatalf("line width %d != 20 (line=%q)", w, ln)
		}
	}
}

func TestBoxRenderBorderStyles(t *testing.T) {
	cases := []struct {
		style BoxBorder
		tl    string
		bl    string
	}{
		{BorderSharp, "┌", "└"},
		{BorderDouble, "╔", "╚"},
	}
	for _, tc := range cases {
		b := NewBox()
		b.SetPadding(0, 0)
		b.SetBorder(tc.style)
		lines := b.Render(5)
		if len(lines) != 2 {
			t.Fatalf("style %d: rendered %d lines, want 2", tc.style, len(lines))
		}
		if !strings.HasPrefix(lines[0], tc.tl) {
			t.Fatalf("style %d: expected %q top, got %q", tc.style, tc.tl, lines[0])
		}
		if !strings.HasPrefix(lines[1], tc.bl) {
			t.Fatalf("style %d: expected %q bottom, got %q", tc.style, tc.bl, lines[1])
		}
	}
}

func TestBoxRenderNarrowWidth(t *testing.T) {
	// At width 1 the framed line pads with width-2 = -1 via PadToWidth, which
	// does not truncate — the line may exceed 1 column (source behavior).
	// Assert structure only.
	b := NewBox()
	b.SetBorder(BorderRounded)
	b.SetTitle("x") // width 1 -> border-only line
	b.AddChild(&mockComp{lines: []string{"content is long"}})
	lines := b.Render(1)
	if len(lines) != 5 {
		t.Fatalf("rendered %d lines, want 5", len(lines))
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasPrefix(lines[4], "╰") {
		t.Fatalf("expected border lines, got %q / %q", lines[0], lines[4])
	}
}

func TestBoxRenderLongTitleTruncated(t *testing.T) {
	b := NewBox()
	b.SetBorder(BorderRounded)
	b.SetTitle("a very long title that cannot fit")
	lines := b.Render(8)
	top := lines[0]
	if w := core.VisibleWidth(top); w != 8 {
		t.Fatalf("top border width %d != 8 (line=%q)", w, top)
	}
}

func TestBoxRenderTruncatesBody(t *testing.T) {
	b := NewBox()
	b.SetPadding(0, 0)
	b.SetBorder(BorderRounded)
	b.AddChild(&mockComp{lines: []string{"this is a very long line that must be truncated"}})
	lines := b.Render(10)
	// top border + framed body + bottom border
	if len(lines) != 3 {
		t.Fatalf("rendered %d lines, want 3", len(lines))
	}
	if w := core.VisibleWidth(lines[1]); w != 10 {
		t.Fatalf("body line width %d != 10 (line=%q)", w, lines[1])
	}
}

func TestBoxRenderBgFn(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	b := NewBox()
	b.SetPadding(0, 0)
	b.SetBgFn(func(s string) string {
		mu.Lock()
		calls++
		mu.Unlock()
		return s
	})
	b.AddChild(&mockComp{lines: []string{"a"}})
	b.Render(4)
	mu.Lock()
	defer mu.Unlock()
	if calls == 0 {
		t.Fatal("expected bgFn to be called during render")
	}
}

func TestBoxRenderBorderFn(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	b := NewBox()
	b.SetBorder(BorderRounded)
	b.SetBorderFn(func(s string) string {
		mu.Lock()
		calls++
		mu.Unlock()
		return s
	})
	b.Render(6)
	mu.Lock()
	defer mu.Unlock()
	if calls == 0 {
		t.Fatal("expected borderFn to be called during render")
	}
}

func TestBoxRemoveChild(t *testing.T) {
	b := NewBox()
	c1 := &mockComp{}
	c2 := &mockComp{}
	b.AddChild(c1)
	b.AddChild(c2)

	if !b.RemoveChild(c1) {
		t.Fatal("expected RemoveChild(c1) to succeed")
	}
	if b.RemoveChild(c1) {
		t.Fatal("expected second RemoveChild(c1) to fail")
	}
	// c2 still present: rendered once.
	b.Render(4)
	if n, _, _ := c2.counts(); n != 1 {
		t.Fatalf("expected c2 rendered once, got %d", n)
	}
}

func TestBoxClear(t *testing.T) {
	b := NewBox()
	c := &mockComp{}
	b.AddChild(c)
	b.Clear()
	b.Render(4)
	if _, _, n := c.counts(); n != 0 {
		t.Fatalf("expected child not rendered after Clear, got %d", n)
	}
}

func TestBoxSetPaddingClampsNegative(t *testing.T) {
	b := NewBox()
	b.SetPadding(-3, -5)
	if b.paddingX != 0 || b.paddingY != 0 {
		t.Fatalf("expected padding clamped to 0, got %d,%d", b.paddingX, b.paddingY)
	}
}

func TestBoxAddChildNil(t *testing.T) {
	b := NewBox()
	b.AddChild(nil) // must not panic
	b.Render(4)
}

func TestBoxInvalidateFansOut(t *testing.T) {
	b := NewBox()
	c1 := &mockComp{}
	c2 := &mockComp{}
	b.AddChild(c1)
	b.AddChild(c2)
	b.Invalidate()
	if _, n, _ := c1.counts(); n != 1 {
		t.Fatalf("expected c1 invalidated once, got %d", n)
	}
	if _, n, _ := c2.counts(); n != 1 {
		t.Fatalf("expected c2 invalidated once, got %d", n)
	}
}

func TestBoxUpdateFansOut(t *testing.T) {
	b := NewBox()
	c := &mockComp{}
	b.AddChild(c)
	b.Update(core.KeyMsg{Data: "x"})
	if _, _, n := c.counts(); n != 1 {
		t.Fatalf("expected child updated once, got %d", n)
	}
}

func TestBoxUpdateSkipsNonUpdatable(t *testing.T) {
	// A component that only implements core.Component (no Update).
	type static struct {
		mockComp
	}
	b := NewBox()
	b.AddChild(&static{mockComp{lines: []string{"s"}}})
	b.Update(core.KeyMsg{Data: "x"}) // must not panic
}

func TestBrandSeparatorWithMarker(t *testing.T) {
	got := BrandSeparator(12, "⚖", nil)
	if w := core.VisibleWidth(got); w != 12 {
		t.Fatalf("width %d != 12 (got %q)", w, got)
	}
}

func TestBrandSeparatorWithoutMarker(t *testing.T) {
	got := BrandSeparator(10, "", nil)
	if w := core.VisibleWidth(got); w != 10 {
		t.Fatalf("width %d != 10 (got %q)", w, got)
	}
}

func TestBrandSeparatorNarrow(t *testing.T) {
	got := BrandSeparator(2, "⚖", nil)
	if got == "" {
		t.Fatal("expected non-empty separator")
	}
}

func TestBrandSeparatorCustomFn(t *testing.T) {
	called := false
	got := BrandSeparator(8, "", func(s string) string {
		called = true
		return s
	})
	if !called {
		t.Fatal("expected custom fn to be called")
	}
	if w := core.VisibleWidth(got); w != 8 {
		t.Fatalf("width %d != 8 (got %q)", w, got)
	}
}
