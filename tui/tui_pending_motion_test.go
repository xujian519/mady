package tui

import (
	"sync"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// TestPendingMotionConcurrentRace exercises the pendingMotion/mouseLast
// sharing between the terminal read goroutine (onMouse/onThrottledMotion)
// and the event-loop goroutine (flushPendingMotion). Run with -race; a
// regression here (unguarded access) fails immediately under the detector.
//
// The TUI is not Started: we stand in for the event loop with a drainer
// goroutine that consumes msgCh, mirroring eventLoop's <-msgCh case, while
// two producer goroutines hammer the mouse path concurrently.
func TestPendingMotionConcurrentRace(t *testing.T) {
	term := &byteSinkTerminal{}
	tui := NewTUI(term, TUIOptions{MouseMode: "sgr"})

	drainDone := make(chan struct{})
	go func() {
		for {
			select {
			case <-tui.msgCh:
			case <-drainDone:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	// Terminal read goroutine: a burst of MouseMotion events, throttled.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 3000; i++ {
			tui.onMouse(core.MouseMsg{
				Action: core.MouseMotion,
				Row:    int64(i % 24),
				Col:    int64(i % 80),
				Button: 1,
			})
		}
	}()
	// Second reader goroutine: interleaved press/release (flushes pending).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 3000; i++ {
			tui.onMouse(core.MouseMsg{
				Action: core.MousePress,
				Row:    int64(i % 24),
				Col:    int64(i % 80),
				Button: 1,
			})
		}
	}()
	// Event-loop goroutine: ticker/tick boundary flushing pending motion.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 3000; i++ {
			tui.flushPendingMotion()
		}
	}()

	wg.Wait()
	close(drainDone)
}
