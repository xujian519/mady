package component

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func testCommandItems() []CommandItem {
	return []CommandItem{
		{Name: "plan", Label: "/plan [on|off]", Category: "模式", Description: "切换计划模式", Status: "开启", Available: true},
		{Name: "patent", Label: "/patent", Category: "专业", Description: "专利分析", Available: true},
		{Name: "legal", Label: "/legal", Category: "专业", Description: "法律分析", Available: false, Reason: "未配置"},
		{Name: "theme", Label: "/theme", Description: "主题切换", Available: true},
	}
}

func TestCommandCenterSetItems(t *testing.T) {
	cc := NewCommandCenter(nil)
	cc.SetItems(testCommandItems())
	cc.SetFocused(true)
	if !cc.IsFocused() {
		t.Fatal("expected focused after SetFocused(true)")
	}
	cc.SetFocused(false)
	if cc.IsFocused() {
		t.Fatal("expected not focused after SetFocused(false)")
	}
	cc.Invalidate() // no-op, must not panic
}

func TestCommandCenterExecuteOnEnter(t *testing.T) {
	cc := NewCommandCenter(testCommandItems())
	var executed CommandItem
	cc.OnExecute(func(it CommandItem) { executed = it })
	cc.SetFocused(true)

	// cursor starts at 0; Enter executes the first item.
	cc.Update(core.KeyMsg{Data: "\r"})
	if executed.Name != "plan" {
		t.Fatalf("expected plan executed, got %q", executed.Name)
	}
}

func TestCommandCenterNavigateAndExecute(t *testing.T) {
	cc := NewCommandCenter(testCommandItems())
	var executed []string
	cc.OnExecute(func(it CommandItem) { executed = append(executed, it.Name) })

	cc.Update(core.KeyMsg{Data: "\x1b[B"}) // down
	cc.Update(core.KeyMsg{Data: "\x1b[B"}) // down
	cc.Update(core.KeyMsg{Data: "\r"})     // enter
	if len(executed) != 1 || executed[0] != "legal" {
		t.Fatalf("expected legal executed, got %v", executed)
	}

	cc.Update(core.KeyMsg{Data: "\x1b[A"}) // up
	cc.Update(core.KeyMsg{Data: "\r"})
	if len(executed) != 2 || executed[1] != "patent" {
		t.Fatalf("expected patent executed, got %v", executed)
	}
}

func TestCommandCenterCursorBounds(t *testing.T) {
	cc := NewCommandCenter(testCommandItems())
	// Up at top stays at 0.
	cc.Update(core.KeyMsg{Data: "\x1b[A"})
	if cc.cursor != 0 {
		t.Fatalf("expected cursor 0 after up at top, got %d", cc.cursor)
	}
	// Down past end clamps.
	for i := 0; i < 10; i++ {
		cc.Update(core.KeyMsg{Data: "\x1b[B"})
	}
	if cc.cursor != len(testCommandItems())-1 {
		t.Fatalf("expected cursor clamped to %d, got %d", len(testCommandItems())-1, cc.cursor)
	}
}

func TestCommandCenterCloseOnEscape(t *testing.T) {
	cc := NewCommandCenter(testCommandItems())
	closed := false
	cc.OnClose(func() { closed = true })
	cc.Update(core.KeyMsg{Data: "\x1b"})
	if !closed {
		t.Fatal("expected onClose to fire on Escape")
	}
}

func TestCommandCenterFilterTyping(t *testing.T) {
	cc := NewCommandCenter(testCommandItems())
	// One key event per character — processKey handles single-char Data.
	for _, ch := range []string{"p", "a", "t"} {
		cc.Update(core.KeyMsg{Data: ch})
	}
	if cc.filter != "pat" {
		t.Fatalf("expected filter 'pat', got %q", cc.filter)
	}
	if len(cc.filtered) != 1 || cc.filtered[0].Name != "patent" {
		t.Fatalf("expected only patent to match, got %v", cc.filtered)
	}
	// Non-printable (control) characters are ignored by the filter.
	cc.Update(core.KeyMsg{Data: "\x03"})
	if cc.filter != "pat" {
		t.Fatalf("expected filter unchanged, got %q", cc.filter)
	}
	// Backspace deletes the last filter rune (matches the registered
	// "tui.editor.deleteCharBackward" binding).
	cc.Update(core.KeyMsg{Data: "\x7f"})
	if cc.filter != "pa" {
		t.Fatalf("expected filter 'pa' after backspace, got %q", cc.filter)
	}
}

func TestCommandCenterFilterMatchesLabelAndDescription(t *testing.T) {
	cc := NewCommandCenter(testCommandItems())
	cc.SetFilter("主题") // matches Label
	if len(cc.filtered) != 1 || cc.filtered[0].Name != "theme" {
		t.Fatalf("expected theme via label match, got %v", cc.filtered)
	}
	cc.SetFilter("法律") // matches Description of legal
	if len(cc.filtered) != 1 || cc.filtered[0].Name != "legal" {
		t.Fatalf("expected legal via description match, got %v", cc.filtered)
	}
	cc.SetFilter("plan") // matches Name
	if len(cc.filtered) != 1 || cc.filtered[0].Name != "plan" {
		t.Fatalf("expected plan via name match, got %v", cc.filtered)
	}
}

func TestCommandCenterFilterNoMatch(t *testing.T) {
	cc := NewCommandCenter(testCommandItems())
	cc.SetFilter("zzzz")
	if len(cc.filtered) != 0 {
		t.Fatalf("expected no matches, got %d", len(cc.filtered))
	}
	// Enter on empty list must not execute.
	executed := false
	cc.OnExecute(func(CommandItem) { executed = true })
	cc.Update(core.KeyMsg{Data: "\r"})
	if executed {
		t.Fatal("expected no execution when filtered list is empty")
	}
}

func TestCommandCenterClearFilter(t *testing.T) {
	cc := NewCommandCenter(testCommandItems())
	cc.SetFilter("plan")
	cc.SetFilter("")
	if len(cc.filtered) != len(testCommandItems()) {
		t.Fatalf("expected all items after clearing filter, got %d", len(cc.filtered))
	}
}

func TestCommandCenterSetItemsKeepsFilter(t *testing.T) {
	// SetItems re-applies the current filter to the new item list.
	cc := NewCommandCenter(testCommandItems())
	cc.SetFilter("plan")
	cc.SetItems(testCommandItems())
	if cc.filter != "plan" {
		t.Fatalf("expected filter kept, got %q", cc.filter)
	}
	if len(cc.filtered) != 1 || cc.filtered[0].Name != "plan" {
		t.Fatalf("expected filtered re-applied to new items, got %v", cc.filtered)
	}
}

func TestCommandCenterClampCursor(t *testing.T) {
	cc := NewCommandCenter(nil)
	cc.cursor = 5
	cc.SetItems(testCommandItems())
	if cc.cursor != len(testCommandItems())-1 {
		t.Fatalf("expected cursor clamped to %d, got %d", len(testCommandItems())-1, cc.cursor)
	}
	cc.cursor = -3
	cc.SetItems(nil)
	if cc.cursor != 0 {
		t.Fatalf("expected cursor 0 for empty, got %d", cc.cursor)
	}
}

func TestCommandCenterRender(t *testing.T) {
	cc := NewCommandCenter(testCommandItems())
	// Note: the title bar is rendered with PadToWidth (no truncation), so the
	// width must be wide enough for the fixed title text.
	lines := cc.Render(80)
	if len(lines) == 0 {
		t.Fatal("expected non-empty render")
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w > 80 {
			t.Fatalf("line width %d > 80 (line=%q)", w, ln)
		}
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "命令中心") {
		t.Fatal("expected title bar")
	}
	if !strings.Contains(joined, "▾ 专业") {
		t.Fatal("expected group header 专业")
	}
	if !strings.Contains(joined, "[1/4]") {
		t.Fatal("expected footer counter")
	}
}

func TestCommandCenterRenderEmptyFiltered(t *testing.T) {
	cc := NewCommandCenter(testCommandItems())
	cc.SetFilter("zzz")
	lines := cc.Render(40)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "无匹配命令") {
		t.Fatalf("expected no-match message, got %q", joined)
	}
}

func TestCommandCenterRenderFilterMatchesCount(t *testing.T) {
	cc := NewCommandCenter(testCommandItems())
	cc.SetFilter("pat")
	lines := cc.Render(40)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "匹配: 1") {
		t.Fatalf("expected match count footer, got %q", joined)
	}
}

func TestCommandCenterRenderManyItemsScrolls(t *testing.T) {
	items := make([]CommandItem, 30)
	for i := range items {
		items[i] = CommandItem{Name: "cmd", Label: "/cmd", Category: "c"}
	}
	cc := NewCommandCenter(items)
	// Move cursor to the end to force start = end - maxVisible.
	for i := 0; i < 29; i++ {
		cc.Update(core.KeyMsg{Data: "\x1b[B"})
	}
	lines := cc.Render(80)
	// title(1) + search(2) + separator(1) + items(12) + footer(1) = 17 max.
	if len(lines) > 17 {
		t.Fatalf("expected max 17 visible lines, got %d", len(lines))
	}
}
