package component

import (
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

// ---------------------------------------------------------------------------
// Input editing primitives: key dispatch, cursor motion, the delete
// family, Emacs-style kill-ring (yank/yank-pop), submit and history
// recall. Split out of input.go so the component API and rendering
// stay readable. All methods require the caller to hold i.mu (or are
// safe under it) unless noted otherwise.
// ---------------------------------------------------------------------------

func (i *Input) processKeys(data string, kittyFlags int64) {
	km := i.km

	// Parse multiple events in one feed (e.g. pasted text).
	keys := terminal.ParseKeys(data, kittyFlags)
	if len(keys) == 0 {
		return
	}

	for _, k := range keys {
		raw := k.Raw

		switch {
		case km.Matches(raw, "tui.input.submit"):
			i.submit()
		case km.Matches(raw, "tui.editor.selectAll"):
			i.SelectAll()
		case km.Matches(raw, "tui.editor.cursorLeft"):
			i.moveCursor(-1)
		case km.Matches(raw, "tui.editor.cursorRight"):
			i.moveCursor(1)
		case km.Matches(raw, "tui.editor.cursorWordLeft"):
			i.moveCursorWord(-1)
		case km.Matches(raw, "tui.editor.cursorWordRight"):
			i.moveCursorWord(1)
		case km.Matches(raw, "tui.editor.cursorLineStart"):
			i.mu.Lock()
			i.allSelected = false
			i.cursor = 0
			i.mu.Unlock()
		case km.Matches(raw, "tui.editor.cursorLineEnd"):
			i.mu.Lock()
			i.allSelected = false
			i.cursor = int64(len(i.runes))
			i.mu.Unlock()
		case km.Matches(raw, "tui.editor.deleteCharBackward"):
			i.deleteBackward(1)
		case km.Matches(raw, "tui.editor.deleteCharForward"):
			i.deleteForward(1)
		case km.Matches(raw, "tui.editor.deleteWordBackward"):
			i.deleteWordBackward()
		case km.Matches(raw, "tui.editor.deleteWordForward"):
			i.deleteWordForward()
		case km.Matches(raw, "tui.editor.deleteToLineStart"):
			i.deleteToLineStart()
		case km.Matches(raw, "tui.editor.deleteToLineEnd"):
			i.deleteToLineEnd()
		case km.Matches(raw, "tui.editor.yank"):
			i.yank()
		case km.Matches(raw, "tui.editor.yankPop"):
			i.yankPop()
		case km.Matches(raw, "tui.select.up"), km.Matches(raw, "tui.editor.cursorUp"):
			i.historyPrev()
		case km.Matches(raw, "tui.select.down"), km.Matches(raw, "tui.editor.cursorDown"):
			i.historyNext()
		default:
			if k.IsPrintable() {
				i.insertRune(k.Rune)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Editing operations
// ---------------------------------------------------------------------------

func (i *Input) insertRune(r rune) {
	i.mu.Lock()
	if i.allSelected {
		i.clearSelectionContentLocked()
	}
	before := i.runes[:i.cursor]
	after := i.runes[i.cursor:]
	newRunes := make([]rune, 0, len(i.runes)+1)
	newRunes = append(newRunes, before...)
	newRunes = append(newRunes, r)
	newRunes = append(newRunes, after...)
	i.runes = newRunes
	i.cursor++
	i.lastKillOp = false
	i.allSelected = false
	value := string(i.runes)
	changeFn := i.onChange
	i.mu.Unlock()
	if changeFn != nil {
		changeFn(value)
	}
}

func (i *Input) moveCursor(delta int64) {
	i.mu.Lock()
	i.allSelected = false
	i.cursor += delta
	if i.cursor < 0 {
		i.cursor = 0
	}
	if i.cursor > int64(len(i.runes)) {
		i.cursor = int64(len(i.runes))
	}
	i.lastKillOp = false
	i.mu.Unlock()
}

func (i *Input) moveCursorWord(delta int64) {
	i.mu.Lock()
	i.allSelected = false
	if delta < 0 {
		i.cursor = core.FindWordBoundaryLeft(i.runes, i.cursor)
	} else {
		i.cursor = core.FindWordBoundaryRight(i.runes, i.cursor)
	}
	i.lastKillOp = false
	i.mu.Unlock()
}

func (i *Input) deleteBackward(n int64) {
	i.mu.Lock()
	if i.allSelected {
		i.clearSelectionContentLocked()
		v := string(i.runes)
		fn := i.onChange
		i.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	if i.cursor <= 0 {
		i.mu.Unlock()
		return
	}
	start := i.cursor - n
	if start < 0 {
		start = 0
	}
	i.runes = append(i.runes[:start], i.runes[i.cursor:]...)
	i.cursor = start
	i.lastKillOp = false
	i.allSelected = false
	v := string(i.runes)
	fn := i.onChange
	i.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (i *Input) deleteForward(n int64) {
	i.mu.Lock()
	if i.allSelected {
		i.clearSelectionContentLocked()
		v := string(i.runes)
		fn := i.onChange
		i.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	end := i.cursor + n
	if end > int64(len(i.runes)) {
		end = int64(len(i.runes))
	}
	if end <= i.cursor {
		i.mu.Unlock()
		return
	}
	i.runes = append(i.runes[:i.cursor], i.runes[end:]...)
	i.lastKillOp = false
	i.allSelected = false
	v := string(i.runes)
	fn := i.onChange
	i.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (i *Input) deleteWordBackward() {
	i.mu.Lock()
	if i.allSelected {
		i.clearSelectionContentLocked()
		v := string(i.runes)
		fn := i.onChange
		i.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	start := core.FindWordBoundaryLeft(i.runes, i.cursor)
	if start >= i.cursor {
		i.mu.Unlock()
		return
	}
	killed := string(i.runes[start:i.cursor])
	i.runes = append(i.runes[:start], i.runes[i.cursor:]...)
	i.cursor = start
	i.pushKillRing(killed)
	v := string(i.runes)
	fn := i.onChange
	i.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (i *Input) deleteWordForward() {
	i.mu.Lock()
	if i.allSelected {
		i.clearSelectionContentLocked()
		v := string(i.runes)
		fn := i.onChange
		i.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	end := core.FindWordBoundaryRight(i.runes, i.cursor)
	if end <= i.cursor {
		i.mu.Unlock()
		return
	}
	killed := string(i.runes[i.cursor:end])
	i.runes = append(i.runes[:i.cursor], i.runes[end:]...)
	i.pushKillRing(killed)
	v := string(i.runes)
	fn := i.onChange
	i.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (i *Input) deleteToLineStart() {
	i.mu.Lock()
	if i.allSelected {
		i.clearSelectionContentLocked()
		v := string(i.runes)
		fn := i.onChange
		i.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	if i.cursor == 0 {
		i.mu.Unlock()
		return
	}
	killed := string(i.runes[:i.cursor])
	i.runes = i.runes[i.cursor:]
	i.cursor = 0
	i.pushKillRing(killed)
	v := string(i.runes)
	fn := i.onChange
	i.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (i *Input) deleteToLineEnd() {
	i.mu.Lock()
	if i.allSelected {
		i.clearSelectionContentLocked()
		v := string(i.runes)
		fn := i.onChange
		i.mu.Unlock()
		if fn != nil {
			fn(v)
		}
		return
	}
	if i.cursor >= int64(len(i.runes)) {
		i.mu.Unlock()
		return
	}
	killed := string(i.runes[i.cursor:])
	i.runes = i.runes[:i.cursor]
	i.pushKillRing(killed)
	v := string(i.runes)
	fn := i.onChange
	i.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (i *Input) pushKillRing(s string) {
	if s == "" {
		return
	}
	i.killRing = append(i.killRing, s)
	if len(i.killRing) > 32 {
		i.killRing = i.killRing[len(i.killRing)-32:]
	}
	i.killIndex = int64(len(i.killRing) - 1)
	i.lastKillOp = true
}

func (i *Input) yank() {
	i.mu.Lock()
	if len(i.killRing) == 0 {
		i.mu.Unlock()
		return
	}
	if i.allSelected {
		i.clearSelectionContentLocked()
	}
	text := i.killRing[i.killIndex]
	i.insertStringLocked(text)
	i.lastKillOp = true
	i.allSelected = false
	v := string(i.runes)
	fn := i.onChange
	i.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (i *Input) yankPop() {
	i.mu.Lock()
	if !i.lastKillOp || len(i.killRing) == 0 {
		i.mu.Unlock()
		return
	}
	// Remove the previously yanked text and insert the ring's previous entry.
	prevText := i.killRing[i.killIndex]
	prevLen := int64(len([]rune(prevText)))
	if i.cursor >= prevLen {
		i.runes = append(i.runes[:i.cursor-prevLen], i.runes[i.cursor:]...)
		i.cursor -= prevLen
	}
	i.killIndex--
	if i.killIndex < 0 {
		i.killIndex = int64(len(i.killRing) - 1)
	}
	text := i.killRing[i.killIndex]
	i.insertStringLocked(text)
	v := string(i.runes)
	fn := i.onChange
	i.mu.Unlock()
	if fn != nil {
		fn(v)
	}
}

func (i *Input) insertStringLocked(s string) {
	if i.allSelected {
		i.clearSelectionContentLocked()
	}
	rs := []rune(s)
	before := i.runes[:i.cursor]
	after := i.runes[i.cursor:]
	newRunes := make([]rune, 0, len(i.runes)+len(rs))
	newRunes = append(newRunes, before...)
	newRunes = append(newRunes, rs...)
	newRunes = append(newRunes, after...)
	i.runes = newRunes
	i.cursor += int64(len(rs))
	i.allSelected = false
}

func (i *Input) clearSelectionContentLocked() {
	i.runes = nil
	i.cursor = 0
	i.scroll = 0
	i.allSelected = false
	i.lastKillOp = false
}

func (i *Input) submit() {
	i.mu.Lock()
	val := string(i.runes)
	fn := i.onSubmit
	i.mu.Unlock()
	if fn != nil {
		fn(val)
	}
}

func (i *Input) historyPrev() {
	i.mu.RLock()
	fn := i.onHistoryPrev
	i.mu.RUnlock()
	if fn == nil {
		return
	}
	if v, ok := fn(); ok {
		i.SetValue(v)
	}
}

func (i *Input) historyNext() {
	i.mu.RLock()
	fn := i.onHistoryNext
	i.mu.RUnlock()
	if fn == nil {
		return
	}
	if v, ok := fn(); ok {
		i.SetValue(v)
	}
}
