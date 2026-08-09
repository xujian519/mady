package component

import (
	"sync"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// Input — single-line editor component.
//
// Supports:
//   - Printable rune insertion (CJK-aware via runeWidth).
//   - Backspace / Delete / word delete (Ctrl+W, Alt+Backspace, Alt+D).
//   - Cursor movement: arrow keys, Ctrl+A/E, Home/End, word navigation.
//   - Ctrl+U / Ctrl+K (delete to line start / end).
//   - Horizontal scrolling when the value is wider than the viewport.
//   - IME cursor positioning via CURSOR_MARKER.
//   - Submit on Enter (OnSubmit callback).
//   - Kill-ring (Ctrl+Y / Alt+Y).
//   - History walking via OnHistoryPrev / OnHistoryNext (bound to Up/Down).
// ---------------------------------------------------------------------------

// Input is a Focusable single-line editor component.
type Input struct {
	mu sync.RWMutex

	runes       []rune
	cursor      int64 // in runes, 0 ≤ cursor ≤ len(runes)
	scroll      int64 // horizontal scroll offset in cells
	allSelected bool
	prompt      string
	promptFn    func(string) string
	placeFn     func(string) string
	textFn      func(string) string
	focused     bool
	paddingX    int64
	placeText   string

	selectedBg string // ANSI background sequence for text selection (set from theme)

	killRing   []string
	killIndex  int64
	lastKillOp bool // whether the last op added to the kill ring (for yank-pop)

	km *terminal.KeybindingsManager

	onSubmit      func(value string)
	onChange      func(value string)
	onHistoryPrev func() (value string, ok bool)
	onHistoryNext func() (value string, ok bool)
}

// NewInput creates a single-line Input bound to the given keybindings
// manager. Pass nil to use the global manager.
func NewInput(km *terminal.KeybindingsManager) *Input {
	if km == nil {
		km = terminal.GetGlobalKeybindings()
	}
	return &Input{
		prompt: "> ",
		km:     km,
	}
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// SetPrompt sets the visible prompt string (default "> ").
func (i *Input) SetPrompt(s string) { i.mu.Lock(); i.prompt = s; i.mu.Unlock() }

// SetPromptFn installs an optional style function for the prompt.
func (i *Input) SetPromptFn(fn func(string) string) { i.mu.Lock(); i.promptFn = fn; i.mu.Unlock() }

// SetTextFn installs an optional style function for the typed text.
func (i *Input) SetTextFn(fn func(string) string) { i.mu.Lock(); i.textFn = fn; i.mu.Unlock() }

// SetSelectedBg sets the ANSI background-color escape sequence used for
// text selection highlighting. Empty string falls back to default blue.
func (i *Input) SetSelectedBg(bg string) { i.mu.Lock(); i.selectedBg = bg; i.mu.Unlock() }

// SetPlaceholder sets a dim placeholder rendered when value is empty.
func (i *Input) SetPlaceholder(s string) { i.mu.Lock(); i.placeText = s; i.mu.Unlock() }

// SetPlaceholderFn customizes placeholder styling.
func (i *Input) SetPlaceholderFn(fn func(string) string) {
	i.mu.Lock()
	i.placeFn = fn
	i.mu.Unlock()
}

// SetPaddingX sets horizontal padding applied before the prompt.
func (i *Input) SetPaddingX(x int64) {
	if x < 0 {
		x = 0
	}
	i.mu.Lock()
	i.paddingX = x
	i.mu.Unlock()
}

// SetValue overwrites the buffer and places the cursor at the end.
func (i *Input) SetValue(s string) {
	i.mu.Lock()
	i.runes = []rune(s)
	i.cursor = int64(len(i.runes))
	i.scroll = 0
	i.allSelected = false
	changeFn := i.onChange
	i.mu.Unlock()
	if changeFn != nil {
		changeFn(s)
	}
}

// GetValue returns the current buffer contents.
func (i *Input) GetValue() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return string(i.runes)
}

// Clear empties the buffer.
func (i *Input) Clear() { i.SetValue("") }

// SelectAll selects the input value without affecting terminal-level text selection.
func (i *Input) SelectAll() {
	i.mu.Lock()
	i.allSelected = len(i.runes) > 0
	i.cursor = int64(len(i.runes))
	i.mu.Unlock()
}

func (i *Input) GetSelectedText() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if !i.allSelected {
		return ""
	}
	return string(i.runes)
}

func (i *Input) ClearSelection() {
	i.mu.Lock()
	i.allSelected = false
	i.mu.Unlock()
}

// OnSubmit registers the callback fired on Enter.
func (i *Input) OnSubmit(fn func(string)) { i.mu.Lock(); i.onSubmit = fn; i.mu.Unlock() }

// OnChange registers the callback fired whenever the buffer changes.
func (i *Input) OnChange(fn func(string)) { i.mu.Lock(); i.onChange = fn; i.mu.Unlock() }

// OnHistoryPrev binds an Up-arrow handler returning the previous value (ok=false to no-op).
func (i *Input) OnHistoryPrev(fn func() (string, bool)) {
	i.mu.Lock()
	i.onHistoryPrev = fn
	i.mu.Unlock()
}

// OnHistoryNext binds a Down-arrow handler returning the next value.
func (i *Input) OnHistoryNext(fn func() (string, bool)) {
	i.mu.Lock()
	i.onHistoryNext = fn
	i.mu.Unlock()
}

// SetFocused marks focus state.
func (i *Input) SetFocused(on bool) { i.mu.Lock(); i.focused = on; i.mu.Unlock() }

// IsFocused reports focus state.
func (i *Input) IsFocused() bool { i.mu.RLock(); defer i.mu.RUnlock(); return i.focused }

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// Render emits a single line: [padX][prompt][scrolled text][cursor marker].
func (i *Input) Render(width int64) []string {
	i.mu.Lock()
	defer i.mu.Unlock()

	pad := repeatSpace(i.paddingX)
	rawPrompt := i.prompt
	prompt := rawPrompt
	if i.promptFn != nil {
		prompt = i.promptFn(prompt)
	}
	promptW := core.VisibleWidth(rawPrompt)
	avail := width - i.paddingX - promptW
	if avail < 1 {
		avail = 1
	}

	var body string
	valueEmpty := len(i.runes) == 0

	if valueEmpty && !i.focused {
		placeholder := i.placeText
		if i.placeFn != nil {
			placeholder = i.placeFn(placeholder)
		} else {
			placeholder = theme.CurrentPalette().Dim.Render(placeholder)
		}
		body = core.PadToWidth(core.TruncateToWidth(placeholder, avail, "…"), avail)
	} else {
		cursorCol := core.CellWidthOfRunes(i.runes, 0, i.cursor)
		i.scroll = core.AdjustHorizontalScroll(i.scroll, cursorCol, avail)

		visible := core.SliceRunesByCells(i.runes, i.scroll, i.scroll+avail)
		displayed := visible.Text
		if i.textFn != nil {
			displayed = i.textFn(displayed)
		}
		if i.allSelected && visible.Text != "" {
			displayed = i.selBg() + core.StripAnsi(displayed) + "\x1b[0m"
		}
		body = core.PadToWidth(displayed, avail)

		if i.focused {
			cursorLocal := cursorCol - i.scroll
			if cursorLocal < 0 {
				cursorLocal = 0
			}
			if cursorLocal > avail {
				cursorLocal = avail
			}
			body = core.InsertMarker(body, cursorLocal)
		}
	}

	line := pad + prompt + body
	return []string{line}
}

// selBg returns the ANSI background-color sequence for selected text.
// Uses the explicit override if set, otherwise falls back to the theme's
// SelectionBg style. If neither is available, uses a plain blue default.
func (i *Input) selBg() string {
	if i.selectedBg != "" {
		return i.selectedBg
	}
	if bg := theme.CurrentPalette().SelectionBg.Render(""); bg != "" {
		return bg
	}
	return "\x1b[44m" // plain blue fallback
}

// Invalidate is a no-op for Input (no cache).
func (i *Input) Invalidate() {}

func (i *Input) Update(msg core.Msg) core.Cmd {
	switch m := msg.(type) {
	case core.KeyMsg:
		i.processKeys(m.Data, m.KittyFlags)
	case core.PasteMsg:
		// Insert pasted text verbatim — do NOT route it through the key
		// parser: a multi-line paste would translate every '\n'/'\r' into
		// an Enter keypress and unexpectedly submit the form (P2-22).
		i.insertPaste(m.Text)
	case core.WindowSizeMsg:
		i.Invalidate()
	}
	return nil
}
