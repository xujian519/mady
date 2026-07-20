package tui

import (
	"testing"

	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

type bridgeTestComponent struct{}

func (bridgeTestComponent) Render(width int64) []string { return []string{"overlay"} }
func (bridgeTestComponent) Invalidate()                 {}

type bridgeTestOverlayRef struct {
	content core.Component
	focus   bool
	dim     bool
}

func (o *bridgeTestOverlayRef) OverlayContent() core.Component { return o.content }
func (o *bridgeTestOverlayRef) SetOverlayFocus(v bool)         { o.focus = v }
func (o *bridgeTestOverlayRef) SetOverlayDimBackground(v bool) { o.dim = v }
func (o *bridgeTestOverlayRef) OverlayWantsFocus() bool        { return o.focus }
func (o *bridgeTestOverlayRef) OverlayDimBackground() bool     { return o.dim }
func (o *bridgeTestOverlayRef) OverlayAnchor() int             { return 0 }
func (o *bridgeTestOverlayRef) OverlayPercentX() int           { return 0 }
func (o *bridgeTestOverlayRef) OverlayPercentY() int           { return 0 }
func (o *bridgeTestOverlayRef) OverlayWidthPct() int           { return 0 }
func (o *bridgeTestOverlayRef) OverlayHeightPct() int          { return 0 }

var _ chat.OverlayRef = (*bridgeTestOverlayRef)(nil)

func TestTuiAppHostRemovesOverlayByRef(t *testing.T) {
	tui := NewTUI(terminal.NewVirtualTerminal(80, 24), TUIOptions{})
	host := &tuiAppHost{TUI: tui}
	ref := &bridgeTestOverlayRef{content: bridgeTestComponent{}, focus: true}

	host.PushOverlay(ref)
	if got := len(tui.Overlays()); got != 1 {
		t.Fatalf("expected 1 overlay after push, got %d", got)
	}

	if !host.RemoveOverlay(ref) {
		t.Fatal("expected RemoveOverlay to remove pushed overlay")
	}
	if got := len(tui.Overlays()); got != 0 {
		t.Fatalf("expected no overlays after remove, got %d", got)
	}
}

// categorisedOverlayRef implements the optional OverlayCategory() method so
// the bridge can propagate it to tui.Overlay. All other OverlayRef methods
// return zero values.
type categorisedOverlayRef struct {
	content  core.Component
	category int
}

func (o *categorisedOverlayRef) OverlayContent() core.Component { return o.content }
func (o *categorisedOverlayRef) SetOverlayFocus(v bool)         {}
func (o *categorisedOverlayRef) SetOverlayDimBackground(v bool) {}
func (o *categorisedOverlayRef) OverlayWantsFocus() bool        { return false }
func (o *categorisedOverlayRef) OverlayDimBackground() bool     { return false }
func (o *categorisedOverlayRef) OverlayAnchor() int             { return 0 }
func (o *categorisedOverlayRef) OverlayPercentX() int           { return 50 }
func (o *categorisedOverlayRef) OverlayPercentY() int           { return 50 }
func (o *categorisedOverlayRef) OverlayWidthPct() int           { return 60 }
func (o *categorisedOverlayRef) OverlayHeightPct() int          { return 60 }
func (o *categorisedOverlayRef) OverlayCategory() int           { return o.category }

func TestTuiAppHostPropagatesCategory(t *testing.T) {
	tui := NewTUI(terminal.NewVirtualTerminal(80, 24), TUIOptions{})
	host := &tuiAppHost{TUI: tui}

	tests := []struct {
		name string
		cat  int
		want OverlayCategory
	}{
		{"default (no OverlayCategory method)", 0, OverlaySelection},
		{"selection", chat.OverlayCatSelection, OverlaySelection},
		{"review", chat.OverlayCatReview, OverlayReview},
		{"gate", chat.OverlayCatGate, OverlayGate},
		{"system", chat.OverlayCatSystem, OverlaySystem},
		{"unknown (falls back to selection)", 99, OverlaySelection},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset overlay state between subtests.
			for _, o := range tui.Overlays() {
				tui.RemoveOverlay(o)
			}
			ref := &categorisedOverlayRef{content: bridgeTestComponent{}, category: tt.cat}
			host.PushOverlay(ref)
			overlays := tui.Overlays()
			if len(overlays) != 1 {
				t.Fatalf("expected 1 overlay, got %d", len(overlays))
			}
			if overlays[0].Category != tt.want {
				t.Errorf("Category = %v, want %v", overlays[0].Category, tt.want)
			}
			t.Logf("overlay Category = %v", overlays[0].Category)
		})
	}
}
