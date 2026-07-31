package theme

import "testing"

func TestQuantizeRGBTo256(t *testing.T) {
	cases := []struct {
		r, g, b uint8
		want    uint8
	}{
		{0, 0, 0, 16},        // cube (0,0,0)
		{255, 0, 0, 196},     // cube (5,0,0)
		{0, 255, 0, 46},      // cube (0,5,0)
		{0, 0, 255, 21},      // cube (0,0,5)
		{255, 255, 255, 231}, // cube (5,5,5)
		{255, 0, 255, 201},   // cube (5,0,5)
		{0, 255, 255, 51},    // cube (0,5,5)
		{128, 128, 128, 244}, // grayscale ramp index 12 (128) -> 232+12
		{10, 10, 10, 232},    // grayscale ramp index 0 (8) -> 232+0
	}
	for _, tc := range cases {
		if got := QuantizeRGBTo256(tc.r, tc.g, tc.b); got != tc.want {
			t.Errorf("QuantizeRGBTo256(%d,%d,%d) = %d, want %d", tc.r, tc.g, tc.b, got, tc.want)
		}
	}
}

func TestQuantizeThemeLevels(t *testing.T) {
	sem := DefaultMadyDark()

	if got := QuantizeTheme(sem, ColorLevelTrueColor, true); got != sem {
		t.Fatal("ColorLevelTrueColor should return the input pointer unchanged")
	}
	if got := QuantizeTheme(sem, ColorLevel256, true); got != sem {
		t.Fatal("ColorLevel256 should return the input pointer unchanged")
	}
	if got := QuantizeTheme(sem, ColorLevelBasic, false); got != sem {
		t.Fatal("ColorLevelBasic should return the input pointer unchanged")
	}
	// nil input is also tolerated through every branch.
	if got := QuantizeTheme(nil, ColorLevel256, true); got != nil {
		t.Fatal("nil input with ColorLevel256 should stay nil")
	}
}

func TestRGBTo16ANSIInvalidHex(t *testing.T) {
	if _, ok := RGBTo16ANSI("not-a-color", true); ok {
		t.Fatal("invalid hex should report ok=false")
	}
	if _, ok := RGBTo16ANSI("#123", true); ok {
		t.Fatal("short hex should report ok=false")
	}
}

func TestFgParams16InvalidHex(t *testing.T) {
	if got := FgParams16("zzz", true); got != "" {
		t.Fatalf("FgParams16 invalid hex = %q, want empty", got)
	}
}

func TestBgParams16InvalidHex(t *testing.T) {
	if got := BgParams16("zzz", true); got != "" {
		t.Fatalf("BgParams16 invalid hex = %q, want empty", got)
	}
}

func TestQuantizeRGBTo16AllChannels(t *testing.T) {
	// Pure primaries on a light background map to the normal ANSI variants.
	cases := []struct {
		r, g, b uint8
		want    uint8
	}{
		{0, 0, 0, 0},       // black
		{170, 0, 0, 1},     // red
		{0, 170, 0, 2},     // green
		{170, 85, 0, 3},    // yellow
		{0, 0, 170, 4},     // blue
		{170, 0, 170, 5},   // magenta
		{0, 170, 170, 6},   // cyan
		{170, 170, 170, 7}, // white/gray
	}
	for _, tc := range cases {
		if got := QuantizeRGBTo16(tc.r, tc.g, tc.b, false); got != tc.want {
			t.Errorf("QuantizeRGBTo16(%d,%d,%d,false) = %d, want %d", tc.r, tc.g, tc.b, got, tc.want)
		}
	}
}
