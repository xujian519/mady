package theme

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/xujian519/mady/tui/terminal"
)

// ColorMode selects how hex colors are encoded for the terminal.
type ColorMode int64

const (
	// ColorModeTruecolor emits 38;2;r;g;b / 48;2;r;g;b when COLORTERM etc.
	// indicates 24-bit support.
	ColorModeTruecolor ColorMode = iota + 1
	// ColorMode256 maps hex to the nearest xterm 256-color index.
	ColorMode256
	// ColorModeBasic maps hex to the nearest 16-color ANSI index (3x/4x or 9x/10x).
	ColorModeBasic
)

// DetectColorMode mirrors common heuristics (similar to pi-mono coding-agent).
// The truecolor check is delegated to terminal.CurrentTerminalContext for
// consistent brand-based detection across the module; 256/Basic fallback
// uses legacy env-var heuristics.
func DetectColorMode() ColorMode {
	// Delegate truecolor detection to the terminal brand-based system, which
	// has a more complete detection table (VTE versions, SSH multiplexers,
	// Windows Terminal, etc.). If the terminal context hasn't been initialized
	// yet, DetectTerminalContext lazy-initializes it.
	ok, _ := terminal.CurrentTerminalContext().HasTrueColor()
	if ok {
		return ColorModeTruecolor
	}

	// Fallback for 256-color / basic detection.
	term := os.Getenv("TERM")
	if term == "dumb" || term == "" || term == "linux" {
		return ColorModeBasic
	}
	if os.Getenv("TERM_PROGRAM") == "Apple_Terminal" {
		return ColorMode256
	}
	if term == "screen" || strings.HasPrefix(term, "screen-") || strings.HasPrefix(term, "screen.") {
		return ColorMode256
	}
	return ColorModeTruecolor
}

// DetectTerminalBackground returns "dark" or "light" using terminal environment
// heuristics. Detection sources in priority order:
//
//  1. COLORFGBG env var (standard in xterm, VTE, iTerm2, WezTerm).
//     The second field is the background color index; indices < 8 indicate a
//     dark background, >= 8 indicate light.
//
//  2. DARK_BACKGROUND env var (set by VS Code and some tmux configs).
//     Any value (including empty) signals dark; absent means "not known".
//
//  3. COLORTERM env var with value "truecolor" (common in modern terminals).
//     White-listed terminals are assumed dark (the most common config).
//
//  4. TERM_PROGRAM environment (Apple_Terminal, vscode, tmux, etc.).
//
//  5. Falls back to "dark" (the safe default — most terminal themes are dark).
func DetectTerminalBackground() string {
	// 1. COLORFGBG: standard xterm/VTE env var with explicit bg index.
	fgbg := os.Getenv("COLORFGBG")
	if fgbg != "" {
		parts := strings.Split(fgbg, ";")
		if len(parts) >= 2 {
			bg, err := strconv.Atoi(parts[1])
			if err == nil {
				if bg < 8 {
					return "dark"
				}
				return "light"
			}
		}
	}

	// 2. DARK_BACKGROUND: set by VS Code and some terminal multiplexers.
	if _, ok := os.LookupEnv("DARK_BACKGROUND"); ok {
		return "dark"
	}

	// 3. COLORTERM truecolor: detect common light-terminal brands.
	// Most TERM_PROGRAM values that set COLORTERM=truecolor default to dark,
	// but Apple_Terminal defaults to light (Pro theme is opt-in).
	if os.Getenv("COLORTERM") == "truecolor" {
		switch os.Getenv("TERM_PROGRAM") {
		case "Apple_Terminal":
			// Terminal.app defaults to light; check for known-dark overrides:
			// "Pro" profile name. This is a heuristic — not all dark themes
			// contain "pro", but it covers the most common case.
			if profile := os.Getenv("TERM_PROFILE"); strings.Contains(strings.ToLower(profile), "pro") ||
				strings.Contains(strings.ToLower(profile), "dark") {
				return "dark"
			}
			return "light"
		case "vscode", "tmux":
			return "dark"
		}
		// Most truecolor-capable terminals default dark.
		return "dark"
	}

	// 4. TERM_PROGRAM-based heuristics for terminals that don't set COLORFGBG.
	switch os.Getenv("TERM_PROGRAM") {
	case "Apple_Terminal":
		return "light"
	case "vscode", "ghostty", "kitty", "alacritty", "foot", "wezterm":
		return "dark"
	}

	return "dark"
}

func hexToRGB(hex string) (r, g, b int64, ok bool) {
	h := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(hex), "#"))
	if len(h) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	n := int64(v)
	return (n >> 16) & 0xff, (n >> 8) & 0xff, n & 0xff, true
}

var cubeValues = []int64{0, 95, 135, 175, 215, 255}

func findClosestCubeIndex(value int64) int64 {
	var minIdx int64
	var minDist int64 = 1 << 62
	for i := int64(0); i < int64(len(cubeValues)); i++ {
		d := value - cubeValues[i]
		if d < 0 {
			d = -d
		}
		if d < minDist {
			minDist = d
			minIdx = i
		}
	}
	return minIdx
}

var grayRamp []int64

func init() {
	grayRamp = make([]int64, 24)
	for i := int64(0); i < 24; i++ {
		grayRamp[i] = 8 + i*10
	}
}

func colorDistance(r1, g1, b1, r2, g2, b2 int64) float64 {
	dr := float64(r1 - r2)
	dg := float64(g1 - g2)
	db := float64(b1 - b2)
	return dr*dr*0.299 + dg*dg*0.587 + db*db*0.114
}

func findClosestGrayIndex(gray int64) int64 {
	var minIdx int64
	var minDist = 1e18
	for i := int64(0); i < int64(len(grayRamp)); i++ {
		gv := grayRamp[i]
		d := colorDistance(gray, gray, gray, gv, gv, gv)
		if d < minDist {
			minDist = d
			minIdx = i
		}
	}
	return minIdx
}

// RGBTo256 maps sRGB components to an xterm 256 palette index (16–231 cube or 232–255 gray).
func RGBTo256(r, g, b int64) int64 {
	rIdx := findClosestCubeIndex(r)
	gIdx := findClosestCubeIndex(g)
	bIdx := findClosestCubeIndex(b)
	cubeR := cubeValues[rIdx]
	cubeG := cubeValues[gIdx]
	cubeB := cubeValues[bIdx]
	cubeIndex := 16 + 36*rIdx + 6*gIdx + bIdx
	cubeDist := colorDistance(r, g, b, cubeR, cubeG, cubeB)

	gray := int64(0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b))
	grayIdx := findClosestGrayIndex(gray)
	grayValue := grayRamp[grayIdx]
	grayIndex := 232 + grayIdx
	grayDist := colorDistance(r, g, b, grayValue, grayValue, grayValue)

	maxC := r
	minC := r
	if g > maxC {
		maxC = g
	}
	if b > maxC {
		maxC = b
	}
	if g < minC {
		minC = g
	}
	if b < minC {
		minC = b
	}
	spread := maxC - minC
	if spread < 10 && grayDist < cubeDist {
		return grayIndex
	}
	return cubeIndex
}

// FgParams returns the CSI SGR parameter segment for a foreground color value.
// Empty string means default foreground. Values: "#rrggbb" or decimal index 0–255.
func FgParams(value string, mode ColorMode) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if n, err := strconv.ParseInt(value, 10, 32); err == nil && n >= 0 && n <= 255 {
		if mode == ColorModeBasic {
			// In basic mode a raw 256-color index must be folded into the
			// 16-color palette — the terminal ignores 38;5;n (P2-8).
			return FgParams16Index(n, currentIsDarkBg())
		}
		return fmt.Sprintf("38;5;%d", n)
	}
	r, g, b, ok := hexToRGB(value)
	if !ok {
		return ""
	}
	if mode == ColorModeTruecolor {
		return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
	}
	if mode == ColorModeBasic {
		// Polarity is derived from the active semantic background so light
		// themes pick legible normal variants instead of always assuming a
		// dark background (P2-9).
		return FgParams16(value, currentIsDarkBg())
	}
	idx := RGBTo256(r, g, b)
	return fmt.Sprintf("38;5;%d", idx)
}

// BgParams returns CSI parameters for a background (48;…).
func BgParams(value string, mode ColorMode) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if n, err := strconv.ParseInt(value, 10, 32); err == nil && n >= 0 && n <= 255 {
		if mode == ColorModeBasic {
			return BgParams16Index(n, currentIsDarkBg())
		}
		return fmt.Sprintf("48;5;%d", n)
	}
	r, g, b, ok := hexToRGB(value)
	if !ok {
		return ""
	}
	if mode == ColorModeTruecolor {
		return fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
	}
	if mode == ColorModeBasic {
		return BgParams16(value, currentIsDarkBg())
	}
	idx := RGBTo256(r, g, b)
	return fmt.Sprintf("48;5;%d", idx)
}

// currentIsDarkBg reports whether the active semantic theme has a dark
// background, used for 16-color polarity decisions. Defaults to true when
// no theme has been applied yet (most themes are dark).
//
// It MUST NOT consult CurrentPalette(): BuildPalette calls FgParams/BgParams,
// which call back into currentIsDarkBg while the palette has not been stored
// yet — an infinite recursion that overflows the stack on the first palette
// build in ColorModeBasic (e.g. TERM=dumb or a non-TTY process). The isDark
// flag that SetSemanticTheme/ToggleTheme already maintain is the same
// backgroundIsDark(sem.Background) value, so reading it here is equivalent
// and side-effect free.
func currentIsDarkBg() bool {
	if isDarkInitialized.Load() {
		return isDark.Load()
	}
	return true
}
