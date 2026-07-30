package tui

// This file contains the TUI event loop — the single goroutine that drains
// the message channel, dispatches Msgs to the focused component, and renders
// frames at a coalesced cadence. It is the junction of lifecycle (doneCh),
// rendering (renderRequested/tickCh/ticker), and input (msgCh); it lives in
// its own file because it does not belong to any single one of those domains.

import (
	"log/slog"
	"sync/atomic"
	"time"
)

func (t *TUI) eventLoop() {
	defer func() {
		if r := recover(); r != nil {
			// Ensure terminal is restored before the process exits.
			// Close stdin first since its flushLoop goroutine is independent
			// of the TUI's Stop path.
			t.stdin.Close()
			// Wrap Stop in a nested recover: if t.Stop() itself panics
			// (extremely unlikely — termios restore should never do this),
			// we still capture the original panic's stack trace rather than
			// losing it to a secondary panic.
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Warn("tui: stop panicked during panic recovery", "err", r)
					}
				}()
				if err := t.Stop(); err != nil {
					slog.Warn("tui: stop failed in panic recovery", "err", err)
				}
			}()
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
			t.flushPendingMotion()
		case <-ticker.C:
			t.flushPendingMotion()
		}
		if atomic.SwapInt64(&t.renderRequested, 0) == 0 {
			continue
		}
		t.renderFrame()
	}
}

// flushPendingMotion delivers the most recently coalesced MouseMotion event
// (if any) to the event loop. Called on every ticker/tick boundary so that
// the final drag position from a burst of throttled motion events reaches
// the component, keeping text-selection endpoints accurate.
func (t *TUI) flushPendingMotion() {
	if t.pendingMotion == nil {
		return
	}
	msg := *t.pendingMotion
	t.pendingMotion = nil
	t.SendMsg(msg)
}
