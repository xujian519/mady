package tui

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// TestThemeStyleCoreRoundTrip locks the cross-model SGR invariant: a string
// produced by theme.Style.Render must survive a full round trip through the
// core cell model — ParseSGR (theme → core.Style) then RenderSGR (core.Style
// → ANSI) then ParseSGR again — without losing or drifting any style
// information. The two style models encode colors differently (theme emits
// ANSI basic colors 30-37 for palette entries, core re-emits them as
// 38;5;n), so this guards the "encode then re-parse" bridge used on every
// frame of rendering.
func TestThemeStyleCoreRoundTrip(t *testing.T) {
	theme.ForceColor(true)
	defer theme.ForceColor(false)

	styles := []struct {
		name  string
		style theme.Style
	}{
		{"plain", theme.NewStyle()},
		{"bold", theme.NewStyle().Bold()},
		{"italic-dim", theme.NewStyle().Italic().Dim()},
		{"basic-fg", theme.NewStyle().Fg(theme.Red)},
		{"bright-bg", theme.NewStyle().Bg(theme.BrightCyan)},
		{"basic-fg-bold", theme.NewStyle().Fg(theme.Green).Bold()},
		{"256-fg", theme.NewStyle().WithFgParams("38;5;196")},
		{"truecolor-fg", theme.NewStyle().WithFgParams("38;2;1;2;3")},
		{"truecolor-bg", theme.NewStyle().WithBgParams("48;2;200;100;50")},
		{"mixed", theme.NewStyle().Fg(theme.Yellow).Bg(theme.Blue).Underline()},
	}

	for _, tc := range styles {
		t.Run(tc.name, func(t *testing.T) {
			rendered := tc.style.Render("x")
			first := core.ParseLine(rendered)
			if len(first.Cells) == 0 {
				t.Fatalf("ParseLine(%q) produced no cells", rendered)
			}
			got := first.Cells[0].Style

			// Re-serialize the core style (from default) and re-parse: the
			// core model's own encoding must be idempotent.
			serialized := core.RenderSGR(core.DefaultStyle, got)
			second := core.ParseLine(serialized + "x")
			if len(second.Cells) == 0 {
				t.Fatalf("re-parse of %q produced no cells", serialized)
			}
			got2 := second.Cells[0].Style
			if !got2.Equal(got) {
				t.Errorf("core round-trip drifted: %v → %q → %v", got, serialized, got2)
			}

			// The core style must express what the theme style intended: for
			// cases that set a color, at least one of fg/bg must survive the
			// bridge as non-default (theme 31 → core palette 1).
			expectsColor := map[string]bool{
				"basic-fg": true, "bright-bg": true, "basic-fg-bold": true,
				"256-fg": true, "truecolor-fg": true, "truecolor-bg": true, "mixed": true,
			}
			if expectsColor[tc.name] && got.Fg.IsDefault() && got.Bg.IsDefault() {
				t.Errorf("theme style %v lost all colors across the bridge: core %v", tc.style, got)
			}
		})
	}
}
