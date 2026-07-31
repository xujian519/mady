package component

import (
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
)

type mockDebugTUI struct {
	fps        float64
	queueDepth int
	alloc      uint64
	msgCount   uint64
	events     []string
	renderDur  time.Duration
	slowFrames uint64
}

func (m *mockDebugTUI) MsgQueueDepth() int            { return m.queueDepth }
func (m *mockDebugTUI) FrameStats() float64           { return m.fps }
func (m *mockDebugTUI) RecentEvents() []string        { return m.events }
func (m *mockDebugTUI) DebugAlloc() uint64            { return m.alloc }
func (m *mockDebugTUI) TotalMsgCount() uint64         { return m.msgCount }
func (m *mockDebugTUI) RenderDuration() time.Duration { return m.renderDur }
func (m *mockDebugTUI) SlowFrameCount() uint64        { return m.slowFrames }

type mockDebugState string

func (s mockDebugState) String() string { return string(s) }

type mockDebugApp struct{ state interface{ String() string } }

func (m *mockDebugApp) State() interface{ String() string } { return m.state }

func TestDebugOverlayBasics(t *testing.T) {
	d := NewDebugOverlay(&mockDebugTUI{}, &mockDebugApp{state: mockDebugState("idle")})
	d.SetMaxVisible(5)
	d.Invalidate()
	d.ScrollBy(-3) // clamps to 0
	if d.offset != 0 {
		t.Fatalf("expected offset 0, got %d", d.offset)
	}
	d.ScrollBy(2)
	if d.offset != 2 {
		t.Fatalf("expected offset 2, got %d", d.offset)
	}
}

func TestDebugOverlayCloseOnEscape(t *testing.T) {
	d := NewDebugOverlay(nil, nil)
	closed := false
	d.SetOnClose(func() { closed = true })
	d.Update(core.KeyMsg{Data: "\x1b"})
	if !closed {
		t.Fatal("expected onClose on Escape")
	}
	// No callback — no panic.
	closed = false
	d2 := NewDebugOverlay(nil, nil)
	d2.Update(core.KeyMsg{Data: "\x1b"})
	_ = closed
}

func TestDebugOverlayScrollKeys(t *testing.T) {
	d := NewDebugOverlay(&mockDebugTUI{events: []string{"a", "b", "c"}}, nil)
	d.Update(core.KeyMsg{Data: "\x1b[A"}) // up -> offset decreases (clamped)
	if d.offset != 0 {
		t.Fatalf("expected offset 0, got %d", d.offset)
	}
	d.Update(core.KeyMsg{Data: "\x1b[B"}) // down -> offset increases
	if d.offset != 1 {
		t.Fatalf("expected offset 1, got %d", d.offset)
	}
	// PageUp/PageDown branches match against the KeyIDs "pgup"/"pgdown",
	// but the key parser canonicalises these sequences to "pageUp" — so the
	// branches are unreachable via real key input (source bug). The offset
	// must remain unchanged.
	d.Update(core.KeyMsg{Data: "\x1b[5~"}) // pgup
	d.Update(core.KeyMsg{Data: "\x1b[6~"}) // pgdown
	if d.offset != 1 {
		t.Fatalf("expected offset 1 (pgup/pgdown bindings unreachable), got %d", d.offset)
	}
}

// plainMsg is an arbitrary Msg that is not KeyMsg/debugTick/WindowSizeMsg,
// exercising the first-Update tick bootstrap path.
type plainMsg struct{ core.MsgBase }

func TestDebugOverlayUpdateStartsTick(t *testing.T) {
	d := NewDebugOverlay(nil, nil)
	cmd := d.Update(plainMsg{}) // first Update kicks off the tick
	if cmd == nil {
		t.Fatal("expected tick cmd on first Update")
	}
	cmd = d.Update(plainMsg{}) // subsequent Updates return nil
	if cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
}

func TestDebugOverlayTickRefreshesData(t *testing.T) {
	src := &mockDebugTUI{
		fps:        60,
		queueDepth: 10,
		alloc:      1 << 20,
		msgCount:   42,
		events:     []string{"key", "tick"},
		renderDur:  5 * time.Millisecond,
	}
	app := &mockDebugApp{state: mockDebugState("running")}
	d := NewDebugOverlay(src, app)
	d.Update(core.WindowSizeMsg{Width: 100, Height: 30}) // sets dirty
	cmd := d.Update(debugTick{})
	if cmd == nil {
		t.Fatal("expected reschedule cmd after debugTick")
	}
	if d.fps != 60 || d.queueDepth != 10 || d.msgCount != 42 || d.fsmState != "running" {
		t.Fatalf("expected refreshed data, got fps=%v queue=%d state=%q",
			d.fps, d.queueDepth, d.fsmState)
	}
}

func TestDebugOverlayRenderWithData(t *testing.T) {
	src := &mockDebugTUI{
		fps:        60,
		queueDepth: 10,
		alloc:      1 << 20,
		msgCount:   42,
		events:     []string{"key", "tick", "render", "mouse"},
		renderDur:  5 * time.Millisecond,
	}
	d := NewDebugOverlay(src, &mockDebugApp{state: mockDebugState("chat")})
	d.Update(debugTick{}) // refresh cached data from sources
	lines := d.Render(50)
	if len(lines) == 0 {
		t.Fatal("expected non-empty render")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Debug Overlay", "FPS:", "Queue:", "Msgs:", "State:", "chat", "Heap:", "Events: (4/4)"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in render, got %q", want, joined)
		}
	}
	// Event lines.
	if !strings.Contains(joined, "▸ key") {
		t.Fatalf("expected event lines, got %q", joined)
	}
}

func TestDebugOverlayRenderNoEventsAndNILState(t *testing.T) {
	d := NewDebugOverlay(&mockDebugTUI{}, nil) // nil app source -> State N/A
	lines := d.Render(50)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "(none)") {
		t.Fatalf("expected (none) events, got %q", joined)
	}
	if !strings.Contains(joined, "N/A") {
		t.Fatalf("expected N/A state, got %q", joined)
	}
}

func TestDebugOverlayRenderFpsThresholds(t *testing.T) {
	// fps < 30 -> Error; 30-54 -> User; >= 55 -> Accent. Only structure asserted.
	for _, fps := range []float64{10, 40, 70} {
		d := NewDebugOverlay(&mockDebugTUI{fps: fps}, nil)
		if lines := d.Render(50); len(lines) == 0 {
			t.Fatalf("fps %v: expected render", fps)
		}
	}
}

func TestDebugOverlayRenderQueueThresholds(t *testing.T) {
	for _, q := range []int{50, 150, 300} {
		d := NewDebugOverlay(&mockDebugTUI{queueDepth: q}, nil)
		if lines := d.Render(50); len(lines) == 0 {
			t.Fatalf("queue %d: expected render", q)
		}
	}
}

func TestDebugOverlayRenderScrollClamp(t *testing.T) {
	src := &mockDebugTUI{events: []string{"a", "b", "c"}}
	d := NewDebugOverlay(src, nil)
	d.Update(debugTick{}) // refresh cached events
	d.ScrollBy(99)        // offset beyond events -> clamped to last event in render
	lines := d.Render(50)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Events: (1/3)") || !strings.Contains(joined, "▸ c") {
		t.Fatalf("expected clamped window showing last event, got %q", joined)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		b    uint64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1 << 10, "1.0 KiB"},
		{1 << 20, "1.0 MiB"},
		{1 << 30, "1.0 GiB"},
		{1536, "1.5 KiB"},
	}
	for _, tc := range cases {
		if got := formatBytes(tc.b); got != tc.want {
			t.Fatalf("formatBytes(%d) = %q, want %q", tc.b, got, tc.want)
		}
	}
}

func TestRenderDurDisplay(t *testing.T) {
	if got := renderDurDisplay(5*time.Millisecond, 0); got == "" || strings.Contains(got, "⚠") {
		t.Fatalf("unexpected normal frame display: %q", got)
	}
	if got := renderDurDisplay(10*time.Millisecond, 0); !strings.Contains(got, "⚡") {
		t.Fatalf("expected slow frame marker, got %q", got)
	}
	if got := renderDurDisplay(20*time.Millisecond, 3); !strings.Contains(got, "⚠") || !strings.Contains(got, "3x16ms+") {
		t.Fatalf("expected overshoot marker, got %q", got)
	}
}
