package theme

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/terminal"
)

// clearTerminalEnv removes env vars that the terminal detection system keys
// on, so DetectColorMode only sees the variables a test explicitly sets.
func clearTerminalEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"COLORTERM", "WT_SESSION", "TERM_PROGRAM", "TERM_PROFILE",
		"KITTY_WINDOW_ID", "WEZTERM_EXECUTABLE", "VSCODE_INJECTION",
		"ALACRITTY_LOG", "GHOSTTY_RESOURCES_DIR", "WARP_IS_LOCAL_SHELL_SESSION",
		"ZELLIJ", "NVIM", "TMUX", "STY",
	} {
		t.Setenv(k, "")
	}
	terminal.ResetTerminalContext()
}

func TestDetectColorModeFallbacks(t *testing.T) {
	cases := []struct {
		term string
		want ColorMode
	}{
		{"dumb", ColorModeBasic},
		{"linux", ColorModeBasic},
		{"", ColorModeBasic},
		{"screen", ColorMode256},
		{"screen-256color", ColorMode256},
		{"screen.xterm", ColorMode256},
		{"xterm-256color", ColorModeTruecolor},
	}
	for _, tc := range cases {
		t.Run("term="+tc.term, func(t *testing.T) {
			clearTerminalEnv(t)
			t.Setenv("TERM", tc.term)
			if got := DetectColorMode(); got != tc.want {
				t.Fatalf("DetectColorMode(TERM=%q) = %v, want %v", tc.term, got, tc.want)
			}
		})
	}
}

func TestDetectTerminalBackgroundBranches(t *testing.T) {
	cases := []struct {
		name string
		set  map[string]string
		want string
	}{
		{"fgbg-no-semicolon", map[string]string{"COLORFGBG": "15"}, "dark"},
		{"fgbg-nonnumeric-bg", map[string]string{"COLORFGBG": "15;abc"}, "dark"},
		{"dark-background-env-empty", map[string]string{"DARK_BACKGROUND": ""}, "dark"},
		{"colorterm-apple-dark-profile", map[string]string{
			"COLORTERM": "truecolor", "TERM_PROGRAM": "Apple_Terminal", "TERM_PROFILE": "Pro",
		}, "dark"},
		{"colorterm-apple-light-profile", map[string]string{
			"COLORTERM": "truecolor", "TERM_PROGRAM": "Apple_Terminal", "TERM_PROFILE": "Default",
		}, "light"},
		{"colorterm-vscode", map[string]string{"COLORTERM": "truecolor", "TERM_PROGRAM": "vscode"}, "dark"},
		{"colorterm-tmux", map[string]string{"COLORTERM": "truecolor", "TERM_PROGRAM": "tmux"}, "dark"},
		{"colorterm-other-terminal", map[string]string{"COLORTERM": "truecolor", "TERM_PROGRAM": "mystery"}, "dark"},
		{"termprog-apple-no-colorterm", map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, "light"},
		{"termprog-vscode", map[string]string{"TERM_PROGRAM": "vscode"}, "dark"},
		{"termprog-ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, "dark"},
		{"termprog-kitty", map[string]string{"TERM_PROGRAM": "kitty"}, "dark"},
		{"empty-env", map[string]string{}, "dark"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearTerminalEnv(t)
			for k, v := range tc.set {
				t.Setenv(k, v)
			}
			if got := DetectTerminalBackground(); got != tc.want {
				t.Fatalf("DetectTerminalBackground(%v) = %q, want %q", tc.set, got, tc.want)
			}
		})
	}
}

func TestHexToRGB(t *testing.T) {
	if _, _, _, ok := hexToRGB("123"); ok {
		t.Fatal("short hex should fail")
	}
	if _, _, _, ok := hexToRGB("zzzzzz"); ok {
		t.Fatal("non-hex characters should fail")
	}
	if _, _, _, ok := hexToRGB(""); ok {
		t.Fatal("empty string should fail")
	}

	r, g, b, ok := hexToRGB("#1a2b3c")
	if !ok || r != 0x1a || g != 0x2b || b != 0x3c {
		t.Fatalf("hexToRGB(#1a2b3c) = %d,%d,%d,%v", r, g, b, ok)
	}

	// Uppercase and missing "#" prefix are accepted.
	r, g, b, ok = hexToRGB("FF0000")
	if !ok || r != 255 || g != 0 || b != 0 {
		t.Fatalf("hexToRGB(FF0000) = %d,%d,%d,%v", r, g, b, ok)
	}
}

func TestFgParamsNumericAndInvalid(t *testing.T) {
	if got := FgParams("15", ColorModeTruecolor); got != "38;5;15" {
		t.Fatalf("numeric fg = %q, want 38;5;15", got)
	}
	if got := FgParams("0", ColorModeTruecolor); got != "38;5;0" {
		t.Fatalf("zero fg = %q, want 38;5;0", got)
	}
	if got := FgParams("255", ColorModeTruecolor); got != "38;5;255" {
		t.Fatalf("max numeric fg = %q, want 38;5;255", got)
	}
	if got := FgParams("256", ColorModeTruecolor); got != "" {
		t.Fatalf("out-of-range numeric fg = %q, want empty", got)
	}
	if got := FgParams("-1", ColorModeTruecolor); got != "" {
		t.Fatalf("negative fg = %q, want empty", got)
	}
	if got := FgParams("bad", ColorModeTruecolor); got != "" {
		t.Fatalf("invalid fg = %q, want empty", got)
	}
	if got := FgParams("", ColorModeTruecolor); got != "" {
		t.Fatalf("empty fg = %q, want empty", got)
	}
	if got := FgParams("  #ff0000  ", ColorModeTruecolor); got != "38;2;255;0;0" {
		t.Fatalf("trimmed fg = %q, want 38;2;255;0;0", got)
	}
	// Basic mode quantizes to the 16-color palette.
	if got := FgParams("#ff0000", ColorModeBasic); got == "" {
		t.Fatal("Basic mode fg should not be empty")
	}
}

func TestBgParamsNumericAndInvalid(t *testing.T) {
	if got := BgParams("15", ColorModeTruecolor); got != "48;5;15" {
		t.Fatalf("numeric bg = %q, want 48;5;15", got)
	}
	if got := BgParams("256", ColorModeTruecolor); got != "" {
		t.Fatalf("out-of-range numeric bg = %q, want empty", got)
	}
	if got := BgParams("bad", ColorModeTruecolor); got != "" {
		t.Fatalf("invalid bg = %q, want empty", got)
	}
	if got := BgParams("", ColorModeTruecolor); got != "" {
		t.Fatalf("empty bg = %q, want empty", got)
	}
	if got := BgParams("#00ff00", ColorModeTruecolor); got != "48;2;0;255;0" {
		t.Fatalf("hex bg = %q, want 48;2;0;255;0", got)
	}
	if got := BgParams("#00ff00", ColorModeBasic); got == "" {
		t.Fatal("Basic mode bg should not be empty")
	}
	if got := BgParams("#00ff00", ColorMode256); !strings.HasPrefix(got, "48;5;") {
		t.Fatalf("256 mode bg = %q, want 48;5; prefix", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "b"); got != "b" {
		t.Fatalf("firstNonEmpty(\"\", b) = %q", got)
	}
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Fatalf("firstNonEmpty(a, b) = %q", got)
	}
}
