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

	// Token accounting for the StatusBar indicator. usagePrompt/Completion
	// accumulate across turns within one agent run; turnStarted is set on
	// AgentStart so onTurnEnd can compute tok/s = completion / elapsed.
	usagePrompt     int64
	usageCompletion int64
	turnStarted     time.Time
}

type ChatApp struct {
	cfg ChatAppConfig

	host      AppHost
	history   *ChatHistory
	editor    *component.Editor
	loader    *component.Loader
	layout    *chatLayout
	header    *component.TruncatedText
	statusBar *component.StatusBar
	ac        *component.Autocomplete
	km        *terminal.KeybindingsManager

	mu    sync.Mutex
	model chatModel

	helpOverlay OverlayRef

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
	if cfg.EditorMinRows <= 0 {
		cfg.EditorMinRows = 1
	}
	if cfg.EditorMaxRows <= 0 {
		cfg.EditorMaxRows = 8
	}
	if cfg.EditorPrompt == "" {
		cfg.EditorPrompt = "> "
	}

	km := terminal.NewKeybindingsManager(terminal.DefaultKeybindings())

	history := NewChatHistory()
	if cfg.Theme != nil {
		history.SetTheme(*cfg.Theme)
	}
	if cfg.ReasoningRenderer != nil {
		history.SetReasoningRenderer(cfg.ReasoningRenderer)
	}

	editor := component.NewEditor(km)
	editor.SetMinRows(cfg.EditorMinRows)
	editor.SetMaxRows(cfg.EditorMaxRows)
	editor.SetPrompt(cfg.EditorPrompt, strings.Repeat(" ", len(cfg.EditorPrompt)))
	editor.SetFocusIndicator("")
	editor.SetPlaceholder("Type a message...")
	editor.SetPlaceholderFn(func(s string) string { return theme.CurrentPalette().Dim.Render(s) })

	loader := component.NewLoader(func() {}, theme.CurrentPalette().Dim.Render("thinking..."))

	statusBar := component.NewStatusBar()
	if cfg.Title != "" {
		statusBar.SetMode(cfg.Title)
	}

	chatApp := &ChatApp{
		cfg:       cfg,
		host:      cfg.Host,
		history:   history,
		editor:    editor,
		loader:    loader,
		statusBar: statusBar,
		km:        km,
		model:     chatModel{ActiveTools: make(map[string]time.Time)},
	}

	var header *component.TruncatedText
	if cfg.Title != "" {
		header = component.NewTruncatedText(theme.CurrentPalette().User.Render(cfg.Title))
	}
	chatApp.header = header

	if len(cfg.Providers) > 0 {
		chatApp.ac = component.NewAutocomplete(cfg.Providers...)
		chatApp.ac.OnApply(func(newValue string, _ int64, _ core.Suggestion) {
			chatApp.skipRefresh = true
			chatApp.editor.SetValue(newValue)
			// Force a refresh only when switching to a file/folder browser so
			// that cascading providers activate immediately.
			if strings.HasPrefix(newValue, "@file:") || strings.HasPrefix(newValue, "@folder:") {
				chatApp.ac.Refresh(newValue, int64(len(newValue)))
			}
			chatApp.host.Focus(chatApp.editor)
			chatApp.host.RequestRender()
		})
		chatApp.ac.OnDismiss(func() {
			// File browser: ESC navigates up one directory level instead of
			// dismissing outright. At the root, dismiss normally.
			value := chatApp.editor.GetValue()
			if (strings.HasPrefix(value, "@file:") || strings.HasPrefix(value, "@folder:")) &&
				len(value) > len("@file:") {
				// Remove the last path segment (file or directory).
				newValue := popLastPathSegment(value)
				chatApp.editor.SetValue(newValue)
				chatApp.ac.Refresh(newValue, int64(len(newValue)))
			}
			chatApp.host.RequestRender()
		})
	}

	layout := &chatLayout{
		host:      chatApp,
		app:       chatApp,
		history:   history,
		editor:    editor,
		loader:    loader,
		statusBar: statusBar,
		ac:        chatApp.ac,
	}
	if header != nil {
		layout.header = header
	}
	chatApp.layout = layout

	editor.OnChange(func(value string) {
		if chatApp.ac != nil {
			if chatApp.skipRefresh {
				chatApp.skipRefresh = false
			} else {
				chatApp.ac.Refresh(value, int64(len(value)))
			}
			chatApp.host.RequestRender()
		}
	})
	editor.OnSubmit(func(value string) {
		chatApp.onEditorSubmit(value)
	})
	editor.OnCancel(func() {
		if cfg.OnQuit != nil {
			cfg.OnQuit()
		}
		chatApp.Stop()
	})

	history.SetOnCopy(func(text string) {
		go func() {
			if err := CopyToClipboard(text); err != nil {
				chatApp.PrintError(fmt.Errorf("clipboard: %w", err))
			}
		}()
	})

	return chatApp
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
	help.SetTitle("Keybindings — Esc to close")
	ov := &overlayHandle{
		content:       help,
		focus:         true,
		dimBackground: true,
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

// OverlayOpts configures an overlay opened via OpenOverlay.
type OverlayOpts struct {
	WidthPct  int  // percentage of terminal width (0 = default 60)
	HeightPct int  // percentage of terminal height (0 = default 60)
	Dim       bool // dim the background while open
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

type overlayHandle struct {
	content       core.Component
	focus         bool
	dimBackground bool
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

func (a *ChatApp) finalizeStreamLocked() {
	id := a.model.StreamID
	a.model.StreamID = ""
	if id != "" {
		a.history.Finalize(id)
	}
}
