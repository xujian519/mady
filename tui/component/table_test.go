package component

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func tableCols() []Column {
	return []Column{
		{Weight: 1, Render: func(i int, w int64) string { return "A" }},
		{Weight: 2, Render: func(i int, w int64) string { return "BB" }},
	}
}

func TestTableBasicSetters(t *testing.T) {
	tab := NewTable()
	if tab.ItemCount() != 0 {
		t.Fatalf("expected 0 items, got %d", tab.ItemCount())
	}
	tab.SetColumns(tableCols())
	tab.SetItems([]any{"a", "b", "c"})
	if tab.ItemCount() != 3 {
		t.Fatalf("expected 3 items, got %d", tab.ItemCount())
	}
	if tab.Selected() != 0 {
		t.Fatalf("expected selection reset to 0, got %d", tab.Selected())
	}
	th := TableTheme{EmptyMsg: "empty"}
	tab.SetTheme(th)
	if tab.Theme().EmptyMsg != "empty" {
		t.Fatalf("expected theme set, got %+v", tab.Theme())
	}
}

func TestTableMoveSelectedWraps(t *testing.T) {
	tab := NewTable()
	tab.SetItems([]any{"a", "b", "c"})
	tab.MoveSelected(1)
	if tab.Selected() != 1 {
		t.Fatalf("expected 1, got %d", tab.Selected())
	}
	tab.MoveSelected(5) // wraps 1->2->0->1->2->0
	if tab.Selected() != 0 {
		t.Fatalf("expected wrap to 0, got %d", tab.Selected())
	}
	tab.MoveSelected(-1) // wraps to last
	if tab.Selected() != 2 {
		t.Fatalf("expected wrap to 2, got %d", tab.Selected())
	}
}

func TestTableMoveSelectedEmpty(t *testing.T) {
	tab := NewTable()
	tab.MoveSelected(3) // no items — no-op
	if tab.Selected() != 0 {
		t.Fatalf("expected 0, got %d", tab.Selected())
	}
}

func TestTableConfirmAndCancel(t *testing.T) {
	tab := NewTable()
	tab.SetItems([]any{"a", "b"})
	var confirmed []int
	tab.OnSelect(func(i int) { confirmed = append(confirmed, i) })
	tab.MoveSelected(1)
	tab.Confirm()
	if len(confirmed) != 1 || confirmed[0] != 1 {
		t.Fatalf("expected confirm(1), got %v", confirmed)
	}

	canceled := false
	tab.OnCancel(func() { canceled = true })
	tab.Cancel()
	if !canceled {
		t.Fatal("expected cancel callback")
	}
}

func TestTableConfirmNoCallback(t *testing.T) {
	tab := NewTable()
	tab.SetItems([]any{"a"})
	tab.Confirm() // no onSelect — no-op
	tab.Cancel()  // no onCancel — no-op
}

func TestTableSetScrollClamps(t *testing.T) {
	tab := NewTable()
	tab.SetItems([]any{"a", "b", "c"})
	tab.SetScroll(-5)
	if tab.Scroll() != 0 {
		t.Fatalf("expected scroll 0, got %d", tab.Scroll())
	}
	tab.SetScroll(99)
	if tab.Scroll() != 2 {
		t.Fatalf("expected scroll clamped to 2, got %d", tab.Scroll())
	}
	tab.SetScroll(1)
	if tab.Scroll() != 1 {
		t.Fatalf("expected scroll 1, got %d", tab.Scroll())
	}
}

func TestTableItemAccessors(t *testing.T) {
	tab := NewTable()
	tab.SetItems([]any{"a", "b"})
	if tab.Item(0) != "a" || tab.Item(1) != "b" {
		t.Fatalf("unexpected Item access")
	}
	if tab.Item(-1) != nil || tab.Item(5) != nil {
		t.Fatal("expected nil for out-of-range index")
	}
	items := tab.Items()
	if len(items) != 2 || items[0] != "a" {
		t.Fatalf("unexpected Items: %v", items)
	}
	// Snapshot semantics.
	items[0] = "MUTATED"
	if tab.Item(0) != "a" {
		t.Fatal("Items must return a copy")
	}
}

func TestTableColWidthsNoColumns(t *testing.T) {
	tab := NewTable()
	if got := tab.ColWidths(100); got != nil {
		t.Fatalf("expected nil widths for no columns, got %v", got)
	}
}

func TestTableColWidthsProportional(t *testing.T) {
	tab := NewTable()
	tab.SetColumns([]Column{
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
		{Weight: 3, Render: func(i int, w int64) string { return "x" }},
	})
	got := tab.ColWidths(40)
	if len(got) != 2 {
		t.Fatalf("expected 2 widths, got %v", got)
	}
	if got[0] != 10 || got[1] != 30 {
		t.Fatalf("expected [10 30], got %v", got)
	}
}

func TestTableColWidthsMinMax(t *testing.T) {
	tab := NewTable()
	tab.SetColumns([]Column{
		{Weight: 1, MinWidth: 25, Render: func(i int, w int64) string { return "x" }},
		{Weight: 3, MaxWidth: 10, Render: func(i int, w int64) string { return "x" }},
	})
	got := tab.ColWidths(40)
	// col0: 40*1/4=10 -> raised to MinWidth 25. col1: 40*3/4=30 -> capped at 10.
	// The 5 unused cells are redistributed to col0 (it has no MaxWidth).
	if got[0] != 30 {
		t.Fatalf("expected col0 30 (min 25 + remainder 5), got %d", got[0])
	}
	if got[1] != 10 {
		t.Fatalf("expected col1 max width 10, got %d", got[1])
	}
}

func TestTableColWidthsRestColumn(t *testing.T) {
	// A Weight==0 column takes whatever the weighted columns leave over.
	// With a single weighted column of weight 1 it consumes everything.
	tab := NewTable()
	tab.SetColumns([]Column{
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
		{Weight: 0, Render: func(i int, w int64) string { return "x" }}, // fills rest
	})
	got := tab.ColWidths(40)
	if got[0] != 40 || got[1] != 0 {
		t.Fatalf("expected [40 0], got %v", got)
	}
	// Rest column with MaxWidth.
	tab.SetColumns([]Column{
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
		{Weight: 0, MaxWidth: 5, Render: func(i int, w int64) string { return "x" }},
	})
	got = tab.ColWidths(40)
	if got[0] != 40 || got[1] != 0 {
		t.Fatalf("expected [40 0], got %v", got)
	}
	// Rest column with MinWidth: the min is enforced even beyond total.
	tab.SetColumns([]Column{
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
		{Weight: 0, MinWidth: 30, Render: func(i int, w int64) string { return "x" }},
	})
	got = tab.ColWidths(40)
	if got[0] != 40 || got[1] != 30 {
		t.Fatalf("expected [40 30], got %v", got)
	}
	// Two weighted columns leave room for the rest column.
	tab.SetColumns([]Column{
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
		{Weight: 0, Render: func(i int, w int64) string { return "x" }},
	})
	got = tab.ColWidths(40)
	if got[0] != 20 || got[1] != 20 || got[2] != 0 {
		t.Fatalf("expected [20 20 0], got %v", got)
	}
}

func TestTableColWidthsRemainderDistribution(t *testing.T) {
	// All columns weighted; rounding remainder goes to the last column.
	tab := NewTable()
	tab.SetColumns([]Column{
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
	})
	got := tab.ColWidths(10)
	if got[0]+got[1]+got[2] != 10 {
		t.Fatalf("expected widths to sum to 10, got %v", got)
	}
	// Remainder with MaxWidth caps the last column.
	tab.SetColumns([]Column{
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
		{Weight: 1, MaxWidth: 3, Render: func(i int, w int64) string { return "x" }},
	})
	got = tab.ColWidths(10)
	if got[2] > 3 {
		t.Fatalf("expected last column capped at 3, got %v", got)
	}
}

func TestTableRenderRow(t *testing.T) {
	tab := NewTable()
	tab.SetColumns([]Column{
		{Weight: 1, Render: func(i int, w int64) string { return "a" }},
		{Weight: 1, Render: func(i int, w int64) string { return "bb" }},
	})
	row := tab.RenderRow(0, 10)
	if core.VisibleWidth(row) != 10 {
		t.Fatalf("expected row width 10, got %d (row=%q)", core.VisibleWidth(row), row)
	}
	if !strings.Contains(row, "a") || !strings.Contains(row, "bb") {
		t.Fatalf("expected cells in row, got %q", row)
	}
	if tab.RowWidth(10) != 10 {
		t.Fatalf("expected RowWidth 10, got %d", tab.RowWidth(10))
	}
}

func TestTableRenderRowTruncatesLongCell(t *testing.T) {
	tab := NewTable()
	tab.SetColumns([]Column{
		{Weight: 1, Render: func(i int, w int64) string { return strings.Repeat("x", 100) }},
	})
	row := tab.RenderRow(0, 8)
	if w := core.VisibleWidth(row); w != 8 {
		t.Fatalf("expected truncated row width 8, got %d (row=%q)", w, row)
	}
	if !strings.Contains(row, "…") {
		t.Fatalf("expected ellipsis in truncated cell, got %q", row)
	}
	if tab.RowWidth(8) != 8 {
		t.Fatalf("expected RowWidth 8, got %d", tab.RowWidth(8))
	}
}

func TestTableRenderRowMaxWidthShort(t *testing.T) {
	// When every column is capped by MaxWidth and the caps sum below total,
	// the row is shorter than total — RowWidth reports the real width.
	tab := NewTable()
	tab.SetColumns([]Column{
		{Weight: 1, MaxWidth: 5, Render: func(i int, w int64) string { return "x" }},
		{Weight: 1, MaxWidth: 5, Render: func(i int, w int64) string { return "x" }},
	})
	if w := tab.RowWidth(20); w != 10 {
		t.Fatalf("expected RowWidth 10, got %d", w)
	}
	row := tab.RenderRow(0, 20)
	if core.VisibleWidth(row) != 10 {
		t.Fatalf("expected row width 10, got %d (row=%q)", core.VisibleWidth(row), row)
	}
	// Single-column MaxWidth: the remainder is redistributed to the other
	// column, so the row still fills the total width.
	tab.SetColumns([]Column{
		{Weight: 1, Render: func(i int, w int64) string { return "x" }},
		{Weight: 1, MaxWidth: 2, Render: func(i int, w int64) string { return "x" }},
	})
	if w := tab.RowWidth(20); w != 20 {
		t.Fatalf("expected RowWidth 20 with redistributed remainder, got %d", w)
	}
}

func TestTableVisibleRange(t *testing.T) {
	tab := NewTable()
	tab.SetItems([]any{"1", "2", "3", "4", "5"})

	start, end := tab.VisibleRange(2)
	if start != 0 || end != 2 {
		t.Fatalf("expected (0,2), got (%d,%d)", start, end)
	}

	// Selection beyond window pushes scroll.
	tab.MoveSelected(4)
	start, end = tab.VisibleRange(2)
	if start != 3 || end != 5 {
		t.Fatalf("expected (3,5), got (%d,%d)", start, end)
	}

	// maxVisible <= 0 -> (0,0).
	tab.MoveSelected(0)
	start, end = tab.VisibleRange(0)
	if start != 0 || end != 0 {
		t.Fatalf("expected (0,0) for maxVisible 0, got (%d,%d)", start, end)
	}
}

func TestTableVisibleRangeEmpty(t *testing.T) {
	tab := NewTable()
	start, end := tab.VisibleRange(5)
	if start != 0 || end != 0 {
		t.Fatalf("expected (0,0) for empty, got (%d,%d)", start, end)
	}
}

func TestTableClampScrollKeepsSelectionVisible(t *testing.T) {
	tab := NewTable()
	tab.SetItems([]any{"1", "2", "3"})
	tab.SetScroll(2)
	tab.sel = 0
	tab.clampScrollLocked()
	if tab.Scroll() != 0 {
		t.Fatalf("expected scroll pulled back to 0, got %d", tab.Scroll())
	}
}
