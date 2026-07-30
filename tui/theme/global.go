package theme

import (
	"os"
	"strings"
	"sync/atomic"
)

var themeChangeHook atomic.Pointer[func()]

// SetOnSemanticThemeChange registers a callback invoked after each successful
// SetSemanticTheme (including JSON reload). Pass nil to clear.
func SetOnSemanticThemeChange(fn func()) {
	if fn == nil {
		themeChangeHook.Store(nil)
		return
	}
	themeChangeHook.Store(&fn)
}

func fireThemeChange() {
	if p := themeChangeHook.Load(); p != nil {
		(*p)()
	}
}

// ---------------------------------------------------------------------------
// Reduce motion (accessibility)
// ---------------------------------------------------------------------------

var reduceMotion atomic.Bool

// SetReduceMotion enables or disables animated UI elements. When true,
// spinners, progress bars, and other moving elements render as static
// placeholders. This supports users with vestibular motion disorders.
func SetReduceMotion(v bool) { reduceMotion.Store(v) }

// IsReduceMotion reports whether animated UI elements should be suppressed.
func IsReduceMotion() bool { return reduceMotion.Load() }

// SetSemanticTheme installs a semantic palette and rebuilds global Style*
// variables. Safe to call from theme hot-reload; concurrent Render may
// briefly see torn Style on 32-bit — prefer reloading when idle.
func SetSemanticTheme(sem *SemanticTheme, mode ColorMode) {
	if sem == nil {
		sem = DefaultSemanticLight()
	}
	SyncPaletteGlobals(sem, mode)
	fireThemeChange()
}

// ColorModeFromEnv reads TUI_COLORMODE: "truecolor" | "256" | "16" | empty = auto.
func ColorModeFromEnv() ColorMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TUI_COLORMODE"))) {
	case "256", "256color", "8bit":
		return ColorMode256
	case "truecolor", "24bit", "rgb":
		return ColorModeTruecolor
	case "16", "16color", "basic", "4bit":
		return ColorModeBasic
	default:
		return DetectColorMode()
	}
}

// DefaultSemanticForTerminal picks built-in dark vs light from COLORFGBG.
func DefaultSemanticForTerminal() *SemanticTheme {
	if DetectTerminalBackground() == "light" {
		return DefaultSemanticLight()
	}
	return DefaultMadyDark()
}

// InitThemeFromEnv loads TUI_THEME or AGENT_TUI_THEME JSON (pi-compatible subset),
// otherwise picks DefaultSemanticForTerminal. Color mode follows TUI_COLORMODE
// or DetectColorMode.
func InitThemeFromEnv() error {
	mode := ColorModeFromEnv()
	path := strings.TrimSpace(os.Getenv("TUI_THEME"))
	if path == "" {
		path = strings.TrimSpace(os.Getenv("AGENT_TUI_THEME"))
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sem, err := ParseSemanticThemeJSON(data, DefaultSemanticForTerminal())
		if err != nil {
			return err
		}
		SetSemanticTheme(sem, mode)
		return nil
	}
	SetSemanticTheme(DefaultSemanticForTerminal(), mode)
	return nil
}

// LoadSemanticThemeFromFile reads JSON and applies it with the given color mode.
func LoadSemanticThemeFromFile(path string, mode ColorMode) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sem, err := ParseSemanticThemeJSON(data, DefaultSemanticForTerminal())
	if err != nil {
		return err
	}
	SetSemanticTheme(sem, mode)
	return nil
}
