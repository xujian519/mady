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
	"unicode/utf8"

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
	lastRows     int64
	headerHeight int
	// editorMaxRows is the baseline (un-shrunk) row budget for the editor,
	// reset on every buildFlex pass so a previous shrink does not stick.
	editorMaxRows int64
	// editorTop is the absolute screen row of the editor's top border, as
	// computed by the most recent Render call. Used to translate MouseMsg
	// screen coordinates into the editor's own row space (see Update).
	editorTop int64
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
// Returns the indices for header and editor frame for ChildRect queries.
// resetEditorBaseline restores the editor to its baseline row budget so a
// previous render's OnAllocate shrink does not contaminate the next
// natural-height measurement. Both buildFlex and recalcMaxRows measure the
// editor's natural height and must call this first.
func (l *chatLayout) resetEditorBaseline() {
	if l.editorMaxRows > 0 {
		if ed, ok := l.editor.(maxRowsSetter); ok {
			ed.SetMaxRows(l.editorMaxRows)
		}
	}
}

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

	// Build and render the main flex.
	mainFlex := layout.NewFlex(layout.DirectionVertical)
	mainFlex.Bounds = &fixedBounds{width: width, height: rows}
	hIdx, eIdx := l.buildFlex(mainFlex)

	out := mainFlex.Render(width)

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
			runeCount := utf8.RuneCountInString(text)
			l.app.PrintSystem(fmt.Sprintf("📋 已复制（%d 字符）", runeCount))
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
					// Bare [s] opens system status overlay when the judgment
					// view is expanded (awaiting_review / blocked) — the user
					// can see the [s] action hint and is typically reviewing,
					// not composing input. The key also reaches the editor;
					// a stray "s" character is harmless.
					if l.judgmentView != nil && l.judgmentView.IsExpanded() {
						mode := "normal"
						if jm := l.judgmentView.Mode(); jm != "" {
							mode = jm
						}
						l.app.OpenSystemStatus(buildSystemStatusData(l.app, mode))
						return nil
					}
				case "e":
					// Bare [e] opens evidence details overlay when the
					// judgment view is expanded. Shows retrieved knowledge
					// sources (law articles, judgments) used in the analysis.
					if l.judgmentView != nil && l.judgmentView.IsExpanded() {
						l.app.OpenEvidenceOverlay(EvidenceOverlayData{})
						return nil
					}
				case "t":
					// Ctrl+T toggles the task list (TodoPanel) overlay.
					if k.Mods&terminal.ModCtrl != 0 {
						l.app.ToggleTodoPanel()
						return nil
					}
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
	// Measure with the editor at its baseline, not a leftover shrink value
	// from the previous render's OnAllocate.
	l.resetEditorBaseline()

	var headerH, jvH, loaderH, editorH, footerH, statusH, acH int64
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
	reserved := headerH + jvH + editorH + loaderH + footerH + statusH + acH
	remaining := height - reserved
	if remaining < 1 {
		remaining = 1
	}
	if l.history != nil {
		l.history.SetMaxRows(remaining)
	}
}

// updateJudgmentView syncs the judgment view state from the ChatApp model.
// It derives the status text from the FSM state (model.state), falling back
// to the Running+StreamID heuristic only when the FSM is in Idle (the FSM
// doesn't track "between-phases-activity" that the old heuristic detected).
// This is the transitional implementation; after full FSM adoption the
// Running+StreamID+ActiveTools fields will be removed.
func (l *chatLayout) updateJudgmentView() {
	if l.judgmentView == nil {
		return
	}
	l.app.mu.Lock()
	fsmState := l.app.model.state
	streamID := l.app.model.StreamID
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
	case StateIdle:
		// Fallback: the old heuristic detected "between-phases" activity via
		// Running+ActiveTools; use it for idle-only until full FSM migration.
		// Running is no longer the single truth source; check ActiveTools only.
		if streamID != "" {
			status = "streaming"
		} else {
			status = "idle"
		}
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
	toolCount := len(app.model.ActiveTools)
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
	case "initializing", "compacting":
		return "info"
	default:
		return "info"
	}
}
