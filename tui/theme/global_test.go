package theme

import (
	"os"
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
			prev := os.Getenv("COLORFGBG")
			if tc.fgbg == "" {
				os.Unsetenv("COLORFGBG")
			} else {
				os.Setenv("COLORFGBG", tc.fgbg)
			}
			t.Cleanup(func() {
				if prev == "" {
					os.Unsetenv("COLORFGBG")
				} else {
					os.Setenv("COLORFGBG", prev)
				}
			})

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
