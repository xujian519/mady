package tui

import (
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
	time.Sleep(50 * time.Millisecond)
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
	time.Sleep(80 * time.Millisecond)

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
	time.Sleep(20 * time.Millisecond)

	app.SendMsg(core.KeyMsg{Data: "b"})
	time.Sleep(80 * time.Millisecond)

	if got := ovContent.msgs.Load(); got != 1 {
		t.Errorf("overlay content received %d keys, want 1 (focused path first)", got)
	}
	if got := probe.msgs.Load(); got != 1 {
		t.Errorf("background probe received %d keys with non-modal overlay, want 1", got)
	}
}
