package theme

import (
	"testing"
)

func TestQuantizeRGBTo16_Red(t *testing.T) {
	// Normal red (170,0,0) → ANSI Red (1) on either background.
	idx := QuantizeRGBTo16(170, 0, 0, false)
	if idx != 1 {
		t.Errorf("normal red: got %d, want 1", idx)
	}

	// Hot pink / light red → BrightRed (9) on dark bg.
	idx = QuantizeRGBTo16(255, 120, 120, true)
	if idx != 9 {
		t.Errorf("light red on dark bg: got %d, want 9 (BrightRed)", idx)
	}

	// On light background, normal red (170,0,0) is the closest.
	idx = QuantizeRGBTo16(170, 0, 0, false)
	if idx != 1 {
		t.Errorf("pure red on light bg: got %d, want 1", idx)
	}
}

func TestQuantizeRGBTo16_Green(t *testing.T) {
	// Normal green (0,170,0) → ANSI Green (2).
	idx := QuantizeRGBTo16(0, 170, 0, false)
	if idx != 2 {
		t.Errorf("normal green: got %d, want 2", idx)
	}

	// Bright green on dark bg.
	idx = QuantizeRGBTo16(80, 255, 80, true)
	if idx != 10 {
		t.Errorf("bright green on dark bg: got %d, want 10 (BrightGreen)", idx)
	}
}

func TestQuantizeRGBTo16_Blue(t *testing.T) {
	// Normal blue (0,0,170) → ANSI Blue (4).
	idx := QuantizeRGBTo16(0, 0, 170, false)
	if idx != 4 {
		t.Errorf("normal blue: got %d, want 4", idx)
	}

	// Bright blue on dark bg.
	idx = QuantizeRGBTo16(60, 60, 255, true)
	if idx != 12 {
		t.Errorf("bright blue on dark bg: got %d, want 12 (BrightBlue)", idx)
	}
}

func TestQuantizeRGBTo16_Gray(t *testing.T) {
	// Mid-gray (136,136,136) should map to a gray slot.
	idx := QuantizeRGBTo16(136, 136, 136, true)
	if idx != 8 && idx != 7 {
		t.Errorf("gray on dark bg: got %d, want 7 or 8", idx)
	}
}

func TestQuantizeRGBTo16_Yellow(t *testing.T) {
	// Warm yellow on dark bg → BrightYellow (11).
	// Uses the 0.85 polarity bonus to prefer the bright variant.
	idx := QuantizeRGBTo16(220, 200, 50, true)
	if idx != 11 {
		t.Errorf("bright yellow on dark bg: got %d, want 11 (BrightYellow)", idx)
	}
}

func TestQuantizeRGBTo16_White(t *testing.T) {
	// Near-white text on dark bg → BrightWhite (15).
	idx := QuantizeRGBTo16(225, 225, 225, true)
	if idx != 15 {
		t.Errorf("white text on dark bg: got %d, want 15 (BrightWhite)", idx)
	}
}

func TestQuantizeRGBTo16_PolarityBonus(t *testing.T) {
	// Same color, different backgrounds → different ANSI indices.
	// Use a medium-brightness gray where polarity bonus should produce
	// different results on dark vs light backgrounds.
	// Mid-gray (128,128,128) is equally close to BrightBlack(8)=(85,85,85)
	// and Gray(7)=(170,170,170). Polarity bonus should tip it differently.
	dark := QuantizeRGBTo16(128, 128, 128, true)
	light := QuantizeRGBTo16(128, 128, 128, false)
	if dark == light {
		t.Errorf("polarity should produce different indices for mid-gray: dark=%d light=%d", dark, light)
	}
	// On dark bg, bright variant (8-15) should be preferred.
	if dark < 8 {
		t.Errorf("dark bg polarity: expected bright variant (>=8), got %d", dark)
	}
	// On light bg, normal variant (0-7) should be preferred.
	if light >= 8 {
		t.Errorf("light bg polarity: expected normal variant (<8), got %d", light)
	}
}
