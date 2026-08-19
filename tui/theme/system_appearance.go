package theme

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// SystemAppearance represents the OS-level color scheme preference.
type SystemAppearance int

const (
	AppearanceUnknown SystemAppearance = iota
	AppearanceDark
	AppearanceLight
)

func (a SystemAppearance) String() string {
	switch a {
	case AppearanceDark:
		return "dark"
	case AppearanceLight:
		return "light"
	default:
		return "unknown"
	}
}

// DetectSystemAppearance probes the OS for the current appearance.
// Returns ApperanceUnknown when detection is not possible.
func DetectSystemAppearance() SystemAppearance {
	return detectAppearance(context.Background())
}

// cachedAppearance stores the last known appearance to avoid unnecessary
// callbacks when nothing changed.
var (
	appMu          sync.RWMutex
	lastAppearance SystemAppearance
)

// WatchSystemAppearance starts a background goroutine that polls the OS for
// appearance changes every `poll` interval (default 2s). When a change is
// detected, onChanged is called with the new appearance. Returns a cancel
// function to stop the watcher.
func WatchSystemAppearance(ctx context.Context, poll time.Duration, onChanged func(SystemAppearance)) func() {
	if poll <= 0 {
		poll = 2 * time.Second
	}
	ctx, cancel := context.WithCancel(ctx)
	appMu.Lock()
	lastAppearance = DetectSystemAppearance()
	appMu.Unlock()
	go runWithRestart(ctx, "system-appearance-watcher", func() {
		t := time.NewTicker(poll)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				current := detectAppearance(ctx)
				if current == AppearanceUnknown {
					// Transient detection failure (e.g. the defaults/gsettings
					// command errored). Treat it as "no change": keep the last
					// known appearance so a flaky probe can't flip the theme
					// (P2-6 — Unknown must not be reported as a change).
					continue
				}
				appMu.Lock()
				changed := current != lastAppearance
				if changed {
					lastAppearance = current
				}
				appMu.Unlock()
				if changed && onChanged != nil {
					onChanged(current)
				}
			}
		}
	})
	return cancel
}

// detectAppearance runs the platform-specific detection command.
// It never fails: macOS detection runs first; if unavailable it falls
// through to Linux and finally returns AppearanceUnknown.
func detectAppearance(ctx context.Context) SystemAppearance {
	// Try macOS first (most reliable).
	if a, err := detectMacOSAppearance(ctx); err == nil {
		return a
	}
	// Try Linux (GNOME-based desktops).
	if a, err := detectLinuxAppearance(ctx); err == nil {
		return a
	}
	return AppearanceUnknown
}

// detectMacOSAppearance runs `defaults read -g AppleInterfaceStyle`.
// Returns (AppearanceLight, nil) when the command succeeds and output
// indicates light mode, or (AppearanceDark, nil) for dark mode.
// Returns an error when the `defaults` command is unavailable (e.g. on Linux),
// allowing detectAppearance to fall through to Linux detection.
func detectMacOSAppearance(ctx context.Context) (SystemAppearance, error) {
	cmd := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleInterfaceStyle")
	out, err := cmd.Output()
	if err != nil {
		// On macOS Light mode, AppleInterfaceStyle key does not exist
		// and defaults read exits with code 1. Only this specific exit
		// code means "light"; any other failure is a detection error and
		// must not misreport a dark system as light (P2-6).
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return AppearanceLight, nil
		}
		return AppearanceUnknown, err // propagate so Linux detection runs
	}
	if strings.Contains(strings.ToLower(string(out)), "dark") {
		return AppearanceDark, nil
	}
	return AppearanceLight, nil
}

// detectLinuxAppearance runs `gsettings get org.gnome.desktop.interface color-scheme`.
func detectLinuxAppearance(ctx context.Context) (SystemAppearance, error) {
	cmd := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.interface", "color-scheme")
	out, err := cmd.Output()
	if err != nil {
		return AppearanceUnknown, err
	}
	s := strings.TrimSpace(string(out))
	if s == "'prefer-dark'" || s == "prefer-dark" {
		return AppearanceDark, nil
	}
	return AppearanceLight, nil
}
