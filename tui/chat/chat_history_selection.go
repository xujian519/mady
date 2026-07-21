package chat

// This file contains ChatHistory selection business logic: getting the
// selected text and clearing the selection. These are extracted from
// chat_history_render.go to separate rendering concerns from user-facing
// selection operations.

import (
	"strings"

	"github.com/xujian519/mady/tui/core"
)

func (h *ChatHistory) isSelectionEmptyLocked() bool {
	return h.selStart.line == h.selEnd.line && h.selStart.col == h.selEnd.col
}

// GetSelectedText returns the currently selected text, or empty string if no selection.
func (h *ChatHistory) GetSelectedText() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.getSelectedTextLocked()
}

// ClearSelection clears the current text selection.
func (h *ChatHistory) ClearSelection() {
	h.mu.Lock()
	h.selActive = false
	h.selDragging = false
	h.mu.Unlock()
	h.invalidate()
}

func (h *ChatHistory) getSelectedTextLocked() string {
	if !h.selActive || h.isSelectionEmptyLocked() {
		return ""
	}

	total := int64(len(h.cachedAll))
	if total == 0 {
		return ""
	}

	topLine := h.selStart.line
	botLine := h.selEnd.line
	topCol := h.selStart.col
	botCol := h.selEnd.col

	// Normalize so topLine <= botLine, and when equal topCol <= botCol
	if topLine > botLine || (topLine == botLine && topCol > botCol) {
		topLine, botLine = botLine, topLine
		topCol, botCol = botCol, topCol
	}

	if topLine < 0 || topLine >= total || botLine < 0 || botLine >= total {
		return ""
	}

	var result []string
	for i := topLine; i <= botLine && i < total; i++ {
		line := h.cachedAll[i]
		var part string
		switch {
		case topLine == botLine:
			part = core.SliceByColumn(line, topCol, botCol)
		case i == topLine:
			part = core.SliceByColumn(line, topCol, core.VisibleWidth(line))
		case i == botLine:
			part = core.SliceByColumn(line, 0, botCol)
		default:
			part = line
		}
		result = append(result, core.StripAnsi(part))
	}

	return strings.Join(result, "\n")
}
