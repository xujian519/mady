package theme

import "strconv"

// quantize.go — Color quantization engine for terminals with limited color
// support. Provides RGB-to-16 conversion.
//
// RGBTo16 maps a 24-bit color to the nearest ANSI 16-color index using
// perceptually-weighted Euclidean distance. Polarity-aware: dark backgrounds
// prefer bright fg variants (ANSI 90-97), light backgrounds prefer normal
// variants (ANSI 30-37). (Theme-level pre-quantization was removed: all
// quantization happens at render time in color_resolve.go.)

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
		return "9" + strconv.FormatInt(int64(idx-8), 10)
	}
	// Normal variants: ANSI 30-37
	return "3" + strconv.FormatInt(int64(idx), 10)
}

// BgParams16 returns the SGR parameter string for a 16-color background.
// For dark backgrounds, prefers the bright variants (100-107) so the
// background stays distinguishable from the canvas.
func BgParams16(hex string, isDarkBg bool) string {
	idx, ok := RGBTo16ANSI(hex, isDarkBg)
	if !ok {
		return ""
	}
	if idx >= 8 {
		// Bright bg: ANSI 100-107
		return "10" + strconv.FormatInt(int64(idx-8), 10)
	}
	// Normal bg: ANSI 40-47
	return "4" + strconv.FormatInt(int64(idx), 10)
}

// indexToRGB resolves an xterm 256-color index (0-255) to its canonical RGB:
// 0-15 system colors, 16-231 the 6×6×6 colour cube, 232-255 the grey ramp.
// Used to fold 256-color indexes into the 16-color palette in basic mode.
func indexToRGB(n int64) (r, g, b int64) {
	sys := [16][3]int64{
		{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0},
		{0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
		{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
		{0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
	}
	switch {
	case n >= 0 && n < 16:
		return sys[n][0], sys[n][1], sys[n][2] //nolint:gosec // G602: 分支前置条件 n∈[0,16) 已保证索引安全，gosec 无法跨分支推导
	case n < 232:
		n -= 16
		cube := [6]int64{0, 95, 135, 175, 215, 255}
		//nolint:gosec // G602: n<232 分支内 n 已减 16，n/36 ∈ [0,5] 不会越界；gosec 静态分析无法推导分支前置条件
		return cube[n/36], cube[(n%36)/6], cube[n%6]
	default:
		v := 8 + 10*(n-232)
		return v, v, v
	}
}

// FgParams16Index folds a 256-color index into the nearest ANSI 16-color
// foreground parameter with the given polarity (P2-8).
func FgParams16Index(n int64, isDarkBg bool) string {
	r, g, b := indexToRGB(n)
	idx := QuantizeRGBTo16(uint8(r), uint8(g), uint8(b), isDarkBg)
	if idx >= 8 {
		return "9" + strconv.FormatInt(int64(idx-8), 10)
	}
	return "3" + strconv.FormatInt(int64(idx), 10)
}

// BgParams16Index folds a 256-color index into the nearest ANSI 16-color
// background parameter with the given polarity (P2-8).
func BgParams16Index(n int64, isDarkBg bool) string {
	r, g, b := indexToRGB(n)
	idx := QuantizeRGBTo16(uint8(r), uint8(g), uint8(b), isDarkBg)
	if idx >= 8 {
		return "10" + strconv.FormatInt(int64(idx-8), 10)
	}
	return "4" + strconv.FormatInt(int64(idx), 10)
}
