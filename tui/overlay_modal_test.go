package tui

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

// modalProbe is an Updatable child that counts received messages.
type modalProbe struct {
	core.MsgBase
	msgs atomic.Int64
}

func (p *modalProbe) Update(msg core.Msg) core.Cmd {
	if _, ok := msg.(core.KeyMsg); ok {
		p.msgs.Add(1)
	}
	return nil
}
func (p *modalProbe) Render(width int64) []string { return []string{"probe"} }
func (p *modalProbe) Invalidate()                 {}

// overlayProbe is the overlay content: it also counts keys and can hold focus.
type overlayProbe struct {
	modalProbe
	overlayRef *Overlay
	focused    atomic.Bool
}

func (o *overlayProbe) SetFocused(f bool) { o.focused.Store(f) }
func (o *overlayProbe) IsFocused() bool   { return o.focused.Load() }

func runModalTUI(t *testing.T) (*TUI, *modalProbe, *overlayProbe, *Overlay) {
	t.Helper()
	term := terminal.NewVirtualTerminal(80, 24)
	app := NewTUI(term)
	probe := &modalProbe{}
	app.AddChild(probe)

	ovContent := &overlayProbe{}
	ov := &Overlay{
		Content: ovContent,
		Focus:   true,
		Width:   OverlaySize{Value: 40},
		Height:  OverlaySize{Value: 10},
	}
	app.PushOverlay(ov)

	done := app.Done()
	go func() { _ = app.Start() }()
	waitTUIStarted(t, app)
	t.Cleanup(func() {
		app.Stop()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Log("tui stop timed out")
		}
	})
	return app, probe, ovContent, ov
}

// TestOverlayModalBlocksBackground verifies the historical default: a modal
// overlay (NonModal=false) stops key events from reaching background children.
func TestOverlayModalBlocksBackground(t *testing.T) {
	app, probe, ovContent, _ := runModalTUI(t)
	if !ovContent.focused.Load() {
		t.Fatal("overlay content did not receive focus")
	}
	app.SendMsg(core.KeyMsg{Data: "a"})
	waitFor(t, 3*time.Second, func() bool { return ovContent.msgs.Load() == 1 },
		"overlay content to process the key")

	if got := probe.msgs.Load(); got != 0 {
		t.Errorf("background probe received %d keys with modal overlay, want 0", got)
	}
	if got := ovContent.msgs.Load(); got != 1 {
		t.Errorf("overlay content received %d keys, want 1", got)
	}
}

// TestOverlayNonModalReachesBackground verifies that NonModal=true lets key
// events reach background children in addition to the focused overlay.
func TestOverlayNonModalReachesBackground(t *testing.T) {
	app, probe, ovContent, ov := runModalTUI(t)

	// Reopen as non-modal: pop and push with NonModal set.
	app.RemoveOverlay(ov)
	ov.NonModal = true
	app.PushOverlay(ov)
	waitFor(t, 3*time.Second, func() bool { return ovContent.focused.Load() },
		"overlay content to regain focus after reopening non-modal")

	app.SendMsg(core.KeyMsg{Data: "b"})
	waitFor(t, 3*time.Second, func() bool { return ovContent.msgs.Load() == 1 },
		"overlay content to process the key")

	if got := ovContent.msgs.Load(); got != 1 {
		t.Errorf("overlay content received %d keys, want 1 (focused path first)", got)
	}
	if got := probe.msgs.Load(); got != 1 {
		t.Errorf("background probe received %d keys with non-modal overlay, want 1", got)
	}
}

// mouseProbe records the last MouseMsg it receives (screen coordinates).
// Guarded by mu: Update runs on the event-loop goroutine while the test
// goroutine reads last.
type mouseProbe struct {
	core.MsgBase
	mu   sync.Mutex
	last core.MouseMsg
}

func (m *mouseProbe) Update(msg core.Msg) core.Cmd {
	if mm, ok := msg.(core.MouseMsg); ok {
		m.mu.Lock()
		m.last = mm
		m.mu.Unlock()
	}
	return nil
}
func (m *mouseProbe) Render(width int64) []string { return []string{"bg"} }
func (m *mouseProbe) Invalidate()                 {}

func (m *mouseProbe) LastMouse() core.MouseMsg {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}

// TestOverlayNonModalMouseUsesScreenCoords verifies that mouse events
// broadcast to background components under a non-modal overlay carry the
// original screen-absolute coordinates, not the overlay-local translation
// delivered to the focused overlay content.
func TestOverlayNonModalMouseUsesScreenCoords(t *testing.T) {
	app, _, ovContent, ov := runModalTUI(t)

	bg := &mouseProbe{}
	app.AddChild(bg)

	app.RemoveOverlay(ov)
	ov.NonModal = true
	app.PushOverlay(ov)
	waitFor(t, 3*time.Second, func() bool { return ovContent.focused.Load() },
		"overlay content to regain focus after reopening non-modal")

	// Overlay anchored at (0,0) sized 40x10: a click at screen (5,5) is
	// inside the overlay; the background must receive (5,5), not (0,0)-local.
	app.SendMsg(core.MouseMsg{Action: core.MousePress, Row: 5, Col: 5, Button: 1})
	waitFor(t, 3*time.Second, func() bool { return bg.LastMouse().Row == 5 },
		"background to receive the mouse event")

	if got := bg.LastMouse(); got.Row != 5 || got.Col != 5 {
		t.Errorf("background mouse = (%d,%d), want screen coords (5,5)", got.Row, got.Col)
	}
}
