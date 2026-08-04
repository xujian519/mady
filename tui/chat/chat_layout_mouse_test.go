package chat

import (
	"fmt"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// TestChatLayoutHitTest verifies mouse hit-testing via the rendered Flex.
func TestChatLayoutHitTest(t *testing.T) {
	app, vt := newTestChatApp(t, ChatAppConfig{Title: "Demo"})
	cols, rows := vt.Size()
	app.layout.Render(cols)

	// Before the first render mainFlex is nil → miss.
	l2 := &chatLayout{}
	if _, _, ok := l2.HitTest(0, 0); ok {
		t.Fatal("HitTest with nil mainFlex should miss")
	}

	// After render: the status bar is the last child, near the bottom.
	child, rect, ok := app.layout.HitTest(rows-1, 0)
	if !ok {
		t.Fatalf("HitTest(%d, 0) should hit the status bar row", rows-1)
	}
	if rect.Row != rows-1 || rect.Height != 1 {
		t.Fatalf("status bar rect = %+v, want row=%d height=1", rect, rows-1)
	}
	_ = child

	// The editor occupies a row above the footer/status bar; find it.
	if app.layout.editorTop <= 0 {
		t.Fatal("editorTop should be set after render")
	}
	child, rect, ok = app.layout.HitTest(app.layout.editorTop, 0)
	if !ok {
		t.Fatalf("HitTest(%d, 0) should hit the editor frame", app.layout.editorTop)
	}
	if rect.Row != app.layout.editorTop {
		t.Fatalf("editor rect row = %d, want %d", rect.Row, app.layout.editorTop)
	}
	_ = child

	// Header row should hit the header child.
	_, _, ok = app.layout.HitTest(0, 0)
	if !ok {
		t.Fatal("HitTest(0,0) should hit the header")
	}

	// Out-of-bounds col misses.
	if _, _, ok := app.layout.HitTest(rows-1, cols+10); ok {
		t.Fatal("HitTest out of bounds should miss")
	}
}

// TestHandleMouseMsgWheelConsumed verifies wheel events route to the history
// and are consumed (no delivery to siblings).
func TestHandleMouseMsgWheelConsumed(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	for i := 0; i < 30; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: fmt.Sprintf("msg %d with content", i)})
	}
	cols, _ := app.TerminalSize()
	app.layout.Render(cols)

	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MouseWheelUp, Row: 5, Col: 5})
	hist.mu.Lock()
	off := hist.offset
	hist.mu.Unlock()
	if off != 3 {
		t.Fatalf("wheel up offset = %d, want 3", off)
	}

	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MouseWheelDown, Row: 5, Col: 5})
	hist.mu.Lock()
	off = hist.offset
	hist.mu.Unlock()
	if off != 0 {
		t.Fatalf("wheel down offset = %d, want 0", off)
	}

	// Horizontal wheel is consumed but does not scroll.
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MouseWheelLeft, Row: 5, Col: 5})
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MouseWheelRight, Row: 5, Col: 5})
	hist.mu.Lock()
	off = hist.offset
	hist.mu.Unlock()
	if off != 0 {
		t.Fatalf("horizontal wheel should not scroll, offset=%d", off)
	}
}

// TestHandleMouseMsgPressOnHistory verifies a plain left-click on the history
// starts a selection and does not crash; motion/release completes the drag.
func TestHandleMouseMsgPressOnHistory(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	for i := 0; i < 5; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: fmt.Sprintf("line %d", i)})
	}
	cols, _ := app.TerminalSize()
	app.layout.Render(cols)
	hist.SetMaxRows(5)

	// Row 0 is the first content line (follow=true, offset=0).
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 1, Button: 1})
	hist.mu.Lock()
	active := hist.selActive
	hist.mu.Unlock()
	if !active {
		t.Fatal("press on history should start a selection")
	}

	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MouseMotion, Row: 2, Col: 5, Button: 1})
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MouseRelease, Row: 2, Col: 5, Button: 1})
	hist.mu.Lock()
	active = hist.selActive
	dragging := hist.selDragging
	hist.mu.Unlock()
	if !active || dragging {
		t.Fatalf("after drag: active=%v dragging=%v", active, dragging)
	}
	if sel := hist.GetSelectedText(); sel == "" {
		t.Fatal("expected non-empty selection after drag")
	}
}

// TestHandleMouseMsgGestureSuppression verifies that a press immediately
// after a wheel event is suppressed (trackpad gesture protection).
func TestHandleMouseMsgGestureSuppression(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	for i := 0; i < 10; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: "x"})
	}
	cols, _ := app.TerminalSize()
	app.layout.Render(cols)

	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MouseWheelUp, Row: 3, Col: 3})
	// Immediate press (within 400ms) must be suppressed.
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MousePress, Row: 3, Col: 3, Button: 1})
	hist.mu.Lock()
	suppress := hist.suppressGesture
	selDrag := hist.selDragging
	hist.mu.Unlock()
	if !suppress {
		t.Fatal("press after wheel should set suppressGesture")
	}
	if selDrag {
		t.Fatal("suppressed press must not start a selection")
	}
	// Release clears the suppression.
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MouseRelease, Row: 3, Col: 3, Button: 1})
	hist.mu.Lock()
	suppress = hist.suppressGesture
	hist.mu.Unlock()
	if suppress {
		t.Fatal("release should clear suppressGesture")
	}
}

// TestHandleMouseMsgScrollbarClick verifies clicking the scrollbar jumps the
// viewport to the proportional position.
func TestHandleMouseMsgScrollbarClick(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	for i := 0; i < 60; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: fmt.Sprintf("msg %d with some content", i)})
	}
	cols, _ := app.TerminalSize()
	hist.SetMaxRows(10)
	app.layout.Render(cols)

	// Scrollbar occupies the rightmost column (sbWidth=1 → col 79 for 80-wide).
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MousePress, Row: 5, Col: cols - 1, Button: 1})
	hist.mu.Lock()
	off := hist.offset
	hist.mu.Unlock()
	total := int64(len(hist.cachedAll))
	if off <= 0 || off >= total-10 {
		t.Fatalf("scrollbar click should jump mid-content: offset=%d total=%d", off, total)
	}
	if hist.follow {
		t.Fatal("scrollbar click should disable follow")
	}

	// Clicking the very top of the scrollbar lands at offset 0.
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MousePress, Row: 0, Col: cols - 1, Button: 1})
	hist.mu.Lock()
	if hist.offset != 0 {
		t.Fatalf("top scrollbar click offset = %d, want 0", hist.offset)
	}
	hist.mu.Unlock()
}

// TestHandleMouseMsgToolGroupToggle verifies clicking a collapsed tool group
// header expands it (and clicking again collapses). A leading zero-height
// message (empty user text) keeps the group header at viewport row 0 —
// renderToolGroup hardcodes startLine=0, so a taller leading message would
// break the click-to-line mapping (pre-existing source bug, out of scope).
func TestHandleMouseMsgToolGroupToggle(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	hist.Append(ChatMessage{Role: RoleUser}) // renders 0 lines
	for i := 0; i < 3; i++ {
		hist.Append(ChatMessage{Role: RoleTool, Meta: fmt.Sprintf("tool%d", i), Text: "..."})
	}
	cols, _ := app.TerminalSize()
	hist.SetMaxRows(10)
	app.layout.Render(cols)

	hist.mu.Lock()
	expanded := hist.expandedGroups[1]
	hist.mu.Unlock()
	if expanded {
		t.Fatal("setup: group should start collapsed")
	}

	// The collapsed group renders a single header line at row 0.
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 0, Button: 1})
	hist.mu.Lock()
	expanded = hist.expandedGroups[1]
	hist.mu.Unlock()
	if !expanded {
		t.Fatal("click on group header should expand the group")
	}

	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 0, Button: 1})
	hist.mu.Lock()
	expanded = hist.expandedGroups[1]
	hist.mu.Unlock()
	if expanded {
		t.Fatal("second click should collapse the group")
	}
}

// TestHandleMouseMsgThinkingToggle verifies clicking a thinking header toggles
// the segment's collapsed state.
func TestHandleMouseMsgThinkingToggle(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	id := hist.AppendDeltaWithKind("", "thinking content", "thinking")
	cols, _ := app.TerminalSize()
	hist.SetMaxRows(10)
	app.layout.Render(cols)

	msgs := hist.Messages()
	if len(msgs) != 1 || len(msgs[0].ThinkingSegments) != 1 {
		t.Fatalf("setup: expected 1 thinking segment, got %+v", msgs)
	}

	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 0, Button: 1})
	hist.mu.Lock()
	msgs = append([]ChatMessage(nil), hist.messages...)
	hist.mu.Unlock()
	if !msgs[0].ThinkingSegments[0].Collapsed {
		t.Fatal("click on thinking header should collapse the segment")
	}
	_ = id
}

// TestHandleMouseMsgToolMessageCollapse verifies clicking a non-group tool
// message toggles its Collapsed flag.
func TestHandleMouseMsgToolMessageCollapse(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	hist.Append(ChatMessage{Role: RoleTool, Meta: "search", Text: "single tool", Collapsed: false})
	cols, _ := app.TerminalSize()
	hist.SetMaxRows(10)
	app.layout.Render(cols)

	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 0, Button: 1})
	hist.mu.Lock()
	msgs := append([]ChatMessage(nil), hist.messages...)
	hist.mu.Unlock()
	if !msgs[0].Collapsed {
		t.Fatal("click on tool message should collapse it")
	}
}

// TestHandleMouseMsgMissFallsBackToLegacy verifies that mouse events outside
// any child rect fall back to the legacy broadcast path (history + editor).
func TestHandleMouseMsgMissFallsBackToLegacy(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	cols, rows := app.TerminalSize()
	// 消息分隔符密度影响每屏能容纳的条数，直接按渲染行数校准溢出条件，
	// 不依赖固定消息条数（密度调整后 10 条不再溢出 24 行视口）。
	for i := 0; ; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: "x"})
		app.layout.Render(cols)
		hist.mu.Lock()
		n := len(hist.cachedAll)
		hist.mu.Unlock()
		// 内容比整个终端至少多 3 行 ⇒ 历史视口必然可滚动 3 行。
		if int64(n) >= rows+3 {
			break
		}
		if i > 1000 {
			t.Fatal("history never overflowed the viewport")
		}
	}

	// A click at a column beyond the flex width misses every child.
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MouseWheelUp, Row: 3, Col: cols + 50})
	hist.mu.Lock()
	off := hist.offset
	hist.mu.Unlock()
	if off != 3 {
		t.Fatalf("legacy fallback wheel should scroll history, offset=%d", off)
	}
}

// TestDeliverMouseToOthersEditorReceives verifies editor delivery path when
// the hit child is the history (child != editor): a press-motion-release
// sequence translated into the editor's local space selects editor text.
func TestDeliverMouseToOthersEditorReceives(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	hist.Append(ChatMessage{Role: RoleAssistant, Text: "x"})
	app.editor.Update(core.KeyMsg{Data: "hello"})
	app.editor.Render(40) // populate lastVisuals (prompt "❯ " is 1 visible col)
	cols, _ := app.TerminalSize()
	app.layout.Render(cols)

	editorTop := app.layout.editorTop
	// Translate screen rows back to editor-local rows (local row 0 = buffer
	// row 0 at editorTop+1).
	press := core.MouseMsg{Action: core.MousePress, Row: editorTop + 1, Col: 3, Button: 1}
	motion := core.MouseMsg{Action: core.MouseMotion, Row: editorTop + 1, Col: 6, Button: 1}
	release := core.MouseMsg{Action: core.MouseRelease, Row: editorTop + 1, Col: 6, Button: 1}
	app.layout.deliverMouseToOthers(hist, press)
	app.layout.deliverMouseToOthers(hist, motion)
	app.layout.deliverMouseToOthers(hist, release)

	// Editor-local col = mouse col - 2 (prompt "❯ " occupies 1 cell plus a
	// 1-cell margin): press col 3 → buffer col 1, motion col 6 → buffer col 4.
	if got := app.editor.GetSelectedText(); got != "ell" {
		t.Fatalf("editor selection = %q, want %q", got, "ell")
	}
}

// TestLegacyMouseFallback verifies the pre-render broadcast path.
func TestLegacyMouseFallback(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	for i := 0; i < 10; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: "y"})
	}
	// No Render call → mainFlex nil → legacyMouseFallback used.
	app.layout.legacyMouseFallback(core.MouseMsg{Action: core.MouseWheelUp, Row: 3, Col: 3})
	hist.mu.Lock()
	off := hist.offset
	hist.mu.Unlock()
	if off != 3 {
		t.Fatalf("legacy fallback offset = %d, want 3", off)
	}

	// Update() with mouse and no mainFlex also takes the fallback path.
	app.layout.mainFlex = nil
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MouseWheelUp, Row: 3, Col: 3})
	hist.mu.Lock()
	off = hist.offset
	hist.mu.Unlock()
	if off != 6 {
		t.Fatalf("fallback via Update offset = %d, want 6", off)
	}
}

// TestChatLayoutInvalidateNoop verifies Invalidate is a no-op on chatLayout.
func TestChatLayoutInvalidateNoop(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.layout.Invalidate() // must not panic
	ef := &editorFrame{editor: app.editor}
	ef.Invalidate() // must not panic
	lines := ef.Render(40)
	if len(lines) < 3 {
		t.Fatalf("editor frame render should add top+bottom borders, got %d lines", len(lines))
	}
}

// TestHandleMouseMsgReleaseMiddleButton verifies middle-button release is
// routed to doCopy without panicking (no selection → copies last assistant).
func TestHandleMouseMsgReleaseMiddleButton(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.History().Append(ChatMessage{Role: RoleAssistant, Text: "last answer"})
	cols, _ := app.TerminalSize()
	app.layout.Render(cols)
	// doCopy spawns a clipboard goroutine; just ensure no panic.
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MouseRelease, Row: 1, Col: 1, Button: 2})
}

// TestToggleThinkingAfterToolGroup is the regression test for the
// msgRange-index/message-index drift: tryToggleThinkingAtLineLocked used the
// ranges-array index directly as a h.messages index. A tool group collapses
// multiple messages into ONE range, so once a group exists the arrays drift
// (ranges = [user, group, thinking] vs messages = [user, tool0, tool1,
// assistant]); toggling the thinking header then acted on the group's last
// tool message and silently did nothing.
func TestToggleThinkingAfterToolGroup(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	hist.Append(ChatMessage{Role: RoleUser}) // renders 0 lines
	hist.Append(ChatMessage{Role: RoleTool, Meta: "tool0", Text: "a"})
	hist.Append(ChatMessage{Role: RoleTool, Meta: "tool1", Text: "b"})
	hist.AppendDeltaWithKind("", "thinking content", "thinking")

	cols, _ := app.TerminalSize()
	hist.SetMaxRows(10)
	app.layout.Render(cols)

	// Locate the thinking (assistant) message's range — its msgIndex is 3,
	// while its range index is 2 (the tool pair shares one range).
	hist.mu.Lock()
	var thinkRange *msgRange
	for i := range hist.cachedMsgRanges {
		r := &hist.cachedMsgRanges[i]
		if r.msgIndex == 3 {
			thinkRange = r
			break
		}
	}
	if thinkRange == nil {
		hist.mu.Unlock()
		t.Fatal("setup: no range for the thinking message (group may not have formed)")
	}
	if len(hist.cachedMsgRanges) != 3 {
		hist.mu.Unlock()
		t.Fatalf("setup: expected 3 ranges (user/group/thinking), got %d", len(hist.cachedMsgRanges))
	}
	hist.mu.Unlock()

	// Toggle at the thinking header line: must collapse the assistant's
	// thinking segment, not silently no-op on a tool message.
	if !hist.tryToggleThinkingAtLineLocked(int64(thinkRange.startLine)) {
		t.Fatal("tryToggleThinkingAtLineLocked returned false for the thinking header")
	}

	msgs := hist.Messages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if len(msgs[3].ThinkingSegments) != 1 || !msgs[3].ThinkingSegments[0].Collapsed {
		t.Error("assistant thinking segment should be collapsed after toggle")
	}
	if msgs[1].Collapsed || msgs[2].Collapsed {
		t.Error("tool messages must be untouched by the thinking toggle")
	}
}
