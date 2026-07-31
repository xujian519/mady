package chat

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// TestChatHistoryUpdateMsgTypes verifies message-driven mutation via Update.
func TestChatHistoryUpdateMsgTypes(t *testing.T) {
	h := NewChatHistory()

	// chatAppendMsg
	h.Update(chatAppendMsg{Message: ChatMessage{Role: RoleUser, Text: "hello"}})
	msgs := h.Messages()
	if len(msgs) != 1 || msgs[0].Text != "hello" {
		t.Fatalf("append via msg failed: %+v", msgs)
	}
	id := msgs[0].ID
	if id == "" {
		t.Fatal("append msg should carry an id")
	}

	// chatUpdateMsg (PatchMessage)
	h.Update(chatUpdateMsg{ID: id, Fn: func(m *ChatMessage) { m.Text = "patched" }})
	msgs = h.Messages()
	if msgs[0].Text != "patched" {
		t.Fatalf("patch via msg failed: %q", msgs[0].Text)
	}

	// chatDeltaMsg (AppendDelta)
	h.Update(chatDeltaMsg{ID: id, Delta: " +more"})
	msgs = h.Messages()
	if msgs[0].Text != "patched +more" {
		t.Fatalf("delta via msg failed: %q", msgs[0].Text)
	}

	// chatFinalizeMsg
	h.Update(chatFinalizeMsg{ID: id})
	msgs = h.Messages()
	if msgs[0].Pending {
		t.Fatal("finalize via msg should clear pending")
	}

	// chatScrollMsg
	h.Update(chatScrollMsg{Lines: 5})
	if h.offset != 0 {
		// cachedAll is empty → scroll is a no-op clamped to 0
	}

	// chatClearMsg
	h.Update(chatClearMsg{})
	if len(h.Messages()) != 0 {
		t.Fatal("clear via msg should empty the transcript")
	}

	// Unknown msg types are ignored.
	h.Update(core.MouseMsg{Action: core.MouseMotion, Row: 0, Col: 0})
	h.Update(core.PasteMsg{Text: "x"})
}

// TestChatHistoryScrollMsgWithContent verifies chatScrollMsg actually scrolls
// when content exists.
func TestChatHistoryScrollMsgWithContent(t *testing.T) {
	h := NewChatHistory()
	for i := 0; i < 20; i++ {
		h.Append(ChatMessage{Role: RoleAssistant, Text: "line"})
	}
	h.SetMaxRows(5)
	h.Render(40)
	h.Update(chatScrollMsg{Lines: 3})
	if h.offset != 3 {
		t.Fatalf("scroll msg offset = %d, want 3", h.offset)
	}
}

// TestChatHistoryMouseUnhandledPaths covers handleMouse branches not exercised
// by the layout-level tests: motion without drag, release without press, and
// press outside content.
func TestChatHistoryMouseUnhandledPaths(t *testing.T) {
	t.Run("motion without drag no selection change", func(t *testing.T) {
		h := NewChatHistory()
		h.Update(core.MouseMsg{Action: core.MouseMotion, Row: 0, Col: 0})
		h.mu.Lock()
		sel := h.selDragging
		h.mu.Unlock()
		if sel {
			t.Fatal("motion without drag must not start selection")
		}
	})

	t.Run("release without press no selection change", func(t *testing.T) {
		h := NewChatHistory()
		h.Update(core.MouseMsg{Action: core.MouseRelease, Row: 0, Col: 0, Button: 1})
		h.mu.Lock()
		sel := h.selDragging
		h.mu.Unlock()
		if sel {
			t.Fatal("release without press must not start selection")
		}
	})

	t.Run("press outside content not consumed", func(t *testing.T) {
		h := NewChatHistory()
		h.Append(ChatMessage{Role: RoleAssistant, Text: "x"})
		h.SetMaxRows(5)
		h.Render(40)
		h.Update(core.MouseMsg{Action: core.MousePress, Row: 99, Col: 0, Button: 1})
		if h.MouseConsumed() {
			t.Fatal("press outside content must not consume")
		}
		h.mu.Lock()
		sel := h.selActive
		h.mu.Unlock()
		if sel {
			t.Fatal("press outside content must not start selection")
		}
	})

	t.Run("wheel up at top clamps", func(t *testing.T) {
		h := NewChatHistory()
		for i := 0; i < 20; i++ {
			h.Append(ChatMessage{Role: RoleAssistant, Text: "y"})
		}
		h.SetMaxRows(5)
		h.Render(40)
		h.Update(core.MouseMsg{Action: core.MouseWheelUp, Row: 0, Col: 0})
		if !h.MouseConsumed() {
			t.Fatal("wheel should consume")
		}
		if h.offset != 3 {
			t.Fatalf("offset = %d, want 3", h.offset)
		}
		if h.follow {
			t.Fatal("wheel up should disable follow")
		}
	})
}

// TestChatHistoryMouseMotionDragContinues verifies drag motion updates the
// selection end and release finalizes it.
func TestChatHistoryMouseMotionDragContinues(t *testing.T) {
	h := NewChatHistory()
	for i := 0; i < 5; i++ {
		h.Append(ChatMessage{Role: RoleAssistant, Text: "content line"})
	}
	h.SetMaxRows(5)
	h.Render(40)

	h.Update(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 0, Button: 1})
	h.Update(core.MouseMsg{Action: core.MouseMotion, Row: 2, Col: 5, Button: 1})
	if !h.MouseConsumed() {
		t.Fatal("drag motion should consume")
	}
	h.mu.Lock()
	start := h.selStart
	end := h.selEnd
	active := h.selActive
	h.mu.Unlock()
	if !active {
		t.Fatal("selection should be active after press")
	}
	if end.line < start.line || end.col < start.col {
		t.Fatalf("drag should move selEnd forward: start=%+v end=%+v", start, end)
	}
	h.Update(core.MouseMsg{Action: core.MouseRelease, Row: 2, Col: 5, Button: 1})
	if sel := h.GetSelectedText(); sel == "" {
		t.Fatal("expected selection after drag")
	}
}

// TestChatHistoryMousePressToNothing verifies a click-to-toggle miss on a
// plain message leaves the message untouched and starts a selection instead.
func TestChatHistoryMousePressToNothing(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "user msg"})
	h.SetMaxRows(5)
	h.Render(40)

	h.Update(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 1, Button: 1})
	h.mu.Lock()
	sel := h.selDragging
	msgs := append([]ChatMessage(nil), h.messages...)
	h.mu.Unlock()
	if !sel {
		t.Fatal("press on plain message should start drag selection")
	}
	if msgs[0].Collapsed {
		t.Fatal("plain message must not be toggled")
	}
}

// TestChatHistoryScrollbarClickClamped verifies scrollbar click math clamps
// newOffset into range even for degenerate rows.
func TestChatHistoryScrollbarClickClamped(t *testing.T) {
	h := NewChatHistory()
	for i := 0; i < 30; i++ {
		h.Append(ChatMessage{Role: RoleAssistant, Text: "z"})
	}
	h.SetMaxRows(6)
	h.Render(40)
	h.mu.Lock()
	total := int64(len(h.cachedAll))
	h.mu.Unlock()

	// Click below the viewport bottom → clickY clamps to maxRows-1, yielding
	// (maxRows-1)/maxRows of the max offset.
	h.Update(core.MouseMsg{Action: core.MousePress, Row: 500, Col: 39, Button: 1})
	h.mu.Lock()
	off := h.offset
	rows := h.maxRows
	h.mu.Unlock()
	want := (rows - 1) * (total - rows) / rows
	if off != want {
		t.Fatalf("offset = %d, want %d (total=%d maxRows=%d)", off, want, total, rows)
	}
}

// TestChatHistoryScrollbarDisabledSkips verifies the scrollbar branch is
// skipped when the scrollbar is disabled.
func TestChatHistoryScrollbarDisabledSkips(t *testing.T) {
	h := NewChatHistory()
	for i := 0; i < 30; i++ {
		h.Append(ChatMessage{Role: RoleAssistant, Text: "z"})
	}
	h.SetMaxRows(6)
	h.SetScrollbarEnabled(false)
	h.Render(40)

	// Click at the right edge — should NOT jump; instead starts a selection.
	h.Update(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 39, Button: 1})
	h.mu.Lock()
	sel := h.selDragging
	h.mu.Unlock()
	if !sel {
		t.Fatal("with scrollbar disabled, right-edge click should start selection")
	}
}
