package theme

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// saveToggleState saves and restores the ToggleTheme globals so tests do not
// leak state into each other (the isDark / isDarkInitialized atomics are
// package-internal; tests may read and write them directly).
func saveToggleState(t *testing.T) {
	t.Helper()
	oldDark := isDark.Load()
	oldInit := isDarkInitialized.Load()
	t.Cleanup(func() {
		isDark.Store(oldDark)
		isDarkInitialized.Store(oldInit)
	})
}

func TestToggleThemeDarkToLight(t *testing.T) {
	savePalette(t)
	saveToggleState(t)
	t.Setenv("TUI_COLORMODE", "truecolor")

	isDark.Store(true)
	isDarkInitialized.Store(true)
	SetSemanticTheme(DefaultMadyDark(), ColorModeTruecolor)

	ToggleTheme()
	if got := CurrentPalette().Semantic.Name; got != "light" {
		t.Fatalf("after first toggle: want light theme, got %q", got)
	}

	ToggleTheme()
	if got := CurrentPalette().Semantic.Name; got != "dark" {
		t.Fatalf("after second toggle: want dark theme, got %q", got)
	}
}

func TestToggleThemeInfersDarkFromPalette(t *testing.T) {
	savePalette(t)
	saveToggleState(t)
	t.Setenv("TUI_COLORMODE", "truecolor")

	// Palette is the dark brand theme (background #07111F) but the toggle
	// state was never initialized -> inference should pick dark and toggle
	// to light.
	SetSemanticTheme(DefaultMadyDark(), ColorModeTruecolor)
	isDarkInitialized.Store(false)

	ToggleTheme()
	if got := CurrentPalette().Semantic.Name; got != "light" {
		t.Fatalf("inferred dark palette should toggle to light, got %q", got)
	}
}

func TestToggleThemeInfersLightFromPalette(t *testing.T) {
	savePalette(t)
	saveToggleState(t)
	t.Setenv("TUI_COLORMODE", "truecolor")

	SetSemanticTheme(DefaultSemanticLight(), ColorModeTruecolor)
	isDarkInitialized.Store(false)

	ToggleTheme()
	if got := CurrentPalette().Semantic.Name; got != "dark" {
		t.Fatalf("inferred light palette should toggle to dark, got %q", got)
	}
}

func TestSetOnSemanticThemeChangeFires(t *testing.T) {
	savePalette(t)
	t.Cleanup(func() { SetOnSemanticThemeChange(nil) })

	fired := make(chan struct{}, 1)
	SetOnSemanticThemeChange(func() { fired <- struct{}{} })
	SetSemanticTheme(DefaultMadyDark(), ColorModeTruecolor)

	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("theme change callback was not invoked")
	}
}

func TestSetOnSemanticThemeChangeNilClears(t *testing.T) {
	savePalette(t)
	SetOnSemanticThemeChange(nil)
	// Must not panic and must not invoke any stale callback.
	SetSemanticTheme(DefaultMadyDark(), ColorModeTruecolor)
	SetSemanticTheme(DefaultSemanticLight(), ColorModeTruecolor)
}

func TestSetReduceMotion(t *testing.T) {
	SetReduceMotion(true)
	if !IsReduceMotion() {
		t.Fatal("IsReduceMotion() = false after SetReduceMotion(true)")
	}
	SetReduceMotion(false)
	if IsReduceMotion() {
		t.Fatal("IsReduceMotion() = true after SetReduceMotion(false)")
	}
}

func TestColorModeFromEnv(t *testing.T) {
	cases := []struct {
		env  string
		want ColorMode
	}{
		{"256", ColorMode256},
		{"256color", ColorMode256},
		{"8bit", ColorMode256},
		{"truecolor", ColorModeTruecolor},
		{"24bit", ColorModeTruecolor},
		{"rgb", ColorModeTruecolor},
		{"16", ColorModeBasic},
		{"16color", ColorModeBasic},
		{"basic", ColorModeBasic},
		{"4bit", ColorModeBasic},
		// Case-insensitive + whitespace trimming.
		{"TRUECOLOR", ColorModeTruecolor},
		{"  256  ", ColorMode256},
	}
	for _, tc := range cases {
		t.Run(tc.env, func(t *testing.T) {
			t.Setenv("TUI_COLORMODE", tc.env)
			if got := ColorModeFromEnv(); got != tc.want {
				t.Fatalf("ColorModeFromEnv(%q) = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
	t.Run("empty-falls-back-to-detection", func(t *testing.T) {
		t.Setenv("TUI_COLORMODE", "")
		if got := ColorModeFromEnv(); got == 0 {
			t.Fatal("ColorModeFromEnv with empty env returned zero ColorMode")
		}
	})
	t.Run("garbage-falls-back-to-detection", func(t *testing.T) {
		t.Setenv("TUI_COLORMODE", "banana")
		if got := ColorModeFromEnv(); got == 0 {
			t.Fatal("ColorModeFromEnv with garbage env returned zero ColorMode")
		}
	})
}

func writeThemeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const testThemeJSON = `{"name":"env-theme","colors":{"accent":"#010203","text":"#040506"}}`

func TestInitThemeFromEnv(t *testing.T) {
	savePalette(t)
	t.Setenv("TUI_COLORMODE", "truecolor")
	t.Setenv("AGENT_TUI_THEME", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "theme.json")
	writeThemeJSON(t, path, testThemeJSON)

	t.Setenv("TUI_THEME", path)
	if err := InitThemeFromEnv(); err != nil {
		t.Fatalf("InitThemeFromEnv: %v", err)
	}
	if got := CurrentPalette().Semantic.Name; got != "env-theme" {
		t.Fatalf("theme name = %q, want env-theme", got)
	}
	if got := CurrentPalette().Semantic.Accent; got != "#010203" {
		t.Fatalf("accent = %q, want #010203", got)
	}
}

func TestInitThemeFromEnvAgentFallback(t *testing.T) {
	savePalette(t)
	t.Setenv("TUI_COLORMODE", "truecolor")

	dir := t.TempDir()
	path := filepath.Join(dir, "theme.json")
	writeThemeJSON(t, path, testThemeJSON)

	// TUI_THEME unset; AGENT_TUI_THEME is used as fallback.
	t.Setenv("TUI_THEME", "")
	t.Setenv("AGENT_TUI_THEME", path)
	if err := InitThemeFromEnv(); err != nil {
		t.Fatalf("InitThemeFromEnv (AGENT_TUI_THEME): %v", err)
	}
	if got := CurrentPalette().Semantic.Name; got != "env-theme" {
		t.Fatalf("theme name = %q, want env-theme", got)
	}
}

func TestInitThemeFromEnvMissingFile(t *testing.T) {
	t.Setenv("TUI_THEME", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("AGENT_TUI_THEME", "")
	if err := InitThemeFromEnv(); err == nil {
		t.Fatal("InitThemeFromEnv with missing file should return error")
	}
}

func TestInitThemeFromEnvInvalidJSON(t *testing.T) {
	t.Setenv("TUI_THEME", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	writeThemeJSON(t, path, "{not valid json")
	t.Setenv("AGENT_TUI_THEME", path)
	if err := InitThemeFromEnv(); err == nil {
		t.Fatal("InitThemeFromEnv with invalid JSON should return error")
	}
}

func TestInitThemeFromEnvNoEnvUsesDefault(t *testing.T) {
	savePalette(t)
	t.Setenv("TUI_THEME", "")
	t.Setenv("AGENT_TUI_THEME", "")
	if err := InitThemeFromEnv(); err != nil {
		t.Fatalf("InitThemeFromEnv with no env: %v", err)
	}
	sem := CurrentPalette().Semantic
	if sem == nil {
		t.Fatal("no semantic theme installed")
	}
	if sem.Name != "dark" && sem.Name != "light" {
		t.Fatalf("expected built-in dark/light theme, got %q", sem.Name)
	}
}

func TestLoadSemanticThemeFromFile(t *testing.T) {
	savePalette(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.json")
	writeThemeJSON(t, path, testThemeJSON)

	if err := LoadSemanticThemeFromFile(path, ColorModeTruecolor); err != nil {
		t.Fatalf("LoadSemanticThemeFromFile: %v", err)
	}
	if got := CurrentPalette().Semantic.Accent; got != "#010203" {
		t.Fatalf("accent = %q, want #010203", got)
	}
	if got := CurrentPalette().Mode; got != ColorModeTruecolor {
		t.Fatalf("mode = %v, want truecolor", got)
	}
}

func TestLoadSemanticThemeFromFileMissing(t *testing.T) {
	if err := LoadSemanticThemeFromFile(filepath.Join(t.TempDir(), "nope.json"), ColorModeTruecolor); err == nil {
		t.Fatal("missing file should return error")
	}
}

func TestLoadSemanticThemeFromFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	writeThemeJSON(t, path, "{}oops")
	if err := LoadSemanticThemeFromFile(path, ColorModeTruecolor); err == nil {
		t.Fatal("invalid JSON should return error")
	}
}

func TestSetSemanticThemeNilUsesLightDefault(t *testing.T) {
	savePalette(t)
	SetSemanticTheme(nil, ColorModeTruecolor)
	if got := CurrentPalette().Semantic.Name; got != "light" {
		t.Fatalf("SetSemanticTheme(nil) should install light default, got %q", got)
	}
}

// TestCurrentIsDarkBgDoesNotBuildPalette is a regression test for the P2-9
// infinite recursion: currentIsDarkBg used to consult CurrentPalette(),
// which re-enters BuildPalette while the palette is not yet stored. In
// ColorModeBasic that recursed until stack overflow; in every mode it
// forced a palette build from a pure polarity query.
func TestCurrentIsDarkBgDoesNotBuildPalette(t *testing.T) {
	savePalette(t)
	atomicPalette.Store(nil)
	origDark := isDark.Load()
	origInit := isDarkInitialized.Load()
	t.Cleanup(func() {
		isDark.Store(origDark)
		isDarkInitialized.Store(origInit)
	})
	_ = currentIsDarkBg()
	if p := atomicPalette.Load(); p != nil {
		t.Fatal("currentIsDarkBg must not construct the palette (infinite recursion regression)")
	}
}
