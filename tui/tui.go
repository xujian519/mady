package tui

// This file defines the TUI container: types (TUI, TUIOptions), functional
// options, the constructor, root-level child management, and accessors. The
// behavioral surface is split across sibling files by responsibility:
//   - tui_lifecycle.go — Start/Stop/Quit/Done/Context/Tick/Every
//   - tui_loop.go      — eventLoop (the lifecycle/render/input junction)
//   - tui_input.go     — processMsg, Cmd execution, input callbacks, mouse mode
//   - tui_render.go    — RequestRender, renderFrame, normalizeLine
//   - tui_focus.go     — focus stack + overlay stack
//   - overlay.go       — the Overlay data type + pure composition helpers

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	core "github.com/xujian519/mady/tui/core"
	terminal "github.com/xujian519/mady/tui/terminal"
)

// errTUIAlreadyStopped is returned by Start when called on a TUI that has
// already been Stop'd. A TUI is one-shot; construct a new one to restart.
var errTUIAlreadyStopped = errors.New("tui: Start called on a stopped TUI; construct a new TUI to restart")

// ---------------------------------------------------------------------------
// TUI — differential-rendering container.
//
// A TUI instance owns:
//   - A Terminal (real or virtual) for raw I/O.
//   - A set of child Components rendered top-to-bottom.
//   - A focus stack routing Update(Msg) to the focused component.
//   - An overlay stack for floating panels.
//   - A message channel for the Elm-Architecture-style event loop.
//
// All user interaction (keys, mouse, paste, resize) is delivered as Msg
// values through the Updatable interface. Components that implement
// Updatable receive messages and return optional Cmd values for
// asynchronous side effects.
// ---------------------------------------------------------------------------

// TUIOptions configures a TUI instance.
type TUIOptions struct {
	// TickInterval is the minimum time between frames when many renders are
	// requested in a burst. Defaults to 8ms (~125 fps) to ensure smooth
	// streaming output; increase to 16ms if CPU usage is a concern.
	TickInterval time.Duration

	// DisableBracketedPaste suppresses paste mode at start. Default is to enable.
	DisableBracketedPaste bool

	// DisableSynchronizedOutput suppresses CSI 2026 wrapping (useful when the
	// terminal doesn't support it — most modern terminals do, and ignoring
	// terminals ignore the sequence harmlessly).
	DisableSynchronizedOutput bool

	// AltScreen switches to the alternate screen buffer on Start and restores
	// the main screen on Stop. This prevents the TUI from polluting the
	// terminal scrollback and gives a clean "app" feel.
	AltScreen bool

	// MouseMode enables mouse event reporting. Supported values:
	//   "" or "off" — no mouse events (default).
	//   "x11"       — X11-style mouse tracking (basic click/wheel).
	//   "sgr"       — SGR-style mouse tracking (extended, preferred).
	//   "on" / "auto" — auto-detect best available (SGR if supported).
	//
	// When enabled, alternate scroll mode (DEC 1007) is disabled so that
	// the terminal sends real mouse wheel events instead of arrow-key
	// sequences.
	MouseMode string

	// Keybindings overrides the keybinding manager for this TUI (nil = global).
	Keybindings *terminal.KeybindingsManager

	// Filter is invoked before the TUI processes a Msg. The filter can
	// return any Msg which will then be handled instead of the original
	// event. If the filter returns nil, the event will be ignored entirely.
	// This is useful for intercepting quit events, implementing confirm
	// dialogs, or globally transforming messages.
	Filter func(c core.Component, msg core.Msg) core.Msg
}

// TUIOption is a functional option for configuring a TUI.
type TUIOption func(*TUIOptions)

// WithFilter supplies an event filter that will be invoked before the TUI
// processes a Msg. The filter can return any Msg which will then be handled
// instead of the original event. If the filter returns nil, the event will
// be ignored and the TUI will not process it.
func WithFilter(filter func(core.Component, core.Msg) core.Msg) TUIOption {
	return func(o *TUIOptions) {
		o.Filter = filter
	}
}

// WithTickInterval sets the minimum time between frames.
func WithTickInterval(d time.Duration) TUIOption {
	return func(o *TUIOptions) {
		o.TickInterval = d
	}
}

// WithoutBracketedPaste disables bracketed paste mode.
func WithoutBracketedPaste() TUIOption {
	return func(o *TUIOptions) {
		o.DisableBracketedPaste = true
	}
}

// WithoutSynchronizedOutput disables CSI 2026 synchronized output.
func WithoutSynchronizedOutput() TUIOption {
	return func(o *TUIOptions) {
		o.DisableSynchronizedOutput = true
	}
}

// WithKeybindings overrides the keybinding manager.
func WithKeybindings(km *terminal.KeybindingsManager) TUIOption {
	return func(o *TUIOptions) {
		o.Keybindings = km
	}
}

// WithAltScreen enables the alternate screen buffer.
func WithAltScreen() TUIOption {
	return func(o *TUIOptions) {
		o.AltScreen = true
	}
}

// WithMouse enables mouse event reporting. mode is one of "off", "x11",
// "sgr", "on", "auto" (empty = "off").
func WithMouse(mode string) TUIOption {
	return func(o *TUIOptions) {
		o.MouseMode = mode
	}
}

// TUI is the top-level differential renderer.
type TUI struct {
	term    terminal.Terminal
	stdin   *terminal.StdinBuffer
	options TUIOptions
	km      *terminal.KeybindingsManager

	mu         sync.Mutex
	children   []core.Component
	overlays   []*Overlay
	focus      []core.Component // focus stack; top is the active target
	focusCycle []core.Component // ordered list for Tab/Shift+Tab traversal

	renderRequested int64
	prevFrame       []core.Row
	prevRaw         []string // raw output strings, for fast line-level change detection
	prevWidth       int64
	firstFrame      bool
	started         bool

	// outMu guards terminal-output state (altScreenOn, mouseMode) that can be
	// mutated from public EnableMouse/DisableMouse calls concurrently with
	// Start/Stop.
	outMu       sync.Mutex
	altScreenOn bool
	mouseMode   string

	doneCh chan struct{}
	tickCh chan struct{}
	msgCh  chan core.Msg

	// stopped is set atomically BEFORE close(doneCh) in Stop. sendMsgSafe
	// checks it to decide whether to enqueue, which closes the TOCTOU window
	// that a pure channel-based check leaves: doneCh-closed and msgCh-writable
	// can both be ready in a select, letting Go pseudorandomly pick the send
	// even after Stop. The atomic flag is observed before the select, so a
	// send is impossible once stopped=true is published.
	stopped atomic.Bool

	// ctx is canceled when the TUI stops. It is the cancellation root for
	// Tick/Every/WithContext Cmds issued via the TUI's helper methods, so
	// background timers and long-running Cmds terminate promptly on Stop.
	ctx    context.Context
	cancel context.CancelFunc

	// OnDebug is invoked for ctrl+shift+d (if the terminal sends that chord).
	OnDebug func()

	// mouseThrottle guards MouseMotion events from flooding the event loop.
	// Trackpad scrolling can produce 60+ motion events per second; we coalesce
	// them to at most one per ~33ms (~30fps) to avoid saturating msgCh and
	// consuming CPU on wasteful re-renders at sub-frame granularity.
	mouseThrottle *time.Ticker
	mouseLast     time.Time
}

// NewTUI constructs a TUI bound to term.
// Accepts either the legacy TUIOptions struct or the new functional options.
func NewTUI(term terminal.Terminal, opts ...TUIOptions) *TUI {
	var o TUIOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.TickInterval <= 0 {
		o.TickInterval = 8 * time.Millisecond
	}
	km := o.Keybindings
	if km == nil {
		km = terminal.GetGlobalKeybindings()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &TUI{
		term:          term,
		stdin:         terminal.NewStdinBuffer(),
		options:       o,
		km:            km,
		firstFrame:    true,
		doneCh:        make(chan struct{}),
		tickCh:        make(chan struct{}, 1),
		msgCh:         make(chan core.Msg, 256),
		mouseThrottle: time.NewTicker(mouseThrottlePeriod), // ~33ms, max 30fps mouse motion
		ctx:           ctx,
		cancel:        cancel,
	}
}

// NewTUIWithOptions constructs a TUI using functional options.
func NewTUIWithOptions(term terminal.Terminal, opts ...TUIOption) *TUI {
	var o TUIOptions
	for _, opt := range opts {
		opt(&o)
	}
	return NewTUI(term, o)
}

// ---------------------------------------------------------------------------
// Children
// ---------------------------------------------------------------------------

// AddChild appends a root-level component.
func (t *TUI) AddChild(c core.Component) {
	if c == nil {
		return
	}
	t.mu.Lock()
	t.children = append(t.children, c)
	t.mu.Unlock()
	t.RequestRender()
}

// RemoveChild removes the first occurrence of c.
func (t *TUI) RemoveChild(c core.Component) bool {
	t.mu.Lock()
	for i, ch := range t.children {
		if ch == c {
			t.children = append(t.children[:i], t.children[i+1:]...)
			t.mu.Unlock()
			t.RequestRender()
			return true
		}
	}
	t.mu.Unlock()
	return false
}

// Children returns a snapshot of root-level children.
func (t *TUI) Children() []core.Component {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := make([]core.Component, len(t.children))
	copy(cp, t.children)
	return cp
}

// Keybindings returns the manager used by this TUI.
func (t *TUI) Keybindings() *terminal.KeybindingsManager { return t.km }

// Terminal returns the underlying Terminal.
func (t *TUI) Terminal() terminal.Terminal { return t.term }
