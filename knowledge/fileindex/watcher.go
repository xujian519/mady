package fileindex

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher uses fsnotify to monitor a project directory for file changes
// and triggers Refresh on the FileIndex after a debounce period.
//
// Design decisions:
//   - Debounce: consecutive events within 500ms are coalesced into a single
//     Refresh call, so rapid save sequences (editor auto-save, bulk copy) don't
//     trigger N refreshes.
//   - Scope: only watches the configured root directory and its immediate
//     subdirectories (not deep recursion) to avoid inotify limit exhaustion
//     on large projects.
//   - Graceful degradation: on inotify limit errors (EINVAL/ENOSPC), prints a
//     warning and falls back to periodic polling (every 60s).
type FileWatcher struct {
	watcher *fsnotify.Watcher
	index   *FileIndex

	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	debounce time.Duration
	pollInt  time.Duration // fallback poll interval when fsnotify fails

	// For debouncing: a timer that gets reset on each incoming event.
	debounceTimer *time.Timer
	debounceMu    sync.Mutex

	// Track directories we've already added to speed up re-adding.
	addedDirs map[string]bool
	addedMu   sync.Mutex
}

// FileWatcherConfig configures the file watcher.
type FileWatcherConfig struct {
	// Debounce is the window for coalescing consecutive events (default 500ms).
	Debounce time.Duration
	// PollInterval is the fallback poll interval when fsnotify cannot be used
	// (default 60s). Set to 0 to disable polling fallback.
	PollInterval time.Duration
}

// NewFileWatcher creates a FileWatcher bound to the given FileIndex.
// The watcher is not started until Start is called.
func NewFileWatcher(index *FileIndex, cfg FileWatcherConfig) *FileWatcher {
	if cfg.Debounce <= 0 {
		cfg.Debounce = 500 * time.Millisecond
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 60 * time.Second
	}
	return &FileWatcher{
		index:     index,
		debounce:  cfg.Debounce,
		pollInt:   cfg.PollInterval,
		addedDirs: make(map[string]bool),
	}
}

// Start begins watching the index directory. It is safe to call multiple times
// (no-op when already running). Blocks briefly to set up fsnotify watches;
// the actual event processing runs in background goroutines.
func (fw *FileWatcher) Start(ctx context.Context) error {
	fw.mu.Lock()
	if fw.running {
		fw.mu.Unlock()
		return nil
	}
	// Keep the lock held until running is set, to prevent TOCTOU races.

	w, err := fsnotify.NewWatcher()
	if err != nil {
		fw.mu.Unlock()
		if fw.pollInt <= 0 {
			return fmt.Errorf("filewatcher: fsnotify unavailable and polling disabled")
		}
		return fw.startPolling(ctx)
	}

	rootDir := fw.index.Dir()
	if err := fw.addDirTree(w, rootDir); err != nil {
		w.Close()
		// Fall back to polling instead of failing entirely (graceful degradation).
		slog.Warn("filewatcher: add watch failed, falling back to polling", "dir", rootDir, "err", err)
		fw.mu.Unlock()
		if fw.pollInt <= 0 {
			return fmt.Errorf("filewatcher: fsnotify unavailable and polling disabled")
		}
		return fw.startPolling(ctx)
	}

	fw.watcher = w
	fw.stopCh = make(chan struct{})
	fw.doneCh = make(chan struct{})
	fw.running = true
	fw.mu.Unlock()

	go fw.eventLoop(w)
	return nil
}

// Stop stops the watcher. Safe to call multiple times. Blocks until the
// event loop has exited.
func (fw *FileWatcher) Stop() {
	fw.mu.Lock()
	// Stop the debounce timer to prevent callbacks on a stale index.
	fw.debounceMu.Lock()
	if fw.debounceTimer != nil {
		fw.debounceTimer.Stop()
		fw.debounceTimer = nil
	}
	fw.debounceMu.Unlock()

	stopCh := fw.stopCh
	doneCh := fw.doneCh
	isRunning := fw.running

	if isRunning && fw.watcher != nil {
		fw.watcher.Close()
	}
	fw.running = false
	fw.addedDirs = make(map[string]bool)
	fw.mu.Unlock()

	if !isRunning {
		return
	}
	close(stopCh)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-doneCh:
	case <-timer.C:
		// Timeout: don't block Stop indefinitely.
	}
}

// IsRunning reports whether the watcher is active.
func (fw *FileWatcher) IsRunning() bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return fw.running
}

// ---------------------------------------------------------------------------
// fsnotify event loop
// ---------------------------------------------------------------------------

func (fw *FileWatcher) eventLoop(w *fsnotify.Watcher) {
	defer close(fw.doneCh)

	for {
		select {
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			fw.handleEvent(event)

		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			slog.Error("filewatcher: error", "err", err)

		case <-fw.stopCh:
			return
		}
	}
}

func (fw *FileWatcher) handleEvent(event fsnotify.Event) {
	// Ignore events on SQLite support files.
	base := strings.ToLower(filepath.Base(event.Name))
	if strings.HasSuffix(base, "-shm") || strings.HasSuffix(base, "-wal") ||
		strings.HasSuffix(base, ".db-journal") || strings.HasSuffix(base, ".db") {
		return
	}

	// For new directories created within the watched root, add them to the watch.
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			if fw.watcher != nil {
				_ = fw.addDirTree(fw.watcher, event.Name)
			}
		}
	}

	// Debounce: reset the timer on each event.
	fw.mu.Lock()
	isRunning := fw.running
	fw.mu.Unlock()
	if !isRunning {
		return // don't start a new timer after Stop()
	}

	fw.debounceMu.Lock()
	if fw.debounceTimer == nil {
		fw.debounceTimer = time.AfterFunc(fw.debounce, func() {
			fw.debounceMu.Lock()
			fw.debounceTimer = nil
			fw.debounceMu.Unlock()

			// Check if watcher is still running (Stop may have been called).
			fw.mu.Lock()
			stillRunning := fw.running
			fw.mu.Unlock()
			if !stillRunning {
				return
			}
			// Trigger Refresh.
			if err := fw.index.Refresh(context.Background()); err != nil {
				slog.Error("filewatcher: refresh after debounce", "err", err)
			}
		})
	} else {
		fw.debounceTimer.Reset(fw.debounce)
	}
	fw.debounceMu.Unlock()
}

// ---------------------------------------------------------------------------
// Directory tree watching
// ---------------------------------------------------------------------------

// addDirTree adds the directory and its immediate subdirectories to the watcher.
// This avoids adding deeply nested trees that could exhaust inotify limits.
func (fw *FileWatcher) addDirTree(w *fsnotify.Watcher, root string) error {
	fw.addedMu.Lock()
	defer fw.addedMu.Unlock()

	if fw.addedDirs[root] {
		return nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}

	// Add root.
	if err := w.Add(root); err != nil {
		return err
	}
	fw.addedDirs[root] = true

	// Add immediate subdirectories (one level deep).
	for _, entry := range entries {
		if entry.IsDir() {
			subPath := filepath.Join(root, entry.Name())
			if !fw.addedDirs[subPath] {
				if err := w.Add(subPath); err != nil {
					// Non-critical; skip directories that can't be watched.
					continue
				}
				fw.addedDirs[subPath] = true
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Polling fallback (when fsnotify fails, e.g. inotify limit exhaustion)
// ---------------------------------------------------------------------------

func (fw *FileWatcher) startPolling(ctx context.Context) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.pollInt <= 0 {
		return fmt.Errorf("filewatcher: fsnotify unavailable and polling disabled")
	}

	fw.stopCh = make(chan struct{})
	fw.doneCh = make(chan struct{})
	fw.running = true

	slog.Info("filewatcher: fsnotify unavailable, falling back to polling", "interval", fw.pollInt)

	go func() {
		defer close(fw.doneCh)

		ticker := time.NewTicker(fw.pollInt)
		defer ticker.Stop()

		// Do an initial refresh.
		_ = fw.index.Refresh(ctx)

		for {
			select {
			case <-ticker.C:
				_ = fw.index.Refresh(ctx)
			case <-fw.stopCh:
				return
			}
		}
	}()

	return nil
}
