package component

// approval_card.go renders a human-review approval gate card with confidence,
// evidence summary, and available actions (/approve, /reject).

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// ApprovalCardTheme carries styling for approval gate cards.
type ApprovalCardTheme struct {
	Title         func(string) string
	Border        func(string) string
	Warning       func(string) string
	Dim           func(string) string
	Action        func(string) string
	Body          func(string) string
	MarkdownTheme MarkdownTheme
}

// DefaultApprovalCardTheme returns a theme from the current palette.
func DefaultApprovalCardTheme() ApprovalCardTheme {
	p := theme.CurrentPalette()
	return ApprovalCardTheme{
		Title:         p.Accent.Render,
		Border:        p.Accent.Render,
		Warning:       p.Accent.Render,
		Dim:           p.Dim.Render,
		Action:        p.Success.Render,
		Body:          p.Assistant.Render,
		MarkdownTheme: DefaultMarkdownTheme(),
	}
}

// RenderApprovalCard renders a DomainMessage of type approval_prompt.
func RenderApprovalCard(msg *agentcore.DomainMessage, t ApprovalCardTheme, width int64) []string {
	divider := t.Border(strings.Repeat("═", int(width)))
	dashDivider := t.Dim(strings.Repeat("─", int(width)))

	var lines []string

	// Header banner
	lines = append(lines,
		divider,
		core.PadToWidth(t.Warning("  ⚠  人 工 审 核 关 卡"), width),
		divider,
	)

	// Conclusion summary
	if msg.Title != "" {
		lines = append(lines, core.PadToWidth("  "+t.Title(msg.Title), width))
	}

	// Confidence bar + evidence summary divider
	confBar := renderApprovalConfBar(msg.Confidence, t, width)
	lines = append(lines, confBar, dashDivider)
	supportN := msg.SupportingSpans()
	counterN := msg.ContradictingSpans()
	lines = append(lines, t.Dim(fmt.Sprintf("  支持证据: %d  ·  反对证据: %d", supportN, counterN)))

	// First evidence snippet (if any)
	for _, sp := range msg.Spans {
		if sp.Snippet != "" {
			lines = append(lines, t.Dim("  "+core.TruncateToWidth(sp.Snippet, width-4, "…")))
			break
		}
	}

	// Actions
	lines = append(lines, dashDivider)
	for _, a := range msg.Actions {
		lines = append(lines, t.Action(fmt.Sprintf("  %s → %s", a.Label, a.Command)))
	}

	// Body text
	if msg.Body != "" {
		lines = append(lines, dashDivider)
		md := NewMarkdown(msg.Body)
		md.SetTheme(t.MarkdownTheme)
		lines = append(lines, md.Render(width)...)
	}

	lines = append(lines, divider)
	return lines
}

func renderApprovalConfBar(conf float64, t ApprovalCardTheme, width int64) string {
	const cells = 10
	pct := int(conf * 100)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := (pct * cells) / 100

	bar := fmt.Sprintf("  置信度: %s %d%%",
		strings.Repeat("█", filled)+strings.Repeat("░", cells-filled),
		pct)
	return core.PadToWidth(bar, width)
}
