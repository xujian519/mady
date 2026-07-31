package component

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func testTodoItems() []TodoItem {
	return []TodoItem{
		{ID: "1", Content: "撰写权利要求书", Status: "pending", Priority: "high"},
		{ID: "2", Content: "检索对比文件", Status: "in_progress"},
		{ID: "3", Content: "撰写说明书", Status: "completed", Priority: "low"},
		{ID: "4", Content: "答复审查意见", Status: "canceled"},
		{ID: "5", Content: "归档", Status: "archived"},
		{ID: "6", Content: "未知状态", Status: "weird"},
	}
}

func TestTodoPanelConstruction(t *testing.T) {
	p := NewTodoPanel()
	if p.theme.Title == "" {
		t.Fatal("expected default title")
	}
	if p.km == nil {
		t.Fatal("expected keybindings manager")
	}
}

func TestTodoPanelSetTitleAndItems(t *testing.T) {
	p := NewTodoPanel()
	p.SetTitle("自定义标题")
	if p.theme.Title != "自定义标题" {
		t.Fatalf("expected title set, got %q", p.theme.Title)
	}
	p.SetItems(testTodoItems())
	if len(p.items) != 6 {
		t.Fatalf("expected 6 items, got %d", len(p.items))
	}
	p.Invalidate() // no-op
}

func TestTodoPanelSetItemsClampsSelected(t *testing.T) {
	p := NewTodoPanel()
	p.selected = 99
	p.SetItems(testTodoItems())
	if p.selected != 5 {
		t.Fatalf("expected selected clamped to 5, got %d", p.selected)
	}
	p.SetItems(nil)
	if p.selected != 0 {
		t.Fatalf("expected selected 0 for empty, got %d", p.selected)
	}
	p.selected = -2
	p.SetItems(testTodoItems())
	if p.selected != 0 {
		t.Fatalf("expected selected clamped to 0, got %d", p.selected)
	}
}

func TestTodoPanelReload(t *testing.T) {
	p := NewTodoPanel()
	invalidated := false
	p.SetOnInvalidate(func() { invalidated = true })
	p.SetDataProvider(func() []TodoItem { return testTodoItems() })
	p.Reload()
	if len(p.items) != 6 {
		t.Fatalf("expected items from provider, got %d", len(p.items))
	}
	if !invalidated {
		t.Fatal("expected onInvalidate after Reload")
	}
}

func TestTodoPanelReloadNoProvider(t *testing.T) {
	p := NewTodoPanel()
	p.Reload() // no provider — must not panic
}

func TestTodoPanelProcessKeys(t *testing.T) {
	p := NewTodoPanel()
	p.SetItems(testTodoItems())

	// up wraps to last
	p.Update(core.KeyMsg{Data: "\x1b[A"})
	if p.selected != 5 {
		t.Fatalf("expected wrap to 5, got %d", p.selected)
	}
	// down wraps to 0
	p.Update(core.KeyMsg{Data: "\x1b[B"})
	if p.selected != 0 {
		t.Fatalf("expected wrap to 0, got %d", p.selected)
	}
	// down again -> 1
	p.Update(core.KeyMsg{Data: "\x1b[B"})
	if p.selected != 1 {
		t.Fatalf("expected 1, got %d", p.selected)
	}
}

func TestTodoPanelMoveSelectedEmpty(t *testing.T) {
	p := NewTodoPanel()
	p.Update(core.KeyMsg{Data: "\x1b[A"}) // moveSelected on empty — no-op
	if p.selected != 0 {
		t.Fatalf("expected selected 0, got %d", p.selected)
	}
}

func TestTodoPanelToggleSelected(t *testing.T) {
	p := NewTodoPanel()
	p.SetItems(testTodoItems())
	p.selected = 1

	var toggled []TodoItem
	p.SetOnToggle(func(it TodoItem) { toggled = append(toggled, it) })

	// Enter toggles.
	p.Update(core.KeyMsg{Data: "\r"})
	if len(toggled) != 1 || toggled[0].ID != "2" {
		t.Fatalf("expected item 2 toggled, got %v", toggled)
	}

	// Enter again toggles the same item (toggle does not move the cursor).
	p.Update(core.KeyMsg{Data: "\r"})
	if len(toggled) != 2 || toggled[1].ID != "2" {
		t.Fatalf("expected item 2 toggled again, got %v", toggled)
	}
}

// TestTodoPanelToggleWithSpace verifies Space triggers toggle via the
// default binding (canonical key name " "). Regression guard for the former
// "space"/"esc" naming mismatch that made the default bindings dead.
func TestTodoPanelToggleWithSpace(t *testing.T) {
	p := NewTodoPanel()
	p.SetItems(testTodoItems())
	toggled := false
	p.SetOnToggle(func(TodoItem) { toggled = true })
	p.Update(core.KeyMsg{Data: " "})
	if !toggled {
		t.Fatal("expected toggle on Space via default binding")
	}
}

func TestTodoPanelToggleReloadsFromProvider(t *testing.T) {
	p := NewTodoPanel()
	calls := 0
	p.SetDataProvider(func() []TodoItem {
		calls++
		return testTodoItems()[:2]
	})
	p.SetItems(testTodoItems())
	p.SetOnToggle(func(TodoItem) {})
	p.Update(core.KeyMsg{Data: "\r"})
	if calls != 1 {
		t.Fatalf("expected dataProvider called once, got %d", calls)
	}
	if len(p.items) != 2 {
		t.Fatalf("expected items replaced by provider, got %d", len(p.items))
	}
}

func TestTodoPanelToggleNoCallback(t *testing.T) {
	p := NewTodoPanel()
	p.SetItems(testTodoItems())
	p.selected = 1
	p.Update(core.KeyMsg{Data: "\r"}) // no onToggle — no-op
	if p.selected != 1 {
		t.Fatalf("expected selected unchanged, got %d", p.selected)
	}
}

func TestTodoPanelToggleSelectedOutOfRange(t *testing.T) {
	p := NewTodoPanel()
	p.selected = 7 // out of range (empty items)
	p.SetOnToggle(func(TodoItem) { t.Fatal("must not toggle") })
	p.Update(core.KeyMsg{Data: "\r"})
}

// TestTodoPanelClose verifies the Escape key fires onClose via the default
// binding. Regression guard for the former "esc" naming mismatch that made
// the default binding dead.
func TestTodoPanelClose(t *testing.T) {
	p := NewTodoPanel()
	closed := false
	p.SetOnClose(func() { closed = true })
	p.Update(core.KeyMsg{Data: "\x1b"})
	if !closed {
		t.Fatal("expected onClose on Escape via default binding")
	}
}

func TestTodoPanelCloseNoCallback(t *testing.T) {
	p := NewTodoPanel()
	p.Update(core.KeyMsg{Data: "\x1b"}) // no callback — no-op
}

func TestTodoPanelRenderStatuses(t *testing.T) {
	p := NewTodoPanel()
	p.SetItems(testTodoItems())
	lines := p.Render(60)
	if len(lines) == 0 {
		t.Fatal("expected non-empty render")
	}
	joined := strings.Join(lines, "\n")
	for _, icon := range []string{"○", "◐", "●", "✗"} {
		if !strings.Contains(joined, icon) {
			t.Fatalf("expected status icon %q in render", icon)
		}
	}
	if !strings.Contains(joined, "[high]") || !strings.Contains(joined, "[low]") {
		t.Fatal("expected priority markers")
	}
	if !strings.Contains(joined, "Pending: 1 | In Progress: 1 | Completed: 1 | Canceled: 2") {
		t.Fatalf("expected summary line, got %q", joined)
	}
}

// TestTodoPanelRenderWidths asserts rendered item lines never exceed the
// panel width. The title and summary lines are rendered without truncation
// (source behavior) and are excluded from the width assertion.
func TestTodoPanelRenderWidths(t *testing.T) {
	p := NewTodoPanel()
	p.SetItems(testTodoItems())
	for _, w := range []int64{80, 40, 20} {
		lines := p.Render(w)
		for _, ln := range lines {
			if strings.HasPrefix(ln, "TODO") || strings.HasPrefix(ln, "Pending:") {
				continue
			}
			if v := core.VisibleWidth(ln); v > w {
				t.Fatalf("width %d: line width %d > %d (line=%q)", w, v, w, ln)
			}
		}
	}
}

// TestTodoPanelRenderNarrowWidth: at width 1 the fixed prefix ("▸ ○ ") plus
// priority already exceeds the panel, so only structure is asserted.
func TestTodoPanelRenderNarrowWidth(t *testing.T) {
	p := NewTodoPanel()
	p.SetItems(testTodoItems())
	lines := p.Render(1)
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
}

func TestTodoPanelRenderMultilineContent(t *testing.T) {
	p := NewTodoPanel()
	p.SetItems([]TodoItem{
		{ID: "1", Content: "第一行\n第二行 continuation", Status: "pending"},
	})
	lines := p.Render(50)
	if len(lines) < 3 {
		t.Fatalf("expected title+summary+2 content lines, got %d", len(lines))
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "第一行") || !strings.Contains(joined, "第二行") {
		t.Fatalf("expected both content lines, got %q", joined)
	}
}

func TestTodoPanelRenderLongContentTruncated(t *testing.T) {
	long := strings.Repeat("很长的内容", 40)
	p := NewTodoPanel()
	p.SetItems([]TodoItem{{ID: "1", Content: long, Status: "pending"}})
	lines := p.Render(24)
	found := false
	for _, ln := range lines {
		if strings.HasPrefix(ln, "TODO") || strings.HasPrefix(ln, "Pending:") {
			continue
		}
		if w := core.VisibleWidth(ln); w > 24 {
			t.Fatalf("line width %d > 24 (line=%q)", w, ln)
		}
		if strings.Contains(ln, "…") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected ellipsis truncation in render")
	}
}

func TestTodoPanelRenderEmpty(t *testing.T) {
	p := NewTodoPanel()
	lines := p.Render(40)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "No tasks") {
		t.Fatalf("expected 'No tasks' message, got %q", joined)
	}
}

func TestTodoPanelRenderSelected(t *testing.T) {
	p := NewTodoPanel()
	p.SetItems(testTodoItems())
	p.selected = 2
	lines := p.Render(60)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "▸") {
		t.Fatal("expected selection cursor")
	}
}

func TestTodoPanelCountStatuses(t *testing.T) {
	p := NewTodoPanel()
	p.SetItems(testTodoItems())
	pend, prog, done, canc := p.countStatuses()
	if pend != 1 || prog != 1 || done != 1 || canc != 2 {
		t.Fatalf("unexpected counts: %d,%d,%d,%d", pend, prog, done, canc)
	}
}

func TestTodoPanelStatusIcons(t *testing.T) {
	p := NewTodoPanel()
	cases := map[string]string{
		"pending":     "○",
		"in_progress": "◐",
		"completed":   "●",
		"canceled":    "✗",
		"archived":    "✗",
		"unknown":     "○",
		"":            "○",
	}
	for status, want := range cases {
		if got := p.statusIcon(status); got != want {
			t.Fatalf("statusIcon(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestTodoPanelStatusStyles(t *testing.T) {
	p := NewTodoPanel()
	for _, status := range []string{"pending", "in_progress", "completed", "canceled", "archived", "other"} {
		_ = p.statusStyle(status) // must not panic for any status
	}
}
