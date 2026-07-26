package theme

// system_appearance.go — System dark/light appearance detection.
//
// Polls the OS for dark/light mode on macOS (defaults read) and Linux
// (gsettings). Detected changes trigger the registered onChanged callback,
// which the theme system uses to switch between dark and light themes when
// the "auto" theme is active.

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
	a, _ := detectAppearance()
	return a
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
				current, err := detectAppearance()
				if err != nil {
					// Detection failed (e.g., defaults command not available).
					// Don't change state; try again next cycle.
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
func detectAppearance() (SystemAppearance, error) {
	// Try macOS first (most reliable).
	if a, err := detectMacOSAppearance(); err == nil {
		return a, nil
	}
	// Try Linux (GNOME-based desktops).
	if a, err := detectLinuxAppearance(); err == nil {
		return a, nil
	}
	return AppearanceUnknown, nil
}

// detectMacOSAppearance runs `defaults read -g AppleInterfaceStyle`.
// Returns (AppearanceLight, nil) when the command succeeds and output
// indicates light mode, or (AppearanceDark, nil) for dark mode.
// Returns an error when the `defaults` command is unavailable (e.g. on Linux),
// allowing detectAppearance to fall through to Linux detection.
func detectMacOSAppearance() (SystemAppearance, error) {
	cmd := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle")
	out, err := cmd.Output()
	if err != nil {
		// On macOS Light mode, AppleInterfaceStyle key does not exist
		// and defaults read exits with code 1. Distinguish this from
		// the command being missing entirely (e.g. on Linux).
		if _, ok := err.(*exec.ExitError); ok {
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
func detectLinuxAppearance() (SystemAppearance, error) {
	cmd := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme")
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
