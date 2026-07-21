package chat

// This file defines the ChatApp: the AppHost/OverlayRef interfaces it talks
// to, ChatAppConfig, the chatModel state it guards, the constructor, accessors,
// and the mutation façade (Print*/Busy/Idle/UpdateStatusBar). It also owns the
// key-help overlay (ToggleKeyHelp/CloseKeyHelp + overlayHandle) since that is
// ChatApp-level overlay state rather than layout.
//
// Event handlers are split by domain:
//   - chat_app_stream.go — editor submit, agent start/delta/end/error
//   - chat_app_tool.go   — tool calls, handoffs, turns, retry, compaction
//   - chat_app_layout.go — the chatLayout root component + input routing

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

type AppHost interface {
	Start() error
	Stop() error
	Done() <-chan struct{}

	AddChild(c core.Component)
	Focus(c core.Component)
	RequestRender()

	PushOverlay(ov OverlayRef)
	RemoveOverlay(ov OverlayRef) bool

	TerminalSize() (cols, rows int64)

	// EnableMouse / DisableMouse toggle SGR mouse reporting at runtime.
	// Used to disable mouse when the editor is focused so the terminal's
	// native right-click menu can appear.
	EnableMouse(mode string)
	DisableMouse()
}

type OverlayRef interface {
	OverlayContent() core.Component
	SetOverlayFocus(bool)
	SetOverlayDimBackground(bool)
	OverlayWantsFocus() bool
	OverlayDimBackground() bool
	OverlayAnchor() int
	OverlayPercentX() int
	OverlayPercentY() int
	OverlayWidthPct() int
	OverlayHeightPct() int
}

type ChatAppConfig struct {
	Title string

	KittyKeyboardMode     string
	KittyKeyboardFlags    int64
	DisableBracketedPaste bool
	AltScreen             bool
	MouseMode             string

	EditorMinRows int64
	EditorMaxRows int64
	EditorPrompt  string

	ShowTimings bool
	ShowTurns   bool

	// ContextWindow is the model's max context size in tokens, used to render
	// the StatusBar context-occupancy bar. 0 hides the bar.
	ContextWindow int64

	// ReasoningRenderer controls how thinking segments are displayed in the
	// chat history. Pass nil (default) to hide reasoning; pass a
	// *DefaultReasoningRenderer to restore the legacy Show/Mode policy; pass
	// a custom implementation for full control (sidebar, overlay, etc.).
	ReasoningRenderer ReasoningRenderer

	Theme *ChatHistoryTheme

	Providers []core.AutocompleteProvider

	// Context is the cancellation root passed to OnSubmit. When nil,
	// context.Background() is used. Callers should wire the TUI's lifecycle
	// context here so in-flight submissions are canceled on Stop.
	Context context.Context

	OnSubmit     func(ctx context.Context, input string)
	OnQuit       func()
	OnInterrupt  func()
	OnImagePaste func() // called when an image paste is detected (clipboard image, empty text)

	Host AppHost

	// SuppressHandoffToolDisplay when true suppresses transfer_to_* tool calls
	// from appearing in the chat history. Used in integrated mode where handoffs
	// are invisible to the user. In Router mode this should be false so users
	// can see the routing process.
	SuppressHandoffToolDisplay bool
}

type chatModel struct {
	StreamID    string
	ActiveTools map[string]time.Time
	Running     bool

	// state is the FSM's current interaction state. It is the single source
	// of truth for UI state display (JudgmentView, StatusBar). Handlers
	// call Transition(state, EventKindFor(e)) to compute the next state.
	// Initialized to StateInitializing at ChatApp creation.
	state AppState

	// Running, StreamID, ActiveTools are retained as compatibility shims
	// during the progressive refactor. New code should read/write state.
	// TODO(sprint-2): remove these after all handlers use Transition().

	// Token accounting for the StatusBar indicator. usagePrompt/Completion
	// accumulate across turns within one agent run; turnStarted is set on
	// AgentStart so onTurnEnd can compute tok/s = completion / elapsed.
	usagePrompt     int64
	usageCompletion int64
	turnStarted     time.Time

	// judgmentSummary carries the current judgment-bar snapshot. It is
	// populated during agent execution (approval prompts, interrupts) and
	// cleared on agent start.
	judgmentSummary JudgmentSummary
}

type ChatApp struct {
	cfg ChatAppConfig

	host         AppHost
	history      *ChatHistory
	editor       *component.Editor
	loader       *component.Loader
	layout       *chatLayout
	header       *component.TruncatedText
	statusBar    *component.StatusBar
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

	skipRefresh bool // suppress autocomplete re-activation after applying a suggestion
}

func NewChatApp(cfg ChatAppConfig) *ChatApp {
	return newChatApp(cfg)
}

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

	chatApp := &ChatApp{
		cfg:          cfg,
		host:         cfg.Host,
		history:      history,
		editor:       editor,
		loader:       loader,
		statusBar:    statusBar,
		km:           km,
		judgmentView: component.NewJudgmentView(),
		model: chatModel{
			state:       StateInitializing,
			ActiveTools: make(map[string]time.Time),
		},
	}

	chatApp.header = newChatHeader(cfg)
	chatApp.ac = newChatAutocomplete(cfg, chatApp)
	chatApp.layout = newChatLayout(cfg, chatApp, history, editor, loader, statusBar)
	bindChatEditorEvents(chatApp, editor, history)
	return chatApp
}

// applyChatDefaults fills in default values for a ChatAppConfig.
func applyChatDefaults(cfg ChatAppConfig) ChatAppConfig {
	if cfg.EditorMinRows <= 0 {
		cfg.EditorMinRows = 1
	}
	if cfg.EditorMaxRows <= 0 {
		cfg.EditorMaxRows = 8
	}
	if cfg.EditorPrompt == "" {
		cfg.EditorPrompt = "> "
	}
	return cfg
}

// newChatHistoryWithConfig creates a ChatHistory and applies theme/renderer.
func newChatHistoryWithConfig(cfg ChatAppConfig) *ChatHistory {
	history := NewChatHistory()
	if cfg.Theme != nil {
		history.SetTheme(*cfg.Theme)
	}
	if cfg.ReasoningRenderer != nil {
		history.SetReasoningRenderer(cfg.ReasoningRenderer)
	}
	return history
}

// newChatEditor creates the input editor with config-driven settings.
func newChatEditor(cfg ChatAppConfig, km *terminal.KeybindingsManager) *component.Editor {
	editor := component.NewEditor(km)
	editor.SetMinRows(cfg.EditorMinRows)
	editor.SetMaxRows(cfg.EditorMaxRows)
	editor.SetPrompt(cfg.EditorPrompt, strings.Repeat(" ", len(cfg.EditorPrompt)))
	editor.SetFocusIndicator("")
	editor.SetPlaceholder("输入消息…（/ 查看命令）")
	editor.SetPlaceholderFn(func(s string) string { return theme.CurrentPalette().Dim.Render(s) })
	return editor
}

// newChatHeader creates the title header component, or nil if no title is set.
func newChatHeader(cfg ChatAppConfig) *component.TruncatedText {
	if cfg.Title == "" {
		return nil
	}
	return component.NewTruncatedText(theme.CurrentPalette().User.Render(cfg.Title))
}

// newChatAutocomplete sets up autocomplete with file/folder navigation callbacks.
func newChatAutocomplete(cfg ChatAppConfig, a *ChatApp) *component.Autocomplete {
	if len(cfg.Providers) == 0 {
		return nil
	}
	ac := component.NewAutocomplete(cfg.Providers...)
	ac.OnApply(func(newValue string, _ int64, _ core.Suggestion) {
		a.skipRefresh = true
		a.editor.SetValue(newValue)
		if strings.HasPrefix(newValue, "@file:") || strings.HasPrefix(newValue, "@folder:") {
			a.ac.Refresh(newValue, int64(len(newValue)))
		}
		a.host.Focus(a.editor)
		a.host.RequestRender()
	})
	ac.OnDismiss(func() {
		value := a.editor.GetValue()
		if (strings.HasPrefix(value, "@file:") || strings.HasPrefix(value, "@folder:")) &&
			len(value) > len("@file:") {
			newValue := popLastPathSegment(value)
			a.editor.SetValue(newValue)
			a.ac.Refresh(newValue, int64(len(newValue)))
		}
		a.host.RequestRender()
	})
	return ac
}

// newChatLayout builds the layout tree for the chat app.
func newChatLayout(cfg ChatAppConfig, a *ChatApp, history *ChatHistory, editor *component.Editor, loader *component.Loader, statusBar *component.StatusBar) *chatLayout {
	layout := &chatLayout{
		host:          a,
		app:           a,
		history:       history,
		judgmentView:  a.judgmentView,
		editor:        editor,
		loader:        loader,
		statusBar:     statusBar,
		ac:            a.ac,
		editorMaxRows: cfg.EditorMaxRows,
	}
	if a.header != nil {
		layout.header = a.header
	}
	return layout
}

// bindChatEditorEvents wires all editor and history event callbacks.
func bindChatEditorEvents(a *ChatApp, editor *component.Editor, history *ChatHistory) {
	if a.ac != nil {
		editor.SetAutocompleteActiveCheck(a.ac.Active)
	}

	editor.OnChange(func(value string) {
		if a.ac != nil {
			if a.skipRefresh {
				a.skipRefresh = false
			} else {
				a.ac.Refresh(value, int64(len(value)))
			}
			a.host.RequestRender()
		}
	})
	editor.OnSubmit(func(value string) {
		a.onEditorSubmit(value)
	})
	editor.OnCopy(func(text string) {
		go func() {
			if err := CopyToClipboard(text); err != nil {
				a.PrintError(fmt.Errorf("clipboard: %w", err))
				return
			}
			runeCount := utf8.RuneCountInString(text)
			a.PrintSystem(fmt.Sprintf("📋 已复制（%d 字符）", runeCount))
		}()
	})
	editor.OnPaste(func() core.Cmd {
		return func() core.Msg {
			text, err := ReadFromClipboard()
			if err != nil {
				a.PrintError(fmt.Errorf("paste: %w", err))
				a.host.RequestRender()
				return nil
			}
			return core.PasteMsg{Text: text}
		}
	})
	editor.OnCancel(func() {
		if a.cfg.OnQuit != nil {
			a.cfg.OnQuit()
		}
		a.Stop()
	})

	history.SetOnCopy(func(text string) {
		go func() {
			if err := CopyToClipboard(text); err != nil {
				a.PrintError(fmt.Errorf("clipboard: %w", err))
			}
		}()
	})
}

func (a *ChatApp) SetHost(host AppHost) {
	a.host = host
	a.loader = component.NewLoader(host.RequestRender, theme.CurrentPalette().Dim.Render("thinking..."))
	a.layout.loader = a.loader
	a.history.SetOnInvalidate(host.RequestRender)
}

func (a *ChatApp) Host() AppHost { return a.host }

func (a *ChatApp) History() *ChatHistory { return a.history }

func (a *ChatApp) Editor() *component.Editor { return a.editor }

func (a *ChatApp) Loader() *component.Loader { return a.loader }

func (a *ChatApp) Keybindings() *terminal.KeybindingsManager { return a.km }

func (a *ChatApp) StatusBar() *component.StatusBar { return a.statusBar }

// UpdateStatusBar updates the status bar title after provider/model/mode switches.
func (a *ChatApp) UpdateStatusBar(provider, model, mode string) {
	if a.statusBar == nil {
		return
	}
	a.statusBar.SetMode(fmt.Sprintf("mady · %s/%s · %s", provider, model, mode))
}

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

func (a *ChatApp) Footer() core.Component {
	return a.layout.footer
}

func (a *ChatApp) Start() error {
	a.host.AddChild(a.layout)
	if err := a.host.Start(); err != nil {
		return err
	}
	a.host.Focus(a.editor)
	return nil
}

func (a *ChatApp) Stop() error { return a.host.Stop() }

func (a *ChatApp) Done() <-chan struct{} { return a.host.Done() }

func (a *ChatApp) isRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model.Running
}

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
}

func (a *ChatApp) PrintSystem(msg string) {
	a.history.Append(ChatMessage{Role: RoleSystem, Text: msg})
}

func (a *ChatApp) PrintError(err error) {
	if err == nil {
		return
	}
	a.history.Append(ChatMessage{Role: RoleError, Text: err.Error()})
}

func (a *ChatApp) PrintUser(input string) {
	a.history.Append(ChatMessage{Role: RoleUser, Text: input})
}

func (a *ChatApp) Busy(message string) {
	if message != "" {
		a.loader.SetMessage(theme.CurrentPalette().Dim.Render(message))
	}
	a.loader.Start()
	a.mu.Lock()
	a.model.Running = true
	a.mu.Unlock()
	if a.statusBar != nil {
		a.statusBar.Busy()
	}
	a.editor.SetPlaceholder("Ctrl+C to interrupt")
	a.host.RequestRender()
}

func (a *ChatApp) Idle() {
	a.loader.Stop()
	a.mu.Lock()
	a.model.Running = false
	a.mu.Unlock()
	if a.statusBar != nil {
		a.statusBar.Idle()
	}
	a.editor.SetPlaceholder("")
	a.host.RequestRender()
}

// SetJudgmentSummary populates the judgment-bar summary from the provided
// snapshot and triggers a render. Called by event handlers when structured
// judgment data arrives (e.g. review gate, approval prompt).
func (a *ChatApp) SetJudgmentSummary(s JudgmentSummary) {
	a.mu.Lock()
	a.model.judgmentSummary = s
	a.mu.Unlock()
	a.layout.updateJudgmentView()
}

// MarkAgentReady signals that agent initialization has completed and
// transitions the FSM from StateInitializing to StateIdle. Called after
// initializeAgentAsync successfully binds the agent to the ChatApp.
func (a *ChatApp) MarkAgentReady() {
	a.mu.Lock()
	a.model.state = Transition(a.model.state, evtAgentReady)
	a.mu.Unlock()
	a.layout.updateJudgmentView()
}

// MarkAgentFailed signals that agent initialization hit a terminal error and
// transitions the FSM to StateFailed. Called from initializeAgentAsync's
// error recovery path so the UI reflects the failed state.
func (a *ChatApp) MarkAgentFailed() {
	a.mu.Lock()
	a.model.state = Transition(a.model.state, evtAgentError)
	a.mu.Unlock()
	a.layout.updateJudgmentView()
}

// ClearJudgmentSummary resets the judgment-bar to idle. Called when a new
// agent run starts so stale data from a previous run doesn't leak.
func (a *ChatApp) ClearJudgmentSummary() {
	a.mu.Lock()
	a.model.judgmentSummary = JudgmentSummary{}
	a.mu.Unlock()
	a.layout.updateJudgmentView()
}

func (a *ChatApp) PrintStatus(message string) {
	a.loader.SetMessage(theme.CurrentPalette().Dim.Render(message))
	a.host.RequestRender()
}

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

// ReviewGateData carries the structured data needed to render a review gate overlay.
// See component.ReviewGate for field descriptions.
type ReviewGateData struct {
	Title      string
	Judgment   string
	Confidence float64
	Evidences  []component.ReviewEvidence
	Checklist  []component.ReviewCheckItem
	Risks      []string
	OnPass     func()
	OnBack     func()
	OnBlock    func()
}

// OpenReviewGate opens a review gate overlay with the given data.
// Lock discipline: a.mu is NOT held during PushOverlay (see ToggleKeyHelp).
func (a *ChatApp) OpenReviewGate(data ReviewGateData) OverlayRef {
	a.mu.Lock()
	if a.reviewGateOverlay != nil {
		// Close existing review gate before opening a new one.
		ov := a.reviewGateOverlay
		a.reviewGateOverlay = nil
		a.mu.Unlock()
		a.host.RemoveOverlay(ov)
		a.mu.Lock()
	}

	rg := component.NewReviewGate(data.Judgment, data.Confidence, data.Evidences, data.Checklist, data.Risks)
	if data.Title != "" {
		rg.SetTitle(data.Title)
	}
	rg.SetKeybindings(a.km)
	if data.OnPass != nil {
		rg.SetOnPass(data.OnPass)
	}
	if data.OnBack != nil {
		rg.SetOnBack(data.OnBack)
	}
	if data.OnBlock != nil {
		rg.SetOnBlock(data.OnBlock)
	}
	rg.SetOnClose(func() { a.CloseReviewGate() })

	ov := &overlayHandle{
		content:       rg,
		focus:         true,
		dimBackground: true,
		category:      OverlayCatGate,
		widthPct:      70,
		heightPct:     75,
	}
	a.reviewGateOverlay = ov
	a.mu.Unlock()
	a.host.PushOverlay(ov)
	a.host.RequestRender()
	return ov
}

// CloseReviewGate closes the review gate overlay if open.
func (a *ChatApp) CloseReviewGate() {
	a.mu.Lock()
	ov := a.reviewGateOverlay
	a.reviewGateOverlay = nil
	a.mu.Unlock()
	if ov != nil {
		a.host.RemoveOverlay(ov)
		a.host.Focus(a.editor)
	}
}

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

// SystemStatusData carries the structured data needed to render a system
// status overlay. See component.SysEvent for event field descriptions.
type SystemStatusData struct {
	Mode       string
	ModeReason string
	Events     []component.SysEvent
	Impacts    []string
}

// OpenSystemStatus opens a system status overlay with the given data.
// Lock discipline: a.mu is NOT held during PushOverlay (see ToggleKeyHelp).
func (a *ChatApp) OpenSystemStatus(data SystemStatusData) OverlayRef {
	a.mu.Lock()
	if a.systemStatusOverlay != nil {
		ov := a.systemStatusOverlay
		a.systemStatusOverlay = nil
		a.mu.Unlock()
		a.host.RemoveOverlay(ov)
		a.mu.Lock()
	}

	ss := component.NewSystemStatus()
	ss.SetMode(data.Mode, data.ModeReason)
	ss.SetEvents(data.Events)
	ss.SetImpacts(data.Impacts)
	ss.SetKeybindings(a.km)
	ss.SetOnClose(func() { a.CloseSystemStatus() })

	ov := &overlayHandle{
		content:       ss,
		focus:         true,
		dimBackground: true,
		category:      OverlayCatSystem,
		widthPct:      50,
		heightPct:     40,
	}
	a.systemStatusOverlay = ov
	a.mu.Unlock()
	a.host.PushOverlay(ov)
	a.host.RequestRender()
	return ov
}

// CloseSystemStatus closes the system status overlay if open.
func (a *ChatApp) CloseSystemStatus() {
	a.mu.Lock()
	ov := a.systemStatusOverlay
	a.systemStatusOverlay = nil
	a.mu.Unlock()
	if ov != nil {
		a.host.RemoveOverlay(ov)
		a.host.Focus(a.editor)
	}
}

// EvidenceOverlayData carries evidence items to display in the evidence
// details overlay. Maps to component.EvidenceItem.
type EvidenceOverlayData struct {
	Items []component.EvidenceItem
}

// OpenEvidenceOverlay opens the evidence details overlay.
// Lock discipline: a.mu is NOT held during PushOverlay (see ToggleKeyHelp).
func (a *ChatApp) OpenEvidenceOverlay(data EvidenceOverlayData) OverlayRef {
	a.mu.Lock()
	if a.evidenceOverlay != nil {
		ov := a.evidenceOverlay
		a.evidenceOverlay = nil
		a.mu.Unlock()
		a.host.RemoveOverlay(ov)
		a.mu.Lock()
	}

	eo := component.NewEvidenceOverlay()
	if len(data.Items) > 0 {
		eo.SetItems(data.Items)
	}
	eo.SetOnClose(func() { a.CloseEvidenceOverlay() })

	ov := &overlayHandle{
		content:       eo,
		focus:         true,
		dimBackground: true,
		category:      OverlayCatReview,
		widthPct:      60,
		heightPct:     60,
	}
	a.evidenceOverlay = ov
	a.mu.Unlock()
	a.host.PushOverlay(ov)
	a.host.RequestRender()
	return ov
}

// CloseEvidenceOverlay closes the evidence details overlay if open.
func (a *ChatApp) CloseEvidenceOverlay() {
	a.mu.Lock()
	ov := a.evidenceOverlay
	a.evidenceOverlay = nil
	a.mu.Unlock()
	if ov != nil {
		a.host.RemoveOverlay(ov)
		a.host.Focus(a.editor)
	}
}

// OverlayCategory constants — these match the categories in the tui package
// but are defined here so the chat package does not import tui directly.
// The bridge (chat_bridge.go in package tui) maps them via type assertion.
const (
	OverlayCatSelection = iota // 选择型
	OverlayCatReview           // 审阅型
	OverlayCatGate             // 复核型
	OverlayCatSystem           // 系统型
)

// OverlayOpts configures an overlay opened via OpenOverlay.
type OverlayOpts struct {
	WidthPct  int  // percentage of terminal width (0 = default 60)
	HeightPct int  // percentage of terminal height (0 = default 60)
	Dim       bool // dim the background while open
	Category  int  // overlay category (OverlayCat*), 0 = selection
}

// OpenOverlay mounts an arbitrary content component as a centered, focused
// overlay and returns a handle the caller can pass to CloseOverlay. This is
// the public entry point for app-level panels (settings, session picker, …)
// that the keyhelp-specific ToggleKeyHelp does not cover.
func (a *ChatApp) OpenOverlay(content core.Component, opts OverlayOpts) OverlayRef {
	if opts.WidthPct == 0 {
		opts.WidthPct = 60
	}
	if opts.HeightPct == 0 {
		opts.HeightPct = 60
	}
	ov := &overlayHandle{
		content:       content,
		focus:         true,
		dimBackground: opts.Dim,
		category:      opts.Category,
		widthPct:      opts.WidthPct,
		heightPct:     opts.HeightPct,
	}
	// Push the overlay outside any ChatApp lock to avoid the lock-ordering
	// hazard documented in ToggleKeyHelp (host.PushOverlay takes host.mu /
	// TUI.mu; ChatApp.mu must never be held across that call).
	a.host.PushOverlay(ov)
	a.host.RequestRender()
	return ov
}

// CloseOverlay removes an overlay previously opened via OpenOverlay (or any
// other OverlayRef returned by the host) and restores focus to the editor.
func (a *ChatApp) CloseOverlay(ov OverlayRef) {
	if ov == nil {
		return
	}
	a.host.RemoveOverlay(ov)
	a.host.Focus(a.editor)
}

// OpenSelectionOverlay opens a selection-type overlay for quick object
// switching (sessions, threads, branches). Default size: 40/30.
func (a *ChatApp) OpenSelectionOverlay(content core.Component) OverlayRef {
	return a.OpenOverlay(content, OverlayOpts{
		WidthPct:  40,
		HeightPct: 30,
		Dim:       true,
		Category:  OverlayCatSelection,
	})
}

// OpenReviewOverlay opens a review-type overlay for viewing details
// (evidence, citations, keybindings). Default size: 60/60.
func (a *ChatApp) OpenReviewOverlay(content core.Component) OverlayRef {
	return a.OpenOverlay(content, OverlayOpts{
		WidthPct:  60,
		HeightPct: 60,
		Dim:       true,
		Category:  OverlayCatReview,
	})
}

// OpenGateOverlay opens a gate-type overlay for structured review
// (review gates, high-risk confirmations). Default size: 70/75.
func (a *ChatApp) OpenGateOverlay(content core.Component) OverlayRef {
	return a.OpenOverlay(content, OverlayOpts{
		WidthPct:  70,
		HeightPct: 75,
		Dim:       true,
		Category:  OverlayCatGate,
	})
}

// OpenSystemOverlay opens a system-type overlay for runtime condition
// explanation (degraded mode, blocked, logs). Default size: 50/40.
func (a *ChatApp) OpenSystemOverlay(content core.Component) OverlayRef {
	return a.OpenOverlay(content, OverlayOpts{
		WidthPct:  50,
		HeightPct: 40,
		Dim:       true,
		Category:  OverlayCatSystem,
	})
}

type overlayHandle struct {
	content       core.Component
	focus         bool
	dimBackground bool
	category      int // OverlayCat* constant, 0 = selection
	anchor        int
	percentX      int
	percentY      int
	widthPct      int
	heightPct     int
}

func (o *overlayHandle) OverlayContent() core.Component { return o.content }
func (o *overlayHandle) SetOverlayFocus(v bool)         { o.focus = v }
func (o *overlayHandle) SetOverlayDimBackground(v bool) { o.dimBackground = v }
func (o *overlayHandle) OverlayWantsFocus() bool        { return o.focus }
func (o *overlayHandle) OverlayDimBackground() bool     { return o.dimBackground }
func (o *overlayHandle) OverlayAnchor() int             { return o.anchor }
func (o *overlayHandle) OverlayPercentX() int           { return o.percentX }
func (o *overlayHandle) OverlayPercentY() int           { return o.percentY }
func (o *overlayHandle) OverlayWidthPct() int           { return o.widthPct }
func (o *overlayHandle) OverlayHeightPct() int          { return o.heightPct }

// OverlayCategory returns the category code for bridge propagation.
// This is NOT part of the OverlayRef interface — it is detected via type
// assertion in tuiAppHost.PushOverlay so the interface stays backward
// compatible.
func (o *overlayHandle) OverlayCategory() int { return o.category }

func (a *ChatApp) finalizeStreamLocked() {
	id := a.model.StreamID
	a.model.StreamID = ""
	if id != "" {
		a.history.Finalize(id)
	}
}
