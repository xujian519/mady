package core

import (
	"testing"
)

// stubComponent is a minimal Component for Container tests.
type stubComponent struct {
	lines       []string
	invalidated bool
}

func (s *stubComponent) Render(width int64) []string { return s.lines }
func (s *stubComponent) Invalidate()                 { s.invalidated = true }

// disposableStub additionally records Dispose calls.
type disposableStub struct {
	stubComponent
	disposed bool
}

func (d *disposableStub) Dispose() { d.disposed = true }

// TestContainerLifecycle covers AddChild/RemoveChild/Clear/Children and the
// Disposable contract on removal.
func TestContainerLifecycle(t *testing.T) {
	c := NewContainer()
	a := &stubComponent{lines: []string{"a"}}
	b := &stubComponent{lines: []string{"b"}}

	c.AddChild(a)
	c.AddChild(nil) // nil children are ignored
	c.AddChild(b)
	if got := len(c.Children()); got != 2 {
		t.Fatalf("Children() len = %d, want 2", got)
	}

	// RemoveChild returns true and disposes Disposable children.
	d := &disposableStub{stubComponent: stubComponent{lines: []string{"d"}}}
	c.AddChild(d)
	if !c.RemoveChild(d) {
		t.Fatal("RemoveChild(d) = false, want true")
	}
	if !d.disposed {
		t.Fatal("Disposable child was not disposed on removal")
	}
	if c.RemoveChild(d) {
		t.Fatal("RemoveChild(d) second call = true, want false (already removed)")
	}

	// RemoveChild on a non-Disposable child still succeeds.
	if !c.RemoveChild(a) {
		t.Fatal("RemoveChild(a) = false, want true")
	}

	c.Clear()
	if got := len(c.Children()); got != 0 {
		t.Fatalf("Children() after Clear = %d, want 0", got)
	}
}

// TestContainerRender verifies vertical concatenation of child renders.
func TestContainerRender(t *testing.T) {
	c := NewContainer()
	c.AddChild(&stubComponent{lines: []string{"a1", "a2"}})
	c.AddChild(&stubComponent{lines: []string{"b1"}})

	got := c.Render(10)
	want := []string{"a1", "a2", "b1"}
	if len(got) != len(want) {
		t.Fatalf("Render() lines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Render()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestContainerInvalidate fans out to every child.
func TestContainerInvalidate(t *testing.T) {
	c := NewContainer()
	a := &stubComponent{}
	b := &stubComponent{}
	c.AddChild(a)
	c.AddChild(b)
	c.Invalidate()
	if !a.invalidated || !b.invalidated {
		t.Fatal("Invalidate() did not fan out to all children")
	}
}

// TestRectContains verifies inclusive top-left / exclusive bottom-right
// boundary semantics of the hit-test rectangle.
func TestRectContains(t *testing.T) {
	r := Rect{Row: 2, Col: 3, Width: 4, Height: 3}
	cases := []struct {
		row, col int64
		want     bool
	}{
		{2, 3, true}, // top-left corner, inclusive
		{4, 6, true}, // interior
		{4, 6 - 1, true},
		{4, 7, false}, // right edge, exclusive
		{4, 7 - 1, true},
		{4, 6 + 1, false},
		{4 + 1, 5, false}, // bottom edge, exclusive
		{2 - 1, 5, false}, // above
		{2, 3 - 1, false}, // left of
	}
	for _, tc := range cases {
		if got := r.Contains(tc.row, tc.col); got != tc.want {
			t.Errorf("Contains(%d,%d) = %v, want %v", tc.row, tc.col, got, tc.want)
		}
	}
}

// TestEnsureWidth verifies the width contract: no output line exceeds the
// target width, short lines are left untouched (never padded), and truncation
// reserves room for the ellipsis.
func TestEnsureWidth(t *testing.T) {
	got := EnsureWidth([]string{"hi", "你好世界", "this is long"}, 4)
	if got[0] != "hi" {
		t.Errorf("EnsureWidth()[0] = %q, want %q (short line untouched, no padding)", got[0], "hi")
	}
	for i, l := range got {
		if w := VisibleWidth(l); w > 4 {
			t.Errorf("EnsureWidth()[%d] = %q exceeds width (visible %d > 4)", i, l, w)
		}
	}
	// Truncation reserves room for the ellipsis: a 5-col line becomes 3 cols + "…".
	if got[2] != "thi…" {
		t.Errorf("EnsureWidth()[2] = %q, want %q", got[2], "thi…")
	}
}

// TestFillLines verifies count/width semantics, including zero-width.
func TestFillLines(t *testing.T) {
	if got := FillLines(0, 10); got != nil {
		t.Fatalf("FillLines(0, 10) = %v, want nil", got)
	}
	lines := FillLines(2, 5)
	if len(lines) != 2 {
		t.Fatalf("FillLines(2, 5) len = %d, want 2", len(lines))
	}
	for i, l := range lines {
		if VisibleWidth(l) != 5 {
			t.Errorf("FillLines[%d] width = %d, want 5", i, VisibleWidth(l))
		}
	}
	if got := FillLines(1, 0); len(got) != 1 || got[0] != "" {
		t.Fatalf("FillLines(1, 0) = %q, want single empty line", got)
	}
}
