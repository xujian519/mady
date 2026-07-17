package component

// evidence_card.go renders a professional evidence card with source attribution,
// direction indicator (supporting/contradicting), and collapsible snippet body.

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// EvidenceCardTheme carries styling functions for evidence cards.
type EvidenceCardTheme struct {
	Title         func(string) string
	Border        func(string) string
	SupportColor  func(string) string
	CounterColor  func(string) string
	Dim           func(string) string
	Body          func(string) string
	MarkdownTheme MarkdownTheme
}

// DefaultEvidenceCardTheme returns a theme built from the current palette.
func DefaultEvidenceCardTheme() EvidenceCardTheme {
	p := theme.CurrentPalette()
	return EvidenceCardTheme{
		Title:         p.Accent.Render,
		Border:        p.BorderMuted.Render,
		SupportColor:  p.EvidenceSupport.Render,
		CounterColor:  p.EvidenceCounter.Render,
		Dim:           p.Dim.Render,
		Body:          p.Assistant.Render,
		MarkdownTheme: DefaultMarkdownTheme(),
	}
}

// RenderEvidenceCard renders a DomainMessage of type evidence_card to width columns.
func RenderEvidenceCard(msg *agentcore.DomainMessage, collapsed bool, t EvidenceCardTheme, width int64) []string {
	bar := t.Border("▌")
	title := msg.Title
	if title == "" {
		title = "证据卡"
	}

	var lines []string

	// Collapsed: one-line summary
	if collapsed {
		supportN := msg.SupportingSpans()
		counterN := msg.ContradictingSpans()
		summary := fmt.Sprintf("[+] %s  ·  支持 %d · 反对 %d", title, supportN, counterN)
		return core.WrapAnsi(bar+" "+t.Title(summary), width)
	}

	// Expanded header
	head := bar + " " + t.Title(title)
	if msg.Confidence > 0 {
		head += t.Dim(fmt.Sprintf("  置信度: %.0f%%", msg.Confidence*100))
	}
	lines = append(lines, core.PadToWidth(head, width))

	// Evidence spans
	for _, sp := range msg.Spans {
		dirIcon := "○"
		dirColor := t.Dim
		switch sp.Direction {
		case agentcore.DirectionSupporting:
			dirIcon = "⊕"
			dirColor = t.SupportColor
		case agentcore.DirectionContradicting:
			dirIcon = "⊖"
			dirColor = t.CounterColor
		}

		loc := sp.SourceURI
		if sp.PageRange != "" {
			loc += " · " + sp.PageRange
		}
		info := fmt.Sprintf("  %s  %s", dirColor(dirIcon+" "+string(sp.Direction)), t.Dim(loc))
		lines = append(lines, core.PadToWidth(core.TruncateToWidth(info, width, "…"), width))

		if sp.Snippet != "" {
			// Render snippet as quoted text
			quote := "  ┃ " + sp.Snippet
			lines = append(lines, core.WrapAnsi(t.Body(quote), width-2)...)
		}
	}

	// Body text (conclusion or analysis)
	if msg.Body != "" {
		lines = append(lines, t.Dim(strings.Repeat("─", int(width))))
		md := NewMarkdown(msg.Body)
		md.SetTheme(t.MarkdownTheme)
		lines = append(lines, md.Render(width)...)
	}

	return lines
}
