package layout

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// stubComponent is a minimal Component for testing Flex layout and hit-testing.
type stubComponent struct {
	id     string
	height int64
}

func (s *stubComponent) Render(width int64) []string {
	out := make([]string, s.height)
	for i := range out {
		out[i] = s.id
	}
	return out
}
func (s *stubComponent) Invalidate() {}

// TestHitTestVertical verifies that HitTest returns the correct child and
// Rect for a vertical Flex layout.
func TestHitTestVertical(t *testing.T) {
	flex := NewFlex(DirectionVertical)
	flex.Bounds = &stubBounds{width: 80, height: 24}

	a := &stubComponent{id: "A", height: 3}
	b := &stubComponent{id: "B", height: 5}
	c := &stubComponent{id: "C", height: 10}

	flex.AddChild(Natural(a))
	flex.AddChild(Natural(b))
	flex.AddChild(Fill(c))

	flex.Render(80)

	// Row 0-2 → child A (rows 0,1,2)
	child, rect, ok := flex.HitTest(1, 10)
	if !ok || child != a {
		t.Fatalf("HitTest(1,10) = (%v, %v, %v), want (a, _, true)", child, rect, ok)
	}
	if rect.Row != 0 || rect.Height != 3 {
		t.Errorf("A rect = %+v, want Row=0 Height=3", rect)
	}

	// Row 3-7 → child B (5 rows)
	child, rect, ok = flex.HitTest(5, 0)
	if !ok || child != b {
		t.Fatalf("HitTest(5,0) = (%v, %v, %v), want (b, _, true)", child, rect, ok)
	}
	if rect.Row != 3 || rect.Height != 5 {
		t.Errorf("B rect = %+v, want Row=3 Height=5", rect)
	}

	// Row 8+ → child C (fill, 16 rows)
	child, rect, ok = flex.HitTest(20, 40)
	if !ok || child != c {
		t.Fatalf("HitTest(20,40) = (%v, %v, %v), want (c, _, true)", child, rect, ok)
	}
	if rect.Row != 8 || rect.Height != 16 {
		t.Errorf("C rect = %+v, want Row=8 Height=16", rect)
	}
}

// TestHitTestMiss verifies that HitTest returns false for coordinates outside
// all children.
func TestHitTestMiss(t *testing.T) {
	flex := NewFlex(DirectionVertical)
	flex.Bounds = &stubBounds{width: 80, height: 24}
	flex.AddChild(Natural(&stubComponent{id: "X", height: 3}))
	flex.Render(80)

	// Row 30 is beyond the flex's total height of 3.
	_, _, ok := flex.HitTest(30, 0)
	if ok {
		t.Fatal("HitTest(30,0) should miss, got ok=true")
	}
}

// TestHitTestBeforeRender verifies that HitTest returns false before Render
// has been called (no rects computed yet).
func TestHitTestBeforeRender(t *testing.T) {
	flex := NewFlex(DirectionVertical)
	flex.AddChild(Natural(&stubComponent{id: "X", height: 3}))
	_, _, ok := flex.HitTest(0, 0)
	if ok {
		t.Fatal("HitTest before Render should return false")
	}
}

// TestHitTestHorizontal verifies horizontal layout hit-testing.
func TestHitTestHorizontal(t *testing.T) {
	flex := NewFlex(DirectionHorizontal)
	flex.Bounds = &stubBounds{width: 80, height: 10}

	left := &stubComponent{id: "L", height: 5}
	right := &stubComponent{id: "R", height: 5}

	flex.AddChild(Fill(left))
	flex.AddChild(Fill(right))
	flex.Render(80)

	// Col 0-39 → left
	child, _, ok := flex.HitTest(0, 10)
	if !ok || child != left {
		t.Fatalf("HitTest(0,10) = (%v, _, %v), want (left, _, true)", child, ok)
	}

	// Col 40-79 → right
	child, _, ok = flex.HitTest(0, 50)
	if !ok || child != right {
		t.Fatalf("HitTest(0,50) = (%v, _, %v), want (right, _, true)", child, ok)
	}
}

// TestRectContains verifies the Rect.Contains helper used by HitTest.
func TestRectContains(t *testing.T) {
	r := core.Rect{Row: 5, Col: 10, Width: 3, Height: 4}

	tests := []struct {
		row, col int64
		want     bool
	}{
		{5, 10, true},  // top-left corner
		{8, 12, true},  // bottom-right (exclusive: row 5+4=9 excluded, col 10+3=13 excluded)
		{4, 10, false}, // above
		{9, 10, false}, // below
		{5, 9, false},  // left
		{5, 13, false}, // right
		{6, 11, true},  // interior
	}
	for _, tt := range tests {
		got := r.Contains(tt.row, tt.col)
		if got != tt.want {
			t.Errorf("Rect(%+v).Contains(%d,%d) = %v, want %v", r, tt.row, tt.col, got, tt.want)
		}
	}
}

// stubBounds implements BoundsProvider for tests.
type stubBounds struct {
	width, height int64
}

func (b *stubBounds) TerminalSize() (cols, rows int64) {
	return b.width, b.height
}
