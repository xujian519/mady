package component

// 输出示例（宽 50 列）：
//
//	▌ 证据卡
//	  ⊕ 支持  file://d1.pdf · p3
//	  ┃ 对比文件1公开了技术特征A...
//	  ⊖ 反对  file://d2.pdf · p5
//	  ┃ 对比文件2未公开区别特征...
//	  ──────────────────────────────────

// evidence_card.go renders a professional evidence card with source attribution,
// direction indicator (supporting/contradicting), and collapsible snippet body.

import (
	"fmt"
	"strings"

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

// RenderEvidenceCard 渲染类型为 evidence_card 的 DomainMessage 到指定宽度。
// 无链接版本（向后兼容）；链接见 RenderEvidenceCardWithLinks。
func RenderEvidenceCard(msg *DomainMessage, collapsed bool, t EvidenceCardTheme, width int64) []string {
	lines, _ := RenderEvidenceCardWithLinks(msg, collapsed, t, width)
	return lines
}

// RenderEvidenceCardWithLinks 渲染证据卡，并返回与渲染行一一对应的链接
// 元数据（每行一个 []core.LinkSpan；无链接行为 nil）。
//
// 链接目标为证据来源位置（EvidenceRef.URL 非空时）：覆盖来源文本所在的
// 列区间。仅当该行未被截断（VisibleWidth ≤ width）时生成链接，避免链接
// 区间超出实际可见文本。
func RenderEvidenceCardWithLinks(msg *DomainMessage, collapsed bool, t EvidenceCardTheme, width int64) ([]string, [][]core.LinkSpan) {
	bar := t.Border("▌")
	title := msg.Title
	if title == "" {
		title = "证据卡"
	}

	var lines []string
	var links [][]core.LinkSpan

	// Collapsed: one-line summary
	if collapsed {
		supportN := msg.SupportingSpans()
		counterN := msg.ContradictingSpans()
		summary := fmt.Sprintf("[+] %s  ·  支持 %d · 反对 %d", title, supportN, counterN)
		return core.WrapAnsi(bar+" "+t.Title(summary), width), nil
	}

	// Expanded header
	head := bar + " " + t.Title(title)
	if msg.Confidence > 0 {
		head += t.Dim(fmt.Sprintf("  置信度: %.0f%%", msg.Confidence*100))
	}
	lines = append(lines, core.PadToWidth(head, width))
	links = append(links, nil)

	// Evidence spans
	for _, sp := range msg.Spans {
		dirIcon := "○"
		dirColor := t.Dim
		switch sp.Direction {
		case DirectionSupporting:
			dirIcon = "⊕"
			dirColor = t.SupportColor
		case DirectionContradicting:
			dirIcon = "⊖"
			dirColor = t.CounterColor
		}

		loc := sp.SourceURI
		if sp.PageRange != "" {
			loc += " · " + sp.PageRange
		}
		dirTxt := dirColor(dirIcon + " " + string(sp.Direction))
		prefix := "  " + dirTxt + "  "
		locText := t.Dim(loc)
		info := prefix + locText
		lines = append(lines, core.PadToWidth(core.TruncateToWidth(info, width, "…"), width))
		// 链接：仅当行未被截断且提供了 URL 时。
		if sp.URL != "" && core.VisibleWidth(info) <= width {
			links = append(links, []core.LinkSpan{core.LinkSpanFor(prefix, locText, sp.URL)})
		} else {
			links = append(links, nil)
		}

		if sp.Snippet != "" {
			// Render snippet as quoted text
			quote := "  ┃ " + sp.Snippet
			snippetLines := core.WrapAnsi(t.Body(quote), width-2)
			lines = append(lines, snippetLines...)
			for range snippetLines {
				links = append(links, nil)
			}
		}
	}

	// Body text (conclusion or analysis)
	if msg.Body != "" {
		lines = append(lines, t.Dim(strings.Repeat("─", int(width))))
		links = append(links, nil)
		md := NewMarkdown(msg.Body)
		md.SetTheme(t.MarkdownTheme)
		body := md.Render(width)
		lines = append(lines, body...)
		for range body {
			links = append(links, nil)
		}
	}

	return lines, links
}
