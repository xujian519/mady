package chat

// This file contains text-selection highlighting logic.
// applySelectionHighlightLocked is the lock-required variant (used by tests).
// applySelectionHighlightSnapshot is the lock-free variant used during
// Phase 2 snapshot rendering in chat_history_render.go.

import (
	"github.com/xujian519/mady/tui/core"
)

func (h *ChatHistory) applySelectionHighlightLocked(lines []string, width int64) {
	total := int64(len(lines))
	if total == 0 || h.maxRows <= 0 {
		return
	}

	// Compute the visible viewport range in absolute indices
	end := total - h.offset
	start := end - h.maxRows
	if start < 0 {
		start = 0
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

	// Clamp selection to visible viewport
	if botLine < start || topLine >= end {
		return
	}
	if topLine < start {
		topLine = start
		topCol = 0
	}
	if botLine >= end {
		botLine = end - 1
		botCol = core.VisibleWidth(lines[botLine])
	}

	selBg := h.theme.SelectedBg
	if selBg == "" {
		selBg = "\x1b[48;5;33m"
	}
	selStyle := core.ParseLine(selBg + " " + "\x1b[0m")
	if selStyle.IsRaw() || len(selStyle.Cells) == 0 {
		return
	}
	selectedStyle := selStyle.Cells[0].Style
	if selectedStyle.Bg.IsDefault() {
		return
	}

	for i := topLine; i <= botLine && i < end; i++ {
		if i < 0 || i >= total {
			continue
		}
		line := lines[i]
		row := core.ParseLine(line)
		if row.IsRaw() {
			continue
		}
		lineWidth := row.VisibleWidth()
		if lineWidth <= 0 {
			continue
		}

		fromCol := int64(0)
		toCol := lineWidth
		switch {
		case topLine == botLine:
			fromCol = topCol
			toCol = botCol
		case i == topLine:
			fromCol = topCol
		case i == botLine:
			toCol = botCol
		}

		if fromCol < 0 {
			fromCol = 0
		}
		if toCol < 0 {
			toCol = 0
		}
		if fromCol > lineWidth {
			fromCol = lineWidth
		}
		if toCol > lineWidth {
			toCol = lineWidth
		}
		if toCol < fromCol {
			toCol = fromCol
		}

		if fromCol != toCol {
			start := int(fromCol)
			endCol := int(toCol)
			if start < 0 {
				start = 0
			}
			if endCol > len(row.Cells) {
				endCol = len(row.Cells)
			}
			for c := start; c < endCol; c++ {
				row.Cells[c].Style = selectedStyle
			}
		}

		highlighted := core.SerializeRow(row)
		// Keep each highlighted row width-stable across drag updates.
		lines[i] = core.PadToWidth(core.TruncateToWidth(highlighted, lineWidth, ""), lineWidth)
	}
}

// applySelectionHighlightSnapshot is the lock-free variant of
// applySelectionHighlightLocked used during snapshot rendering. It takes
// selection state as explicit parameters and highlights the full content
// (viewport clipping happens later in Render).
func (h *ChatHistory) applySelectionHighlightSnapshot(lines []string, width int64, selStart, selEnd selectionPos) {
	total := int64(len(lines))
	if total == 0 {
		return
	}

	topLine := selStart.line
	botLine := selEnd.line
	topCol := selStart.col
	botCol := selEnd.col

	// Normalize so topLine <= botLine, and when equal topCol <= botCol
	if topLine > botLine || (topLine == botLine && topCol > botCol) {
		topLine, botLine = botLine, topLine
		topCol, botCol = botCol, topCol
	}

	// Clamp to content bounds
	if topLine < 0 {
		topLine = 0
		topCol = 0
	}
	if botLine >= total {
		botLine = total - 1
		botCol = core.VisibleWidth(lines[botLine])
	}

	selBg := h.theme.SelectedBg
	if selBg == "" {
		selBg = "\x1b[48;5;33m"
	}
	selStyle := core.ParseLine(selBg + " " + "\x1b[0m")
	if selStyle.IsRaw() || len(selStyle.Cells) == 0 {
		return
	}
	selectedStyle := selStyle.Cells[0].Style
	if selectedStyle.Bg.IsDefault() {
		return
	}

	for i := topLine; i <= botLine && i < total; i++ {
		line := lines[i]
		row := core.ParseLine(line)
		if row.IsRaw() {
			continue
		}
		lineWidth := row.VisibleWidth()
		if lineWidth <= 0 {
			continue
		}

		fromCol := int64(0)
		toCol := lineWidth
		switch {
		case topLine == botLine:
			fromCol = topCol
			toCol = botCol
		case i == topLine:
			fromCol = topCol
		case i == botLine:
			toCol = botCol
		}

		if fromCol < 0 {
			fromCol = 0
		}
		if toCol < 0 {
			toCol = 0
		}
		if fromCol > lineWidth {
			fromCol = lineWidth
		}
		if toCol > lineWidth {
			toCol = lineWidth
		}
		if toCol < fromCol {
			toCol = fromCol
		}

		if fromCol != toCol {
			start := int(fromCol)
			endCol := int(toCol)
			if start < 0 {
				start = 0
			}
			if endCol > len(row.Cells) {
				endCol = len(row.Cells)
			}
			for c := start; c < endCol; c++ {
				row.Cells[c].Style = selectedStyle
			}
		}

		highlighted := core.SerializeRow(row)
		lines[i] = core.PadToWidth(core.TruncateToWidth(highlighted, lineWidth, ""), lineWidth)
	}
}
