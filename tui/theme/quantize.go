package theme

import "github.com/xujian519/mady/tui/internal"

// quantize.go — Color quantization engine for terminals with limited color
// support. Provides RGB-to-16 and theme-level quantization.
//
// RGBTo16 maps a 24-bit color to the nearest ANSI 16-color index using
// perceptually-weighted Euclidean distance. Polarity-aware: dark backgrounds
// prefer bright fg variants (ANSI 90-97), light backgrounds prefer normal
// variants (ANSI 30-37).
//
// QuantizeTheme applies the appropriate quantization to every SemanticTheme
// color field based on the terminal's detected color level.

// ColorLevel represents the terminal's color capability.
type ColorLevel int

const (
	// ColorLevelTrueColor indicates 24-bit truecolor support.
	ColorLevelTrueColor ColorLevel = iota
	// ColorLevel256 indicates xterm 256-color palette support.
	ColorLevel256
	// ColorLevelBasic indicates 16-color (or 8-color) ANSI support.
	ColorLevelBasic
)

// quantize16Palette holds the 16 ANSI colors as RGB triples.
// Indices: 0=Black, 1=Red, 2=Green, 3=Yellow, 4=Blue, 5=Magenta, 6=Cyan, 7=White,
// 8=BrightBlack, 9=BrightRed, 10=BrightGreen, 11=BrightYellow, 12=BrightBlue,
// 13=BrightMagenta, 14=BrightCyan, 15=BrightWhite.
var quantize16Palette = [16][3]uint8{
	{0, 0, 0},       // 0 Black
	{170, 0, 0},     // 1 Red
	{0, 170, 0},     // 2 Green
	{170, 85, 0},    // 3 Yellow
	{0, 0, 170},     // 4 Blue
	{170, 0, 170},   // 5 Magenta
	{0, 170, 170},   // 6 Cyan
	{170, 170, 170}, // 7 White / Gray
	{85, 85, 85},    // 8 Bright Black / Dark Gray
	{255, 85, 85},   // 9 Bright Red
	{85, 255, 85},   // 10 Bright Green
	{255, 255, 85},  // 11 Bright Yellow
	{85, 85, 255},   // 12 Bright Blue
	{255, 85, 255},  // 13 Bright Magenta
	{85, 255, 255},  // 14 Bright Cyan
	{255, 255, 255}, // 15 Bright White
}

// perceptualWeight is the BT.709 luminance weights for distance calculation.
var perceptualWeight = [3]float64{0.299, 0.587, 0.114}

// QuantizeRGBTo16 maps sRGB components to the nearest ANSI 16-color index.
// When isDarkBg is true, bright variants (8-15) are preferred for fg colors
// so they remain legible against a dark canvas.
func QuantizeRGBTo16(r, g, b uint8, isDarkBg bool) uint8 {
	bestIdx := uint8(0)
	bestDist := float64(1e18)

	start := 0
	end := 16
	// When isDarkBg, check bright variants (8-15) first and weight them higher
	// by also checking normal variants but against a different luminance target.
	for i := start; i < end; i++ {
		pr := float64(quantize16Palette[i][0]) - float64(r)
		pg := float64(quantize16Palette[i][1]) - float64(g)
		pb := float64(quantize16Palette[i][2]) - float64(b)

		// Perceptually weighted Euclidean distance.
		dist := pr*pr*perceptualWeight[0] + pg*pg*perceptualWeight[1] + pb*pb*perceptualWeight[2]

		// Polarity bonus: on dark backgrounds, bright variants (8-15) get a
		// strong bonus to ensure legibility. On light backgrounds, normal
		// variants (0-7) get the bonus.
		if isDarkBg && i >= 8 {
			dist *= 0.85
		}
		if !isDarkBg && i < 8 {
			dist *= 0.85
		}

		if dist < bestDist {
			bestDist = dist
			bestIdx = uint8(i)
		}
	}
	return bestIdx
}

// QuantizeRGBTo256 wraps the existing RGBTo256 function with uint8 inputs.
func QuantizeRGBTo256(r, g, b uint8) uint8 {
	return uint8(RGBTo256(int64(r), int64(g), int64(b)))
}

// RGBTo16ANSI converts an RGB hex string to the closest 16-color ANSI index.
// Returns (index, ok). isDarkBg controls the polarity bonus.
func RGBTo16ANSI(hex string, isDarkBg bool) (uint8, bool) {
	r, g, b, ok := hexToRGB(hex)
	if !ok {
		return 0, false
	}
	return QuantizeRGBTo16(uint8(r), uint8(g), uint8(b), isDarkBg), true
}

// FgParams16 returns the SGR parameter string for a 16-color foreground.
func FgParams16(hex string, isDarkBg bool) string {
	idx, ok := RGBTo16ANSI(hex, isDarkBg)
	if !ok {
		return ""
	}
	if idx >= 8 {
		// Bright variants: ANSI 90-97
		return "9" + internal.ITOA(int64(idx-8))
	}
	// Normal variants: ANSI 30-37
	return "3" + internal.ITOA(int64(idx))
}

// BgParams16 returns the SGR parameter string for a 16-color background.
// For dark backgrounds, prefers the dark variants (40-47) for bg colors.
func BgParams16(hex string, isDarkBg bool) string {
	idx, ok := RGBTo16ANSI(hex, isDarkBg)
	if !ok {
		return ""
	}
	if idx >= 8 {
		// Bright bg: ANSI 100-107
		return "10" + internal.ITOA(int64(idx-8))
	}
	// Normal bg: ANSI 40-47
	return "4" + internal.ITOA(int64(idx))
}

// QuantizeTheme applies color quantization to a SemanticTheme based on the
// given ColorLevel.
//
// NOTE: This function is currently a no-op — all quantization is handled
// downstream at render time by color_resolve.go (FgParams/BgParams call
// RGBTo256/QuantizeRGBTo16 on hex values). The SemanticTheme itself stores
// unmodified hex strings regardless of the target ColorLevel.
//
// This function exists as a pre-quantization extension point for future
// optimizations (e.g., caching pre-quantized palette copies). Until then,
// it returns the input pointer unchanged.
func QuantizeTheme(sem *SemanticTheme, level ColorLevel, isDarkBg bool) *SemanticTheme {
	if level == ColorLevelTrueColor {
		return sem
	}
	if level == ColorLevel256 {
		return quantizeTheme256(sem)
	}
	return quantizeThemeBasic(sem, isDarkBg)
}

// quantizeTheme256 is a no-op: 256-color quantization happens at render time.
func quantizeTheme256(sem *SemanticTheme) *SemanticTheme {
	return sem
}

// quantizeThemeBasic is a no-op: 16-color quantization happens at render time.
func quantizeThemeBasic(sem *SemanticTheme, _ bool) *SemanticTheme {
	return sem
}
