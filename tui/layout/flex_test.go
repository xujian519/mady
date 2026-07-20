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
// 参考 chat_app_layout.go 的 sidebar 布局修复（Natural 让主区域只剩 1 列）。
func TestFlexHorizontalNaturalStarvesFill(t *testing.T) {
	sidebar := &fixedComp{lines: []string{"▎ Mady", "📂 会话"}}
	main := &fixedComp{lines: []string{"header", "editor"}}
	flex := NewFlex(DirectionHorizontal, Natural(sidebar), Fill(main))
	flex.Bounds = &bounds{w: 100, h: 10}
	flex.Render(100)
	// Natural 在水平模式占满 100，Fill(main) 被压到最小宽度 1。
	if r := flex.ChildRect(0); r.Width != 100 {
		t.Fatalf("Natural sidebar Width=%d, want 100 (gotcha: Natural starves siblings)", r.Width)
	}
	if r := flex.ChildRect(1); r.Width != 1 {
		t.Fatalf("Fill main Width=%d, want 1 (Natural starved it to the min-guard)", r.Width)
	}
}

// TestFlexHorizontalFixedWithFill 验证固定宽度列（如 sidebar）应使用 Fixed
// 而非 Natural：Fixed(24) + FillWeight 得到正确的宽度分配。
func TestFlexHorizontalFixedWithFill(t *testing.T) {
	sidebar := &fixedComp{lines: []string{"▎ Mady", "📂 会话"}}
	main := &fixedComp{lines: []string{"header", "editor"}}
	flex := NewFlex(DirectionHorizontal, Fixed(sidebar, 24), FillWeight(main, 1))
	flex.Bounds = &bounds{w: 100, h: 10}
	flex.Render(100)
	if r := flex.ChildRect(0); r.Width != 24 {
		t.Fatalf("Fixed sidebar Width=%d, want 24", r.Width)
	}
	if r := flex.ChildRect(1); r.Width != 76 {
		t.Fatalf("Fill main Width=%d, want 76 (100-24)", r.Width)
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
