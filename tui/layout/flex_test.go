package layout

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
)

type fixedComp struct {
	lines        []string
	renderCount  int
	measureCount int
}

func (c *fixedComp) Render(width int64) []string {
	c.renderCount++
	out := make([]string, len(c.lines))
	for i, l := range c.lines {
		out[i] = core.PadToWidth(l, width)
	}
	return out
}

func (c *fixedComp) Invalidate() {}

func (c *fixedComp) Measure(width int64) int64 {
	c.measureCount++
	return int64(len(c.lines))
}

type bounds struct{ w, h int64 }

func (b *bounds) TerminalSize() (cols, rows int64) { return b.w, b.h }

func TestFlexVerticalNatural(t *testing.T) {
	a := &fixedComp{lines: []string{"a1", "a2"}}
	b := &fixedComp{lines: []string{"b1"}}
	flex := NewFlex(DirectionVertical, Natural(a), Natural(b))
	flex.Bounds = &bounds{w: 10, h: 10}

	out := flex.Render(10)
	want := []string{"a1        ", "a2        ", "b1        "}
	assertLines(t, out, want)
}

func TestFlexVerticalFill(t *testing.T) {
	a := &fixedComp{lines: []string{"header"}}
	b := &fixedComp{lines: []string{"body"}}
	flex := NewFlex(DirectionVertical, Natural(a), Fill(b))
	flex.Bounds = &bounds{w: 10, h: 6}

	out := flex.Render(10)
	// header=1, fill=5
	want := []string{
		"header    ",
		"body      ", "          ", "          ", "          ", "          ",
	}
	assertLines(t, out, want)
}

func TestFlexVerticalFillWithCallback(t *testing.T) {
	a := &fixedComp{lines: []string{"header"}}
	b := &fixedComp{lines: []string{"body"}}
	var allocated int64
	flex := NewFlex(DirectionVertical, Natural(a), Fill(b).WithAllocate(func(h int64) {
		allocated = h
	}))
	flex.Bounds = &bounds{w: 10, h: 6}

	flex.Render(10)
	if allocated != 5 {
		t.Fatalf("allocated=%d, want 5", allocated)
	}
}

func TestFlexVerticalFixedAndNatural(t *testing.T) {
	a := &fixedComp{lines: []string{"a"}}
	b := &fixedComp{lines: []string{"b"}}
	c := &fixedComp{lines: []string{"c"}}
	flex := NewFlex(DirectionVertical, Natural(a), Fixed(b, 3), Natural(c))
	flex.Bounds = &bounds{w: 10, h: 10}

	out := flex.Render(10)
	want := []string{
		"a         ",
		"b         ", "          ", "          ",
		"c         ",
	}
	assertLines(t, out, want)
}

func TestFlexChildRect(t *testing.T) {
	a := &fixedComp{lines: []string{"a"}}
	b := &fixedComp{lines: []string{"b"}}
	c := &fixedComp{lines: []string{"c"}}
	flex := NewFlex(DirectionVertical, Natural(a), Fixed(b, 3), Natural(c))
	flex.Bounds = &bounds{w: 10, h: 10}

	flex.Render(10)
	if r := flex.ChildRect(0); r.Row != 0 || r.Height != 1 {
		t.Fatalf("rect0=%+v", r)
	}
	if r := flex.ChildRect(1); r.Row != 1 || r.Height != 3 {
		t.Fatalf("rect1=%+v", r)
	}
	if r := flex.ChildRect(2); r.Row != 4 || r.Height != 1 {
		t.Fatalf("rect2=%+v", r)
	}
}

func TestFlexHorizontalFixed(t *testing.T) {
	a := &fixedComp{lines: []string{"aa"}}
	b := &fixedComp{lines: []string{"bbb"}}
	flex := NewFlex(DirectionHorizontal, Fixed(a, 3), Fixed(b, 4))
	flex.Bounds = &bounds{w: 20, h: 10}

	out := flex.Render(20)
	if len(out) != 1 {
		t.Fatalf("len(out)=%d, want 1", len(out))
	}
	if out[0] != "aa bbb " {
		t.Fatalf("line=%q", out[0])
	}
}

func TestFlexSizerAvoidsDoubleRender(t *testing.T) {
	a := &fixedComp{lines: []string{"a", "b"}}
	b := &fixedComp{lines: []string{"c"}}
	// SizeMin/SizeMax use Sizer for measurement; each child should be
	// measured once and rendered once.
	flex := NewFlex(DirectionVertical, Min(a, 1), Max(b, 5))
	flex.Bounds = &bounds{w: 10, h: 10}
	flex.Render(10)

	if a.renderCount != 1 || b.renderCount != 1 {
		t.Fatalf("a.renderCount=%d b.renderCount=%d, both want 1", a.renderCount, b.renderCount)
	}
	if a.measureCount != 1 || b.measureCount != 1 {
		t.Fatalf("a.measureCount=%d b.measureCount=%d, both want 1", a.measureCount, b.measureCount)
	}
}

func TestFlexNoBounds(t *testing.T) {
	a := &fixedComp{lines: []string{"a"}}
	b := &fixedComp{lines: []string{"b"}}
	flex := NewFlex(DirectionVertical, Natural(a), Natural(b))
	out := flex.Render(10)
	want := []string{"a         ", "b         "}
	assertLines(t, out, want)
}

func TestFlexInvalidation(t *testing.T) {
	a := &fixedComp{lines: []string{"a"}}
	flex := NewFlex(DirectionVertical, Natural(a))
	flex.Invalidate()
}

func TestFlexHorizontalFill(t *testing.T) {
	a := &fixedComp{lines: []string{"a"}}
	b := &fixedComp{lines: []string{"b"}}
	flex := NewFlex(DirectionHorizontal, Natural(a), Fill(b))
	flex.Bounds = &bounds{w: 10, h: 10}
	out := flex.Render(10)
	if len(out) != 1 {
		t.Fatalf("len(out)=%d, want 1", len(out))
	}
	if out[0] != "a         b" {
		t.Fatalf("line=%q", out[0])
	}
}

// TestFlexHorizontalNaturalStarvesFill 记录一个已知的布局陷阱：
// renderHorizontal 对 SizeNatural 会分配全部父宽度（见 flex.go 的注释
// "treat it as the full parent width for now"），导致同级的 Fill 子组件
// 被压缩到最小宽度 1（renderHorizontal 有 if w < 1 { w = 1 } 保护）。
// 需要固定宽度列时必须用 Fixed，而不是 Natural。
func TestFlexHorizontalNaturalStarvesFill(t *testing.T) {
	left := &fixedComp{lines: []string{"left"}}
	main := &fixedComp{lines: []string{"header", "editor"}}
	flex := NewFlex(DirectionHorizontal, Natural(left), Fill(main))
	flex.Bounds = &bounds{w: 100, h: 10}
	flex.Render(100)
	// Natural 在水平模式占满 100，Fill(main) 被压到最小宽度 1。
	if r := flex.ChildRect(0); r.Width != 100 {
		t.Fatalf("Natural left Width=%d, want 100 (gotcha: Natural starves siblings)", r.Width)
	}
	if r := flex.ChildRect(1); r.Width != 1 {
		t.Fatalf("Fill main Width=%d, want 1 (Natural starved it to the min-guard)", r.Width)
	}
}

// TestFlexHorizontalFixedWithFill 验证固定宽度列应使用 Fixed 而非 Natural：
// Fixed(24) + FillWeight 得到正确的宽度分配。
func TestFlexHorizontalFixedWithFill(t *testing.T) {
	left := &fixedComp{lines: []string{"left"}}
	main := &fixedComp{lines: []string{"header", "editor"}}
	flex := NewFlex(DirectionHorizontal, Fixed(left, 24), FillWeight(main, 1))
	flex.Bounds = &bounds{w: 100, h: 10}
	flex.Render(100)
	if r := flex.ChildRect(0); r.Width != 24 {
		t.Fatalf("Fixed left Width=%d, want 24", r.Width)
	}
	if r := flex.ChildRect(1); r.Width != 76 {
		t.Fatalf("Fill main Width=%d, want 76 (100-24)", r.Width)
	}
}

// shrinkComp simulates a component that can cap its visible rows at runtime
// (like the Editor). maxRows==0 means unrestricted; OnAllocate drives SetMaxRows.
type shrinkComp struct {
	lines   []string
	maxRows int64
}

func (c *shrinkComp) Render(width int64) []string {
	lines := c.lines
	if c.maxRows > 0 && int64(len(lines)) > c.maxRows {
		lines = lines[:c.maxRows]
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = core.PadToWidth(l, width)
	}
	return out
}

func (c *shrinkComp) SetMaxRows(n int64) { c.maxRows = n }
func (c *shrinkComp) Invalidate()        {}

func TestFlexVerticalShrinkable(t *testing.T) {
	// editor: 8 natural rows, min 2.
	ed := &shrinkComp{lines: []string{"e1", "e2", "e3", "e4", "e5", "e6", "e7", "e8"}}
	header := &fixedComp{lines: []string{"h1"}}
	var alloc int64
	flex := NewFlex(DirectionVertical,
		Natural(header),
		Shrinkable(ed, 2).WithAllocate(func(h int64) { alloc = h; ed.SetMaxRows(h) }),
	)
	flex.Bounds = &bounds{w: 10, h: 5}
	out := flex.Render(10)
	// header=1, editor natural=8, used=9 > 5, over=4. slack=8-2=6.
	// cut = 4*6/6 = 4 → newSize=4. total = 1+4 = 5.
	if len(out) != 5 {
		t.Fatalf("len(out)=%d, want 5", len(out))
	}
	if alloc != 4 {
		t.Fatalf("OnAllocate got %d, want 4", alloc)
	}
	if ed.maxRows != 4 {
		t.Fatalf("ed.maxRows=%d, want 4", ed.maxRows)
	}
}

func TestFlexVerticalShrinkableHitsMinThenSafetyNet(t *testing.T) {
	// header is Natural (non-shrinkable) and already large; editor shrinks to
	// its Min but the total still overflows → safety net trims the top.
	ed := &shrinkComp{lines: []string{"e1", "e2", "e3", "e4", "e5"}}
	header := &fixedComp{lines: []string{"h1", "h2", "h3"}}
	flex := NewFlex(DirectionVertical,
		Natural(header),
		Shrinkable(ed, 3).WithAllocate(func(h int64) { ed.SetMaxRows(h) }),
	)
	flex.Bounds = &bounds{w: 10, h: 4}
	out := flex.Render(10)
	// header=3, editor natural=5, used=8 > 4, over=4. slack=5-3=2.
	// cut = 4*2/2 = 4 → newSize=5-4=1 < min 3 → clamp 3. cutTotal=2.
	// greedy: rest=2, editor already at min → no progress.
	// used = 3+3 = 6 > 4. safety net drops top 2 → len=4, editor rows visible.
	if len(out) != 4 {
		t.Fatalf("len(out)=%d, want 4 (safety net)", len(out))
	}
	if ed.maxRows != 3 {
		t.Fatalf("ed.maxRows=%d, want 3 (shrunk to min)", ed.maxRows)
	}
	// Bottom row must be an editor row (e3), not a header row.
	if out[len(out)-1] != "e3        " {
		t.Fatalf("bottom line=%q, want editor row (input area must stay visible)", out[len(out)-1])
	}
	// Safety net must shift rects so ChildRect matches the cropped output:
	// 6 rows trimmed to 4 drops the top 2, so editor (was Row 3) is now Row 1.
	// Mouse coord translation relies on this (chatLayout.editorTop).
	if r := flex.ChildRect(1); r.Row != 1 {
		t.Fatalf("editor Row=%d after safety-net trim, want 1 (rects must shift with the crop)", r.Row)
	}
	if r := flex.ChildRect(0); r.Row != -2 {
		t.Fatalf("header Row=%d after safety-net trim, want -2 (scrolled off top)", r.Row)
	}
}

func TestFlexVerticalShrinkableProportional(t *testing.T) {
	// Two shrinkable children: larger slack absorbs more of the overflow.
	a := &shrinkComp{lines: []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"}} // 8, min 2
	b := &shrinkComp{lines: []string{"b1", "b2", "b3", "b4"}}                         // 4, min 1
	flex := NewFlex(DirectionVertical,
		Shrinkable(a, 2).WithAllocate(func(h int64) { a.SetMaxRows(h) }),
		Shrinkable(b, 1).WithAllocate(func(h int64) { b.SetMaxRows(h) }),
	)
	flex.Bounds = &bounds{w: 10, h: 6}
	out := flex.Render(10)
	// used=12 > 6, over=6. totalSlack=6+3=9.
	// a cut=6*6/9=4 → newSize=4. b cut=6*3/9=2 → newSize=2. total=4+2=6.
	if len(out) != 6 {
		t.Fatalf("len(out)=%d, want 6", len(out))
	}
	if a.maxRows != 4 {
		t.Fatalf("a.maxRows=%d, want 4", a.maxRows)
	}
	if b.maxRows != 2 {
		t.Fatalf("b.maxRows=%d, want 2", b.maxRows)
	}
}

func assertLines(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, len(want)=%d\ngot=%q\nwant=%q", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
