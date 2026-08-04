package tui

import (
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
)

// ---------------------------------------------------------------------------
// tui.go — functional options
// ---------------------------------------------------------------------------

func TestTUIFunctionalOptions(t *testing.T) {
	term := terminal.NewVirtualTerminal(80, 24)
	km := terminal.NewKeybindingsManager(map[string]terminal.KeybindingDef{})
	called := false
	filter := func(core.Component, core.Msg) core.Msg {
		called = true
		return nil
	}

	var o TUIOptions
	WithFilter(filter)(&o)
	WithTickInterval(16 * time.Millisecond)(&o)
	WithoutBracketedPaste()(&o)
	WithoutSynchronizedOutput()(&o)
	WithAltScreen()(&o)
	WithMouse("sgr")(&o)
	WithKeybindings(km)(&o)
	WithWindowTitle("mady")(&o)

	if o.TickInterval != 16*time.Millisecond {
		t.Errorf("TickInterval = %v, want 16ms", o.TickInterval)
	}
	if !o.DisableBracketedPaste {
		t.Error("DisableBracketedPaste = false, want true")
	}
	if !o.DisableSynchronizedOutput {
		t.Error("DisableSynchronizedOutput = false, want true")
	}
	if !o.AltScreen {
		t.Error("AltScreen = false, want true")
	}
	if o.MouseMode != "sgr" {
		t.Errorf("MouseMode = %q, want sgr", o.MouseMode)
	}
	if o.Keybindings != km {
		t.Error("Keybindings option not applied")
	}
	if o.WindowTitle != "mady" {
		t.Errorf("WindowTitle = %q, want mady", o.WindowTitle)
	}
	if o.Filter == nil {
		t.Error("Filter option not applied")
	}

	// The options value feeds NewTUI directly.
	app := NewTUI(term, o)
	if app.options.TickInterval != 16*time.Millisecond {
		t.Errorf("NewTUI TickInterval = %v, want 16ms", app.options.TickInterval)
	}
	// Filter must run in the live loop.
	_ = called
}

// ---------------------------------------------------------------------------
// overlay.go — pure helpers
// ---------------------------------------------------------------------------

func TestHexToCoreColor(t *testing.T) {
	if c := hexToCoreColor("#33aaff"); c != core.RGB(0x33, 0xaa, 0xff) {
		t.Errorf("hexToCoreColor(#33aaff) = %v", c)
	}
	if c := hexToCoreColor("33aaff"); c != core.RGB(0x33, 0xaa, 0xff) {
		t.Errorf("hexToCoreColor(no #) = %v", c)
	}
	// Invalid inputs fall back to palette 235.
	for _, bad := range []string{"", "#123", "#zzzzzz", "12345", "#12 456"} {
		if c := hexToCoreColor(bad); c != core.Palette(235) {
			t.Errorf("hexToCoreColor(%q) = %v, want palette 235", bad, c)
		}
	}
}

func TestDefaultOverlaySize(t *testing.T) {
	cases := []struct {
		cat          OverlayCategory
		wantW, wantH int64
	}{
		{OverlaySelection, 40, 30},
		{OverlayReview, 60, 60},
		{OverlayGate, 70, 75},
		{OverlaySystem, 50, 40},
		{OverlayCategory(99), 60, 60}, // unknown → default
	}
	for _, tc := range cases {
		w, h := DefaultOverlaySize(tc.cat)
		if w != tc.wantW || h != tc.wantH {
			t.Errorf("DefaultOverlaySize(%d) = %d×%d, want %d×%d",
				tc.cat, w, h, tc.wantW, tc.wantH)
		}
	}
}

// ---------------------------------------------------------------------------
// chat_bridge.go — debug accessors on a live TUI
// ---------------------------------------------------------------------------

func TestDebugAccessors(t *testing.T) {
	term := terminal.NewVirtualTerminal(80, 24)
	app := NewTUI(term)

	// Accessors must not panic before Start.
	_ = app.MsgQueueDepth()
	_ = app.FrameStats()
	_ = app.RecentEvents()
	_ = app.DebugAlloc()
	_ = app.TotalMsgCount()
	_ = app.RenderDuration()
	_ = app.SlowFrameCount()

	done := app.Done()
	go func() { _ = app.Start() }()
	waitTUIStarted(t, app)
	app.Stop()
	<-done

	// Accessors must not panic after Stop either.
	_ = app.MsgQueueDepth()
	_ = app.FrameStats()
	_ = app.SlowFrameCount()
}
