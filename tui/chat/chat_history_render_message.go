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

func (h *ChatHistory) renderMessage(m ChatMessage, chatTheme ChatHistoryTheme, width int64, mdCache *component.BlockCache) ([]string, [][]core.LinkSpan) {
	h.renderCount++
	if m.DomainMsg != nil {
		return h.renderDomainCard(m, chatTheme, width)
	}

	switch m.Role {
	case RoleUser:
		return h.renderUserRole(m.Text, width, chatTheme), nil
	case RoleAssistant:
		if m.Collapsed && m.Text != "" {
			firstLine := m.Text
			if idx := strings.IndexByte(firstLine, '\n'); idx > 0 {
				firstLine = firstLine[:idx]
			}
			if len([]rune(firstLine)) > 200 {
				firstLine = string([]rune(firstLine)[:197]) + "..."
			}
			head := core.TruncateToWidth(chatTheme.DimStyle.Render(firstLine), width, "…")
			expand := core.TruncateToWidth("  "+chatTheme.DimStyle.Render("▸ expand"), width, "")
			collapsedLines := []string{head, expand}
			return applyBubbleBg(collapsedLines, width, chatTheme.AssistantBgStyle), nil
		}

		innerWidth := width
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
					lines[len(lines)-1] = appendStreamCursor(last, width, chatTheme.UserStyle.Render("▊"))
				}
			}
			allLines = append(allLines, lines...)
		} else if len(m.ThinkingSegments) > 0 && m.Pending {
			if len(allLines) == 0 {
				allLines = []string{chatTheme.DimStyle.Render("…")}
			} else {
				last := allLines[len(allLines)-1]
				allLines[len(allLines)-1] = appendStreamCursor(last, width, chatTheme.ThinkingStyle.Render("▊"))
			}
		}

		if len(allLines) == 0 {
			allLines = []string{""}
		}
		return applyBubbleBg(allLines, innerWidth, chatTheme.AssistantBgStyle), nil
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
	if prefix == "" {
		lines := h.renderMarkdownRole(text, width, chatTheme)
		return applyBubbleBg(lines, width, chatTheme.UserBgStyle)
	}
	prefixW := core.VisibleWidth(prefix)
	innerWidth := width - prefixW
	if innerWidth < 1 {
		innerWidth = 1
	}
	lines := h.renderMarkdownRole(text, innerWidth, chatTheme)
	marker := chatTheme.UserStyle.Render(prefix)
	indent := strings.Repeat(" ", int(prefixW))
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
			out = append(out, indent+ln)
		}
	}
	return applyBubbleBg(out, width, chatTheme.UserBgStyle)
}

func (h *ChatHistory) renderMarkdownRole(text string, width int64, chatTheme ChatHistoryTheme) []string {
	if text == "" {
		return nil
	}
	md := component.NewMarkdown(text)
	md.SetTheme(chatTheme.MarkdownTheme)
	return md.Render(width)
}

// appendStreamCursor appends a streaming cursor to the last rendered line
// without pushing it past width. Markdown block rendering (padded code fences,
// tables, hard-wrapped paragraphs) can already produce lines exactly as wide
// as width; blindly appending "▊" would overflow by one cell and the scrollbar
// / engine normalizeLayer would hard-truncate the line — dropping the trailing
// real character along with the cursor. When the line is at capacity we trim
// one cell so the total stays within width.
func appendStreamCursor(line string, width int64, cursor string) string {
	if core.VisibleWidth(line)+core.VisibleWidth(cursor) <= width {
		return line + cursor
	}
	keep := width - core.VisibleWidth(cursor)
	return core.TruncateToWidth(line, keep, "") + cursor
}

func applyBubbleBg(lines []string, width int64, bgStyle theme.Style) []string {
	bgSGR := bgStyle.BgStrip()
	if bgSGR == "" {
		return lines
	}
	if len(lines) == 0 {
		return lines
	}
	reset := theme.Reset
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		padded := core.PadToWidth(ln, width)
		out = append(out, applyBgOneLine(padded, bgSGR, reset))
	}
	return out
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
