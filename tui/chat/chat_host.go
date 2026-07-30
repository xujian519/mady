package chat

import "github.com/xujian519/mady/tui/core"

// ---------------------------------------------------------------------------
// AppHost interface family (ISP split)
//
// AppHost is the combined interface that ChatApp needs from its host.
// It composes four smaller role interfaces for the Interface Segregation
// Principle — consumers that need only a subset (e.g. overlay or terminal
// access) can depend on the narrower interface.
// ---------------------------------------------------------------------------

// LifecycleHost manages the component's run loop lifecycle.
type LifecycleHost interface {
	Start() error
	Stop() error
	Done() <-chan struct{}
}

// ComponentHost manages the component tree (add/focus/repaint).
type ComponentHost interface {
	AddChild(c core.Component)
	Focus(c core.Component)
	RequestRender()
}

// OverlayHost manages modal overlay push/pop.
type OverlayHost interface {
	PushOverlay(ov OverlayRef)
	RemoveOverlay(ov OverlayRef) bool
}

// TerminalHost provides terminal capabilities.
type TerminalHost interface {
	TerminalSize() (cols, rows int64)
	// EnableMouse / DisableMouse toggle SGR mouse reporting at runtime.
	// Used to disable mouse when the editor is focused so the terminal's
	// native right-click menu can appear.
	EnableMouse(mode string)
	DisableMouse()
}

// AppHost composes the four narrower host interfaces that ChatApp needs.
type AppHost interface {
	LifecycleHost
	ComponentHost
	OverlayHost
	TerminalHost
}

// OverlayRef is the handle returned by overlay-opening methods. Callers hold
// the ref to later close the overlay via CloseOverlay or the host's
// RemoveOverlay. Implementations must be comparable (pointer types).
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
