package chat

// This file contains per-message rendering logic: renderMessageCachedWithCache
// (cache-aware entry point), renderDomainCard (professional card router), and
// renderMessage (role-based dispatch).
//
// Pipeline orchestration is in chat_history_render.go.

import (
	"strings"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// renderMessageCachedWithCache is the cache-parameterized variant used during
// lock-free snapshot rendering. It reads from and writes to the provided cache
// map instead of h.msgCache, so the snapshot render can run without holding
// h.mu while still benefiting from per-message caching.
func (h *ChatHistory) renderMessageCachedWithCache(m ChatMessage, chatTheme ChatHistoryTheme, width int64, cache map[string]cachedMessage) ([]string, [][]core.LinkSpan) {
	if m.ID == "" {
		return h.renderMessage(m, chatTheme, width, nil)
	}
	var bc *component.BlockCache
	if cached, ok := cache[m.ID]; ok {
		if cached.width == width && !m.Pending {
			return cached.lines, cached.links
		}
		if m.Pending {
			bc = cached.blockCache
		}
	}
	if bc == nil && m.Pending && m.Role == RoleAssistant && m.Text != "" {
		bc = &component.BlockCache{}
	}
	lines, msgLinks := h.renderMessage(m, chatTheme, width, bc)
	trimmed, startOff := trimBlankEdges(lines)
	cachedLines := make([]string, len(trimmed))
	copy(cachedLines, trimmed)
	var cachedLinks [][]core.LinkSpan
	if msgLinks != nil {
		end := startOff + len(trimmed)
		if end > len(msgLinks) {
			end = len(msgLinks)
		}
		cachedLinks = msgLinks[startOff:end]
	}
	cache[m.ID] = cachedMessage{lines: cachedLines, links: cachedLinks, width: width, blockCache: bc}
	return cachedLines, cachedLinks
}

// renderDomainCard routes a DomainMessage to the appropriate professional card renderer.
// 返回渲染行与一一对应的链接元数据（无链接行为 nil）。
func (h *ChatHistory) renderDomainCard(m ChatMessage, chatTheme ChatHistoryTheme, width int64) ([]string, [][]core.LinkSpan) {
	dm := m.DomainMsg
	if dm == nil {
		return nil, nil
	}
	switch dm.Type {
	case "evidence_card":
		ecTheme := component.DefaultEvidenceCardTheme()
		return component.RenderEvidenceCardWithLinks(dm, m.Collapsed, ecTheme, width)
	case "conclusion_card":
		ccTheme := component.DefaultConclusionCardTheme()
		return component.RenderConclusionCard(dm, ccTheme, width), nil
	case "approval_prompt":
		acTheme := component.DefaultApprovalCardTheme()
		return component.RenderApprovalCard(dm, acTheme, width), nil
	default:
		md := component.NewMarkdown(dm.Body)
		md.SetTheme(chatTheme.MarkdownTheme)
		return md.Render(width), nil
	}
}

// nilLinks 返回 n 个空链接行（全部 nil 元素）。仅用于 renderMessagesRange：
// 消息渲染返回顶层 nil（无链接）时，兜底补齐与输出行等长的元数据，
// 保持 outLinks 与 out 一一对应。
func nilLinks(n int) [][]core.LinkSpan {
	return make([][]core.LinkSpan, n)
}

// messageBubbleWidth computes the effective inner width for a message bubble
// given the viewport width and an indent (left padding in cells).
//
// The rule (short-term UX fix P2): messages no longer stretch to 100% of the
// viewport. A max-bubble-width cap of 85% (min 40 cols) keeps long paragraphs
// readable (optimal line length ~60-80 chars per line is a standard readability
// heuristic) and introduces consistent breathing room on both sides. For
// terminals narrower than 60 cols the cap is relaxed to 100% because every
// pixel matters on small displays.
func messageBubbleWidth(viewport, indent int64) int64 {
	remain := viewport - indent
	if remain < 1 {
		return 1
	}
	if viewport < theme.NarrowCols {
		return remain
	}
	bubbleCap := int64(float64(viewport) * theme.BubbleMaxRatio)
	if bubbleCap < theme.BubbleMinColumns {
		bubbleCap = theme.BubbleMinColumns
	}
	if bubbleCap > remain {
		bubbleCap = remain
	}
	return bubbleCap
}

// renderMessageCore wraps a bubble (list of rendered content lines) with its
// visual envelope: left indent padding, optional background-fill to
// bubbleWidth, and separator-free vertical spacing. bgWidth should equal the
// effective bubble width as returned by messageBubbleWidth.
func envelopeBubble(lines []string, indent, bgWidth int64, bgStyle theme.Style) []string {
	leftPad := ""
	if indent > 0 {
		leftPad = strings.Repeat(" ", int(indent))
	}
	padded := make([]string, 0, len(lines))
	for _, ln := range lines {
		padded = append(padded, leftPad+ln)
	}
	if bgStyle.BgStrip() == "" {
		return padded
	}
	// Background fill spans only the bubble envelope (indent -> indent+bubbleWidth).
	// We build background-filled lines by inserting the bg SGR at the start of
	// the indent-padded portion and reconstructing at every SGR reset. The
	// visible content itself must already fit within bgWidth.
	bgSGR := bgStyle.BgStrip()
	reset := theme.Reset
	totalBgWidth := indent + bgWidth
	out := make([]string, 0, len(padded))
	for _, ln := range padded {
		vis := core.VisibleWidth(ln)
		var full string
		if vis < totalBgWidth {
			full = ln + strings.Repeat(" ", int(totalBgWidth-vis))
		} else {
			full = ln
		}
		out = append(out, applyBgOneLine(full, bgSGR, reset))
	}
	return out
}

func (h *ChatHistory) renderMessage(m ChatMessage, chatTheme ChatHistoryTheme, width int64, mdCache *component.BlockCache) ([]string, [][]core.LinkSpan) {
	h.renderCount++
	if m.DomainMsg != nil {
		return h.renderDomainCard(m, chatTheme, width)
	}

	switch m.Role {
	case RoleUser:
		return h.renderUserRole(m.Text, width, chatTheme), nil
	case RoleAssistant:
		indent := int64(0)
		bubbleW := messageBubbleWidth(width, indent)
		if m.Collapsed && m.Text != "" {
			firstLine := m.Text
			if idx := strings.IndexByte(firstLine, '\n'); idx > 0 {
				firstLine = firstLine[:idx]
			}
			if len([]rune(firstLine)) > 200 {
				firstLine = string([]rune(firstLine)[:197]) + "..."
			}
			head := core.TruncateToWidth(chatTheme.DimStyle.Render(firstLine), bubbleW, "…")
			expand := core.TruncateToWidth("  "+chatTheme.DimStyle.Render("▸ expand"), bubbleW, "")
			collapsedLines := []string{head, expand}
			return envelopeBubble(collapsedLines, indent, bubbleW, chatTheme.AssistantBgStyle), nil
		}

		innerWidth := bubbleW
		if innerWidth < 1 {
			innerWidth = 1
		}

		var allLines []string

		if h.reasoningRenderer != nil {
			if rendered := h.reasoningRenderer.RenderThinking(m, innerWidth); len(rendered) > 0 {
				allLines = append(allLines, rendered...)
			}
		}

		if m.Text != "" {
			var lines []string
			if mdCache != nil {
				lines = component.RenderMarkdownIncremental(m.Text, innerWidth, chatTheme.MarkdownTheme, mdCache)
			} else {
				md := component.NewMarkdown(m.Text)
				md.SetTheme(chatTheme.MarkdownTheme)
				lines = md.Render(innerWidth)
			}
			if m.Pending {
				if len(lines) == 0 {
					lines = []string{chatTheme.DimStyle.Render("…")}
				} else {
					last := lines[len(lines)-1]
					lines[len(lines)-1] = appendStreamCursor(last, innerWidth, chatTheme.UserStyle.Render(theme.StreamCursor))
				}
			}
			allLines = append(allLines, lines...)
		} else if len(m.ThinkingSegments) > 0 && m.Pending {
			if len(allLines) == 0 {
				allLines = []string{chatTheme.DimStyle.Render("…")}
			} else {
				last := allLines[len(allLines)-1]
				allLines[len(allLines)-1] = appendStreamCursor(last, innerWidth, chatTheme.ThinkingStyle.Render(theme.StreamCursor))
			}
		}

		if len(allLines) == 0 {
			allLines = []string{""}
		}
		return envelopeBubble(allLines, indent, innerWidth, chatTheme.AssistantBgStyle), nil
	case RoleSystem:
		return h.renderMarkdownRole(m.Text, width, chatTheme), nil
	case RoleTool:
		tcTheme := component.ToolCardTheme{
			Border:        chatTheme.ToolBorder.Render,
			Success:       chatTheme.SuccessStyle.Render,
			Error:         chatTheme.ErrorStyle.Render,
			Title:         func(s string) string { return chatTheme.ToolStyle.Render(chatTheme.ToolPrefix + s) },
			Dim:           chatTheme.DimStyle.Render,
			MarkdownTheme: chatTheme.MarkdownTheme,
		}
		return component.RenderToolCard(component.ToolCardConfig{
			Name:      m.Meta,
			Seq:       m.Seq,
			Status:    m.Text,
			Duration:  m.Duration,
			Collapsed: m.Collapsed,
		}, tcTheme, width), nil
	case RoleError:
		return h.renderMarkdownRole(m.Text, width, chatTheme), nil
	case RoleDivider:
		return []string{""}, nil
	default:
		return core.WrapAnsi(m.Text, width), nil
	}
}

// renderUserRole renders a user message as Markdown with the configured
// UserPrefix marker (styled via UserStyle) and indented continuation lines,
// so the user's turn reads as one visually distinct block against the
// assistant's unprefixed output. An empty prefix falls back to plain
// Markdown for themes that opt out.
func (h *ChatHistory) renderUserRole(text string, width int64, chatTheme ChatHistoryTheme) []string {
	prefix := chatTheme.UserPrefix
	indent := int64(0)
	bubbleW := messageBubbleWidth(width, indent)
	if prefix == "" {
		lines := h.renderMarkdownRole(text, bubbleW, chatTheme)
		return envelopeBubble(lines, indent, bubbleW, chatTheme.UserBgStyle)
	}
	prefixW := core.VisibleWidth(prefix)
	innerWidth := bubbleW - prefixW
	if innerWidth < 1 {
		innerWidth = 1
	}
	lines := h.renderMarkdownRole(text, innerWidth, chatTheme)
	marker := chatTheme.UserStyle.Render(prefix)
	lineIndent := strings.Repeat(" ", int(prefixW))
	bgEnabled := chatTheme.UserBgStyle.BgStrip() != ""
	out := make([]string, 0, len(lines))
	for i, ln := range lines {
		if i == 0 {
			out = append(out, marker+ln)
			continue
		}
		if strings.TrimSpace(core.StripAnsi(ln)) == "" {
			if bgEnabled {
				out = append(out, strings.Repeat(" ", int(prefixW)))
			} else {
				out = append(out, ln)
			}
		} else {
			out = append(out, lineIndent+ln)
		}
	}
	return envelopeBubble(out, indent, bubbleW, chatTheme.UserBgStyle)
}

func (h *ChatHistory) renderMarkdownRole(text string, width int64, chatTheme ChatHistoryTheme) []string {
	if text == "" {
		return nil
	}
	md := component.NewMarkdown(text)
	md.SetTheme(chatTheme.MarkdownTheme)
	return md.Render(width)
}

func appendStreamCursor(line string, width int64, cursor string) string {
	if core.VisibleWidth(line)+core.VisibleWidth(cursor) <= width {
		return line + cursor
	}
	keep := width - core.VisibleWidth(cursor)
	return core.TruncateToWidth(line, keep, "") + cursor
}

func applyBgOneLine(line, bgSGR, reset string) string {
	if !strings.Contains(line, "\x1b") {
		return bgSGR + line + reset
	}
	var b strings.Builder
	b.Grow(len(line) + 32)
	b.WriteString(bgSGR)
	i := 0
	for i < len(line) {
		c := line[i]
		if c == 0x1B {
			adv := core.SkipAnsiSeq(line, i)
			if adv > 0 {
				seq := line[i : i+adv]
				b.WriteString(seq)
				if isSgrReset(seq) {
					b.WriteString(bgSGR)
				}
				i += adv
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	b.WriteString(reset)
	return b.String()
}

func isSgrReset(seq string) bool {
	if len(seq) < 3 || seq[0] != 0x1B || seq[1] != '[' {
		return false
	}
	if seq[len(seq)-1] != 'm' {
		return false
	}
	params := seq[2 : len(seq)-1]
	if params == "" {
		return true
	}
	for _, p := range strings.Split(params, ";") {
		for _, ch := range p {
			if ch != '0' {
				return false
			}
		}
	}
	return true
}

// renderRoleTransitionBand renders a 1-line visual accent that precedes a
// message when its role differs from the previous one. This achieves the
// "same-role tight / cross-role loose" density heuristic without touching
// renderMessageSeparator (which is locked down by 14 strict assertions in
// TestRenderMessageSeparator).
//
// Visual style (lazy vertical-timeline / lazygit branch-graph inspired):
//
//	▏────────────────────────  (1-cell left accent in the NEW role's color,
//	                           followed by a thin muted rule out to ~85% width)
func renderRoleTransitionBand(newRole ChatRole, width int64, chatTheme ChatHistoryTheme) []string {
	if width < 8 {
		return nil
	}
	bubbleW := messageBubbleWidth(width, 0)
	if bubbleW < 4 {
		bubbleW = 4
	}
	var accentFn func(string) string
	switch newRole {
	case RoleUser:
		accentFn = chatTheme.UserStyle.Render
	case RoleAssistant:
		accentFn = chatTheme.AssistantStyle.Render
	case RoleSystem:
		accentFn = chatTheme.SystemStyle.Render
	case RoleTool:
		accentFn = chatTheme.ToolBorder.Render
	case RoleError:
		accentFn = chatTheme.ErrorStyle.Render
	default:
		accentFn = chatTheme.DimStyle.Render
	}
	accent := accentFn(theme.SymbolBarAccentThin)
	ruleLen := theme.RoleTransitionBandRule(bubbleW)
	rule := chatTheme.DimStyle.Render(strings.Repeat(theme.SymbolRuleLightH, ruleLen))
	return []string{accent + rule}
}
