package tui

import (
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
)

// waitFor polls cond until it returns true or the timeout elapses, failing
// the test otherwise. Use it instead of fixed time.Sleep to wait for
// asynchronous effects (event-loop processing, loader state) so slow CI
// runners don't flake.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for %s", timeout, what)
}

// waitStartedMsg is a no-op probe: it has no handler, but its consumption by
// the event loop is observable via TotalMsgCount.
type waitStartedMsg struct{ core.MsgBase }

// waitTUIStarted blocks until the TUI event loop is running. The started
// field is written by Start without a lock, so polling it would race;
// instead we enqueue a probe message and wait for the loop to consume it
// (TotalMsgCount is mutex-guarded).
func waitTUIStarted(t *testing.T, app *TUI) {
	t.Helper()
	before := app.TotalMsgCount()
	app.SendMsg(waitStartedMsg{})
	waitFor(t, 3*time.Second, func() bool { return app.TotalMsgCount() > before },
		"TUI event loop to process the probe message")
}
