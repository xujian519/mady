package tui

import (
	"sync"
	"testing"
	"time"

	core "github.com/xujian519/mady/tui/core"
	terminal "github.com/xujian519/mady/tui/terminal"
)

// resizeProbe counts WindowSizeMsg deliveries through the normal dispatch
// path (focused Update). It is used to verify the debounce→deliver pipeline
// end to end: rapid resizes must coalesce into a single delivery, and the
// debounce timer must not re-arm itself forever.
type resizeProbe struct {
	mu      sync.Mutex
	count   int
	focused bool
}

func (p *resizeProbe) Render(int64) []string { return nil }
func (p *resizeProbe) Invalidate()           {}
func (p *resizeProbe) SetFocused(f bool)     { p.focused = f }
func (p *resizeProbe) IsFocused() bool       { return p.focused }

func (p *resizeProbe) Update(msg core.Msg) core.Cmd {
	if _, ok := msg.(core.WindowSizeMsg); ok {
		p.mu.Lock()
		p.count++
		p.mu.Unlock()
	}
	return nil
}

func (p *resizeProbe) resizeCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

// TestWindowSizeMsgDebounceDeliversOnce is the P0-1 regression test.
//
// Pre-fix behaviour: the debounce timer re-sent the raw WindowSizeMsg, which
// re-entered the debounce branch in processMsg, re-armed the timer and
// returned — forming a self-perpetuating 100ms loop while components never
// received a single resize message. This test locks in the fix:
//  1. rapid resize events within the debounce window coalesce into exactly
//     one delivery, and
//  2. after the debounce fires, delivery stops (no self-perpetuating loop).
func TestWindowSizeMsgDebounceDeliversOnce(t *testing.T) {
	vt := terminal.NewVirtualTerminal(80, 24)
	app := NewTUI(vt, TUIOptions{})
	probe := &resizeProbe{}
	app.AddChild(probe)
	app.Focus(probe)

	if err := app.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer app.Stop()

	// Start() enqueues an initial WindowSizeMsg; wait for it to be
	// debounced and delivered so the baseline is stable.
	waitFor(t, 3*time.Second, func() bool { return probe.resizeCount() >= 1 },
		"initial resize to be delivered")
	baseline := probe.resizeCount()

	// Rapid burst: 5 resizes within the debounce window must coalesce
	// into exactly one delivery.
	for i := 0; i < 5; i++ {
		app.SendMsg(core.WindowSizeMsg{Width: 81 + int64(i), Height: 25})
	}
	waitFor(t, 3*time.Second, func() bool { return probe.resizeCount() == baseline+1 },
		"debounced burst to be delivered exactly once")

	// Settle window: no further deliveries may arrive. The buggy
	// implementation kept re-arming the timer, so the count grew forever.
	time.Sleep(350 * time.Millisecond)
	if got := probe.resizeCount(); got != baseline+1 {
		t.Fatalf("resize messages kept arriving after debounce (self-perpetuating loop): count=%d, want=%d",
			got, baseline+1)
	}
}
