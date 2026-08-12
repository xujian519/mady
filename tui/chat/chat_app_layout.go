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
	"time"

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

// charCounter is implemented by the Editor component so the wrapping
// editorFrame can render a non-intrusive character counter in the bottom
// border without intruding into the editor's own layout budget.
type charCounter interface {
	// CharCount returns the current buffer length in runes (not bytes),
	// and a recommended soft-limit after which the counter should render
	// in a warning hue (0 = no limit).
	CharCount() (count, softLimit int64)
}

// editorFrame wraps the editor with a rounded brand border so the input
// area reads as a distinct panel below the chat history. It exists so the
// editor border can participate in the declarative Flex layout.
//
// Visual style (Reasonix-inspired brand box):
//
//	╭─ 输入 (Enter 发送 · Shift+Enter 换行) ─────╮
//	│ ❯ hello world                             │
//	╰─────────────────────── N chars · ↑↓历史╯
type editorFrame struct {
	editor core.Component
}

func (f *editorFrame) Render(width int64) []string {
	pal := theme.CurrentPalette()
	bfn := pal.BorderAccent.Render
	if width < 4 {
		width = 4
	}
	inner := width - 2
	if inner < 1 {
		inner = 1
	}
	lines := f.editor.Render(inner)

	counter := ""
	if cc, ok := f.editor.(charCounter); ok {
		n, limit := cc.CharCount()
		if n > 0 {
			counter = fmt.Sprintf(" %d 字符 ", n)
			if limit > 0 && n > limit {
				counter = pal.Warning.Render(counter)
			} else {
				counter = pal.Dim.Render(counter)
			}
		}
	}
	counterW := int64(0)
	if counter != "" {
		counterW = core.VisibleWidth(counter)
	}

	title := " 输入 (Enter 发送 · Shift+Enter 换行) "
	tw := core.VisibleWidth(title)
	if tw > inner {
		title = core.TruncateToWidth(title, inner, "…")
		tw = core.VisibleWidth(title)
	}
	headRune := "╭"
	diff := inner - tw
	if diff < 0 {
		diff = 0
	}
	topBody := title + strings.Repeat("─", int(diff))
	top := bfn(headRune) + topBody + bfn("╮")

	out := make([]string, 0, len(lines)+2)
	out = append(out, top)

	v := bfn("│")
	for _, ln := range lines {
		body := core.PadToWidth(ln, inner)
		out = append(out, v+body+v)
	}

	// Bottom border: counter on the far left when present, hint on the far
	// right, with ─ fill in between. If too narrow, drop hint first, then
	// counter, finally fall back to plain bar.
	hint := " ↑↓历史 · ⏎发送 · ⇧⏎换行 "
	hintW := core.VisibleWidth(hint)
	dimHint := pal.Dim.Render(hint)

	var bottom string
	switch {
	case counter != "" && inner >= counterW+hintW+6:
		fill := inner - counterW - hintW
		if fill < 0 {
			fill = 0
		}
		bottom = bfn("╰") + counter + strings.Repeat("─", int(fill)) + dimHint + bfn("╯")
	case inner >= hintW+4:
		fill := inner - hintW
		if fill < 0 {
			fill = 0
		}
		bottom = bfn("╰") + strings.Repeat("─", int(fill)) + dimHint + bfn("╯")
	case counter != "" && inner >= counterW+4:
		fill := inner - counterW
		if fill < 0 {
			fill = 0
		}
		bottom = bfn("╰") + counter + strings.Repeat("─", int(fill)) + bfn("╯")
	default:
		bottom = bfn("╰") + strings.Repeat("─", int(inner)) + bfn("╯")
	}
	bottom = core.TruncateToWidth(bottom, width, "…")
	out = append(out, bottom)
	return out
}

func (f *editorFrame) Invalidate() {}

// todoBar is a compact, always-visible task anchor mounted in the layout
// flow directly above the editor frame (Reasonix todoArgs pattern). It
// surfaces the top 1-3 in-flight tasks so the user sees them while typing
// without opening the full TodoPanel overlay (Ctrl+T).
type todoBar struct {
	app *ChatApp
}

func (t *todoBar) Render(width int64) []string {
	if t.app == nil {
		return nil
	}
	items := t.app.collectTodoItems()
	var active []component.TodoItem
collectLoop:
	for _, it := range items {
		switch it.Status {
		case "pending", "in_progress":
			active = append(active, it)
			if len(active) == 3 {
				break collectLoop
			}
		}
	}
	if len(active) == 0 {
		return nil
	}
	if width < 20 {
		width = 20
	}
	const prefixW = 4 // "│ " + icon + " "
	contentW := width - prefixW - 1
	if contentW < 8 {
		contentW = 8
	}

	pal := theme.CurrentPalette()
	header := fmt.Sprintf("📋 %d 待办  Ctrl+T 全览", len(active))
	lines := make([]string, 0, len(active)+2)

	// Top border: ╭ ─ header ─ ╮
	head := pal.BorderAccent.Render("╭ ") + pal.Accent.Render(header) + " "
	headRemain := width - core.VisibleWidth(head) - 1
	if headRemain < 0 {
		headRemain = 0
	}
	head += pal.BorderAccent.Render(strings.Repeat("─", int(headRemain)) + "╮")
	lines = append(lines, core.TruncateToWidth(head, width, "…"))

	// Body rows: │ ○ content [pri]  │
	v := pal.BorderAccent.Render("│")
	for _, it := range active {
		icon := "○"
		var bodyStyle func(string) string
		switch it.Status {
		case "in_progress":
			icon = "◐"
			bodyStyle = pal.Accent.Render
		default:
			bodyStyle = pal.Assistant.Render
		}
		pri := ""
		if it.Priority != "" {
			pri = " [" + it.Priority + "]"
		}
		content := strings.ReplaceAll(it.Content, "\n", " ")
		maxCW := contentW - core.VisibleWidth(pri)
		if maxCW < 1 {
			maxCW = 1
		}
		content = core.TruncateToWidth(content, maxCW, "…")
		row := v + " " + icon + " " + bodyStyle(content) + pal.Dim.Render(pri)
		row = core.TruncateToWidth(row, width-1, "…")
		fill := width - core.VisibleWidth(row) - 1
		if fill < 0 {
			fill = 0
		}
		row = row + strings.Repeat(" ", int(fill)) + v
		lines = append(lines, row)
	}

	// Bottom border: ╰ ─ ╯
	tail := pal.BorderAccent.Render("╰" + strings.Repeat("─", int(width)-2) + "╯")
	lines = append(lines, core.TruncateToWidth(tail, width, "…"))
	return lines
}

func (t *todoBar) Invalidate() {}

type chatLayout struct {
	host         layoutHost
	app          *ChatApp
	header       core.Component
	judgmentView *component.JudgmentView
	history      *ChatHistory
	loader       *component.Loader
	editor       core.Component
	statusBar    *component.StatusBar
	footer       core.Component
	ac           *component.Autocomplete
	todoBar      *todoBar
	lastRows     int64
	headerHeight int
	// editorMaxRows is the baseline (un-shrunk) row budget for the editor,
	// reset on every buildFlex pass so a previous shrink does not stick.
	editorMaxRows int64
	// editorTop is the absolute screen row of the editor's top border, as
	// computed by the most recent Render call. Used to translate MouseMsg
	// screen coordinates into the editor's own row space (see Update).
	editorTop int64
	// mainFlex holds the most recently rendered Flex, used for mouse
	// hit-testing. Populated in Render; nil before the first render.
	mainFlex *layout.Flex

	// breakpoint is the current layout regime based on terminal width.
	// Recalculated each Render call; stored for use by child components.
	breakpoint layout.LayoutBreakpoint

	// pendingCmd holds a Cmd produced during Update that must be returned
	// to the TUI event loop (e.g. clipboard read from a paste shortcut).
	pendingCmd core.Cmd
}

type textSelectionComponent interface {
	GetSelectedText() string
	ClearSelection()
}

// maxRowsSetter is implemented by components whose visible row count can be
// capped at runtime (Editor, ChatHistory). buildFlex uses it to reset the
// editor to its baseline and to shrink it when the Flex is over-committed.
type maxRowsSetter interface {
	SetMaxRows(n int64)
}

// buildFlex populates a vertical Flex with the standard chat components.

func (l *chatLayout) buildFlex(flex *layout.Flex) (headerIndex, editorIndex int) {
	headerIndex = -1

	// Reset the editor to its baseline row budget before measuring; see
	// resetEditorBaseline for why.
	l.resetEditorBaseline()

	if l.header != nil {
		headerIndex = len(flex.Children)
		flex.AddChild(layout.Natural(l.header))
	}
	if l.judgmentView != nil && !l.judgmentView.IsEmpty() {
		flex.AddChild(layout.Natural(l.judgmentView))
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
	// Queued input indicator: show the count of messages buffered while
	// the agent was streaming. Inputs are flushed on stream end.
	if qc := l.app.QueuedInputCount(); qc > 0 {
		qText := fmt.Sprintf("待发送：%d 条消息（排队中）", qc)
		flex.AddChild(layout.Natural(component.NewText(theme.CurrentPalette().Dim.Render(qText))))
	}
	// Reasonix-style anchored todo bar: compact view of top 1-3 in-flight
	// tasks between the queue text and the editor frame. Render returns nil
	// (0 rows) when no active tasks exist, so the layout collapses silently.
	if l.todoBar != nil {
		flex.AddChild(layout.Natural(l.todoBar))
	}
	ef := &editorFrame{editor: l.editor}
	editorIndex = len(flex.Children)
	// editorFrame is Shrinkable (min 3 = top border + ≥1 editor row + bottom
	// border): when header + editor + autocomplete + status bar overfill the
	// terminal, the Flex squeezes the editor (via OnAllocate → SetMaxRows)
	// instead of pushing the input area off-screen.
	flex.AddChild(layout.Shrinkable(ef, 3).WithAllocate(func(h int64) {
		if ed, ok := l.editor.(maxRowsSetter); ok {
			rows := h - 2 // subtract top + bottom borders
			if rows < 1 {
				rows = 1
			}
			ed.SetMaxRows(rows)
		}
	}))
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

	// Detect the current breakpoint and inform child components.
	bp := layout.DetectLayoutBreakpoint(width)
	l.breakpoint = bp

	// Adjust child component rendering based on breakpoint.
	if l.footer != nil {
		if ft, ok := l.footer.(*component.Footer); ok {
			ft.SetCompact(bp == layout.LayoutCompact)
		}
	}

	// Build and render the main flex.
	flex := layout.NewFlex(layout.DirectionVertical)
	flex.Bounds = &fixedBounds{width: width, height: rows}
	hIdx, eIdx := l.buildFlex(flex)

	out := flex.Render(width)

	// Store the rendered flex for mouse hit-testing (see HitTest / Update).
	l.mainFlex = flex

	// Extract layout metadata from the rendered flex.
	if hIdx >= 0 {
		l.headerHeight = int(flex.ChildRect(hIdx).Height)
	}
	if eIdx >= 0 {
		l.editorTop = flex.ChildRect(eIdx).Row
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

// HitTest implements core.MouseTarget by delegating to the most recently
// rendered Flex. This allows the TUI's processMsg to route MouseMsg events
// directly to the child at (row, col) with local coordinates, bypassing
// the manual offset math in Update.
func (l *chatLayout) HitTest(row, col int64) (core.Component, core.Rect, bool) {
	if l.mainFlex == nil {
		return nil, core.Rect{}, false
	}
	return l.mainFlex.HitTest(row, col)
}

func (l *chatLayout) Update(msg core.Msg) core.Cmd {
	// Reset any carry-over pending Cmd from the previous Update tick.
	l.pendingCmd = nil

	// Handle global messages that don't depend on l.history.
	// If consumed (e.g. image paste), skip all further processing.
	if l.handleGlobalMsg(msg) {
		return nil
	}

	if l.history == nil {
		remaining := l.updateRemaining(msg)
		return core.Sequence(l.pendingCmd, remaining)
	}

	// Delegate to type-specific handlers. The consumed flag prevents consumed
	// keys from reaching the autocomplete popup. Handlers may set l.pendingCmd
	// (e.g. paste shortcut triggers an async clipboard-read Cmd) that we fold
	// into the return value below.
	var consumed bool
	switch m := msg.(type) {
	case core.MouseMsg:
		l.handleMouseMsg(m)
	case core.KeyMsg:
		consumed = l.handleKeyMsg(m)
	}

	// Autocomplete gets a chance to process the event only when no handler
	// consumed it (original Update behavior — consumed keys returned nil
	// before reaching the AC update at the bottom of the function).
	if !consumed && l.ac != nil && l.ac.Active() {
		if _, ok := msg.(core.KeyMsg); ok {
			l.ac.Update(msg)
		}
	}
	remaining := l.updateRemaining(msg)
	return core.Sequence(l.pendingCmd, remaining)
}

// handleGlobalMsg processes WindowSizeMsg and PasteMsg — events that don't
// require l.history or user interaction routing.
// Returns true if the message was fully consumed (no further processing needed).
func (l *chatLayout) handleGlobalMsg(msg core.Msg) bool {
	switch m := msg.(type) {
	case core.WindowSizeMsg:
		l.lastRows = m.Height
		l.recalcMaxRows(m.Width, m.Height)
	case core.PasteMsg:
		if m.Text == "" || (len(m.Text) < 4 && m.Text == "\r") {
			if l.app.cfg.OnImagePaste != nil {
				l.app.cfg.OnImagePaste()
			}
			return true // image paste consumed, no further processing
		}
	}
	return false
}

// handleMouseMsg routes mouse events through HitTest or legacy fallback.
func (l *chatLayout) handleMouseMsg(m core.MouseMsg) {
	if m.Action == core.MouseRelease && m.Button == 2 {
		doCopy(l)
		return
	}
	if l.mainFlex != nil {
		if child, rect, hit := l.mainFlex.HitTest(m.Row, m.Col); hit {
			local := m
			local.Row -= rect.Row
			local.Col -= rect.Col
			if u, ok := child.(core.Updatable); ok {
				u.Update(local)
			}
			if mc, ok := child.(core.MouseConsumer); ok && mc.MouseConsumed() {
				return
			}
			l.deliverMouseToOthers(child, m)
			return
		}
	}
	l.legacyMouseFallback(m)
}

// deliverMouseToOthers sends the mouse event to the history and/or editor
// when the primary HitTest target did not consume it.
func (l *chatLayout) deliverMouseToOthers(child core.Component, m core.MouseMsg) {
	if child != l.history && l.history != nil {
		histAdjusted := m
		histAdjusted.Row -= int64(l.headerHeight)
		if histAdjusted.Row >= 0 {
			l.history.Update(histAdjusted)
		}
	}
	if child != l.editor {
		if upd, ok := l.editor.(core.Updatable); ok {
			editorAdjusted := m
			editorAdjusted.Row -= l.editorTop + 1
			upd.Update(editorAdjusted)
		}
	}
}

// legacyMouseFallback is the pre-HitTest broadcast path, kept as a safety
// net for the first frame before mainFlex is populated.
func (l *chatLayout) legacyMouseFallback(m core.MouseMsg) {
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
}

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

// handleCopyOrInterrupt handles Ctrl+C (interrupt/quit) and copy shortcuts.
func (l *chatLayout) handleCopyOrInterrupt(k terminal.Key, name string) bool {
	if name == "c" && k.Mods&terminal.ModCtrl != 0 &&
		k.Mods&terminal.ModSuper == 0 && k.Mods&terminal.ModMeta == 0 &&
		k.Mods&terminal.ModShift == 0 {
		if l.app == nil {
			return true
		}
		switch {
		case l.app.isRunning():
			if l.app.cfg.OnInterrupt != nil {
				l.app.cfg.OnInterrupt()
			}
		case l.app.cfg.OnQuit != nil:
			l.app.cfg.OnQuit()
		default:
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

// updateRemaining handles status bar and autocomplete updates after other
// message processing is complete.
func (l *chatLayout) updateRemaining(msg core.Msg) core.Cmd {
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
	// Measure with the editor at its baseline, not a leftover shrink value
	// from the previous render's OnAllocate.
	l.resetEditorBaseline()

	var headerH, jvH, loaderH, editorH, footerH, statusH, acH, todoBarH int64
	if l.header != nil {
		headerH = int64(len(l.header.Render(width)))
	}
	if l.judgmentView != nil {
		jvH = l.judgmentView.Height(width)
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
	if l.todoBar != nil {
		todoBarH = int64(len(l.todoBar.Render(width)))
	}
	reserved := headerH + jvH + editorH + loaderH + footerH + statusH + acH + todoBarH
	remaining := height - reserved
	if remaining < 1 {
		remaining = 1
	}
	if l.history != nil {
		l.history.SetMaxRows(remaining)
	}
}

// updateJudgmentView syncs the judgment view state from the ChatApp model.
// The status text is derived purely from the FSM state. The former
// Running+StreamID fallback heuristic (which papered over "between-phases"
// activity the FSM didn't track) is gone: transitionFromIdle now treats a
// late delta as streaming activity, so the FSM is the single truth source.
func (l *chatLayout) updateJudgmentView() {
	if l.judgmentView == nil {
		return
	}
	l.app.mu.Lock()
	fsmState := l.app.model.state
	js := l.app.model.judgmentSummary
	l.app.mu.Unlock()

	// Derive status text from FSM state.
	var status string
	switch fsmState {
	case StateInitializing:
		status = "initializing"
	case StateStreaming:
		status = "streaming"
	case StateToolRunning:
		status = "analyzing"
	case StateAwaitingConfirm:
		status = "awaiting_review"
	case StateCompacting:
		status = "compacting"
	case StateFailed:
		status = "failed"
	case StateInterrupted:
		status = "interrupted"
	case StateConfirmPending:
		status = "confirming"
	case StateIdle:
		status = "idle"
	}
	l.judgmentView.SetStatus(status)

	// Populate judgment-bar content from the model snapshot.
	if js.Phase != "" {
		l.judgmentView.SetPhase(js.Phase)
	}
	if js.Judgment != "" {
		l.judgmentView.SetJudgment(js.Judgment)
	}
	if js.Confidence >= 0 {
		l.judgmentView.SetConfidence(int(js.Confidence * 100))
	} else {
		l.judgmentView.SetConfidence(-1)
	}
	l.judgmentView.SetPending(js.Pending)

	// Action hints: evidence and system status always available; review
	// action appears when the view is expanded (awaiting_review/blocked).
	actions := []component.JudgmentAction{
		{Key: "e", Label: "证据详情"},
		{Key: "s", Label: "系统态"},
	}
	if l.judgmentView.IsExpanded() {
		actions = append(actions, component.JudgmentAction{Key: "r", Label: "复核"})
	}
	l.judgmentView.SetActions(actions)

	if l.app.host != nil {
		l.app.host.RequestRender()
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

// buildSystemStatusData constructs a SystemStatusData from the ChatApp's
// FSM state, active tools, and judgment summary. Called when the user opens
// the system status overlay via the [s] shortcut.
func buildSystemStatusData(app *ChatApp, mode string) SystemStatusData {
	app.mu.Lock()
	fsmState := app.model.state
	toolCount := len(app.model.activeTools)
	js := app.model.judgmentSummary
	app.mu.Unlock()

	// Build events list from FSM state.
	var modeReason string
	var events []component.SysEvent

	// Event 1: Current FSM state.
	stateLabel := fsmState.String()
	events = append(events, component.SysEvent{
		Time:    "",
		Message: fmt.Sprintf("Agent 状态: %s", stateLabel),
		Level:   stateLevel(stateLabel),
	})

	// Event 2: Active tools (if any).
	if toolCount > 0 {
		events = append(events, component.SysEvent{
			Time:    "",
			Message: fmt.Sprintf("活跃工具: %d 个进行中", toolCount),
			Level:   "info",
		})
	}

	// Event 3: Approval state (if awaiting review).
	if js.Phase != "" || js.Judgment != "" {
		judgmentSnippet := js.Judgment
		if len(judgmentSnippet) > 40 {
			judgmentSnippet = judgmentSnippet[:40] + "..."
		}
		phaseLabel := js.Phase
		if phaseLabel == "" {
			phaseLabel = "分析中"
		}
		msg := fmt.Sprintf("审批: %s", phaseLabel)
		if judgmentSnippet != "" {
			msg += " · " + judgmentSnippet
		}
		events = append(events, component.SysEvent{
			Time:    "",
			Message: msg,
			Level:   "info",
		})
		if mode == "" {
			mode = "awaiting_review"
		}
	}

	// Derive mode reason from state.
	switch fsmState {
	case StateInterrupted:
		modeReason = "等待人工复核"
	case StateAwaitingConfirm:
		modeReason = "等待人工复核"
	case StateFailed:
		if mode == "" {
			mode = "degraded"
		}
		modeReason = "上次操作未正常完成"
	case StateInitializing:
		modeReason = "Agent 正在后台初始化"
	case StateStreaming, StateToolRunning:
		modeReason = "Agent 正在执行任务"
	case StateCompacting:
		modeReason = "上下文窗口压缩中"
	default:
		modeReason = "就绪"
	}

	// Impacts: storage persistence hints from the status bar.
	var impacts []string
	if mode == "degraded" || mode == "" {
		impacts = append(impacts, "部分组件以降级模式运行，功能可能受限")
	}

	return SystemStatusData{
		Mode:       mode,
		ModeReason: modeReason,
		Events:     events,
		Impacts:    impacts,
	}
}

// stateLevel maps an AppState to a SysEvent severity level.
func stateLevel(s string) string {
	switch s {
	case "failed":
		return "error"
	case "awaiting-confirm":
		return "warn"
	case "initializing", "compacting", "interrupted":
		return "warn"
	default:
		return "info"
	}
}
