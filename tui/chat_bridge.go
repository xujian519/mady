package tui

import (
	"sync"
	"time"

	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/terminal"
)

func NewChatApp(cfg chat.ChatAppConfig) *chat.ChatApp {
	term := terminal.NewProcessTerminal()
	if cfg.KittyKeyboardMode != "" {
		term.SetKittyKeyboardMode(cfg.KittyKeyboardMode)
	}
	if cfg.KittyKeyboardFlags > 0 {
		term.SetKittyKeyboardFlags(cfg.KittyKeyboardFlags)
	}
	km := terminal.NewKeybindingsManager(terminal.DefaultKeybindings())
	opts := TUIOptions{
		Keybindings:           km,
		DisableBracketedPaste: cfg.DisableBracketedPaste,
	}
	if cfg.AltScreen {
		opts.AltScreen = true
	}
	if cfg.MouseMode != "" {
		opts.MouseMode = cfg.MouseMode
	}
	app := NewTUI(term, opts)
	host := &tuiAppHost{TUI: app}
	cfg.Host = host
	// Wire the TUI's lifecycle context so OnSubmit submissions are canceled
	// when the TUI stops (instead of receiving an un-cancellable
	// context.Background()).
	if cfg.Context == nil {
		cfg.Context = app.Context()
	}
	chatApp := chat.NewChatAppWithHost(cfg, host)
	chatApp.SetHost(host)

	// Wire the debug overlay toggle (ctrl+shift+d).
	var debugOverlay *Overlay
	app.OnDebug = func() {
		if debugOverlay != nil {
			// Toggle off: remove the overlay and nil the reference.
			app.RemoveOverlay(debugOverlay)
			debugOverlay = nil
			return
		}
		// Toggle on: construct the debug overlay.
		tuiSrc := &tuiDebugSource{t: app}
		appSrc := &chatDebugSource{a: chatApp}
		d := component.NewDebugOverlay(tuiSrc, appSrc)
		d.SetOnClose(func() {
			app.RemoveOverlay(debugOverlay)
			debugOverlay = nil
		})
		debugOverlay = NewCenteredOverlay(d, 60, 60)
		debugOverlay.Focus = true
		debugOverlay.DimBackground = true
		app.PushOverlay(debugOverlay)
	}

	return chatApp
}

// tuiDebugSource adapts *TUI to component.DebugTUISource.
type tuiDebugSource struct {
	t *TUI
}

func (s *tuiDebugSource) MsgQueueDepth() int            { return s.t.MsgQueueDepth() }
func (s *tuiDebugSource) FrameStats() float64           { return s.t.FrameStats() }
func (s *tuiDebugSource) RecentEvents() []string        { return s.t.RecentEvents() }
func (s *tuiDebugSource) DebugAlloc() uint64            { return s.t.DebugAlloc() }
func (s *tuiDebugSource) TotalMsgCount() uint64         { return s.t.TotalMsgCount() }
func (s *tuiDebugSource) RenderDuration() time.Duration { return s.t.RenderDuration() }
func (s *tuiDebugSource) SlowFrameCount() uint64        { return s.t.SlowFrameCount() }

// chatDebugSource adapts *chat.ChatApp to component.DebugAppSource.
type chatDebugSource struct {
	a *chat.ChatApp
}

func (s *chatDebugSource) State() interface{ String() string } { return s.a.State() }

type tuiAppHost struct {
	*TUI
	mu       sync.Mutex
	overlays map[chat.OverlayRef]*Overlay
}

func (h *tuiAppHost) PushOverlay(ov chat.OverlayRef) {
	wantsFocus := ov.OverlayWantsFocus()
	dimBg := ov.OverlayDimBackground()
	anchor := OverlayAnchor(ov.OverlayAnchor())
	px := ov.OverlayPercentX()
	py := ov.OverlayPercentY()
	wp := ov.OverlayWidthPct()
	hp := ov.OverlayHeightPct()

	if px == 0 {
		px = 50
	}
	if py == 0 {
		py = 50
	}
	if wp == 0 {
		wp = 70
	}
	if hp == 0 {
		hp = 70
	}

	// Determine overlay category via optional interface. overlayHandle (in
	// chat) implements OverlayCategory() for this; other OverlayRef
	// implementations return 0 (OverlaySelection) by default.
	cat := OverlaySelection
	if categoriser, ok := ov.(interface{ OverlayCategory() int }); ok {
		switch categoriser.OverlayCategory() {
		case chat.OverlayCatSelection:
			cat = OverlaySelection
		case chat.OverlayCatReview:
			cat = OverlayReview
		case chat.OverlayCatGate:
			cat = OverlayGate
		case chat.OverlayCatSystem:
			cat = OverlaySystem
		}
	}

	o := &Overlay{
		Content:       ov.OverlayContent(),
		Category:      cat,
		Focus:         wantsFocus,
		DimBackground: dimBg,
		Anchor:        anchor,
		PercentX:      int64(px),
		PercentY:      int64(py),
		Width:         OverlaySize{Value: int64(wp), Percent: true, Min: 10},
		Height:        OverlaySize{Value: int64(hp), Percent: true, Min: 3},
	}
	ov.SetOverlayFocus(o.Focus)
	ov.SetOverlayDimBackground(o.DimBackground)
	h.mu.Lock()
	if h.overlays == nil {
		h.overlays = make(map[chat.OverlayRef]*Overlay)
	}
	h.overlays[ov] = o
	// PushOverlay must happen under h.mu so the overlays map and the TUI
	// overlay stack update atomically. Otherwise a concurrent RemoveOverlay
	// could find the map entry missing while the overlay was already pushed
	// to the TUI stack (leak), or vice-versa. Lock order h.mu -> TUI.mu is
	// consistent with RemoveOverlay below.
	h.TUI.PushOverlay(o)
	h.mu.Unlock()
}

func (h *tuiAppHost) RemoveOverlay(ov chat.OverlayRef) bool {
	h.mu.Lock()
	top := h.overlays[ov]
	if top != nil {
		delete(h.overlays, ov)
		// Mirror PushOverlay: do the TUI stack mutation under h.mu so the
		// map delete and stack removal stay in sync.
		removed := h.TUI.RemoveOverlay(top)
		h.mu.Unlock()
		return removed
	}
	h.mu.Unlock()
	return false
}

func (h *tuiAppHost) TerminalSize() (cols, rows int64) {
	return h.TUI.Terminal().Size()
}
