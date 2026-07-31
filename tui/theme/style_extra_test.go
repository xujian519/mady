package theme

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/terminal"
)

func TestStyleFgBg(t *testing.T) {
	s := NewStyle().Fg(Red).Bg(Blue)
	if s.fg != Red {
		t.Fatalf("fg = %v, want Red", s.fg)
	}
	if s.bg != Blue+10 {
		t.Fatalf("bg = %v, want Blue+10 (%v)", s.bg, Blue+10)
	}
	// Fg/Bg reset the params segments.
	s2 := NewStyle().WithFgParams("38;5;100").WithBgParams("48;5;101").Fg(Green)
	if s2.fgParams != "" {
		t.Fatal("Fg should clear fgParams")
	}
	s3 := NewStyle().WithFgParams("38;5;100").Bg(Cyan)
	if s3.bgParams != "" {
		t.Fatal("Bg should clear bgParams")
	}
}

func TestStyleWithParams(t *testing.T) {
	s := NewStyle().WithFgParams("38;2;1;2;3")
	if s.fg != Default {
		t.Fatal("WithFgParams should reset fg to Default")
	}
	if s.fgParams != "38;2;1;2;3" {
		t.Fatalf("fgParams = %q", s.fgParams)
	}
	s2 := NewStyle().WithBgParams("48;2;1;2;3")
	if s2.bg != Default {
		t.Fatal("WithBgParams should reset bg to Default")
	}
	if s2.bgParams != "48;2;1;2;3" {
		t.Fatalf("bgParams = %q", s2.bgParams)
	}
}

func TestStyleAttributes(t *testing.T) {
	s := NewStyle().Underline().Strike().Bold().Dim().Italic()
	want := []Attr{Underline, Strike, Bold, Dim, Italic}
	if len(s.attrs) != len(want) {
		t.Fatalf("attrs = %v, want %v", s.attrs, want)
	}
	for i := range want {
		if s.attrs[i] != want[i] {
			t.Fatalf("attrs[%d] = %v, want %v", i, s.attrs[i], want[i])
		}
	}
}

func TestBgStrip(t *testing.T) {
	if got := NewStyle().BgStrip(); got != "" {
		t.Fatalf("default style BgStrip = %q, want empty", got)
	}
	if got := NewStyle().Bg(Blue).BgStrip(); got != "\x1b[44m" {
		t.Fatalf("Bg(Blue) BgStrip = %q, want %q", got, "\x1b[44m")
	}
	if got := NewStyle().WithBgParams("48;2;1;2;3").BgStrip(); got != "\x1b[48;2;1;2;3m" {
		t.Fatalf("params BgStrip = %q", got)
	}
}

func TestRenderCombined(t *testing.T) {
	ForceColor(true)
	t.Cleanup(func() { colorOverride.Store(nil) })

	s := NewStyle().Fg(Red).Bg(Blue).Bold().Underline()
	if got, want := s.Render("x"), "\x1b[1;4;31;44mx\x1b[0m"; got != want {
		t.Fatalf("Render = %q, want %q", got, want)
	}

	s2 := NewStyle().WithFgParams("38;2;1;2;3").Bold()
	if got, want := s2.Render("y"), "\x1b[1;38;2;1;2;3my\x1b[0m"; got != want {
		t.Fatalf("Render params = %q, want %q", got, want)
	}

	// No style parts -> text returned unchanged (even with color enabled).
	if got := NewStyle().Render("z"); got != "z" {
		t.Fatalf("plain Render = %q, want z", got)
	}
}

func TestRenderColorDisabled(t *testing.T) {
	ForceColor(false)
	t.Cleanup(func() { colorOverride.Store(nil) })
	if got := NewStyle().Fg(Red).Bold().Render("q"); got != "q" {
		t.Fatalf("Render with color disabled = %q, want q", got)
	}
}

func TestColorEnabledForceOverride(t *testing.T) {
	t.Cleanup(func() { colorOverride.Store(nil) })
	ForceColor(true)
	if !ColorEnabled() {
		t.Fatal("ForceColor(true) should enable color")
	}
	ForceColor(false)
	if ColorEnabled() {
		t.Fatal("ForceColor(false) should disable color")
	}
}

func TestColorEnabledEnvBranches(t *testing.T) {
	t.Cleanup(func() { colorOverride.Store(nil) })
	colorOverride.Store(nil)

	t.Setenv("NO_COLOR", "1")
	if ColorEnabled() {
		t.Fatal("NO_COLOR=1 should disable color")
	}

	// NO_COLOR wins over FORCE_COLOR (checked first).
	t.Setenv("FORCE_COLOR", "1")
	if ColorEnabled() {
		t.Fatal("NO_COLOR should take precedence over FORCE_COLOR")
	}

	t.Setenv("NO_COLOR", "")
	if !ColorEnabled() {
		t.Fatal("FORCE_COLOR=1 should enable color")
	}

	t.Setenv("FORCE_COLOR", "")
	t.Setenv("TERM", "dumb")
	if ColorEnabled() {
		t.Fatal("TERM=dumb should disable color")
	}

	// go test captures stdout as a pipe, so the char-device probe fails and
	// color is disabled. This assertion is stable across environments.
	t.Setenv("TERM", "xterm")
	if ColorEnabled() {
		t.Fatal("non-TTY stdout should disable color")
	}
}

func TestResolveIcon(t *testing.T) {
	ic := Icon{NerdFont: "nf", Unicode: "uni", ASCII: "asc"}

	t.Run("nerd-font-available", func(t *testing.T) {
		t.Setenv("NERD_FONT", "1")
		terminal.DetectNerdFonts() // refresh the cached status
		if got := ResolveIcon(ic); got != "nf" {
			t.Fatalf("ResolveIcon = %q, want nf", got)
		}
	})
	t.Run("nerd-font-unavailable-unicode", func(t *testing.T) {
		t.Setenv("NERD_FONT", "0")
		terminal.DetectNerdFonts()
		if got := ResolveIcon(ic); got != "uni" {
			t.Fatalf("ResolveIcon = %q, want uni", got)
		}
	})
	t.Run("nerd-font-empty-falls-through-to-unicode", func(t *testing.T) {
		t.Setenv("NERD_FONT", "1")
		terminal.DetectNerdFonts()
		if got := ResolveIcon(Icon{Unicode: "uni", ASCII: "asc"}); got != "uni" {
			t.Fatalf("ResolveIcon = %q, want uni", got)
		}
	})
	t.Run("unicode-empty-ascii", func(t *testing.T) {
		t.Setenv("NERD_FONT", "0")
		terminal.DetectNerdFonts()
		if got := ResolveIcon(Icon{ASCII: "asc"}); got != "asc" {
			t.Fatalf("ResolveIcon = %q, want asc", got)
		}
	})
	t.Run("all-empty-ascii", func(t *testing.T) {
		t.Setenv("NERD_FONT", "0")
		terminal.DetectNerdFonts()
		if got := ResolveIcon(Icon{}); got != "" {
			t.Fatalf("ResolveIcon = %q, want empty", got)
		}
	})
}

func TestCommonIconsRender(t *testing.T) {
	// Spot-check that common icons resolve to non-empty strings regardless
	// of detection result.
	for name, ic := range map[string]Icon{
		"folder": IconFolder, "file": IconFile, "check": IconCheck,
		"x": IconX, "gear": IconGear, "user": IconUser,
		"search": IconSearch, "time": IconTime, "branch": IconBranch,
		"lock": IconLock,
	} {
		if got := ResolveIcon(ic); got == "" {
			t.Errorf("ResolveIcon(%s) returned empty", name)
		}
	}
}

func TestSymbolConstants(t *testing.T) {
	if !strings.HasPrefix(SymbolCheck, "✓") {
		t.Fatalf("SymbolCheck = %q", SymbolCheck)
	}
	// All symbols must be non-empty.
	for name, s := range map[string]string{
		"SymbolCheck": SymbolCheck, "SymbolCross": SymbolCross,
		"SymbolArrow": SymbolArrow, "SymbolBullet": SymbolBullet,
		"SymbolDot": SymbolDot, "SymbolStar": SymbolStar,
		"SymbolWarning": SymbolWarning, "SymbolInfo": SymbolInfo,
		"SymbolThinking": SymbolThinking, "SymbolRight": SymbolRight,
		"SymbolDown": SymbolDown,
	} {
		if s == "" {
			t.Errorf("%s is empty", name)
		}
	}
}
