package chat

// This file handles input and viewport scrolling for ChatHistory: the Msg
// types routed through Update, mouse handling (wheel scroll, click-to-toggle
// collapsed tool/diff/thinking blocks, text selection drag), and the
// viewport offset math (ScrollBy/scrollByLocked/FollowTail).
//
// Click-to-toggle relies on cachedMsgRanges (built during renderAll in
// chat_history_render.go) to map a viewport row to the message that owns it.

import (
	"time"

	"github.com/xujian519/mady/tui/core"
)

type chatAppendMsg struct {
	Message ChatMessage
}

func (chatAppendMsg) MsgMarker() {}

type chatUpdateMsg struct {
	ID string
	Fn func(m *ChatMessage)
}

func (chatUpdateMsg) MsgMarker() {}

type chatDeltaMsg struct {
	ID    string
	Delta string
}

func (chatDeltaMsg) MsgMarker() {}

type chatFinalizeMsg struct {
	ID string
}

func (chatFinalizeMsg) MsgMarker() {}

type chatClearMsg struct{}

func (chatClearMsg) MsgMarker() {}

type chatScrollMsg struct {
	Lines int64
}

func (chatScrollMsg) MsgMarker() {}

// Update implements Updatable. It processes Msg values from the TUI event
// loop, enabling message-driven mutation of the chat history.
func (h *ChatHistory) Update(msg core.Msg) core.Cmd {
	switch m := msg.(type) {
	case chatAppendMsg:
		h.Append(m.Message)
	case chatUpdateMsg:
		h.PatchMessage(m.ID, m.Fn)
	case chatDeltaMsg:
		h.AppendDelta(m.ID, m.Delta)
	case chatFinalizeMsg:
		h.Finalize(m.ID)
	case chatClearMsg:
		h.Clear()
	case chatScrollMsg:
		h.ScrollBy(m.Lines)
	case core.WindowSizeMsg:
		h.Invalidate()
	case core.MouseMsg:
		h.handleMouse(m)
	}
	return nil
}

func (h *ChatHistory) handleMouse(m core.MouseMsg) {
	h.mu.Lock()

	needInvalidate := false

	switch m.Action {
	case core.MouseWheelUp:
		h.lastWheelAt = time.Now()
		h.scrollByLocked(3)
		needInvalidate = true
	case core.MouseWheelDown:
		h.lastWheelAt = time.Now()
		h.scrollByLocked(-3)
		needInvalidate = true
	case core.MousePress:
		// Left button press: check if clicking on thinking header to toggle collapse
		absLine := h.viewportRowToAbsoluteLocked(m.Row)
		if absLine < 0 {
			h.mu.Unlock()
			return
		}
		// If this press follows a wheel event within 400ms, it is part of a
		// scroll gesture (e.g. a trackpad two-finger slide that emits stray
		// press/release events). Suppress the whole gesture BEFORE any toggle
		// or selection so that scrolling never expands/collapses diffs or tool
		// groups and never starts an accidental selection.
		if time.Since(h.lastWheelAt) < 400*time.Millisecond {
			h.suppressGesture = true
			h.mu.Unlock()
			return
		}
		if h.tryToggleThinkingAtLineLocked(absLine) {
			h.dirty = true
			h.mu.Unlock()
			h.invalidate()
			return
		}
		// Start selection — convert viewport row to absolute line index
		mappedCol := h.mapMouseColToVisibleColLocked(absLine, m.Col)
		h.selDragging = true
		h.selActive = true
		h.selStart = selectionPos{line: absLine, col: mappedCol}
		h.selEnd = selectionPos{line: absLine, col: mappedCol}
		// Don't trigger render yet — selection is empty and invisible until drag motion.
	case core.MouseMotion:
		if h.suppressGesture {
			break
		}
		if h.selDragging {
			absLine := h.viewportRowToAbsoluteLocked(m.Row)
			if absLine >= 0 {
				mappedCol := h.mapMouseColToVisibleColLocked(absLine, m.Col)
				h.selEnd = selectionPos{line: absLine, col: mappedCol}
				h.dirty = true // force re-render to update selection highlight
				needInvalidate = true
			}
		}
	case core.MouseRelease:
		if h.suppressGesture {
			h.suppressGesture = false
			break
		}
		if h.selDragging {
			h.selDragging = false
			// If selection is empty (no movement), clear it
			if h.isSelectionEmptyLocked() {
				h.selActive = false
			}
			h.dirty = true // force re-render for final selection state
			needInvalidate = true
		}
	}

	h.mu.Unlock()

	if needInvalidate {
		h.invalidate()
	}
}

// viewportRowToAbsoluteLocked converts a viewport-relative row to an absolute
// line index in cachedAll. Returns -1 if the row is out of range.
func (h *ChatHistory) viewportRowToAbsoluteLocked(viewportRow int64) int64 {
	total := int64(len(h.cachedAll))
	if total == 0 || h.maxRows <= 0 || viewportRow < 0 {
		return -1
	}
	end := total - h.offset
	start := end - h.maxRows
	if start < 0 {
		start = 0
	}
	// Clamp offset if it's now past the valid range (content shrank).
	if total <= h.maxRows {
		h.offset = 0
		start = 0
	} else if h.offset > total-h.maxRows {
		h.offset = total - h.maxRows
		start = h.offset
	}
	// When the scrollback is scrolled up (!follow, offset > 0), Render inserts
	// a "^ N more lines" indicator row at viewport position 0 and drops the
	// last visible line. The indicator is not selectable — skip past it so
	// mouse rows map to the content actually displayed beneath it. Without
	// this adjustment every selection is off by one (clicking the first
	// content line selects the second, etc.).
	if !h.follow && h.offset > 0 {
		viewportRow--
		if viewportRow < 0 {
			return -1 // click landed on the indicator row itself
		}
	}
	absIdx := start + viewportRow
	if absIdx >= total {
		return -1
	}
	return absIdx
}

// mapMouseColToVisibleColLocked normalizes mouse columns into a stable
// visible-column coordinate for the target line. Continuation cells (the
// right half of a wide rune) are mapped back to the wide rune's start.
func (h *ChatHistory) mapMouseColToVisibleColLocked(absLine, mouseCol int64) int64 {
	if absLine < 0 || absLine >= int64(len(h.cachedAll)) {
		if mouseCol < 0 {
			return 0
		}
		return mouseCol
	}
	line := h.cachedAll[absLine]
	row := core.ParseLine(line)
	lineWidth := row.VisibleWidth()
	if mouseCol < 0 {
		mouseCol = 0
	}
	if mouseCol > lineWidth {
		mouseCol = lineWidth
	}
	if row.IsRaw() || len(row.Cells) == 0 {
		return mouseCol
	}
	idx := int(mouseCol)
	if idx >= len(row.Cells) {
		idx = len(row.Cells) - 1
	}
	if idx >= 0 && row.Cells[idx].IsContinuation() {
		for idx > 0 && row.Cells[idx].IsContinuation() {
			idx--
		}
		return int64(idx)
	}
	return mouseCol
}

// tryToggleThinkingAtLineLocked checks if the clicked line is within a thinking
// segment and toggles its collapsed state. Returns true if toggled.
func (h *ChatHistory) expandToolGroup(msgRangeIdx int) {
	if h.expandedGroups == nil {
		h.expandedGroups = make(map[int]bool)
	}
	r := h.cachedMsgRanges[msgRangeIdx]
	if r.toolGroup {
		if h.expandedGroups[r.msgIndex] {
			delete(h.expandedGroups, r.msgIndex)
		} else {
			h.expandedGroups[r.msgIndex] = true
		}
	}
	h.dirty = true
	h.clearMsgCacheLocked()
}

func (h *ChatHistory) tryToggleThinkingAtLineLocked(absLine int64) bool {
	// Find which message contains this line
	msgIdx := -1
	for i, r := range h.cachedMsgRanges {
		if absLine >= int64(r.startLine) && absLine < int64(r.endLine) {
			msgIdx = i
			break
		}
	}
	if msgIdx < 0 || msgIdx >= len(h.cachedMsgRanges) {
		return false
	}

	// Check if it's a tool group
	if r := &h.cachedMsgRanges[msgIdx]; r.toolGroup {
		h.expandToolGroup(msgIdx)
		h.dirty = true
		return true
	}

	msg := &h.messages[msgIdx]
	if msg.Role == RoleTool && msg.Collapsed {
		msg.Collapsed = false
		h.invalidateMessageLocked(msg.ID)
		return true
	}
	if msg.Role == RoleTool && !msg.Collapsed {
		msg.Collapsed = true
		h.invalidateMessageLocked(msg.ID)
		return true
	}
	if msg.Role == RoleAssistant && msg.Collapsed {
		msg.Collapsed = false
		h.invalidateMessageLocked(msg.ID)
		return true
	}
	if msg.Role == RoleAssistant && !msg.Collapsed && msg.Meta == "diff" {
		msg.Collapsed = true
		h.invalidateMessageLocked(msg.ID)
		return true
	}
	if msg.Role != RoleAssistant || len(msg.ThinkingSegments) == 0 {
		return false
	}

	// Calculate line offset within the message
	msgStart := h.cachedMsgRanges[msgIdx].startLine
	lineOffset := absLine - int64(msgStart)

	// Track line positions to find which thinking segment contains this line
	currentLine := int64(0)
	for i := range msg.ThinkingSegments {
		seg := &msg.ThinkingSegments[i]
		if seg.Text == "" {
			continue
		}

		// Header line (e.g., "◐ thinking")
		if lineOffset == currentLine {
			seg.Collapsed = !seg.Collapsed
			h.invalidateMessageLocked(msg.ID)
			return true
		}
		currentLine++

		if !seg.Collapsed {
			// Content lines
			contentLines := int64(len(core.WrapAnsi(seg.Text, h.cachedWidth)))
			if lineOffset > currentLine && lineOffset <= currentLine+contentLines {
				seg.Collapsed = !seg.Collapsed
				h.invalidateMessageLocked(msg.ID)
				return true
			}
			currentLine += contentLines + 1 // +1 for separator
		} else {
			// Collapsed line (summary)
			if lineOffset == currentLine {
				seg.Collapsed = !seg.Collapsed
				h.invalidateMessageLocked(msg.ID)
				return true
			}
			currentLine++
		}
	}

	return false
}

// ScrollBy moves the viewport by n lines (positive = up, negative = down).
// Stops auto-following the tail when user scrolls up.
func (h *ChatHistory) ScrollBy(n int64) {
	h.mu.Lock()
	h.scrollByLocked(n)
	h.mu.Unlock()
	h.invalidate()
}

func (h *ChatHistory) scrollByLocked(n int64) {
	total := int64(len(h.cachedAll))
	h.offset += n
	if h.offset < 0 {
		h.offset = 0
		h.follow = true
	} else {
		h.follow = false
	}
	if h.maxRows > 0 && h.offset > total-h.maxRows {
		if total >= h.maxRows {
			h.offset = total - h.maxRows
		} else {
			h.offset = 0
		}
	}
}

// FollowTail resets the viewport to the bottom.
func (h *ChatHistory) FollowTail() {
	h.mu.Lock()
	h.offset = 0
	h.follow = true
	h.mu.Unlock()
	h.invalidate()
}
