package chat

// This file defines the chatLayout — the root Component that stacks header,
// chat history, autocomplete, loader, editor (bordered), footer, and status
// bar via the declarative Flex layout. It also owns the input router
// (Update), translating keys/mouse/paste into the right child action
// (scrolling, copy-vs-interrupt, autocomplete, image paste), and the
// copy/copy-shortcut helpers.

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/layout"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

func (a *ChatApp) TerminalSize() (cols, rows int64) {
	if a.host != nil {
		return a.host.TerminalSize()
	}
	return 80, 24
}

type layoutHost interface {
	TerminalSize() (cols, rows int64)
}

// editorFrame wraps the editor with a horizontal top/bottom border. It exists
// so the editor border can participate in the declarative Flex layout.
type editorFrame struct {
	editor core.Component
}

func (f *editorFrame) Render(width int64) []string {
	lines := f.editor.Render(width)
	border := theme.CurrentPalette().Border.Render(strings.Repeat("─", int(width)))
	out := make([]string, 0, len(lines)+2)
	out = append(out, border)
	out = append(out, lines...)
	out = append(out, border)
	return out
}

func (f *editorFrame) Invalidate() {}

// sidebarWidth is the fixed column count for the sidebar when width >= 96.
const sidebarWidth = 24

type chatLayout struct {
	host         layoutHost
	app          *ChatApp
	header       core.Component
	history      *ChatHistory
	loader       *component.Loader
	editor       core.Component
	statusBar    *component.StatusBar
	footer       core.Component
	ac           *component.Autocomplete
	sidebar      core.Component // Phase 4.4: optional sidebar panel
	lastRows     int64
	headerHeight int
	// editorTop is the absolute screen row of the editor's top border, as
	// computed by the most recent Render call. Used to translate MouseMsg
	// screen coordinates into the editor's own row space (see Update).
	editorTop int64
}

type textSelectionComponent interface {
	GetSelectedText() string
	ClearSelection()
}

// buildFlex populates a vertical Flex with the standard chat components.
// Returns the indices for header and editor frame for ChildRect queries.
func (l *chatLayout) buildFlex(flex *layout.Flex) (headerIndex, editorIndex int) {
	headerIndex = -1
	editorIndex = -1

	if l.header != nil {
		headerIndex = len(flex.Children)
		flex.AddChild(layout.Natural(l.header))
	}
	if l.history != nil {
		flex.AddChild(layout.FillWeight(l.history, 1).WithAllocate(func(h int64) {
			l.history.SetMaxRowsDirect(h)
		}))
	}
	if l.ac != nil && l.ac.Active() {
		flex.AddChild(layout.Natural(l.ac))
	}
	if l.loader != nil && l.loader.IsRunning() {
		flex.AddChild(layout.Natural(l.loader))
	}
	editorFrame := &editorFrame{editor: l.editor}
	editorIndex = len(flex.Children)
	flex.AddChild(layout.Natural(editorFrame))
	if l.footer != nil {
		flex.AddChild(layout.Natural(l.footer))
	}
	if l.statusBar != nil {
		flex.AddChild(layout.Natural(l.statusBar))
	}
	return
}

func (l *chatLayout) Render(width int64) []string {
	var rows int64
	if l.host != nil {
		_, rows = l.host.TerminalSize()
	}
	if rows <= 0 {
		rows = l.lastRows
	}
	if rows <= 0 {
		rows = 24
	}
	l.lastRows = rows

	bounds := &fixedBounds{width: width, height: rows}

	// Phase 4.4: responsive sidebar — ≥96 columns shows sidebar + main, else single column
	useSidebar := l.sidebar != nil && width >= 96
	mainWidth := width
	if useSidebar {
		mainWidth = width - sidebarWidth
	}

	// Build and render the main flex once. ChildRect is populated by Render().
	mainFlex := layout.NewFlex(layout.DirectionVertical)
	mainFlex.Bounds = &fixedBounds{width: mainWidth, height: rows}
	hIdx, eIdx := l.buildFlex(mainFlex)

	var out []string
	if useSidebar {
		outer := layout.NewFlex(layout.DirectionHorizontal)
		outer.Bounds = bounds
		outer.AddChild(layout.Natural(l.sidebar))
		outer.AddChild(layout.FillWeight(mainFlex, 1))
		out = outer.Render(width)
	} else {
		out = mainFlex.Render(width)
	}

	// Extract layout metadata from the rendered flex.
	if hIdx >= 0 {
		l.headerHeight = int(mainFlex.ChildRect(hIdx).Height)
	}
	if eIdx >= 0 {
		l.editorTop = mainFlex.ChildRect(eIdx).Row
	}
	return out
}

type fixedBounds struct {
	width, height int64
}

func (b *fixedBounds) TerminalSize() (cols, rows int64) {
	return b.width, b.height
}

func (l *chatLayout) Invalidate() {}

func doCopy(l *chatLayout) {
	// Copy editor selection first.
	if sel, ok := l.editor.(textSelectionComponent); ok {
		if text := sel.GetSelectedText(); text != "" {
			doCopyToClipboard(l, text)
			return
		}
	}
	// Copy history selection
	text := l.history.GetSelectedText()
	if text != "" {
		doCopyToClipboard(l, text)
		return
	}
	// 无显式选区时，复制最后一条助手消息（最常用场景）。
	msgs := l.history.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleAssistant && msgs[i].Text != "" {
			doCopyToClipboard(l, msgs[i].Text)
			return
		}
	}
}

func doCopyToClipboard(l *chatLayout, text string) {
	go func() {
		if err := CopyToClipboard(text); err != nil {
			l.app.PrintError(fmt.Errorf("clipboard: %w", err))
		} else {
			truncated := text
			if core.VisibleWidth(truncated) > 60 {
				truncated = core.TruncateToWidth(truncated, 57, "...")
			}
			l.app.PrintSystem("📋 已复制: " + truncated)
		}
	}()
}

// hasSelection reports whether the editor or chat history currently has a
// non-empty text selection, without clearing it. Used to decide whether
// Ctrl/Cmd+C should copy instead of interrupting the running agent.
func hasSelection(l *chatLayout) bool {
	if sel, ok := l.editor.(textSelectionComponent); ok && sel.GetSelectedText() != "" {
		return true
	}
	return l.history.GetSelectedText() != ""
}

func isPrimaryShortcutMod(mods terminal.Modifier) bool {
	return mods&terminal.ModCtrl != 0 || mods&terminal.ModSuper != 0 || mods&terminal.ModMeta != 0
}

func isCopyShortcut(k terminal.Key) bool {
	name := strings.ToLower(k.Name)
	if name == "c" {
		return isPrimaryShortcutMod(k.Mods)
	}
	if name == "insert" {
		return k.Mods&terminal.ModCtrl != 0 || k.Mods&terminal.ModShift != 0
	}
	return false
}

func (l *chatLayout) Update(msg core.Msg) core.Cmd {
	switch m := msg.(type) {
	case core.WindowSizeMsg:
		l.lastRows = m.Height
		l.recalcMaxRows(m.Width, m.Height)
	case core.PasteMsg:
		// Image paste detection: when clipboard has an image, the terminal
		// sends empty/short text.  Let the caller hook pasteImageFn.
		if m.Text == "" || (len(m.Text) < 4 && m.Text == "\r") {
			if l.app.cfg.OnImagePaste != nil {
				l.app.cfg.OnImagePaste()
				return nil
			}
		}
	}
	if l.history != nil {
		switch m := msg.(type) {
		case core.MouseMsg:
			// Right-click (Button 2) → copy selected text.
			if m.Action == core.MouseRelease && m.Button == 2 {
				doCopy(l)
				return nil
			}
			adjusted := m
			adjusted.Row -= int64(l.headerHeight)
			if adjusted.Row >= 0 {
				l.history.Update(adjusted)
			}
			if upd, ok := l.editor.(core.Updatable); ok {
				editorAdjusted := m
				editorAdjusted.Row -= l.editorTop + 1
				upd.Update(editorAdjusted)
			}
		case core.KeyMsg:
			for _, k := range terminal.ParseKeys(m.Data) {
				name := strings.ToLower(k.Name)
				switch name {
				case "v":
					if isPrimaryShortcutMod(k.Mods) &&
						k.Mods&terminal.ModAlt != 0 {
						if l.app.cfg.OnImagePaste != nil {
							l.app.cfg.OnImagePaste()
						}
						return nil
					}
				case "escape":
					if l.ac != nil && l.ac.Active() {
						l.ac.Hide()
						value := l.app.editor.GetValue()
						if (strings.HasPrefix(value, "@file:") || strings.HasPrefix(value, "@folder:")) &&
							len(value) > len("@file:") {
							newValue := popLastPathSegment(value)
							l.app.editor.SetValue(newValue)
							l.app.skipRefresh = false
							l.ac.Refresh(newValue, int64(len(newValue)))
						}
						return nil
					}
				case "pageUp":
					l.history.ScrollBy(-5)
				case "pageDown":
					l.history.ScrollBy(5)
				case "c", "insert":
					if isCopyShortcut(k) {
						if hasSelection(l) {
							doCopy(l)
							return nil
						}
						if k.Mods&terminal.ModCtrl != 0 && l.app.cfg.OnInterrupt != nil && l.app.isRunning() {
							l.app.cfg.OnInterrupt()
							return nil
						}
						doCopy(l)
						return nil
					}
				}
			}
		}
	}
	if l.statusBar != nil {
		l.statusBar.Update(msg)
	}
	if l.ac != nil && l.ac.Active() {
		if _, ok := msg.(core.KeyMsg); ok {
			l.ac.Update(msg)
		}
	}
	return nil
}

func (l *chatLayout) recalcMaxRows(width, height int64) {
	var headerH, loaderH, editorH, footerH, statusH, acH int64
	if l.header != nil {
		headerH = int64(len(l.header.Render(width)))
	}
	if l.editor != nil {
		editorH = int64(len(l.editor.Render(width))) + 2
	}
	if l.loader != nil && l.loader.IsRunning() {
		loaderH = int64(len(l.loader.Render(width)))
	}
	if l.footer != nil {
		footerH = int64(len(l.footer.Render(width)))
	}
	if l.statusBar != nil {
		statusH = int64(len(l.statusBar.Render(width)))
	}
	if l.ac != nil && l.ac.Active() {
		acH = int64(len(l.ac.Render(width)))
	}
	reserved := headerH + editorH + loaderH + footerH + statusH + acH
	remaining := height - reserved
	if remaining < 1 {
		remaining = 1
	}
	if l.history != nil {
		l.history.SetMaxRows(remaining)
	}
}

// popLastPathSegment removes the trailing directory or file name from a value
// like "@file:cmd/mady/" → "@file:cmd/" or "@file:main.go" → "@file:".
func popLastPathSegment(value string) string {
	// Strip trailing slash if present, then remove the last segment.
	trimmed := strings.TrimSuffix(value, "/")
	idx := strings.LastIndexAny(trimmed, "/:")
	if idx < 0 {
		return value
	}
	return trimmed[:idx+1]
}
