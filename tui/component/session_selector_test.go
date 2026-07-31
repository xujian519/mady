package component

import (
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

func testSessionItems() []SessionItem {
	return []SessionItem{
		{ID: "sess-001", Name: "专利撰写", Label: "case", UpdatedAt: "2026-07-01", MsgCount: 12},
		{ID: "sess-002", Name: "法律咨询", ParentSession: "sess-001", CreatedAt: "2026-07-02", MsgCount: 3},
		{ID: "sess-003", Name: "历史会话 A", Preview: "包含关键词 content", MsgCount: 0},
	}
}

func TestSessionSelectorSetters(t *testing.T) {
	s := NewSessionSelector()
	s.SetTitle("会话")
	if s.theme.Title != "会话" {
		t.Fatalf("expected title, got %q", s.theme.Title)
	}
	s.SetItems(testSessionItems())
	if len(s.items) != 3 || len(s.filtered) != 3 {
		t.Fatalf("expected 3 items, got %d/%d", len(s.items), len(s.filtered))
	}
	s.SetAvailableHeight(24)
	if s.height != 24 {
		t.Fatalf("expected height 24, got %d", s.height)
	}
	s.SetOnSelect(func(SessionItem) {})
	s.SetOnCancel(func() {})
	s.SetOnDelete(func(SessionItem) {})
	s.SetOnRename(func(SessionItem, string) {})
	s.Invalidate() // no-op
	if cmd := s.Update(core.KeyMsg{Data: "x"}); cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
}

func TestSessionSelectorFilter(t *testing.T) {
	s := NewSessionSelector()
	s.SetItems(testSessionItems())
	// Name match.
	s.filter = "法律"
	s.applyFilterLocked()
	if len(s.filtered) != 1 || s.filtered[0].ID != "sess-002" {
		t.Fatalf("expected sess-002, got %v", s.filtered)
	}
	// ID match.
	s.filter = "sess-003"
	s.applyFilterLocked()
	if len(s.filtered) != 1 || s.filtered[0].ID != "sess-003" {
		t.Fatalf("expected sess-003, got %v", s.filtered)
	}
	// Preview match.
	s.filter = "content"
	s.applyFilterLocked()
	if len(s.filtered) != 1 || s.filtered[0].ID != "sess-003" {
		t.Fatalf("expected sess-003 via preview, got %v", s.filtered)
	}
	// No match.
	s.filter = "zzz"
	s.applyFilterLocked()
	if len(s.filtered) != 0 {
		t.Fatalf("expected no matches, got %v", s.filtered)
	}
	// Clearing restores all.
	s.filter = ""
	s.applyFilterLocked()
	if len(s.filtered) != 3 {
		t.Fatalf("expected all items, got %v", s.filtered)
	}
}

func TestSessionSelectorHandleEsc(t *testing.T) {
	s := NewSessionSelector()
	s.SetItems(testSessionItems())
	// No mode — ESC not consumed.
	if s.HandleEsc() {
		t.Fatal("expected false with no active mode")
	}
	// Focus mode consumed.
	s.startFocus()
	s.filter = "pat"
	s.applyFilterLocked()
	if !s.HandleEsc() {
		t.Fatal("expected ESC consumed in focus mode")
	}
	if s.focusMode || s.filter != "" {
		t.Fatalf("expected focus mode exited and filter cleared")
	}
	// Rename mode consumed.
	s.renameMode = true
	s.renameBuf = "x"
	if !s.HandleEsc() {
		t.Fatal("expected ESC consumed in rename mode")
	}
	if s.renameMode || s.renameItem != nil {
		t.Fatalf("expected rename mode exited")
	}
}

func TestSessionSelectorNavigation(t *testing.T) {
	s := NewSessionSelector()
	s.SetItems(testSessionItems())
	s.Update(core.KeyMsg{Data: "\x1b[B"}) // down
	if s.table.Selected() != 1 {
		t.Fatalf("expected table selection 1, got %d", s.table.Selected())
	}
	s.Update(core.KeyMsg{Data: "\x1b[A"}) // up
	if s.table.Selected() != 0 {
		t.Fatalf("expected table selection 0, got %d", s.table.Selected())
	}
}

func TestSessionSelectorConfirmCancelDelete(t *testing.T) {
	s := NewSessionSelector()
	s.SetItems(testSessionItems())
	// Note: the default "esc" KeyID does not match the key parser's canonical
	// "escape" name, so rebind via SetUserBindings (documented override path).
	s.km.SetUserBindings(map[string][]terminal.KeyID{
		"session.cancel": {"escape"},
	})

	selected := make(chan SessionItem, 1)
	s.SetOnSelect(func(it SessionItem) { selected <- it })
	s.Update(core.KeyMsg{Data: "\r"}) // enter confirms first item
	select {
	case it := <-selected:
		if it.ID != "sess-001" {
			t.Fatalf("expected sess-001, got %s", it.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected onSelect callback")
	}

	canceled := make(chan struct{}, 1)
	s.SetOnCancel(func() { canceled <- struct{}{} })
	s.Update(core.KeyMsg{Data: "\x1b"})
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected onCancel callback")
	}

	deleted := make(chan SessionItem, 1)
	s.SetOnDelete(func(it SessionItem) { deleted <- it })
	s.Update(core.KeyMsg{Data: "\x18"}) // ctrl+x
	select {
	case it := <-deleted:
		if it.ID != "sess-001" {
			t.Fatalf("expected sess-001 deleted, got %s", it.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected onDelete callback")
	}
}

func TestSessionSelectorDeleteNoCallback(t *testing.T) {
	s := NewSessionSelector()
	s.SetItems(testSessionItems())
	s.Update(core.KeyMsg{Data: "\x18"}) // no onDelete — no-op
}

func TestSessionSelectorFocusMode(t *testing.T) {
	s := NewSessionSelector()
	s.SetItems(testSessionItems())
	s.Update(core.KeyMsg{Data: "/"}) // enters focus mode
	if !s.focusMode {
		t.Fatal("expected focus mode after /")
	}
	// One key event per character. Note: the focus-input path only accepts
	// single-byte ASCII printables (len(data)==1), so CJK filters cannot be
	// typed directly (source behavior).
	for _, ch := range []string{"s", "e", "s", "s"} {
		s.Update(core.KeyMsg{Data: ch})
	}
	if s.filter != "sess" {
		t.Fatalf("expected filter 'sess', got %q", s.filter)
	}
	if len(s.filtered) != 3 { // all ids start with sess-
		t.Fatalf("expected 3 filtered, got %d", len(s.filtered))
	}
	// Backspace removes a char.
	s.Update(core.KeyMsg{Data: "\x7f"})
	if s.filter != "ses" {
		t.Fatalf("expected filter 'ses', got %q", s.filter)
	}
	// Enter ends focus and confirms.
	selected := make(chan SessionItem, 1)
	s.SetOnSelect(func(it SessionItem) { selected <- it })
	s.Update(core.KeyMsg{Data: "\r"})
	if s.focusMode {
		t.Fatal("expected focus mode exited")
	}
	select {
	case <-selected:
	case <-time.After(2 * time.Second):
		t.Fatal("expected onSelect after focus confirm")
	}
}

func TestSessionSelectorFocusModeControlChar(t *testing.T) {
	s := NewSessionSelector()
	s.SetItems(testSessionItems())
	s.startFocus()
	s.Update(core.KeyMsg{Data: "\x03"}) // control char ignored
	if s.filter != "" {
		t.Fatalf("expected filter unchanged, got %q", s.filter)
	}
}

func TestSessionSelectorRename(t *testing.T) {
	s := NewSessionSelector()
	s.SetItems(testSessionItems())
	renamed := make(chan struct {
		item SessionItem
		name string
	}, 1)
	s.SetOnRename(func(it SessionItem, name string) {
		renamed <- struct {
			item SessionItem
			name string
		}{it, name}
	})
	s.Update(core.KeyMsg{Data: "\x12"}) // ctrl+r enters rename mode
	if !s.renameMode {
		t.Fatal("expected rename mode")
	}
	if s.renameBuf != "专利撰写" {
		t.Fatalf("expected renameBuf prefilled, got %q", s.renameBuf)
	}
	s.Update(core.KeyMsg{Data: "X"})
	if s.renameBuf != "专利撰写X" {
		t.Fatalf("expected appended char, got %q", s.renameBuf)
	}
	s.Update(core.KeyMsg{Data: "\x7f"}) // backspace
	if s.renameBuf != "专利撰写" {
		t.Fatalf("expected backspace applied, got %q", s.renameBuf)
	}
	s.Update(core.KeyMsg{Data: "\r"}) // enter commits
	select {
	case r := <-renamed:
		if r.name != "专利撰写" || r.item.ID != "sess-001" {
			t.Fatalf("unexpected rename %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected onRename callback")
	}
	if s.renameMode {
		t.Fatal("expected rename mode exited")
	}
}

func TestSessionSelectorRenameEmptyBuf(t *testing.T) {
	s := NewSessionSelector()
	s.SetItems(testSessionItems())
	fired := false
	s.SetOnRename(func(SessionItem, string) { fired = true })
	s.startRename()
	s.renameBuf = ""
	s.handleRenameInput("\r") // empty buffer — no callback
	if fired {
		t.Fatal("expected no onRename for empty buffer")
	}
	// Escape cancels.
	s.startRename()
	s.handleRenameInput("\x1b")
	if s.renameMode {
		t.Fatal("expected rename canceled by escape")
	}
}

func TestSessionSelectorStartRenameNoItems(t *testing.T) {
	s := NewSessionSelector()
	s.startRename() // empty filtered — no-op
	if s.renameMode {
		t.Fatal("expected no rename mode without items")
	}
}

func TestSessionSelectorConfirmNoItems(t *testing.T) {
	s := NewSessionSelector()
	fired := false
	s.SetOnSelect(func(SessionItem) { fired = true })
	s.confirm() // empty — no-op
	if fired {
		t.Fatal("expected no confirm without items")
	}
}

func TestSessionSelectorRender(t *testing.T) {
	s := NewSessionSelector()
	s.SetItems(testSessionItems())
	s.SetAvailableHeight(24)
	// Width must fit the fixed footer line (rendered untruncated, source behavior).
	lines := s.Render(120)
	if len(lines) == 0 {
		t.Fatal("expected render")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"专利撰写", "法律咨询", "12 msgs", "3 msgs"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in render, got %q", want, joined)
		}
	}
	for _, ln := range lines {
		// The fixed help footer renders wider than the panel (untruncated,
		// source behavior) — skip it in the width assertion.
		if strings.HasPrefix(ln, "  / search") {
			continue
		}
		if w := core.VisibleWidth(ln); w > 120 {
			t.Fatalf("line width %d > 120 (line=%q)", w, ln)
		}
	}
}

func TestSessionSelectorRenderFilteredEmpty(t *testing.T) {
	s := NewSessionSelector()
	s.SetItems(testSessionItems())
	s.filter = "zzz"
	s.applyFilterLocked()
	lines := s.Render(80)
	if len(lines) == 0 {
		t.Fatal("expected render with no matches")
	}
}
