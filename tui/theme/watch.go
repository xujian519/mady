package theme

import (
	"context"
	"log/slog"
	"os"
	"runtime/debug"
	"time"
)

// StartSemanticThemeWatcher polls a theme JSON path and reapplies the palette
// when the file mtime changes. Returns a cancel function. Zero poll uses 800ms.
func StartSemanticThemeWatcher(path string, poll time.Duration, onReload func()) func() {
	if path == "" {
		return func() {}
	}
	if poll <= 0 {
		poll = 800 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Outer loop: restart after panic with a backoff.
		// Without this, a single panic (e.g. from LoadSemanticThemeFromFile)
		// would permanently kill the watcher, preventing all future hot-reloads.
		for {
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("theme: watcher goroutine panicked, will restart after backoff",
							"err", r, "stack", string(debug.Stack()))
						time.Sleep(5 * time.Second)
					}
				}()
				var lastMtime int64 = -1
				t := time.NewTicker(poll)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						st, err := os.Stat(path)
						if err != nil {
							continue
						}
						mt := st.ModTime().UnixNano()
						if lastMtime < 0 {
							if err := LoadSemanticThemeFromFile(path, ColorModeFromEnv()); err != nil {
								slog.Error("theme: initial load failed", "path", path, "error", err)
							}
							lastMtime = mt
							continue
						}
						if mt == lastMtime {
							continue
						}
						// Only update lastMtime on successful reload so that a
						// transient error (e.g. atomic-write in progress) does not
						// permanently skip the file.
						if err := LoadSemanticThemeFromFile(path, ColorModeFromEnv()); err != nil {
							slog.Error("theme: reload failed, will retry", "path", path, "error", err)
							continue
						}
						lastMtime = mt
						if onReload != nil {
							onReload()
						}
					}
				}
			}()
			// Check if the watcher was canceled before restarting.
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	return cancel
}
