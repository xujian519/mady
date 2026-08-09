package component

// This file holds the Editor key dispatch (processKeys) and the buffer
// editing primitives it drives: insertRune, cursor motion (moveCursor /
// moveWord), and the delete family (deleteBackward/Forward/Word*/ToLine*).
// All primitives take the write lock, push an undo snapshot, fire onChange,
// and clear stale mouse/Select-All selection state.

import (
	"strings"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

func (e *Editor) processKeys(data string, kittyFlags int64) {
	keys := terminal.ParseKeys(data, kittyFlags)
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
			case e.isAutocompleteActive():
				// Autocomplete active: let the SelectList handle up/down
				// for suggestion navigation. Skip both history and cursor move.
			case e.focused && e.row == 0 && e.historyPrev():
			default:
				e.moveCursor(-1, 0)
			}
		case km.Matches(raw, "tui.editor.cursorDown"):
			switch {
			case e.isAutocompleteActive():
				// Autocomplete active: let the SelectList handle up/down
				// for suggestion navigation. Skip both history and cursor move.
			case e.focused && e.row >= int64(len(e.lines)-1) && e.historyNext():
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
		case km.Matches(raw, "tui.editor.redo"):
			// Matches a registered binding id (not a literal key string —
			// literal strings are never present in the resolved binding
			// table, so the old `Matches(raw, "ctrl+shift+z")` was a
			// runtime-dead branch and redo was unreachable from the
			// keyboard, P1-8). Default keys come from keybindings.go
			// "tui.editor.redo" (ctrl+shift+z). ctrl+y remains the yank
			// key ("tui.editor.yank") — redo must not shadow it.
			e.redo()
		case km.Matches(raw, "tui.input.copy"):
			e.handleCopy()
		case km.Matches(raw, "tui.input.paste"):
			e.handlePaste()
		case km.Matches(raw, "tui.input.tab"):
			// Tab while autocomplete is active: let the autocomplete handle it
			// (either apply the suggestion or dismiss). Don't insert a literal
			// tab character into the buffer.
			if e.isAutocompleteActive() {
				continue
			}
			e.insertRune(k.Rune)
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
		// Relocate chips below the split point one row down, and chips that
		// start at/after the cursor column into the new lower line. Runs
		// exactly once against the pre-increment e.row: a second identical
		// pass after e.row++ would double-shift every chip below the cursor.
		for i := range e.chips.chips {
			cp := &e.chips.chips[i]
			if cp.HardRow > e.row {
				cp.HardRow++
			} else if cp.HardRow == e.row && cp.RuneStart >= e.col {
				cp.HardRow = e.row + 1
				cp.RuneStart -= e.col
				cp.RuneEnd = cp.RuneStart
			}
		}
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
		e.chips.ShiftAfter(e.row, e.col-1, 1)
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
		adjustChipsLineMerge(e.chips, e.row, int64(len(prev)))
		e.row--
	} else {
		cur := e.lines[e.row]
		e.lines[e.row] = append(cur[:e.col-1], cur[e.col:]...)
		e.col--
		e.chips.ShiftAfter(e.row, e.col, -1)
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
		curLen := int64(len(cur))
		e.lines[e.row] = append(cur, next...)
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
		adjustChipsLineMerge(e.chips, e.row+1, curLen)
	} else {
		e.lines[e.row] = append(cur[:e.col], cur[e.col+1:]...)
		e.chips.ShiftAfter(e.row, e.col+1, -1)
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
		prevLen := int64(len(prev))
		e.col = prevLen
		e.lines[e.row-1] = append(prev, cur...)
		e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
		adjustChipsLineMerge(e.chips, e.row, prevLen)
		e.row--
		// Fall through to the shared exit below so onChange fires for the
		// line-merge too (P1-7). Merging does not push the kill ring — the
		// deleted content is a newline join, not a word.
	} else {
		start := core.FindWordBoundaryLeft(e.lines[e.row], e.col)
		killed := string(e.lines[e.row][start:e.col])
		e.lines[e.row] = append(e.lines[e.row][:start], e.lines[e.row][e.col:]...)
		e.col = start
		e.pushKillRingLocked(killed)
	}
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
		curLen := int64(len(cur))
		e.lines[e.row] = append(cur, next...)
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
		adjustChipsLineMerge(e.chips, e.row+1, curLen)
		// Fall through to the shared exit below so onChange fires for the
		// line-merge too (P1-7). Merging does not push the kill ring.
	} else {
		end := core.FindWordBoundaryRight(cur, e.col)
		killed := string(cur[e.col:end])
		e.lines[e.row] = append(cur[:e.col], cur[end:]...)
		e.pushKillRingLocked(killed)
	}
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

func (e *Editor) handleCopy() {
	e.mu.RLock()
	onCopy := e.onCopy
	selected := ""
	if e.allSelected {
		selected = e.valueLocked()
	} else if start, end, ok := e.normalizedSelectionLocked(); ok {
		var b strings.Builder
		for r := start.row; r <= end.row; r++ {
			if r >= int64(len(e.lines)) {
				break
			}
			line := e.lines[r]
			lo, hi := int64(0), int64(len(line))
			if r == start.row {
				lo = start.col
			}
			if r == end.row {
				hi = end.col
			}
			if lo < 0 {
				lo = 0
			}
			if hi > int64(len(line)) {
				hi = int64(len(line))
			}
			if lo < hi {
				b.WriteString(string(line[lo:hi]))
			}
			if r != end.row {
				b.WriteByte('\n')
			}
		}
		selected = b.String()
	}
	e.mu.RUnlock()
	if onCopy != nil && selected != "" {
		onCopy(selected)
	}
}

func (e *Editor) handlePaste() {
	e.mu.RLock()
	fn := e.onPaste
	e.mu.RUnlock()
	if fn == nil {
		return
	}
	// onPaste returns a core.Cmd that reads clipboard in a goroutine.
	// Store it as pastePendingCmd; Editor.Update will return it to the TUI
	// event loop for async execution. The Cmd returns core.PasteMsg{Text}
	// which flows back through Update → insertText for batch insertion.
	cmd := fn()
	if cmd != nil {
		e.mu.Lock()
		e.pastePendingCmd = cmd
		e.mu.Unlock()
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
		if e.row >= int64(len(e.lines)-1) {
			e.mu.Unlock()
			return
		}
		next := e.lines[e.row+1]
		curLen := int64(len(cur))
		e.lines[e.row] = append(cur, next...)
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
		adjustChipsLineMerge(e.chips, e.row+1, curLen)
		e.pushKillRingLocked("\n")
		// Fall through to the shared exit below so onChange fires for the
		// line-merge too (P1-7).
	} else {
		killed := string(cur[e.col:])
		e.lines[e.row] = cur[:e.col]
		e.pushKillRingLocked(killed)
	}
	fn := e.onChange
	v := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

// insertText inserts a block of text into the editor buffer at the cursor
// position, handling newlines. It pushes a single undo snapshot and fires
// onChange once, making it suitable for large pastes where per-rune insertRune
// would trigger O(n) snapshots and callbacks.
func (e *Editor) insertText(text string) {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	e.pushSnapshotLocked()
	if e.allSelected {
		e.clearSelectionContentLocked()
	}

	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		e.mu.Unlock()
		return
	}

	// First line is appended to the current row at cursor.
	cur := e.lines[e.row]
	prefix := make([]rune, e.col)
	copy(prefix, cur[:e.col])
	suffix := make([]rune, len(cur)-int(e.col))
	copy(suffix, cur[e.col:])

	prefix = append(prefix, []rune(lines[0])...)
	e.lines[e.row] = prefix

	// Middle lines create new rows inserted after current row.
	for i := 1; i < len(lines); i++ {
		row := []rune(lines[i])
		// Grow slice and shift elements right to make room.
		e.lines = append(e.lines, nil)
		copy(e.lines[e.row+2:], e.lines[e.row+1:])
		e.lines[e.row+1] = row
		e.row++
	}

	// Append the original suffix (content after cursor) to the last row.
	if len(lines) > 1 {
		e.lines[e.row] = append(e.lines[e.row], suffix...)
		e.col = int64(len(e.lines[e.row]) - len(suffix))
	} else {
		e.lines[e.row] = append(prefix, suffix...)
		e.col = int64(len(prefix))
	}

	// Adjust chip positions for single-line paste.
	if len(lines) == 1 {
		e.chips.ShiftAfter(e.row, e.col-int64(len([]rune(lines[0]))), int64(len([]rune(lines[0]))))
	} else {
		// Multi-line paste: chips after cursor move to the last inserted row.
		insertedLines := int64(len(lines) - 1)
		lastLineLen := int64(len([]rune(lines[len(lines)-1])))
		origRow := e.row - insertedLines
		origCol := e.col - lastLineLen
		for i := range e.chips.chips {
			cp := &e.chips.chips[i]
			if cp.HardRow == origRow {
				if cp.RuneStart >= origCol {
					cp.HardRow += insertedLines
					cp.RuneStart = cp.RuneStart - origCol + lastLineLen
					if cp.RuneStart < 0 {
						cp.RuneStart = 0
					}
					cp.RuneEnd = cp.RuneStart
				}
			} else if cp.HardRow > origRow {
				cp.HardRow += insertedLines
			}
		}
	}
	e.allSelected = false
	fn := e.onChange
	v := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}
