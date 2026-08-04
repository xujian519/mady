package theme

import (
	"strings"
	"testing"
)

func savePalette(t *testing.T) {
	t.Helper()
	orig := atomicPalette.Load()
	t.Cleanup(func() {
		if orig != nil {
			atomicPalette.Store(orig)
		} else {
			atomicPalette.Store(nil)
		}
	})
}

func TestDefaultSemanticForTerminal(t *testing.T) {
	savePalette(t)

	cases := []struct {
		name string
		fgbg string
		want string
	}{
		{"empty env defaults to dark", "", "dark"},
		{"malformed fgbg defaults to dark", "15", "dark"},
		{"dark background (0-7)", "15;0", "dark"},
		{"dark background 7", "15;7", "dark"},
		{"light background 8", "15;8", "light"},
		{"light background 15", "15;15", "light"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv sets the var and auto-restores the previous value at
			// test end, replacing the manual os.Setenv/Unsetenv + Cleanup.
			// An empty value is equivalent to unset for DetectTerminalBackground.
			t.Setenv("COLORFGBG", tc.fgbg)

			sem := DefaultSemanticForTerminal()
			if sem == nil {
				t.Fatal("DefaultSemanticForTerminal returned nil")
			}
			if sem.Name != tc.want {
				t.Fatalf("DefaultSemanticForTerminal() for COLORFGBG=%q: want name %q, got %q", tc.fgbg, tc.want, sem.Name)
			}
		})
	}
}

func TestSetSemanticThemeAndCurrentPalette(t *testing.T) {
	savePalette(t)

	sem := DefaultSemanticLight()
	SetSemanticTheme(sem, ColorModeTruecolor)

	p := CurrentPalette()
	if p == nil {
		t.Fatal("CurrentPalette nil after SetSemanticTheme")
	}
	if p.Semantic != sem {
		t.Fatal("CurrentPalette.Semantic should point to the theme set")
	}
	if p.Mode != ColorModeTruecolor {
		t.Fatalf("want ColorModeTruecolor, got %v", p.Mode)
	}
}

// TestLightThemePhase1Tokens verifies DefaultSemanticLight defines the
// Phase-1 background/surface/evidence tokens. Without them palette.go falls
// back to the dark brand values (#07111F etc.), so a light theme renders
// dark blue surfaces — and overlay dimBgColor (which derives from
// sem.Background) darkens overlays instead of lightening them.
func TestLightThemePhase1Tokens(t *testing.T) {
	sem := DefaultSemanticLight()
	if sem.Background == "" || sem.Surface == "" || sem.SurfaceRaised == "" {
		t.Fatalf("light theme missing phase-1 tokens: Background=%q Surface=%q SurfaceRaised=%q",
			sem.Background, sem.Surface, sem.SurfaceRaised)
	}
	for name, tok := range map[string]string{
		"EvidenceSupport":  sem.EvidenceSupport,
		"EvidenceCounter":  sem.EvidenceCounter,
		"ConfidenceLow":    sem.ConfidenceLow,
		"ConfidenceMedium": sem.ConfidenceMedium,
		"ConfidenceHigh":   sem.ConfidenceHigh,
	} {
		if tok == "" {
			t.Errorf("light theme missing token %q", name)
		}
	}
}

// TestLightThemeSurfaceIsLight renders the palette under the light theme and
// asserts the surface background is a light color (not the dark fallback).
func TestLightThemeSurfaceIsLight(t *testing.T) {
	savePalette(t)
	SetSemanticTheme(DefaultSemanticLight(), ColorModeTruecolor)
	p := CurrentPalette()
	if p == nil {
		t.Fatal("CurrentPalette nil")
	}
	strip := p.BackgroundBg.BgStrip()
	// #f7f9fc → 48;2;247;249;252
	if !strings.Contains(strip, "48;2;247;249;252") {
		t.Errorf("light BackgroundBg = %q, want truecolor 48;2;247;249;252 (light)", strip)
	}
	if strings.Contains(strip, "48;2;7;17;31") {
		t.Errorf("light BackgroundBg = %q contains dark fallback #07111F", strip)
	}
}
