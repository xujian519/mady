package theme

import (
	"reflect"
	"testing"
)

func TestSemStyle(t *testing.T) {
	// Valid hex, truecolor mode.
	s := SemStyle("#ff0000", ColorModeTruecolor)
	if s.fgParams != "38;2;255;0;0" {
		t.Fatalf("SemStyle fgParams = %q", s.fgParams)
	}

	// Whitespace around the hex is trimmed.
	s = SemStyle("  #00ff00  ", ColorModeTruecolor)
	if s.fgParams != "38;2;0;255;0" {
		t.Fatalf("SemStyle trimmed fgParams = %q", s.fgParams)
	}

	// Numeric 256-palette index.
	s = SemStyle("196", ColorModeTruecolor)
	if s.fgParams != "38;5;196" {
		t.Fatalf("SemStyle numeric fgParams = %q", s.fgParams)
	}

	// Empty value -> plain style.
	s = SemStyle("", ColorModeTruecolor)
	if s.fgParams != "" || s.fg != Default {
		t.Fatalf("SemStyle empty = %+v, want default style", s)
	}

	// Invalid hex -> plain style.
	s = SemStyle("nope", ColorModeTruecolor)
	if s.fgParams != "" {
		t.Fatalf("SemStyle invalid fgParams = %q, want empty", s.fgParams)
	}
}

func TestBuildPaletteFallbacks(t *testing.T) {
	// A theme with every color empty exercises all hard-coded fallbacks.
	sem := &SemanticTheme{Name: "bare"}
	p := BuildPalette(sem, ColorModeTruecolor)

	if p.User.fgParams != "" {
		t.Fatalf("User: empty colors should yield no params, got %q", p.User.fgParams)
	}
	if p.Assistant.fg != BrightWhite {
		t.Fatalf("Assistant fallback: want BrightWhite, got %v", p.Assistant.fg)
	}
	if p.System.fg != BrightYellow {
		t.Fatalf("System fallback: want BrightYellow, got %v", p.System.fg)
	}
	if p.Tool.fg != BrightMagenta {
		t.Fatalf("Tool fallback: want BrightMagenta, got %v", p.Tool.fg)
	}
	if p.ToolName.fg != Magenta {
		t.Fatalf("ToolName fallback: want Magenta, got %v", p.ToolName.fg)
	}
	if p.Error.fg != BrightRed {
		t.Fatalf("Error fallback: want BrightRed, got %v", p.Error.fg)
	}
	if p.Success.fg != BrightGreen {
		t.Fatalf("Success fallback: want BrightGreen, got %v", p.Success.fg)
	}
	if len(p.Dim.attrs) != 1 || p.Dim.attrs[0] != Dim {
		t.Fatalf("Dim fallback: want Dim attr, got %v", p.Dim.attrs)
	}
	if p.Handoff.fg != BrightBlue {
		t.Fatalf("Handoff fallback: want BrightBlue, got %v", p.Handoff.fg)
	}
	if p.Code.fg != BrightGreen {
		t.Fatalf("Code fallback: want BrightGreen, got %v", p.Code.fg)
	}
	if p.CodeBlock.fg != Green {
		t.Fatalf("CodeBlock fallback: want Green, got %v", p.CodeBlock.fg)
	}
	if p.Usage.fg != BrightBlack {
		t.Fatalf("Usage fallback: want BrightBlack, got %v", p.Usage.fg)
	}
	if p.Accent.fg != BrightCyan {
		t.Fatalf("Accent fallback: want BrightCyan, got %v", p.Accent.fg)
	}
	if p.SelectHighlight.fg != BrightCyan {
		t.Fatalf("SelectHighlight fallback: want BrightCyan, got %v", p.SelectHighlight.fg)
	}
	if p.SelectionBg.bgParams == "" {
		t.Fatal("SelectionBg should fall back to #3366ff background params")
	}
	if p.SettingsKey.fg != BrightCyan {
		t.Fatalf("SettingsKey fallback: want BrightCyan, got %v", p.SettingsKey.fg)
	}
	if p.SettingsValueSelected.fg != BrightYellow {
		t.Fatalf("SettingsValueSelected fallback: want BrightYellow, got %v", p.SettingsValueSelected.fg)
	}
	if p.LoaderSpinner.fg != Cyan {
		t.Fatalf("LoaderSpinner fallback: want Cyan, got %v", p.LoaderSpinner.fg)
	}
	if p.ProgressPrompt.fg != Blue {
		t.Fatalf("ProgressPrompt fallback: want Blue, got %v", p.ProgressPrompt.fg)
	}
	if p.Thinking.fg != BrightBlack {
		t.Fatalf("Thinking fallback: want BrightBlack, got %v", p.Thinking.fg)
	}
	// Surface hierarchy fallbacks.
	if p.Background.fgParams == "" {
		t.Fatal("Background fallback params should not be empty")
	}
	if p.Surface.fgParams == "" || p.SurfaceRaised.fgParams == "" {
		t.Fatal("Surface/SurfaceRaised fallback params should not be empty")
	}
	if p.EvidenceSupport.fgParams == "" || p.EvidenceCounter.fgParams == "" {
		t.Fatal("evidence fallback params should not be empty")
	}
	if p.ConfidenceLow.fgParams == "" || p.ConfidenceMedium.fgParams == "" || p.ConfidenceHigh.fgParams == "" {
		t.Fatal("confidence fallback params should not be empty")
	}
}

func TestBuildPaletteDerivedColors(t *testing.T) {
	// Partial theme: UserMessage/AssistantText/LoaderSpinner/ProgressBar
	// are empty and fall back to Accent/BorderAccent/Border.
	sem := &SemanticTheme{
		Name:              "partial",
		Accent:            "#111111",
		Border:            "#222222",
		BorderAccent:      "#333333",
		Warning:           "#444444",
		Error:             "#555555",
		Success:           "#666666",
		Dim:               "#777777",
		MdCode:            "#888888",
		MdCodeBlock:       "#999999",
		MdCodeBlockBorder: "#aaaaaa",
		Background:        "#bbbbbb",
	}
	p := BuildPalette(sem, ColorModeTruecolor)

	if p.User.fgParams != "38;2;17;17;17" {
		t.Fatalf("User should derive from Accent, got %q", p.User.fgParams)
	}
	if p.System.fgParams != "38;2;68;68;68" {
		t.Fatalf("System should derive from Warning, got %q", p.System.fgParams)
	}
	if p.LoaderSpinner.fgParams != "38;2;51;51;51" {
		t.Fatalf("LoaderSpinner should derive from BorderAccent, got %q", p.LoaderSpinner.fgParams)
	}
	if p.ProgressPrompt.fgParams != "38;2;34;34;34" {
		t.Fatalf("ProgressPrompt should derive from Border, got %q", p.ProgressPrompt.fgParams)
	}
	if !reflect.DeepEqual(p.ProgressCompletion, p.Success) {
		t.Fatal("ProgressCompletion should alias Success")
	}
	if !reflect.DeepEqual(p.Warning, p.SettingsValueSelected) {
		t.Fatal("Warning should alias SettingsValueSelected")
	}
	if !reflect.DeepEqual(p.SelectDescription, p.Dim) {
		t.Fatal("SelectDescription should alias Dim")
	}
	if p.Thinking.fgParams != "38;2;119;119;119" {
		t.Fatalf("Thinking should derive from Dim, got %q", p.Thinking.fgParams)
	}
}

func TestCurrentPaletteLazyInit(t *testing.T) {
	savePalette(t)
	// Force the "not yet initialized" path: CurrentPalette builds the
	// palette from the light default theme on first use.
	atomicPalette.Store(nil)
	p := CurrentPalette()
	if p == nil {
		t.Fatal("CurrentPalette returned nil")
	}
	if p.Semantic == nil || p.Semantic.Name != "light" {
		t.Fatalf("lazy init should use light default, got %+v", p.Semantic)
	}
	// Subsequent calls return the cached palette.
	if p2 := CurrentPalette(); p2 != p {
		t.Fatal("CurrentPalette should return the cached palette")
	}
}

func TestBuildPaletteNilSemantic(t *testing.T) {
	p := BuildPalette(nil, ColorModeTruecolor)
	if p == nil {
		t.Fatal("BuildPalette(nil) returned nil")
	}
	if p.Semantic.Name != "light" {
		t.Fatalf("BuildPalette(nil) should use light default, got %q", p.Semantic.Name)
	}
}

func TestBuildPaletteBackgroundBgs(t *testing.T) {
	sem := DefaultMadyDark()
	p := BuildPalette(sem, ColorModeTruecolor)
	// BackgroundBg uses BgParams (48;2;...) while Background uses FgParams.
	if p.BackgroundBg.bgParams == "" {
		t.Fatal("BackgroundBg should carry background params")
	}
	if p.SurfaceBg.bgParams == "" || p.SurfaceRaisedBg.bgParams == "" {
		t.Fatal("SurfaceBg/SurfaceRaisedBg should carry background params")
	}
	if p.Background.fgParams == "" {
		t.Fatal("Background should carry foreground params")
	}
}

func TestSyncPaletteGlobalsUpdatesAliases(t *testing.T) {
	savePalette(t)
	sem := DefaultMadyDark()
	SyncPaletteGlobals(sem, ColorModeTruecolor)
	if UserStyle.Load() == nil || DimStyle.Load() == nil ||
		SystemStyle.Load() == nil || ToolStyle.Load() == nil ||
		ToolBorder.Load() == nil || SuccessStyle.Load() == nil ||
		ErrorStyle.Load() == nil || ThinkingStyle.Load() == nil {
		t.Fatal("syncAliases should populate all package-level style aliases")
	}
	if got := CurrentPalette(); got == nil || got.Semantic != sem {
		t.Fatal("CurrentPalette should point at the synced theme")
	}
}
