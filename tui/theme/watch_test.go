package theme

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// writeThemeFileWithAccent writes a minimal theme JSON file whose accent is
// distinguishable, and backdates its mtime so the next write is guaranteed
// to produce a different mtime (file watcher triggers on mtime changes).
func writeThemeFileWithAccent(t *testing.T, path, accent string) {
	t.Helper()
	content := fmt.Sprintf(`{"name":"watched","colors":{"accent":%q}}`, accent)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
}

func TestStartSemanticThemeWatcherReload(t *testing.T) {
	savePalette(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "theme.json")
	writeThemeFileWithAccent(t, path, "#ff0000")

	reloaded := make(chan struct{}, 8)
	cancel := StartSemanticThemeWatcher(path, 20*time.Millisecond, func() {
		reloaded <- struct{}{}
	})
	t.Cleanup(cancel)

	// Give the watcher time to do its initial load, then change the file.
	time.Sleep(80 * time.Millisecond)
	writeThemeFileWithAccent(t, path, "#00ff00")

	select {
	case <-reloaded:
	case <-time.After(3 * time.Second):
		t.Fatal("onReload was not called after theme file change")
	}
	if got := CurrentPalette().Semantic.Accent; got != "#00ff00" {
		t.Fatalf("accent after reload = %q, want #00ff00", got)
	}
}

func TestStartSemanticThemeWatcherEmptyPath(t *testing.T) {
	// Empty path returns an immediate no-op cancel; must not spawn anything.
	cancel := StartSemanticThemeWatcher("", time.Millisecond, nil)
	cancel()
}

func TestStartSemanticThemeWatcherMissingPath(t *testing.T) {
	// Path does not exist: Stat fails each tick and the watcher must survive.
	cancel := StartSemanticThemeWatcher(filepath.Join(t.TempDir(), "missing.json"), 10*time.Millisecond, nil)
	t.Cleanup(cancel)
	time.Sleep(60 * time.Millisecond)
}

func TestStartSemanticThemeWatcherInvalidThenValid(t *testing.T) {
	savePalette(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "theme.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	reloaded := make(chan struct{}, 8)
	cancel := StartSemanticThemeWatcher(path, 20*time.Millisecond, func() {
		reloaded <- struct{}{}
	})
	t.Cleanup(cancel)

	// Initial load fails (invalid JSON) — the watcher must keep polling.
	time.Sleep(80 * time.Millisecond)
	select {
	case <-reloaded:
		t.Fatal("unexpected reload callback for invalid JSON file")
	default:
	}

	// Now make the file valid: the next poll should reload and fire onReload.
	writeThemeFileWithAccent(t, path, "#112233")
	select {
	case <-reloaded:
	case <-time.After(3 * time.Second):
		t.Fatal("onReload not called after file became valid")
	}
	if got := CurrentPalette().Semantic.Accent; got != "#112233" {
		t.Fatalf("accent = %q, want #112233", got)
	}
}

func TestStartSemanticThemeWatcherDefaultPoll(t *testing.T) {
	savePalette(t)
	// poll <= 0 defaults to 800ms; start/cancel without touching the file.
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.json")
	writeThemeFileWithAccent(t, path, "#010101")
	cancel := StartSemanticThemeWatcher(path, 0, nil)
	t.Cleanup(cancel)
	time.Sleep(60 * time.Millisecond)
}

func TestRunWithRestartPanicRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	calls := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		runWithRestart(ctx, "test-watcher", func() {
			mu.Lock()
			calls++
			n := calls
			mu.Unlock()
			if n == 1 {
				panic("boom")
			}
		})
	}()

	// Wait for the restart (second call) — takes ~restartBackoff after panic.
	deadline := time.Now().Add(restartBackoff + 2*time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := calls
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	n := calls
	mu.Unlock()
	if n < 2 {
		t.Fatal("runWithRestart did not restart the watcher after panic")
	}

	// Cancellation must stop the restart loop.
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runWithRestart did not exit after context cancellation")
	}
}

func TestRunWithRestartCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runWithRestart(ctx, "test-watcher-cancel", func() {
			<-ctx.Done() // simulate an inner loop that blocks on ctx
		})
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runWithRestart did not exit after cancellation")
	}
}
