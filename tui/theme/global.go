package theme

import (
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

var themeChangeHook atomic.Pointer[func()]

// backgroundIsDark reports whether a semantic background hex colour is dark,
// using a WCAG-style relative-luminance threshold. This replaces the brittle
// `== "#07111F"` magic-hex check, which misclassified dark named themes
// (tokyo-night, high-contrast, grok-night, …) as light and made ToggleTheme
// switch between two dark themes (P2-7). Unknown/unparseable colors default
// to dark (the safer assumption for a terminal UI).
func backgroundIsDark(hex string) bool {
	r, g, b, ok := hexToRGB(hex)
	if !ok {
		return true
	}
	lin := func(c int64) float64 {
		v := float64(c) / 255.0
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	l := 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
	return l < 0.4
}

// ToggleTheme switches between the built-in light and dark semantic themes.
// It is safe to call concurrently; the first call infers the current theme
// from CurrentPalette() if SetSemanticTheme has not been called yet.
func ToggleTheme() {
	toggleMu.Lock()
	defer toggleMu.Unlock()
	mode := ColorModeFromEnv()
	// If isDark was never initialized, infer from the current palette.
	if !isDarkInitialized.Load() {
		if p := CurrentPalette(); p != nil && p.Semantic != nil {
			isDark.Store(backgroundIsDark(p.Semantic.Background))
		} else {
			// Default to dark if no palette available.
			isDark.Store(true)
		}
		isDarkInitialized.Store(true)
	}
	var sem *SemanticTheme
	if isDark.Load() {
		sem = DefaultSemanticLight()
	} else {
		sem = DefaultMadyDark()
	}
	isDark.Store(!isDark.Load())
	SetSemanticTheme(sem, mode)
	// A manual toggle is an explicit non-"auto" choice: stop following
	// the OS appearance so a later OS change cannot silently undo it
	// (P1-4 invariant; ToggleTheme bypasses ApplyThemeByName).
	currentThemeName.Store("")
}

var (
	toggleMu          sync.Mutex
	isDark            atomic.Bool // tracks the last SetSemanticTheme call for toggling
	isDarkInitialized atomic.Bool // false until ToggleTheme or SetSemanticTheme sets isDark
)

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
	// Track whether the current theme is dark for ToggleTheme.
	isDark.Store(backgroundIsDark(sem.Background))
	isDarkInitialized.Store(true)
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
	// A JSON-loaded theme is an explicit non-"auto" choice: stop
	// following the OS appearance (P1-4 invariant).
	currentThemeName.Store("")
	return nil
}
