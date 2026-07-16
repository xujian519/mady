package tui

// This file contains the TUI event loop — the single goroutine that drains
// the message channel, dispatches Msgs to the focused component, and renders
// frames at a coalesced cadence. It is the junction of lifecycle (doneCh),
// rendering (renderRequested/tickCh/ticker), and input (msgCh); it lives in
// its own file because it does not belong to any single one of those domains.

import (
	"sync/atomic"
	"time"
)

func (t *TUI) eventLoop() {
	defer func() {
		if r := recover(); r != nil {
			// Ensure terminal is restored before the process exits
			t.Stop()
			panic(r) // re-panic after cleanup so the stack trace still shows
		}
	}()

	ticker := time.NewTicker(t.options.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.doneCh:
			return
		case msg := <-t.msgCh:
			t.processMsg(msg)
		case <-t.tickCh:
		case <-ticker.C:
		}
		// Promote a pending lone ESC byte into an actual key event so
		// ESC-only presses reach the application (not just CSI sequences).
		if t.stdin != nil {
			t.stdin.FlushEsc()
		}
		if atomic.SwapInt64(&t.renderRequested, 0) == 0 {
			continue
		}
		t.renderFrame()
	}
}
