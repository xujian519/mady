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
	// TickInterval is the period of the ticker that drives flushPendingMotion
	// and periodic housekeeping. Defaults to 8ms. Note: it is NOT a frame-rate
	// cap — renderFrame runs after every processed message that requested a
	// render; this interval only bounds how often pending mouse-motion events
	// are flushed.
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

	// WindowTitle sets the terminal window title via OSC 0 sequence when
	// the TUI starts. Empty string (default) leaves the title unchanged.
	WindowTitle string
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

// WithWindowTitle sets the terminal window title displayed in the terminal
// tab or title bar. The title is set when the TUI starts (in Start).
// Example: WithWindowTitle("Mady — 审查意见答复")
func WithWindowTitle(title string) TUIOption {
	return func(o *TUIOptions) {
		o.WindowTitle = title
	}
}

// TUI is the top-level differential renderer.
type TUI struct {
	term    terminal.Terminal
	stdin   *terminal.StdinBuffer
	options TUIOptions
	km      *terminal.KeybindingsManager

	mu       sync.Mutex
	children []core.Component
	overlays []*Overlay
	focus    []core.Component // focus stack; top is the active target

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

	// kittyFlags captures the negotiated Kitty keyboard protocol flags from
	// the terminal at startup, so onKey can stamp them on every KeyMsg for
	// downstream CSI u parsing.
	kittyFlags int64

	// mouseThrottle guards MouseMotion events from flooding the event loop.
	// Trackpad scrolling can produce 60+ motion events per second; we coalesce
	// them to at most one per ~33ms (~30fps) to avoid saturating msgCh and
	// consuming CPU on wasteful re-renders at sub-frame granularity.
	mouseThrottle *time.Ticker
	mouseLast     time.Time

	// pendingMotion holds the most recent MouseMotion event that was
	// coalesced (throttled) instead of dispatched immediately. The event
	// loop flushes it on the next ticker tick so the final drag position
	// is never lost — this is a merge, not a drop. nil when no motion is
	// pending. Written by onMouse/onThrottledMotion on the terminal read
	// goroutine and read/cleared by flushPendingMotion on the event-loop
	// goroutine, so it is guarded by t.mu (see tui_input.go / tui_loop.go).
	pendingMotion *core.MouseMsg

	// lastCursor tracks the cursor state emitted in the previous frame.
	// renderFrame compares against it to avoid redundant ShowCursor/HideCursor
	// and MoveTo commands — cursor blink timers are no longer reset every frame.
	lastCursor struct {
		visible  bool
		row, col int64
		first    bool // true before first frame emits cursor
	}

	// resizeThrottle guards against rapid SIGWINCH events. Terminal multiplexer
	// resize operations (tmux pane resize, window split drag) can produce 10+
	// resize events in under 100ms. The throttle coalesces them so only the
	// last resize within the window triggers a re-render.
	//
	// timer is accessed from two goroutines: processMsg (event loop) writes
	// it, and Stop (any goroutine) reads it to cancel pending timers. Both
	// use atomic.Load/Store to avoid a data race.
	resizeThrottle struct {
		timer atomic.Pointer[time.Timer] // 100ms debounce timer; nil when idle
	}

	// watchog monitors event-loop health. If processMsg blocks for more than
	// watchdogThreshold, a warning is logged to aid debugging stuck-TUI issues.
	watchdog struct {
		lastEvent atomic.Int64  // UnixNano timestamp; updated after every processMsg
		threshold time.Duration // default 5s
		triggered atomic.Bool   // true while a diagnostic is pending
	}

	// debugMetrics accumulates runtime diagnostics for the ctrl+shift+d
	// debug overlay. All fields are accessed under t.mu.
	frameStamps    [debugFrameCap]time.Time // circular buffer of frame timestamps
	frameHead      int                      // index of oldest entry in frameStamps
	frameRingCount int                      // valid entries in ring (capped at debugFrameCap)
	frameTotal     uint64                   // total frames rendered (for periodic sampling)
	msgCount       uint64                   // total messages processed (incremented in processMsg)
	lastAlloc      uint64                   // last sampled heap allocation (bytes)
	lastFPS        float64                  // last computed FPS, updated each frame
	lastRenderDur  time.Duration            // most recent frame render duration
	slowFrameCount uint64                   // frames exceeding 16ms budget
	eventLog       [debugEventCap]string    // ring buffer of recent event descriptions
	eventLogIdx    int                      // next write index in eventLog ring
}

// Metrics constants for the debug overlay.
const (
	debugFrameCap = 120 // ~2s ring at 60fps
	debugEventCap = 32  // recent event ring capacity
)

// MsgQueueDepth returns the current number of pending messages in msgCh.
func (t *TUI) MsgQueueDepth() int {
	return len(t.msgCh)
}

// FrameStats returns the current FPS computed from frame timestamps.
func (t *TUI) FrameStats() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastFPS
}

// RecentEvents returns a copy of the event-log ring buffer (most recent last).
func (t *TUI) RecentEvents() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(t.eventLog)
	out := make([]string, 0, n)
	// Walk the ring from oldest to newest.
	for i := 0; i < n; i++ {
		idx := (t.eventLogIdx + i) % n
		if t.eventLog[idx] != "" {
			out = append(out, t.eventLog[idx])
		}
	}
	return out
}

// DebugAlloc returns the last sampled heap allocation in bytes.
func (t *TUI) DebugAlloc() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastAlloc
}

// TotalMsgCount returns the total number of messages processed.
func (t *TUI) TotalMsgCount() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.msgCount
}

// RenderDuration returns the most recent frame render duration.
func (t *TUI) RenderDuration() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastRenderDur
}

// SlowFrameCount returns the total number of frames that exceeded the 16ms
// rendering budget since the TUI was created.
func (t *TUI) SlowFrameCount() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.slowFrameCount
}

// NewTUI constructs a TUI bound to term. It accepts an optional TUIOptions
// struct for configuration. This is the primary constructor — external
// callers should always use NewTUI.
//
// Example:
//
//	app := tui.NewTUI(term)                              // defaults
//	app := tui.NewTUI(term, tui.TUIOptions{AltScreen: true}) // customized
func NewTUI(term terminal.Terminal, opts ...TUIOptions) *TUI {
	var o TUIOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.TickInterval <= 0 {
		o.TickInterval = 8 * time.Millisecond
	}

	// In CI environments, disable interactive features that depend on a
	// real terminal — synchronized output, mouse capture, and alt screen
	// are all meaningless or harmful in a CI log.
	// Only override when the caller hasn't explicitly opted into these
	// features (tests that set AltScreen/MouseMode use mock terminals).
	if terminal.IsCIEnvironment() {
		o.DisableSynchronizedOutput = true
		if o.MouseMode == "" {
			o.MouseMode = "off"
		}
		// AltScreen: zero value is already false (safe default). When the
		// caller explicitly sets AltScreen:true we preserve it — tests
		// that need alt-screen features use mock terminals.
	}

	// OSC 8 超链接能力：按终端能力检测结果开关（未知/老终端默认关闭，
	// 链接退化为纯文本）。CurrentTerminalContext 惰性初始化，与主题层
	// 的颜色检测共用同一检测结果。
	if ok, _ := terminal.CurrentTerminalContext().SupportsOSC8Hyperlinks(); !ok {
		core.SetOSC8Enabled(false)
	}
	km := o.Keybindings
	if km == nil {
		km = terminal.GetGlobalKeybindings()
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &TUI{
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
	t.lastCursor.first = true
	t.watchdog.threshold = 5 * time.Second
	t.watchdog.lastEvent.Store(time.Now().UnixNano())
	return t
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

// RemoveChild removes the first occurrence of c. If c implements Disposable,
// Dispose is called after removal so the component can release resources.
func (t *TUI) RemoveChild(c core.Component) bool {
	t.mu.Lock()
	for i, ch := range t.children {
		if ch == c {
			t.children = append(t.children[:i], t.children[i+1:]...)
			t.mu.Unlock()
			if d, ok := c.(core.Disposable); ok {
				d.Dispose()
			}
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
