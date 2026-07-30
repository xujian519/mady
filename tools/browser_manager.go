package tools

import (
	"log"
	"os"
	"runtime/debug"
	"sync"
	"time"
)

var defaultBrowserMgr struct {
	mu       sync.Mutex
	mgr      *BrowserManager
	initOnce sync.Once
}

// initDefaultBrowserManager performs one-time lazy initialization of the
// default browser manager, signal handler, and orphan reaper.
func initDefaultBrowserManager() {
	defaultBrowserMgr.initOnce.Do(func() {
		abEnabled := os.Getenv("AGENT_BROWSER_ENABLED") == "true" || os.Getenv("AGENT_BROWSER_PATH") != ""
		bm := NewBrowserManager(&BrowserConfig{
			Headless:            true,
			AllowPrivate:        false,
			CommandTimeout:      30 * time.Second,
			CDPURL:              "",
			CamofoxURL:          "",
			AgentBrowserEnabled: abEnabled,
		})
		SetDefaultBrowserManager(bm)

		SetupSignalHandler()

		reaper := NewOrphanReaper()
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[PANIC] browser_manager: orphan reaper panicked: %v\n%s", r, debug.Stack())
				}
			}()
			reaper.ReapOrphans() //nolint:gosec // G104: cleanup-only; orphan reaper runs in background goroutine
		}()
	})
}

// DefaultBrowserManager returns the default browser manager, initializing it
// lazily on first access.
func DefaultBrowserManager() *BrowserManager {
	initDefaultBrowserManager()
	defaultBrowserMgr.mu.Lock()
	defer defaultBrowserMgr.mu.Unlock()
	return defaultBrowserMgr.mgr
}

func SetDefaultBrowserManager(bm *BrowserManager) {
	defaultBrowserMgr.mu.Lock()
	old := defaultBrowserMgr.mgr
	defaultBrowserMgr.mgr = bm
	defaultBrowserMgr.mu.Unlock()
	// Stop the old manager's background goroutine outside the lock.
	if old != nil {
		old.Stop()
	}
}
