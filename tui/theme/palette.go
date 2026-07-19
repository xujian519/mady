package theme

import (
	"strings"
	"sync/atomic"
)

// Palette holds pre-built Style values for one semantic theme + color mode.
type Palette struct {
	Semantic *SemanticTheme
	Mode     ColorMode

	User, Assistant, System, Tool, ToolName, Error, Success Style
	Dim, Bold, Handoff, Code, CodeBlock, Usage              Style
	Accent, Muted, BorderMuted, Border, BorderAccent        Style
	SelectHighlight, SelectDescription                      Style
	SettingsKey, SettingsValueSelected                      Style
	LoaderSpinner, ProgressPrompt, ProgressCompletion       Style
	Thinking                                                Style

	// Phase 1 新增：背景层次与证据/置信度可视化
	Background, Surface, SurfaceRaised              Style
	EvidenceSupport, EvidenceCounter                Style
	ConfidenceLow, ConfidenceMedium, ConfidenceHigh Style
}

var atomicPalette atomic.Pointer[Palette]

func CurrentPalette() *Palette {
	p := atomicPalette.Load()
	if p == nil {
		p = BuildPalette(DefaultSemanticLight(), DetectColorMode())
		atomicPalette.Store(p)
	}
	return p
}

// BuildPalette materializes SemanticTheme into Style values.
func BuildPalette(sem *SemanticTheme, mode ColorMode) *Palette {
	if sem == nil {
		sem = DefaultSemanticLight()
	}
	p := &Palette{Semantic: sem, Mode: mode}

	fg := func(hex string) Style {
		s := NewStyle()
		c := FgParams(hex, mode)
		if c != "" {
			s = s.WithFgParams(c)
		}
		return s
	}

	userHex := sem.UserMessage
	if userHex == "" {
		userHex = sem.Accent
	}
	p.User = fg(userHex).Bold()

	assistHex := sem.AssistantText
	if assistHex == "" {
		assistHex = sem.Text
	}
	if assistHex == "" {
		p.Assistant = NewStyle().Fg(BrightWhite)
	} else {
		p.Assistant = fg(assistHex)
	}

	sysHex := sem.System
	if sysHex == "" {
		sysHex = sem.Warning
	}
	p.System = fg(sysHex).Italic()
	if FgParams(sysHex, mode) == "" {
		p.System = NewStyle().Fg(BrightYellow).Italic()
	}

	p.Tool = fg(sem.Accent)
	if FgParams(sem.Accent, mode) == "" {
		p.Tool = NewStyle().Fg(BrightMagenta)
	}

	p.ToolName = fg(sem.Accent).Bold()
	if FgParams(sem.Accent, mode) == "" {
		p.ToolName = NewStyle().Fg(Magenta).Bold()
	}

	p.Error = fg(sem.Error).Bold()
	if FgParams(sem.Error, mode) == "" {
		p.Error = NewStyle().Fg(BrightRed).Bold()
	}

	p.Success = fg(sem.Success)
	if FgParams(sem.Success, mode) == "" {
		p.Success = NewStyle().Fg(BrightGreen)
	}

	p.Dim = fg(sem.Dim)
	if FgParams(sem.Dim, mode) == "" {
		p.Dim = NewStyle().Dim()
	}

	p.Bold = NewStyle().Bold()
	p.Handoff = fg(sem.Border).Bold()
	if FgParams(sem.Border, mode) == "" {
		p.Handoff = NewStyle().Fg(BrightBlue).Bold()
	}

	p.Code = fg(sem.MdCode)
	if FgParams(sem.MdCode, mode) == "" {
		p.Code = NewStyle().Fg(BrightGreen)
	}

	p.CodeBlock = fg(sem.MdCodeBlock).Dim()
	if FgParams(sem.MdCodeBlock, mode) == "" {
		p.CodeBlock = NewStyle().Fg(Green).Dim()
	}

	p.Usage = fg(sem.MdCodeBlockBorder)
	if FgParams(sem.MdCodeBlockBorder, mode) == "" {
		p.Usage = NewStyle().Fg(BrightBlack)
	}

	p.Accent = fg(sem.Accent).Bold()
	if FgParams(sem.Accent, mode) == "" {
		p.Accent = NewStyle().Fg(BrightCyan).Bold()
	}

	p.Muted = fg(sem.Muted)
	p.BorderMuted = fg(sem.BorderMuted)
	p.Border = fg(sem.Border)
	p.BorderAccent = fg(sem.BorderAccent)

	p.SelectHighlight = fg(sem.Accent).Bold()
	if FgParams(sem.Accent, mode) == "" {
		p.SelectHighlight = NewStyle().Fg(BrightCyan).Bold()
	}
	p.SelectDescription = p.Dim

	p.SettingsKey = fg(sem.Accent).Bold()
	if FgParams(sem.Accent, mode) == "" {
		p.SettingsKey = NewStyle().Fg(BrightCyan).Bold()
	}
	p.SettingsValueSelected = fg(sem.Warning)
	if FgParams(sem.Warning, mode) == "" {
		p.SettingsValueSelected = NewStyle().Fg(BrightYellow)
	}

	spin := sem.LoaderSpinner
	if spin == "" {
		spin = sem.BorderAccent
	}
	if spin == "" {
		spin = sem.Accent
	}
	p.LoaderSpinner = fg(spin)
	if FgParams(spin, mode) == "" {
		p.LoaderSpinner = NewStyle().Fg(Cyan)
	}

	prog := sem.ProgressBar
	if prog == "" {
		prog = sem.Border
	}
	p.ProgressPrompt = fg(prog)
	if FgParams(prog, mode) == "" {
		p.ProgressPrompt = NewStyle().Fg(Blue)
	}
	p.ProgressCompletion = p.Success

	thinkingHex := sem.ThinkingText
	if thinkingHex == "" {
		thinkingHex = sem.Dim
	}
	p.Thinking = fg(thinkingHex).Italic()
	if FgParams(thinkingHex, mode) == "" {
		p.Thinking = NewStyle().Fg(BrightBlack).Italic()
	}

	// Phase 1 新增：背景与表面层次
	p.Background = fg(firstNonEmpty(sem.Background, "#07111F"))
	p.Surface = fg(firstNonEmpty(sem.Surface, "#0C1B2A"))
	p.SurfaceRaised = fg(firstNonEmpty(sem.SurfaceRaised, "#102638"))

	// Phase 1 新增：证据方向着色
	p.EvidenceSupport = fg(firstNonEmpty(sem.EvidenceSupport, "#5BC0EB"))
	p.EvidenceCounter = fg(firstNonEmpty(sem.EvidenceCounter, "#CFA7FF"))

	// Phase 1 新增：置信度梯度着色
	p.ConfidenceLow = fg(firstNonEmpty(sem.ConfidenceLow, "#D7B65C"))
	p.ConfidenceMedium = fg(firstNonEmpty(sem.ConfidenceMedium, "#38C8F4"))
	p.ConfidenceHigh = fg(firstNonEmpty(sem.ConfidenceHigh, "#52D6A0"))

	return p
}

// SyncPaletteGlobals updates the atomic palette snapshot for the given
// semantic theme + color mode. Callers should use CurrentPalette() to
// read the active styles.
func SyncPaletteGlobals(sem *SemanticTheme, mode ColorMode) {
	p := BuildPalette(sem, mode)
	atomicPalette.Store(p)
}

func init() {
	SyncPaletteGlobals(DefaultSemanticLight(), DetectColorMode())
}

// firstNonEmpty returns a if non-empty, otherwise b.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// SemStyle builds a foreground Style from a hex / 256 index string.
func SemStyle(hex string, mode ColorMode) Style {
	s := NewStyle()
	c := FgParams(strings.TrimSpace(hex), mode)
	if c != "" {
		s = s.WithFgParams(c)
	}
	return s
}
