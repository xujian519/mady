package theme

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"
)

// restartBackoff is the cooldown before a panicked watcher goroutine is
// restarted. Short enough to recover quickly from a transient failure,
// long enough to avoid a tight panic-restart loop flooding logs.
const restartBackoff = 5 * time.Second

// runWithRestart runs fn in the current goroutine until ctx is canceled.
// If fn panics, it logs the error, waits restartBackoff, and calls fn again.
// fn is expected to run its own inner loop and return when ctx is canceled.
//
// This extracts the panic-recovery + backoff-restart pattern that both
// theme watchers (file-watch and system-appearance-watch) share, so the
// restart policy lives in exactly one place.
func runWithRestart(ctx context.Context, name string, fn func()) {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("theme: watcher panicked, will restart after backoff",
						"name", name, "err", r, "stack", string(debug.Stack()))
					time.Sleep(restartBackoff)
				}
			}()
			fn()
		}()
		// Check whether the watcher was canceled while fn was running
		// (or during the backoff). If so, stop; otherwise restart.
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}
