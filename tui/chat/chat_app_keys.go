package chat

import (
	"strings"
	"time"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// handleKeyMsg dispatches key events for the chat layout.
// Returns true if at least one key was consumed.
func (l *chatLayout) handleKeyMsg(m core.KeyMsg) bool {
	for _, k := range terminal.ParseKeys(m.Data, m.KittyFlags) {
		if l.dispatchKey(k) {
			return true
		}
	}
	return false
}

// dispatchKey handles a single parsed key.
// Returns true if the key was consumed; false to continue iteration.
func (l *chatLayout) dispatchKey(k terminal.Key) bool {
	name := strings.ToLower(k.Name)

	// When search is active, route all printable characters to search.
	if l.history.SearchMode() {
		return l.dispatchSearchKey(k)
	}

	// When an inline confirmation is pending, only y/n/Esc are accepted.
	if l.app != nil && l.app.State() == StateConfirmPending {
		return l.dispatchConfirmKey(k)
	}

	switch name {
	case "f2":
		l.app.ToggleMousePassthrough()
		return true
	case "enter", " ":
		// Space/Enter to toggle fold at viewport center (tool groups,
		// thinking segments). Only when Ctrl is held, to avoid stealing
		// the Enter key from the editor (submit) or Space from the input.
		if k.Mods&terminal.ModCtrl != 0 {
			if l.app != nil && l.app.State() == StateIdle && l.history != nil {
				l.history.ToggleFoldAtViewportCenter()
				if l.app.host != nil {
					l.app.host.RequestRender()
				}
				return true
			}
		}
	case "f":
		// Alt+F to toggle fold at viewport center (no modifier conflict
		// with editor or search). F = "Fold".
		if k.Mods&terminal.ModAlt != 0 {
			if l.app != nil && l.app.State() == StateIdle && l.history != nil {
				l.history.ToggleFoldAtViewportCenter()
				if l.app.host != nil {
					l.app.host.RequestRender()
				}
				return true
			}
		}
	case "v":
		if k.Mods&(terminal.ModCtrl|terminal.ModSuper|terminal.ModMeta) != 0 &&
			k.Mods&terminal.ModAlt != 0 {
			if l.app.cfg.OnImagePaste != nil {
				l.app.cfg.OnImagePaste()
			}
			return true
		}
		// Route regular paste shortcuts (Ctrl+V, Super+V, etc.) through
		// the unified handler so isPasteShortcut can trigger RequestPaste.
		return l.handleCopyOrInterrupt(k, name)
	case "escape":
		return l.handleEscapeKey(k)
	case "pageup":
		l.history.ScrollBy(l.history.MaxRows())
	case "pagedown":
		l.history.ScrollBy(-l.history.MaxRows())
	case "up":
		if k.Mods&terminal.ModAlt != 0 {
			l.history.ScrollBy(1)
		}
	case "down":
		if k.Mods&terminal.ModAlt != 0 {
			l.history.ScrollBy(-1)
		}
	case "end":
		l.history.FollowTail()
	case "s":
		if l.judgmentView != nil && l.judgmentView.IsExpanded() {
			mode := "normal"
			if jm := l.judgmentView.Mode(); jm != "" {
				mode = jm
			}
			l.app.OpenSystemStatus(buildSystemStatusData(l.app, mode))
			return true
		}
	case "e":
		if l.judgmentView != nil && l.judgmentView.IsExpanded() {
			l.app.OpenEvidenceOverlay(EvidenceOverlayData{})
			return true
		}
	case "t":
		// Ctrl+Alt+T toggles theme; Ctrl+T (without Alt) toggles todo panel.
		if k.Mods&(terminal.ModCtrl|terminal.ModAlt) == (terminal.ModCtrl | terminal.ModAlt) {
			theme.ToggleTheme()
			if l.app != nil && l.app.host != nil {
				l.app.host.RequestRender()
			}
			return true
		}
		if k.Mods&terminal.ModCtrl != 0 {
			l.app.ToggleTodoPanel()
			return true
		}
	case "slash":
		l.history.SearchActivate()
		if l.app.host != nil {
			l.app.host.RequestRender()
		}
		return true
	case "question":
		if l.app != nil {
			l.app.ToggleKeyHelp()
			return true
		}
	case "p":
		// Ctrl+P opens the command palette. Only with Ctrl held (a bare
		// "p" must keep typing into the editor), and excluding super/meta
		// so ⌘P on macOS (system print) never triggers it. The palette
		// itself is a host-level overlay (cmd/mady builds it from the
		// slash registry), so the chat layer only forwards via
		// OnCommandCenter.
		if k.Mods&terminal.ModCtrl != 0 &&
			k.Mods&terminal.ModSuper == 0 && k.Mods&terminal.ModMeta == 0 &&
			l.app != nil && l.app.cfg.OnCommandCenter != nil {
			l.app.cfg.OnCommandCenter()
			return true
		}
	case "c", "insert":
		return l.handleCopyOrInterrupt(k, name)
	}
	return false
}

// dispatchConfirmKey handles key events while an inline confirmation is
// pending. Only y (yes), n (no), and Esc (no) are accepted.
func (l *chatLayout) dispatchConfirmKey(k terminal.Key) bool {
	name := strings.ToLower(k.Name)
	switch name {
	case "y":
		l.app.ConfirmYes()
		return true
	case "n", "escape":
		l.app.ConfirmNo()
		return true
	}
	return false
}

// dispatchSearchKey handles key events while search mode is active.
// All printable characters are appended to the search query; navigation
// keys (n/N, Esc, Enter, Backspace) control search mode behavior.
func (l *chatLayout) dispatchSearchKey(k terminal.Key) bool {
	name := strings.ToLower(k.Name)
	reqRender := func() {
		if l.app.host != nil {
			l.app.host.RequestRender()
		}
	}
	switch name {
	case "escape":
		l.history.SearchDeactivate()
		reqRender()
		return true
	case "enter":
		l.history.SearchDeactivate()
		reqRender()
		return true
	case "n":
		if k.Mods&terminal.ModShift == 0 {
			l.history.SearchNext()
		} else {
			l.history.SearchPrev()
		}
		reqRender()
		return true
	case "backspace":
		if len(l.history.SearchQuery()) == 0 {
			// Empty query + Backspace = exit search mode.
			l.history.SearchDeactivate()
		} else {
			l.history.SearchBackspace()
		}
		reqRender()
		return true
	default:
		// Only single printable characters (no modifiers) feed the search.
		if len(name) == 1 && k.Mods == 0 {
			l.history.SearchAppend(rune(name[0]))
			reqRender()
			return true
		}
	}
	return false
}

// handleEscapeKey implements the double-escape guard and autocomplete pop.
func (l *chatLayout) handleEscapeKey(k terminal.Key) bool {
	if l.app != nil {
		state := l.app.State()
		if state == StateStreaming || state == StateToolRunning || state == StateCompacting {
			l.app.mu.Lock()
			lastEsc := l.app.lastEscAt
			isDoubleEsc := !lastEsc.IsZero() && time.Since(lastEsc) < escInterruptWindow
			if isDoubleEsc {
				l.app.lastEscAt = time.Time{}
			} else {
				l.app.lastEscAt = time.Now()
			}
			l.app.mu.Unlock()
			if isDoubleEsc {
				if l.app.cfg.OnInterrupt != nil {
					l.app.cfg.OnInterrupt()
				}
				return true
			}
			l.app.PrintSystem("\u518d\u6b21\u6309 Esc \u53ef\u4e2d\u65ad\u5f53\u524d\u64cd\u4f5c")
			return true
		}
	}
	if l.app != nil && l.ac != nil && l.ac.Active() {
		l.ac.Hide()
		value := l.app.editor.GetValue()
		if (strings.HasPrefix(value, "@file:") || strings.HasPrefix(value, "@folder:")) &&
			len(value) > len("@file:") {
			newValue := popLastPathSegment(value)
			l.app.editor.SetValue(newValue)
			l.app.skipRefresh = false
			l.ac.Refresh(newValue, int64(len(newValue)))
		}
		return true
	}
	return false
}

// handleCopyOrInterrupt handles Ctrl+C (interrupt/quit) and copy/paste shortcuts.
func (l *chatLayout) handleCopyOrInterrupt(k terminal.Key, name string) bool {
	if name == "c" && k.Mods&terminal.ModCtrl != 0 &&
		k.Mods&terminal.ModSuper == 0 && k.Mods&terminal.ModMeta == 0 {
		// Ctrl+Shift+C: the Editor's keybinding system already handles the copy
		// on Kitty-capable terminals. Consume the key here to prevent doCopy
		// below from firing a second time (double-copy regression).
		if k.Mods&terminal.ModShift != 0 {
			return true
		}
		// Plain Ctrl+C (no Shift). Priority: interrupt (when running) > copy
		// (idle with selection) > quit (idle without selection).
		if l.app != nil && l.app.isRunning() {
			if l.app.cfg.OnInterrupt != nil {
				l.app.cfg.OnInterrupt()
			}
			return true
		}
		// On non-Kitty terminals (tmux, Terminal.app), Ctrl+Shift+C is parsed
		// without the Shift bit and arrives here. When idle with a selection,
		// treat Ctrl+C as copy — matching the convention of modern terminal
		// emulators (Alacritty vi mode, WezTerm, etc.).
		if hasSelection(l) {
			doCopy(l)
			return true
		}
		if l.app == nil {
			return true
		}
		if l.app.cfg.OnQuit != nil {
			l.app.cfg.OnQuit()
		}
		return true
	}
	if isCopyShortcut(k) {
		doCopy(l)
		return true
	}
	if isPasteShortcut(k) {
		if e, ok := l.editor.(*component.Editor); ok {
			l.pendingCmd = e.RequestPaste()
		}
		return true
	}
	return false
}
