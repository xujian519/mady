package component

// This file defines the Editor type, its constructor, configuration setters,
// the value/selection public API (SetValue/GetValue/SelectAll/
// GetSelectedText/ClearSelection), the Focusable implementation, and the
// Update entry point that routes Msgs to the key processor / mouse handler.
//
// Behavioral code is split across sibling files:
//   - editor_render.go   — Render, handleMouse, hitTest, selection helpers
//   - editor_edit.go     — processKeys + buffer editing primitives
//   - editor_killring.go — kill-ring (yank/yankPop) + insert/delete helpers
//   - editor_history.go  — undo/redo stack + input recall history

import (
	"strings"
	"sync"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

// ---------------------------------------------------------------------------
// Editor — multi-line text editor component.
//
// Features:
//   - Multi-line buffer, hard newlines via Shift+Enter / Alt+Enter.
//   - Enter submits (OnSubmit); Ctrl+J inserts newline regardless.
//   - Emacs-style keybindings for cursor motion and deletion (reuses the
//     tui.editor.* keybindings).
//   - Soft wrap at viewport width (CJK-aware via VisibleWidth).
//   - Kill-ring (Ctrl+Y / Alt+Y).
//   - Undo/redo stack (Ctrl+Z / Ctrl+Shift+Z).
//   - Auto-growing height with MinRows / MaxRows caps.
//   - CURSOR_MARKER emitted so the TUI can drive the hardware cursor for IME.
// ---------------------------------------------------------------------------

// Editor is a Focusable multi-line editor component.
type Editor struct {
	mu sync.RWMutex

	lines [][]rune // one entry per hard line
	row   int64    // 0-based cursor row
	col   int64    // 0-based cursor rune col within the row

	allSelected bool // editor-scoped selection created by Select All

	// Mouse-drag text selection — independent of allSelected/Select All.
	// selStart/selEnd are buffer positions (hard row + rune column);
	// selDragging is true while the mouse button is held down.
	selDragging bool
	selActive   bool
	selStart    editorSelPos
	selEnd      editorSelPos

	// lastVisuals/lastPadX/lastPromptW* cache the layout computed by the
	// most recent Render call, so MouseMsg events (which arrive between
	// renders, with screen-relative row/col) can be translated back into
	// logical buffer positions for hit-testing.
	lastVisuals      []editorVisualRow
	lastPadX         int64
	lastPromptWFirst int64
	lastPromptWCont  int64

	minRows int64
	maxRows int64
	padX    int64

	promptFirst string // prompt on first visual line
	promptCont  string // prompt on continuation lines
	promptFn    func(string) string
	textFn      func(string) string
	placeText   string
	placeFn     func(string) string

	focused        bool
	focusIndicator string
	km             *terminal.KeybindingsManager

	killRing  []string
	killIndex int64
	lastKill  bool

	history    []editorSnapshot
	future     []editorSnapshot
	historyMax int64

	// inputHistory stores submitted values for up/down recall.
	inputHistory      []string
	inputHistoryIndex int64 // -1 = not browsing history, >=0 = index into inputHistory
	inputHistoryMax   int64
	inputHistoryDraft string // buffer saved when entering history browse mode

	onSubmit func(value string)
	onChange func(value string)
	onCancel func()

	// autocompleteActiveCheck, when non-nil, is called before processing up/down
	// keys for input history navigation. When it returns true, history navigation
	// is skipped so the autocomplete's SelectList can handle the key instead.
	autocompleteActiveCheck func() bool
}

type editorSnapshot struct {
	lines [][]rune
	row   int64
	col   int64
}

// editorSelPos identifies a buffer position for mouse-drag selection: row is
// an index into Editor.lines (a "hard" line), col is a rune offset within it.
type editorSelPos struct {
	row int64
	col int64
}

// editorVisualRow records, for one rendered screen row, which hard line it
// came from and the rune range [colStart, colEnd) of that line it displays.
// hardRow is -1 for rows with no backing content (e.g. MinRows growth
// padding beyond the buffer's actual line count).
type editorVisualRow struct {
	hardRow    int64
	colStart   int64
	colEnd     int64
	isFirstSeg bool
}

// NewEditor creates a multi-line editor bound to km (nil = global).
func NewEditor(km *terminal.KeybindingsManager) *Editor {
	if km == nil {
		km = terminal.GetGlobalKeybindings()
	}
	return &Editor{
		lines:             [][]rune{{}},
		minRows:           1,
		maxRows:           10,
		promptFirst:       "> ",
		promptCont:        "  ",
		km:                km,
		historyMax:        200,
		inputHistoryMax:   1000,
		inputHistoryIndex: -1,
	}
}

// SetPrompt sets the first-line and continuation-line prompt strings.
func (e *Editor) SetPrompt(first, cont string) {
	e.mu.Lock()
	e.promptFirst = first
	e.promptCont = cont
	e.mu.Unlock()
}

// SetPromptFn styles the prompt.
func (e *Editor) SetPromptFn(fn func(string) string) { e.mu.Lock(); e.promptFn = fn; e.mu.Unlock() }

// SetTextFn styles the body text.
func (e *Editor) SetTextFn(fn func(string) string) { e.mu.Lock(); e.textFn = fn; e.mu.Unlock() }

// SetPlaceholder sets text shown when empty & unfocused.
func (e *Editor) SetPlaceholder(s string) { e.mu.Lock(); e.placeText = s; e.mu.Unlock() }

// SetPlaceholderFn styles the placeholder.
func (e *Editor) SetPlaceholderFn(fn func(string) string) {
	e.mu.Lock()
	e.placeFn = fn
	e.mu.Unlock()
}

// SetMinRows / SetMaxRows control auto-growth of the component height.
func (e *Editor) SetMinRows(n int64) {
	if n < 1 {
		n = 1
	}
	e.mu.Lock()
	e.minRows = n
	e.mu.Unlock()
}
func (e *Editor) SetMaxRows(n int64) {
	if n < 1 {
		n = 1
	}
	e.mu.Lock()
	e.maxRows = n
	e.mu.Unlock()
}

// SetPaddingX sets left/right padding (cells).
func (e *Editor) SetPaddingX(n int64) {
	if n < 0 {
		n = 0
	}
	e.mu.Lock()
	e.padX = n
	e.mu.Unlock()
}

// SetValue overwrites the buffer and moves the cursor to the end.
func (e *Editor) SetValue(s string) {
	e.mu.Lock()
	e.pushSnapshotLocked()
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
	e.allSelected = false
	e.selActive = false
	e.selDragging = false
	fn := e.onChange
	val := e.valueLocked()
	e.mu.Unlock()
	if fn != nil {
		fn(val)
	}
}

// GetValue returns the current buffer as a string.
func (e *Editor) GetValue() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.valueLocked()
}

func (e *Editor) valueLocked() string {
	segs := make([]string, len(e.lines))
	for i, ln := range e.lines {
		segs[i] = string(ln)
	}
	return strings.Join(segs, "\n")
}

// Clear empties the buffer.
func (e *Editor) Clear() { e.SetValue("") }

// SelectAll selects the editor buffer without affecting terminal-level text selection.
func (e *Editor) SelectAll() {
	e.mu.Lock()
	e.clearMouseSelectionLocked()
	e.allSelected = e.valueLocked() != ""
	e.row = int64(len(e.lines) - 1)
	e.col = int64(len(e.lines[e.row]))
	e.mu.Unlock()
}

func (e *Editor) GetSelectedText() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.allSelected {
		return e.valueLocked()
	}
	start, end, ok := e.normalizedSelectionLocked()
	if !ok || start.row < 0 || end.row >= int64(len(e.lines)) {
		return ""
	}
	var b strings.Builder
	for r := start.row; r <= end.row; r++ {
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
		if hi < lo {
			hi = lo
		}
		b.WriteString(string(line[lo:hi]))
		if r != end.row {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// normalizedSelectionLocked returns the current mouse-drag selection with
// start <= end (row/col order), and whether a non-empty selection exists.
// Must be called with e.mu held (read or write lock).
func (e *Editor) normalizedSelectionLocked() (start, end editorSelPos, ok bool) {
	if !e.selActive || e.selStart == e.selEnd {
		return editorSelPos{}, editorSelPos{}, false
	}
	start, end = e.selStart, e.selEnd
	if start.row > end.row || (start.row == end.row && start.col > end.col) {
		start, end = end, start
	}
	return start, end, true
}

func (e *Editor) ClearSelection() {
	e.mu.Lock()
	e.allSelected = false
	e.selActive = false
	e.selDragging = false
	e.mu.Unlock()
}

// clearMouseSelectionLocked resets mouse-drag selection state. Must be
// called with e.mu already held.
func (e *Editor) clearMouseSelectionLocked() {
	e.selActive = false
	e.selDragging = false
}

// OnSubmit / OnChange / OnCancel register callbacks.
func (e *Editor) OnSubmit(fn func(string)) { e.mu.Lock(); e.onSubmit = fn; e.mu.Unlock() }
func (e *Editor) OnChange(fn func(string)) { e.mu.Lock(); e.onChange = fn; e.mu.Unlock() }
func (e *Editor) OnCancel(fn func())       { e.mu.Lock(); e.onCancel = fn; e.mu.Unlock() }

// SetAutocompleteActiveCheck registers a function that returns whether an
// autocomplete popup is currently active. When active, up/down keys skip
// input-history navigation and pass through so the autocomplete's SelectList
// can handle them instead. Pass nil to clear the check.
func (e *Editor) SetAutocompleteActiveCheck(fn func() bool) {
	e.mu.Lock()
	e.autocompleteActiveCheck = fn
	e.mu.Unlock()
}

// isAutocompleteActive returns true when the registered autocomplete check
// (if any) reports the popup as active. Safe to call from any goroutine.
func (e *Editor) isAutocompleteActive() bool {
	e.mu.RLock()
	fn := e.autocompleteActiveCheck
	e.mu.RUnlock()
	return fn != nil && fn()
}

// SetFocused / IsFocused implement Focusable.
func (e *Editor) SetFocused(on bool) { e.mu.Lock(); e.focused = on; e.mu.Unlock() }
func (e *Editor) IsFocused() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.focused
}

func (e *Editor) SetFocusIndicator(indicator string) {
	e.mu.Lock()
	e.focusIndicator = indicator
	e.mu.Unlock()
}

// Invalidate is a no-op (no cache).
func (e *Editor) Invalidate() {}

func (e *Editor) Update(msg core.Msg) core.Cmd {
	switch m := msg.(type) {
	case core.KeyMsg:
		e.processKeys(m.Data)
	case core.PasteMsg:
		e.processKeys(m.Text)
	case core.WindowSizeMsg:
		e.Invalidate()
	case core.MouseMsg:
		e.handleMouse(m)
	}
	return nil
}
