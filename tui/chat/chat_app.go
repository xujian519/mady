// Package chat provides the ChatApp — the main application layer that owns
// the chat model, chat history, editor, overlays, tool panels, and the
// explicit FSM-driven lifecycle for streaming, tool calls, and compaction.
//
// Files within this package:
//
//	chat_app.go       — ChatApp struct + constructors + lifecycle + accessors
//	chat_host.go      — AppHost interface family (ISP split)
//	chat_model.go     — configuration, model types, overlay helpers
//	chat_builder.go   — sub-component creation (history/editor/header/etc.)
//	chat_display.go   — display helpers (Print/Busy/Idle/overlays/paste)
//	chat_app_stream.go — editor submit, agent start/delta/end/error
//	chat_app_tool.go   — tool calls, handoffs, turns, retry, compaction
//	chat_app_layout.go — chatLayout root component + input routing
//	chat_app_todo.go   — task list (TodoPanel) event handlers
package chat

import (
	"fmt"
	"sync"
	"time"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

// ---------------------------------------------------------------------------
// ChatApp — the main application-layer struct
// ---------------------------------------------------------------------------

// ChatApp is the main application layer that owns the chat model, chat history,
// editor, overlays, tool panels, and manages the FSM-driven lifecycle for
// streaming, tool calls, and context compaction.
type ChatApp struct {
	cfg ChatAppConfig

	host         AppHost
	history      *ChatHistory
	editor       *component.Editor
	loader       *component.Loader
	layout       *chatLayout
	header       *component.TruncatedText
	statusBar    *component.StatusBar
	todoPanel    *component.TodoPanel
	todoOverlay  OverlayRef
	tasks        map[string]component.TodoItem
	ac           *component.Autocomplete
	km           *terminal.KeybindingsManager
	judgmentView *component.JudgmentView

	mu    sync.Mutex
	model chatModel

	helpOverlay OverlayRef

	// reviewGateOverlay tracks the active review gate overlay, if any.
	// Access is guarded by mu; see OpenReviewGate/CloseReviewGate for
	// lock-ordering discipline.
	reviewGateOverlay OverlayRef

	// systemStatusOverlay tracks the active system status overlay, if any.
	// Access is guarded by mu; see OpenSystemStatus/CloseSystemStatus for
	// lock-ordering discipline.
	systemStatusOverlay OverlayRef

	// evidenceOverlay tracks the active evidence details overlay, if any.
	// Access is guarded by mu; see OpenEvidenceOverlay/CloseEvidenceOverlay.
	evidenceOverlay OverlayRef

	// SuppressAutoRetry suppresses auto-retry messages from being printed to
	// the chat history. When true, retry events are silently dropped instead of
	// showing "⚠ retry N/M in D". Used by mady to buffer retry messages
	// and only flush them on final failure (Hermes-style buffered retry).
	SuppressAutoRetry bool

	// defaultPlaceholder holds the original editor placeholder text so Idle()
	// can restore it after Busy() temporarily sets "Ctrl+C to interrupt".
	defaultPlaceholder string

	// mousePassthrough tracks whether mouse passthrough mode is active.
	// In passthrough mode, the TUI's mouse reporting is disabled so the
	// terminal's native text selection works. Toggle with F2.
	mousePassthrough bool

	// lastEscAt records the timestamp of the most recent Esc key press.
	// Used by the double-Esc guard: the first Esc during streaming shows a
	// hint, the second within escInterruptWindow actually interrupts.
	lastEscAt time.Time

	skipRefresh bool // suppress autocomplete re-activation after applying a suggestion
}

// ---------------------------------------------------------------------------
// Constructors
// ---------------------------------------------------------------------------

// NewChatApp creates a ChatApp with the given configuration.
func NewChatApp(cfg ChatAppConfig) *ChatApp {
	return newChatApp(cfg)
}

// NewChatAppWithHost creates a ChatApp and sets its host in one call.
func NewChatAppWithHost(cfg ChatAppConfig, host AppHost) *ChatApp {
	cfg.Host = host
	return newChatApp(cfg)
}

func newChatApp(cfg ChatAppConfig) *ChatApp {
	cfg = applyChatDefaults(cfg)
	km := terminal.NewKeybindingsManager(terminal.DefaultKeybindings())
	history := newChatHistoryWithConfig(cfg)
	editor := newChatEditor(cfg, km)
	loader := component.NewLoader(func() {}, theme.CurrentPalette().Dim.Render("thinking..."))

	statusBar := component.NewStatusBar()
	if cfg.Title != "" {
		statusBar.SetMode(cfg.Title)
	}

	footer := component.NewFooter()

	chatApp := &ChatApp{
		cfg:                cfg,
		host:               cfg.Host,
		history:            history,
		editor:             editor,
		loader:             loader,
		statusBar:          statusBar,
		km:                 km,
		judgmentView:       component.NewJudgmentView(),
		todoPanel:          component.NewTodoPanel(),
		tasks:              make(map[string]component.TodoItem),
		defaultPlaceholder: "输入消息…（/ 查看命令）",
		model: chatModel{
			state:       StateInitializing,
			activeTools: make(map[string]time.Time),
			pastedTexts: make(map[int]string),
		},
	}

	chatApp.header = newChatHeader(cfg)
	chatApp.ac = newChatAutocomplete(cfg, chatApp)
	chatApp.layout = newChatLayout(cfg, chatApp, history, editor, loader, statusBar)
	chatApp.SetFooter(footer)
	chatApp.todoPanel.SetDataProvider(chatApp.collectTodoItems)
	if chatApp.host != nil {
		chatApp.todoPanel.SetOnInvalidate(chatApp.host.RequestRender)
	}
	bindChatEditorEvents(chatApp, editor, history)
	return chatApp
}

// ---------------------------------------------------------------------------
// Accessors
// ---------------------------------------------------------------------------

// SetHost attaches the AppHost after construction.
func (a *ChatApp) SetHost(host AppHost) {
	a.host = host
	a.loader = component.NewLoader(host.RequestRender, theme.CurrentPalette().Dim.Render("thinking..."))
	a.layout.loader = a.loader
	a.history.SetOnInvalidate(host.RequestRender)
	if a.todoPanel != nil {
		a.todoPanel.SetOnInvalidate(host.RequestRender)
	}
}

// Host returns the AppHost backing this ChatApp.
func (a *ChatApp) Host() AppHost { return a.host }

// History returns the ChatHistory component.
func (a *ChatApp) History() *ChatHistory { return a.history }

// Editor returns the input Editor component.
func (a *ChatApp) Editor() *component.Editor { return a.editor }

// Loader returns the Loader component.
func (a *ChatApp) Loader() *component.Loader { return a.loader }

// Keybindings returns the keybindings manager.
func (a *ChatApp) Keybindings() *terminal.KeybindingsManager { return a.km }

// StatusBar returns the StatusBar component.
func (a *ChatApp) StatusBar() *component.StatusBar { return a.statusBar }

// UpdateStatusBar updates the status bar title after provider/model/mode switches.
func (a *ChatApp) UpdateStatusBar(provider, model, mode string) {
	if a.statusBar == nil {
		return
	}
	a.statusBar.SetMode(fmt.Sprintf("mady · %s/%s · %s", provider, model, mode))
}

// SetFooter sets the footer component on the layout.
func (a *ChatApp) SetFooter(f core.Component) {
	a.layout.footer = f
	if a.host != nil {
		a.host.RequestRender()
	}
}

// UpdateJudgmentView triggers a re-render of the judgment view. Call this
// after mutating the component returned by JudgmentView() to apply changes.
func (a *ChatApp) UpdateJudgmentView() {
	if a.judgmentView != nil {
		a.layout.updateJudgmentView()
	}
}

// JudgmentView returns the component so callers can configure it directly.
// After calling setters on the returned component, call UpdateJudgmentView()
// to apply the changes.
func (a *ChatApp) JudgmentView() *component.JudgmentView {
	return a.judgmentView
}

// Footer returns the footer component.
func (a *ChatApp) Footer() core.Component {
	return a.layout.footer
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Start begins the UI component lifecycle by adding the layout to the host
// and focusing the editor.
func (a *ChatApp) Start() error {
	a.host.AddChild(a.layout)
	if err := a.host.Start(); err != nil {
		return err
	}
	a.host.Focus(a.editor)
	return nil
}

// Stop stops the UI lifecycle.
func (a *ChatApp) Stop() error { return a.host.Stop() }

// Done returns a channel that is closed when the lifecycle ends.
func (a *ChatApp) Done() <-chan struct{} { return a.host.Done() }

// isRunning reports whether an agent run is in progress, derived purely
// from the FSM state (the old model.Running flag was removed in the FSM
// migration). Used by Ctrl+C to decide whether to fire OnInterrupt.
func (a *ChatApp) isRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch a.model.state {
	case StateStreaming, StateToolRunning, StateCompacting, StateAwaitingConfirm, StateInterrupted:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Inline confirmation
// ---------------------------------------------------------------------------

// ConfirmPending returns the active confirmation, or nil if none.
func (a *ChatApp) ConfirmPending() *InlineConfirm {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.model.confirm == nil {
		return nil
	}
	return &a.model.confirm.confirm
}

// StartConfirm shows an inline confirmation prompt and enters the
// ConfirmPending FSM state.
//
// Note: this whole confirm chain currently has no production caller —
// dispatchConfirmKey is only reachable from StateConfirmPending, which only
// StartConfirm can enter. The wiring is kept for the planned approval/undo
// flows; treat it as active code, not dead weight.
func (a *ChatApp) StartConfirm(ic InlineConfirm) {
	if ic.Timeout <= 0 {
		ic.Timeout = 10 * time.Second
	}
	a.mu.Lock()
	// A confirm prompt pauses any in-flight stream: end the current assistant
	// message so its Pending flag is cleared and a later delta starts fresh.
	a.finalizeStreamLocked()
	a.model.state = Transition(a.model.state, evtConfirmRequest)
	a.model.confirm = &confirmPending{
		confirm: ic,
		timer: time.AfterFunc(ic.Timeout, func() {
			a.confirmTimeout()
		}),
	}
	a.mu.Unlock()
	a.host.RequestRender()
}

// resolveConfirm resolves the pending confirmation: clears it, drives the
// FSM back to Idle via evtConfirmDecision, stops the timer (unless the timer
// itself fired — stopTimer=false), and invokes fire outside the lock with the
// resolved prompt so callbacks never run while holding a.mu.
func (a *ChatApp) resolveConfirm(stopTimer bool, fire func(ic *InlineConfirm)) {
	a.mu.Lock()
	c := a.model.confirm
	a.model.confirm = nil
	if a.model.state == StateConfirmPending {
		a.model.state = Transition(a.model.state, evtConfirmDecision)
	} else {
		// Defensive: resolving without a pending prompt (or after a
		// non-confirm state) must not leave the FSM in ConfirmPending.
		a.model.state = StateIdle
	}
	a.mu.Unlock()
	if c != nil {
		if stopTimer {
			c.timer.Stop()
		}
		if fire != nil {
			fire(&c.confirm)
		}
	}
	a.host.RequestRender()
}

// ConfirmYes resolves the pending confirmation with "yes".
func (a *ChatApp) ConfirmYes() {
	a.resolveConfirm(true, func(ic *InlineConfirm) {
		if ic.OnYes != nil {
			ic.OnYes()
		}
	})
}

// ConfirmNo resolves the pending confirmation with "no".
func (a *ChatApp) ConfirmNo() {
	a.resolveConfirm(true, func(ic *InlineConfirm) {
		if ic.OnNo != nil {
			ic.OnNo()
		}
	})
}

func (a *ChatApp) confirmTimeout() {
	a.resolveConfirm(false, func(ic *InlineConfirm) {
		if ic.OnNo != nil {
			ic.OnNo()
		}
	})
}

// ---------------------------------------------------------------------------
// Event subscription
// ---------------------------------------------------------------------------

// Subscribe registers all internal event handlers on the given subscriber.
func (a *ChatApp) Subscribe(sub EventSubscriber) {
	sub.On(ChatEventAgentStart, a.onAgentStart)
	sub.On(ChatEventMessageDelta, a.onMessageDelta)
	sub.On(ChatEventToolCallStart, a.onToolStart)
	sub.On(ChatEventToolCallEnd, a.onToolEnd)
	sub.On(ChatEventHandoffStart, a.onHandoffStart)
	sub.On(ChatEventHandoffEnd, a.onHandoffEnd)
	sub.On(ChatEventTurnStart, a.onTurnStart)
	sub.On(ChatEventTurnEnd, a.onTurnEnd)
	sub.On(ChatEventCompactionStart, a.onCompactionStart)
	sub.On(ChatEventCompactionEnd, a.onCompactionEnd)
	sub.On(ChatEventAutoRetry, a.onAutoRetry)
	sub.On(ChatEventAgentError, a.onAgentError)
	sub.On(ChatEventAgentEnd, a.onAgentEnd)
	sub.On(ChatEventAgentInterrupt, a.onAgentInterrupt)
	sub.On(ChatEventApprovalPrompt, a.onApprovalPrompt)
	sub.On(ChatEventTaskCreated, a.onTaskCreated)
	sub.On(ChatEventTaskUpdated, a.onTaskUpdated)
	sub.On(ChatEventPlanTaskStatusChanged, a.onPlanTaskStatusChanged)
	sub.On(ChatEventPlanTaskFeedbackAdded, a.onPlanTaskFeedbackAdded)
	sub.On(ChatEventPlanTaskInterrupted, a.onPlanTaskInterrupted)
}

// ---------------------------------------------------------------------------
// Key help overlay
// ---------------------------------------------------------------------------

// ToggleKeyHelp opens or closes the keybindings help overlay.
func (a *ChatApp) ToggleKeyHelp() OverlayRef {
	a.mu.Lock()
	if a.helpOverlay != nil {
		// Close path: capture the overlay under the lock, then call host
		// methods outside the lock. Calling host.RemoveOverlay/PushOverlay
		// under a.mu would acquire host.mu / TUI.mu; if any other path took
		// those locks first and then ChatApp.mu, we'd deadlock.
		ov := a.helpOverlay
		a.helpOverlay = nil
		editor := a.editor
		a.mu.Unlock()
		a.host.RemoveOverlay(ov)
		a.host.Focus(editor)
		return nil
	}
	help := component.NewKeyHelp(a.km)
	help.SetTitle("Keybindings — ↑↓ 翻页 · Esc 关闭")
	help.SetOnClose(func() { a.CloseKeyHelp() })
	ov := &overlayHandle{
		content:       help,
		focus:         true,
		dimBackground: true,
		category:      OverlayCatReview,
		widthPct:      70,
		heightPct:     70,
	}
	a.helpOverlay = ov
	a.mu.Unlock()
	a.host.PushOverlay(ov)
	return ov
}

// CloseKeyHelp closes the keybindings help overlay if open.
func (a *ChatApp) CloseKeyHelp() {
	a.mu.Lock()
	ov := a.helpOverlay
	a.helpOverlay = nil
	a.mu.Unlock()
	if ov != nil {
		a.host.RemoveOverlay(ov)
		a.host.Focus(a.editor)
	}
}

// ---------------------------------------------------------------------------
// Review gate helpers
// ---------------------------------------------------------------------------

// openReviewGateFromData constructs and opens a review gate overlay from
// typed payload data. The payload is already parsed by the adapter layer;
// this method simply maps it to ReviewGateData (with callbacks attached).
func (a *ChatApp) openReviewGateFromData(data *ReviewGatePayload) {
	if data == nil || data.Judgment == "" {
		return
	}
	gateData := ReviewGateData{
		Title:      data.Title,
		Judgment:   data.Judgment,
		Confidence: data.Confidence,
		Evidences:  data.Evidences,
		Checklist:  data.Checklist,
		Risks:      data.Risks,
		OnPass: func() {
			a.submitApprovalCommand("/approve")
		},
		OnBack: func() {
			a.submitApprovalCommand("请补充证据后重新分析")
		},
		OnBlock: func() {
			a.submitApprovalCommand("/reject 当前条件不满足，标记为阻塞")
		},
	}
	a.OpenReviewGate(gateData)
}

// submitApprovalCommand submits a command or text as user input.
// Used by review gate callbacks to communicate approval decisions.
func (a *ChatApp) submitApprovalCommand(cmd string) {
	a.CloseReviewGate()
	a.onEditorSubmit(cmd)
}
