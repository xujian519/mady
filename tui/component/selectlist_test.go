package component

import (
	"fmt"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestSelectListNavigation(t *testing.T) {
	items := []SelectItem{
		{Value: "a", Label: "Alpha"},
		{Value: "b", Label: "Beta"},
		{Value: "c", Label: "Gamma"},
	}
	sl := NewSelectList(items)
	sl.SetFocused(true)

	var selected SelectItem
	sl.OnSelect(func(it SelectItem) { selected = it })

	// Down twice, confirm -> Gamma
	sl.Update(core.KeyMsg{Data: "\x1b[B"})
	sl.Update(core.KeyMsg{Data: "\x1b[B"})
	sl.Update(core.KeyMsg{Data: "\r"})

	if selected.Value != "c" {
		t.Fatalf("expected c, got %q", selected.Value)
	}
}

func TestSelectListFilter(t *testing.T) {
	items := []SelectItem{
		{Value: "alpha", Label: "Alpha"},
		{Value: "beta", Label: "Beta"},
		{Value: "gamma", Label: "Gamma"},
		{Value: "banana", Label: "Banana"},
	}
	sl := NewSelectList(items)
	sl.SetFilter("ba")
	// "Banana" should match strongly ahead of "Beta"; alpha/gamma excluded.
	cur, ok := sl.CurrentItem()
	if !ok {
		t.Fatalf("expected at least one match")
	}
	if cur.Label != "Banana" && cur.Label != "Beta" {
		t.Fatalf("unexpected top match %q", cur.Label)
	}
}

func TestSelectListCurrentItemEmpty(t *testing.T) {
	sl := NewSelectList(nil)
	if _, ok := sl.CurrentItem(); ok {
		t.Error("expected no current item for empty list")
	}
}

func TestSelectListSetMaxVisibleMinimum(t *testing.T) {
	sl := NewSelectList([]SelectItem{{Value: "a", Label: "A"}})
	sl.SetMaxVisible(1)
	if sl.maxVisible != 3 {
		t.Errorf("SetMaxVisible should clamp to minimum 3, got %d", sl.maxVisible)
	}
}

func TestSelectListOnCancel(t *testing.T) {
	items := []SelectItem{{Value: "a", Label: "A"}}
	sl := NewSelectList(items)
	sl.SetFocused(true)

	canceled := false
	sl.OnCancel(func() { canceled = true })
	sl.Update(core.KeyMsg{Data: "\x1b"}) // escape

	if !canceled {
		t.Error("expected cancel callback to fire on Escape")
	}
}

func TestSelectListOnSelectionChange(t *testing.T) {
	items := []SelectItem{
		{Value: "a", Label: "Alpha"},
		{Value: "b", Label: "Beta"},
	}
	sl := NewSelectList(items)
	sl.SetFocused(true)

	var changed []string
	sl.OnSelectionChange(func(it SelectItem) { changed = append(changed, it.Value) })
	sl.Update(core.KeyMsg{Data: "\x1b[B"}) // down

	if len(changed) != 1 || changed[0] != "b" {
		t.Errorf("expected selection change to b, got %v", changed)
	}
}

func TestSelectListRenderEmpty(t *testing.T) {
	sl := NewSelectList(nil)
	lines := sl.Render(20)
	if len(lines) != 1 {
		t.Fatalf("expected one line, got %d", len(lines))
	}
	if !contains(lines[0], "no items") {
		t.Errorf("expected 'no items' message, got %q", lines[0])
	}
}

func TestSelectListRenderFilteredEmpty(t *testing.T) {
	sl := NewSelectList([]SelectItem{{Value: "a", Label: "A"}})
	sl.SetFilter("zzz")
	lines := sl.Render(20)
	if len(lines) != 1 {
		t.Fatalf("expected one line, got %d", len(lines))
	}
	if !contains(lines[0], "no matches") {
		t.Errorf("expected 'no matches' message, got %q", lines[0])
	}
}

func TestSelectListRenderGroupHeaders(t *testing.T) {
	items := []SelectItem{
		{Value: "a", Label: "Alpha", Group: "Letters"},
		{Value: "1", Label: "One", Group: "Numbers"},
	}
	sl := NewSelectList(items)
	lines := sl.Render(30)
	if len(lines) < 3 {
		t.Fatalf("expected group header + items, got %d lines", len(lines))
	}
}

func TestSelectListScrolling(t *testing.T) {
	items := make([]SelectItem, 20)
	for i := range items {
		items[i] = SelectItem{Value: fmt.Sprintf("%d", i), Label: fmt.Sprintf("Item %d", i)}
	}
	sl := NewSelectList(items)
	sl.SetFocused(true)
	sl.SetMaxVisible(5)

	for i := 0; i < 10; i++ {
		sl.Update(core.KeyMsg{Data: "\x1b[B"}) // down
	}

	cur, _ := sl.CurrentItem()
	if cur.Value != "10" {
		t.Errorf("expected cursor at 10, got %s", cur.Value)
	}
	if sl.scroll < 1 {
		t.Errorf("expected viewport to scroll, got scroll=%d", sl.scroll)
	}
}
