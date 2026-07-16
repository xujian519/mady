package component

// This file implements the Editor kill-ring (Emacs-style clipboard internal
// to the editor): pushKillRingLocked records killed text, yank pastes the
// most recent entry, yankPop cycles to an older entry. It also holds the
// low-level insert/delete-before-cursor helpers they share, plus
// clearSelectionContentLocked (used by every destructive op when Select All
// is active).

func (e *Editor) pushKillRingLocked(s string) {
	if s == "" {
		return
	}
	e.killRing = append(e.killRing, s)
	if len(e.killRing) > 64 {
		e.killRing = e.killRing[len(e.killRing)-64:]
	}
	e.killIndex = int64(len(e.killRing) - 1)
	e.lastKill = true
}

func (e *Editor) yank() {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	if len(e.killRing) == 0 {
		e.mu.Unlock()
		return
	}
	e.pushSnapshotLocked()
	text := e.killRing[e.killIndex]
	e.insertStringLocked(text)
	e.lastKill = true
	fn := e.onChange
	v := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (e *Editor) yankPop() {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	if !e.lastKill || len(e.killRing) == 0 {
		e.mu.Unlock()
		return
	}
	e.pushSnapshotLocked()
	prev := e.killRing[e.killIndex]
	e.removeBeforeCursorLocked(int64(len([]rune(prev))))
	e.killIndex--
	if e.killIndex < 0 {
		e.killIndex = int64(len(e.killRing) - 1)
	}
	e.insertStringLocked(e.killRing[e.killIndex])
	fn := e.onChange
	v := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (e *Editor) insertStringLocked(s string) {
	if e.allSelected {
		e.clearSelectionContentLocked()
	}
	for _, r := range s {
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
			continue
		}
		cur := e.lines[e.row]
		newLine := make([]rune, 0, len(cur)+1)
		newLine = append(newLine, cur[:e.col]...)
		newLine = append(newLine, r)
		newLine = append(newLine, cur[e.col:]...)
		e.lines[e.row] = newLine
		e.col++
	}
	e.allSelected = false
}

func (e *Editor) clearSelectionContentLocked() {
	e.lines = [][]rune{{}}
	e.row = 0
	e.col = 0
	e.allSelected = false
	e.lastKill = false
}

func (e *Editor) removeBeforeCursorLocked(n int64) {
	// Remove n runes immediately before the cursor (may span line breaks).
	for n > 0 {
		if e.col >= n {
			cur := e.lines[e.row]
			e.lines[e.row] = append(cur[:e.col-n], cur[e.col:]...)
			e.col -= n
			return
		}
		n -= e.col
		if e.row == 0 {
			e.lines[e.row] = e.lines[e.row][e.col:]
			e.col = 0
			return
		}
		// Merge with previous line.
		prev := e.lines[e.row-1]
		cur := e.lines[e.row][e.col:]
		e.col = int64(len(prev))
		e.lines[e.row-1] = append(prev, cur...)
		e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
		e.row--
		n-- // the newline itself counted as one rune
	}
}
