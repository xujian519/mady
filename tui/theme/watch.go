package theme

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// StartSemanticThemeWatcher polls a theme JSON path and reapplies the palette
// when the file mtime changes. Returns a cancel function. Zero poll uses 800ms.
//
// The watcher goroutine is automatically restarted with a 5s backoff if it
// panics, so a single bad file reload cannot permanently disable hot-reload.
func StartSemanticThemeWatcher(path string, poll time.Duration, onReload func()) func() {
	if path == "" {
		return func() {}
	}
	if poll <= 0 {
		poll = 800 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	go runWithRestart(ctx, "theme-file-watcher", func() {
		// lastMtime is declared inside fn so a restart re-baselines from
		// the file's current mtime rather than a stale value.
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
	})
	return cancel
}
