package component

// This file holds the Editor key dispatch (processKeys) and the buffer
// editing primitives it drives: insertRune, cursor motion (moveCursor /
// moveWord), and the delete family (deleteBackward/Forward/Word*/ToLine*).
// All primitives take the write lock, push an undo snapshot, fire onChange,
// and clear stale mouse/Select-All selection state.

import (
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

func (e *Editor) processKeys(data string) {
	keys := terminal.ParseKeys(data)
	if len(keys) == 0 {
		return
	}
	km := e.km
	for _, k := range keys {
		raw := k.Raw
		switch {
		case km.Matches(raw, "tui.input.newLine"):
			e.insertRune('\n')
		case km.Matches(raw, "tui.input.submit"):
			e.submit()
		case km.Matches(raw, "tui.editor.selectAll"):
			e.SelectAll()
		case km.Matches(raw, "tui.editor.cursorLeft"):
			e.moveCursor(0, -1)
		case km.Matches(raw, "tui.editor.cursorRight"):
			e.moveCursor(0, 1)
		case km.Matches(raw, "tui.editor.cursorUp"):
			switch {
			case e.focused && e.row == 0 && e.historyPrev():
			case e.isAutocompleteActive():
				// Autocomplete active: let the SelectList handle up/down
				// for suggestion navigation. Skip both history and cursor move.
			default:
				e.moveCursor(-1, 0)
			}
		case km.Matches(raw, "tui.editor.cursorDown"):
			switch {
			case e.focused && e.row >= int64(len(e.lines)-1) && e.historyNext():
			case e.isAutocompleteActive():
				// Autocomplete active: let the SelectList handle up/down
				// for suggestion navigation. Skip both history and cursor move.
			default:
				e.moveCursor(1, 0)
			}
		case km.Matches(raw, "tui.editor.cursorWordLeft"):
			e.moveWord(-1)
		case km.Matches(raw, "tui.editor.cursorWordRight"):
			e.moveWord(1)
		case km.Matches(raw, "tui.editor.cursorLineStart"):
			e.mu.Lock()
			e.allSelected = false
			e.clearMouseSelectionLocked()
			e.col = 0
			e.mu.Unlock()
		case km.Matches(raw, "tui.editor.cursorLineEnd"):
			e.mu.Lock()
			e.allSelected = false
			e.clearMouseSelectionLocked()
			e.col = int64(len(e.lines[e.row]))
			e.mu.Unlock()
		case km.Matches(raw, "tui.editor.deleteCharBackward"):
			e.deleteBackward()
		case km.Matches(raw, "tui.editor.deleteCharForward"):
			e.deleteForward()
		case km.Matches(raw, "tui.editor.deleteWordBackward"):
			e.deleteWordBackward()
		case km.Matches(raw, "tui.editor.deleteWordForward"):
			e.deleteWordForward()
		case km.Matches(raw, "tui.editor.deleteToLineStart"):
			e.deleteToLineStart()
		case km.Matches(raw, "tui.editor.deleteToLineEnd"):
			e.deleteToLineEnd()
		case km.Matches(raw, "tui.editor.yank"):
			e.yank()
		case km.Matches(raw, "tui.editor.yankPop"):
			e.yankPop()
		case km.Matches(raw, "tui.editor.undo"):
			e.undo()
		case km.Matches(raw, "ctrl+shift+z"), km.Matches(raw, "ctrl+y"):
			e.redo()
		default:
			if k.Rune == '\n' || k.Rune == '\r' {
				continue
			}
			if k.IsPrintable() {
				e.insertRune(k.Rune)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Editing
// ---------------------------------------------------------------------------

func (e *Editor) insertRune(r rune) {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	e.pushSnapshotLocked()
	if e.allSelected {
		e.clearSelectionContentLocked()
	}
	if r == '\n' {
		cur := e.lines[e.row]
		before := append([]rune{}, cur[:e.col]...)
		after := append([]rune{}, cur[e.col:]...)
		e.lines[e.row] = before
		newLines := make([][]rune, 0, len(e.lines)+1)
		newLines = append(newLines, e.lines[:e.row+1]...)
		newLines = append(newLines, after)
		newLines = append(newLines, e.lines[e.row+1:]...)
		e.lines = newLines
		e.row++
		e.col = 0
	} else {
		cur := e.lines[e.row]
		newLine := make([]rune, 0, len(cur)+1)
		newLine = append(newLine, cur[:e.col]...)
		newLine = append(newLine, r)
		newLine = append(newLine, cur[e.col:]...)
		e.lines[e.row] = newLine
		e.col++
	}
	e.lastKill = false
	e.allSelected = false
	fn := e.onChange
	v := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (e *Editor) moveCursor(dRow, dCol int64) {
	e.mu.Lock()
	e.allSelected = false
	e.clearMouseSelectionLocked()
	if dRow != 0 {
		e.row += dRow
		if e.row < 0 {
			e.row = 0
		}
		if e.row >= int64(len(e.lines)) {
			e.row = int64(len(e.lines) - 1)
		}
		if e.col > int64(len(e.lines[e.row])) {
			e.col = int64(len(e.lines[e.row]))
		}
	}
	if dCol != 0 {
		e.col += dCol
		if e.col < 0 {
			if e.row > 0 {
				e.row--
				e.col = int64(len(e.lines[e.row]))
			} else {
				e.col = 0
			}
		}
		if e.col > int64(len(e.lines[e.row])) {
			if e.row < int64(len(e.lines)-1) {
				e.row++
				e.col = 0
			} else {
				e.col = int64(len(e.lines[e.row]))
			}
		}
	}
	e.lastKill = false
	e.mu.Unlock()
}

func (e *Editor) moveWord(delta int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.allSelected = false
	e.clearMouseSelectionLocked()
	if delta < 0 {
		if e.col == 0 && e.row > 0 {
			e.row--
			e.col = int64(len(e.lines[e.row]))
			return
		}
		e.col = core.FindWordBoundaryLeft(e.lines[e.row], e.col)
	} else {
		if e.col == int64(len(e.lines[e.row])) && e.row < int64(len(e.lines)-1) {
			e.row++
			e.col = 0
			return
		}
		e.col = core.FindWordBoundaryRight(e.lines[e.row], e.col)
	}
	e.lastKill = false
}

func (e *Editor) deleteBackward() {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	e.pushSnapshotLocked()
	if e.allSelected {
		e.clearSelectionContentLocked()
		fn := e.onChange
		v := e.valueLocked()
		e.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	if e.col == 0 {
		if e.row == 0 {
			e.mu.Unlock()
			return
		}
		prev := e.lines[e.row-1]
		cur := e.lines[e.row]
		e.col = int64(len(prev))
		e.lines[e.row-1] = append(prev, cur...)
		e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
		e.row--
	} else {
		cur := e.lines[e.row]
		e.lines[e.row] = append(cur[:e.col-1], cur[e.col:]...)
		e.col--
	}
	fn := e.onChange
	v := e.valueLocked()
	e.lastKill = false
	e.allSelected = false
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (e *Editor) deleteForward() {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	e.pushSnapshotLocked()
	if e.allSelected {
		e.clearSelectionContentLocked()
		fn := e.onChange
		v := e.valueLocked()
		e.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	cur := e.lines[e.row]
	if e.col >= int64(len(cur)) {
		if e.row >= int64(len(e.lines)-1) {
			e.mu.Unlock()
			return
		}
		next := e.lines[e.row+1]
		e.lines[e.row] = append(cur, next...)
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
	} else {
		e.lines[e.row] = append(cur[:e.col], cur[e.col+1:]...)
	}
	fn := e.onChange
	v := e.valueLocked()
	e.lastKill = false
	e.allSelected = false
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (e *Editor) deleteWordBackward() {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	e.pushSnapshotLocked()
	if e.allSelected {
		e.clearSelectionContentLocked()
		fn := e.onChange
		v := e.valueLocked()
		e.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	if e.col == 0 {
		if e.row == 0 {
			e.mu.Unlock()
			return
		}
		prev := e.lines[e.row-1]
		cur := e.lines[e.row]
		e.col = int64(len(prev))
		e.lines[e.row-1] = append(prev, cur...)
		e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
		e.row--
		e.mu.Unlock()
		return
	}
	start := core.FindWordBoundaryLeft(e.lines[e.row], e.col)
	killed := string(e.lines[e.row][start:e.col])
	e.lines[e.row] = append(e.lines[e.row][:start], e.lines[e.row][e.col:]...)
	e.col = start
	e.pushKillRingLocked(killed)
	fn := e.onChange
	v := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (e *Editor) deleteWordForward() {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	e.pushSnapshotLocked()
	if e.allSelected {
		e.clearSelectionContentLocked()
		fn := e.onChange
		v := e.valueLocked()
		e.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	cur := e.lines[e.row]
	if e.col >= int64(len(cur)) {
		if e.row >= int64(len(e.lines)-1) {
			e.mu.Unlock()
			return
		}
		next := e.lines[e.row+1]
		e.lines[e.row] = append(cur, next...)
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
		e.mu.Unlock()
		return
	}
	end := core.FindWordBoundaryRight(cur, e.col)
	killed := string(cur[e.col:end])
	e.lines[e.row] = append(cur[:e.col], cur[end:]...)
	e.pushKillRingLocked(killed)
	fn := e.onChange
	v := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (e *Editor) deleteToLineStart() {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	e.pushSnapshotLocked()
	if e.allSelected {
		e.clearSelectionContentLocked()
		fn := e.onChange
		v := e.valueLocked()
		e.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	cur := e.lines[e.row]
	if e.col == 0 {
		e.mu.Unlock()
		return
	}
	killed := string(cur[:e.col])
	e.lines[e.row] = cur[e.col:]
	e.col = 0
	e.pushKillRingLocked(killed)
	fn := e.onChange
	v := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (e *Editor) deleteToLineEnd() {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	e.pushSnapshotLocked()
	if e.allSelected {
		e.clearSelectionContentLocked()
		fn := e.onChange
		v := e.valueLocked()
		e.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	cur := e.lines[e.row]
	if e.col >= int64(len(cur)) {
		// Merge with next line if any.
		if e.row < int64(len(e.lines)-1) {
			next := e.lines[e.row+1]
			e.lines[e.row] = append(cur, next...)
			e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
			e.pushKillRingLocked("\n")
		}
		e.mu.Unlock()
		return
	}
	killed := string(cur[e.col:])
	e.lines[e.row] = cur[:e.col]
	e.pushKillRingLocked(killed)
	fn := e.onChange
	v := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}
