package theme

import "testing"

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
