package component

// This file holds the Editor undo/redo stack (pushSnapshotLocked / undo /
// redo / cloneRuneLines) and the submitted-input recall history
// (PushInputHistory / historyPrev / historyNext). These are two independent
// histories: the undo stack records every buffer mutation, while input
// history records only submitted values for up/down-arrow recall.

import (
	"strings"
)

// ---------------------------------------------------------------------------
// Undo / redo
// ---------------------------------------------------------------------------

func (e *Editor) pushSnapshotLocked() {
	snap := editorSnapshot{
		lines: cloneRuneLines(e.lines),
		row:   e.row,
		col:   e.col,
	}
	e.history = append(e.history, snap)
	if int64(len(e.history)) > e.historyMax {
		e.history = e.history[len(e.history)-int(e.historyMax):]
	}
	e.future = nil
}

func (e *Editor) undo() {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	if len(e.history) == 0 {
		e.mu.Unlock()
		return
	}
	current := editorSnapshot{
		lines: cloneRuneLines(e.lines),
		row:   e.row,
		col:   e.col,
	}
	snap := e.history[len(e.history)-1]
	e.history = e.history[:len(e.history)-1]
	e.future = append(e.future, current)
	e.lines = snap.lines
	e.row = snap.row
	e.col = snap.col
	e.allSelected = false
	fn := e.onChange
	v := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (e *Editor) redo() {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	if len(e.future) == 0 {
		e.mu.Unlock()
		return
	}
	current := editorSnapshot{
		lines: cloneRuneLines(e.lines),
		row:   e.row,
		col:   e.col,
	}
	snap := e.future[len(e.future)-1]
	e.future = e.future[:len(e.future)-1]
	e.history = append(e.history, current)
	e.lines = snap.lines
	e.row = snap.row
	e.col = snap.col
	e.allSelected = false
	fn := e.onChange
	v := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func cloneRuneLines(lines [][]rune) [][]rune {
	out := make([][]rune, len(lines))
	for i, ln := range lines {
		cp := make([]rune, len(ln))
		copy(cp, ln)
		out[i] = cp
	}
	return out
}

// submit invokes OnSubmit with the full value.
func (e *Editor) submit() {
	e.mu.RLock()
	val := e.valueLocked()
	fn := e.onSubmit
	e.mu.RUnlock()
	if fn != nil {
		fn(val)
	}
}

// ---------------------------------------------------------------------------
// Input history (up/down arrow recall)
// ---------------------------------------------------------------------------

// PushInputHistory saves a submitted value to the recall history.
func (e *Editor) PushInputHistory(value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if value == "" {
		return
	}
	// Avoid duplicating the most recent entry.
	if len(e.inputHistory) > 0 && e.inputHistory[len(e.inputHistory)-1] == value {
		return
	}
	e.inputHistory = append(e.inputHistory, value)
	if int64(len(e.inputHistory)) > e.inputHistoryMax {
		e.inputHistory = e.inputHistory[len(e.inputHistory)-int(e.inputHistoryMax):]
	}
}

// historyPrev recalls the previous (older) input from history.
// Returns true if history was consumed (cursor movement should be suppressed).
func (e *Editor) historyPrev() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.inputHistory) == 0 {
		return false
	}
	switch {
	case e.inputHistoryIndex < 0:
		// First time entering history mode — save current draft.
		e.inputHistoryDraft = e.valueLocked()
		e.inputHistoryIndex = int64(len(e.inputHistory) - 1)
	case e.inputHistoryIndex > 0:
		e.inputHistoryIndex--
	default:
		// Already at oldest entry; stay there.
		return true
	}
	e.setValueLocked(e.inputHistory[e.inputHistoryIndex])
	return true
}

// historyNext recalls the next (newer) input from history, or restores the
// draft when moving past the newest entry. Returns true if history was consumed.
func (e *Editor) historyNext() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inputHistoryIndex < 0 {
		return false
	}
	e.inputHistoryIndex++
	if int(e.inputHistoryIndex) >= len(e.inputHistory) {
		// Past newest — restore draft and exit history mode.
		e.setValueLocked(e.inputHistoryDraft)
		e.inputHistoryIndex = -1
		e.inputHistoryDraft = ""
	} else {
		e.setValueLocked(e.inputHistory[e.inputHistoryIndex])
	}
	return true
}

// setValueLocked overwrites the buffer without pushing an undo snapshot.
func (e *Editor) setValueLocked(s string) {
	raw := strings.Split(s, "\n")
	lines := make([][]rune, 0, len(raw))
	for _, line := range raw {
		lines = append(lines, []rune(line))
	}
	if len(lines) == 0 {
		lines = append(lines, []rune{})
	}
	e.lines = lines
	e.row = int64(len(lines) - 1)
	e.col = int64(len(e.lines[e.row]))
}
