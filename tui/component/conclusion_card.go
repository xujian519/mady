package component

// 输出示例（宽 50 列）：
//
//	▌ 分析结论
//	  置信度: ████████░░ 80%  高
//	  支持证据: 3  ·  反对证据: 1
//	  分类: 创造性 · 非显而易见
//	  ──────────────────────────────────
//	  基于现有技术分析，该技术方案...

// conclusion_card.go renders a professional conclusion card with confidence bar,
// evidence counts, classification label, and body text.

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// ConclusionCardTheme carries styling functions for conclusion cards.
type ConclusionCardTheme struct {
	Title         func(string) string
	Border        func(string) string
	Dim           func(string) string
	HighConf      func(string) string
	MediumConf    func(string) string
	LowConf       func(string) string
	Body          func(string) string
	MarkdownTheme MarkdownTheme
}

// DefaultConclusionCardTheme returns a theme from the current palette.
func DefaultConclusionCardTheme() ConclusionCardTheme {
	p := theme.CurrentPalette()
	return ConclusionCardTheme{
		Title:         p.Accent.Render,
		Border:        p.BorderMuted.Render,
		Dim:           p.Dim.Render,
		HighConf:      p.ConfidenceHigh.Render,
		MediumConf:    p.ConfidenceMedium.Render,
		LowConf:       p.ConfidenceLow.Render,
		Body:          p.Assistant.Render,
		MarkdownTheme: DefaultMarkdownTheme(),
	}
}

// RenderConclusionCard 渲染类型为 conclusion_card 的 DomainMessage。
func RenderConclusionCard(msg *DomainMessage, t ConclusionCardTheme, width int64) []string {
	bar := t.Border("▌")
	title := msg.Title
	if title == "" {
		title = "分析结论"
	}

	var lines []string

	// Header
	head := bar + " " + t.Title(title)
	lines = append(lines, core.PadToWidth(head, width))

	// Confidence bar (10-cell)
	colors := &ConfidenceBarColors{High: t.HighConf, Medium: t.MediumConf, Low: t.LowConf}
	confBar := RenderConfidenceBar(msg.Confidence, colors, width, false)
	lines = append(lines, confBar)

	// Evidence summary
	supportN := msg.SupportingSpans()
	counterN := msg.ContradictingSpans()
	evSummary := fmt.Sprintf("  支持证据: %d  ·  反对证据: %d", supportN, counterN)
	if cls, ok := msg.Extra["classification"]; ok && cls != "" {
		evSummary += "  ·  分类: " + cls
	}
	lines = append(lines, t.Dim(evSummary))

	// Body
	if msg.Body != "" {
		lines = append(lines, t.Dim(strings.Repeat("─", int(width))))
		md := NewMarkdown(msg.Body)
		md.SetTheme(t.MarkdownTheme)
		lines = append(lines, md.Render(width)...)
	}

	return lines
}
