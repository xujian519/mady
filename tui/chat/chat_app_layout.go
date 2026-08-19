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
	"github.com/xujian519/mady/tui/theme"
)

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
		l.updateRemaining(msg)
		return l.pendingCmd
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
	l.updateRemaining(msg)
	return l.pendingCmd
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

// updateRemaining handles status bar and autocomplete updates after other
// message processing is complete.
func (l *chatLayout) updateRemaining(msg core.Msg) {
	if l.statusBar != nil {
		l.statusBar.Update(msg)
	}
	if l.ac != nil && l.ac.Active() {
		if _, ok := msg.(core.KeyMsg); ok {
			l.ac.Update(msg)
		}
	}
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
