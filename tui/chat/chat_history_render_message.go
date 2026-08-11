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
)

// renderMessageCachedWithCache is the cache-parameterized variant used during
// lock-free snapshot rendering. It reads from and writes to the provided cache
// map instead of h.msgCache, so the snapshot render can run without holding
// h.mu while still benefiting from per-message caching.
func (h *ChatHistory) renderMessageCachedWithCache(m ChatMessage, theme ChatHistoryTheme, width int64, cache map[string]cachedMessage) ([]string, [][]core.LinkSpan) {
	if m.ID == "" {
		return h.renderMessage(m, theme, width, nil)
	}
	var bc *component.BlockCache
	if cached, ok := cache[m.ID]; ok {
		// 同宽度且内容未变（非 Pending）时直接复用渲染行与链接。
		if cached.width == width && !m.Pending {
			return cached.lines, cached.links
		}
		// 宽度不一致（如展开工具组以 innerW 渲染）或 Pending 增量更新时
		// 重渲染，但复用块缓存，避免整条消息重新解析。
		if m.Pending {
			bc = cached.blockCache
		}
	}
	if bc == nil && m.Pending && m.Role == RoleAssistant && m.Text != "" {
		bc = &component.BlockCache{}
	}
	lines, msgLinks := h.renderMessage(m, theme, width, bc)
	// Trim blank edges before caching so the stored version matches what
	// renderAll callers need (trimBlankEdges is idempotent on already-trimmed).
	// 链接元数据同步裁剪，保证与缓存行一一对应。
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
func (h *ChatHistory) renderDomainCard(m ChatMessage, theme ChatHistoryTheme, width int64) ([]string, [][]core.LinkSpan) {
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
		// Fallback: render body as markdown
		md := component.NewMarkdown(dm.Body)
		md.SetTheme(theme.MarkdownTheme)
		return md.Render(width), nil
	}
}

// nilLinks 返回 n 个空链接行（全部 nil 元素）。仅用于 renderMessagesRange：
// 消息渲染返回顶层 nil（无链接）时，兜底补齐与输出行等长的元数据，
// 保持 outLinks 与 out 一一对应。
func nilLinks(n int) [][]core.LinkSpan {
	return make([][]core.LinkSpan, n)
}

func (h *ChatHistory) renderMessage(m ChatMessage, theme ChatHistoryTheme, width int64, mdCache *component.BlockCache) ([]string, [][]core.LinkSpan) {
	h.renderCount++
	// Phase 5: route domain messages to professional card renderers
	if m.DomainMsg != nil {
		return h.renderDomainCard(m, theme, width)
	}

	switch m.Role {
	case RoleUser:
		return h.renderMarkdownRole(m.Text, width, theme), nil
	case RoleAssistant:
		// Collapsed assistant messages (e.g. collapsed diffs)
		if m.Collapsed && m.Text != "" {
			firstLine := m.Text
			if idx := strings.IndexByte(firstLine, '\n'); idx > 0 {
				firstLine = firstLine[:idx]
			}
			if len(firstLine) > 200 {
				firstLine = firstLine[:197] + "..."
			}
			head := theme.DimStyle.Render(firstLine)
			lines := core.WrapAnsi(head, width)
			lines = append(lines, core.TruncateToWidth("  "+theme.DimStyle.Render("▸ expand"), width, ""))
			return lines, nil
		}

		innerWidth := width
		if innerWidth < 1 {
			innerWidth = 1
		}

		var allLines []string

		// Render thinking segments first — delegated to the injected
		// ReasoningRenderer. The default implementation honors the
		// legacy Show/Mode policy; custom renderers can draw reasoning
		// anywhere (sidebar, overlay, etc.).
		if h.reasoningRenderer != nil {
			if rendered := h.reasoningRenderer.RenderThinking(m, innerWidth); len(rendered) > 0 {
				allLines = append(allLines, rendered...)
			}
		}

		// Render text content. When a block cache is supplied (streaming
		// Pending messages), reuse the per-block render output so each delta
		// only re-renders the tail block instead of the whole message.
		if m.Text != "" {
			var lines []string
			if mdCache != nil {
				lines = component.RenderMarkdownIncremental(m.Text, innerWidth, theme.MarkdownTheme, mdCache)
			} else {
				md := component.NewMarkdown(m.Text)
				md.SetTheme(theme.MarkdownTheme)
				lines = md.Render(innerWidth)
			}
			if m.Pending {
				if len(lines) == 0 {
					lines = []string{theme.DimStyle.Render("…")}
				} else {
					last := lines[len(lines)-1]
					lines[len(lines)-1] = appendStreamCursor(last, width, theme.UserStyle.Render("▊"))
				}
			}
			allLines = append(allLines, lines...)
		} else if len(m.ThinkingSegments) > 0 && m.Pending {
			if len(allLines) == 0 {
				allLines = []string{theme.DimStyle.Render("…")}
			} else {
				last := allLines[len(allLines)-1]
				allLines[len(allLines)-1] = appendStreamCursor(last, width, theme.ThinkingStyle.Render("▊"))
			}
		}

		if len(allLines) == 0 {
			allLines = []string{""}
		}
		return allLines, nil
	case RoleSystem:
		return h.renderMarkdownRole(m.Text, width, theme), nil
	case RoleTool:
		// Tool results are rendered via the shared ToolCard component so the
		// collapsed/expanded treatment stays consistent with diffs and future
		// reasoning blocks. ToolCard owns no state — collapsed state is read
		// from the message, and the chat_theme→toolcard theme bridge keeps
		// styling identical to the previous inline implementation.
		tcTheme := component.ToolCardTheme{
			Border:        theme.ToolBorder.Render,
			Success:       theme.SuccessStyle.Render,
			Error:         theme.ErrorStyle.Render,
			Title:         func(s string) string { return theme.ToolStyle.Render(theme.ToolPrefix + s) },
			Dim:           theme.DimStyle.Render,
			MarkdownTheme: theme.MarkdownTheme,
		}
		return component.RenderToolCard(component.ToolCardConfig{
			Name:      m.Meta,
			Seq:       m.Seq,
			Status:    m.Text,
			Duration:  m.Duration,
			Collapsed: m.Collapsed,
		}, tcTheme, width), nil
	case RoleError:
		return h.renderMarkdownRole(m.Text, width, theme), nil
	case RoleDivider:
		return []string{""}, nil
	default:
		return core.WrapAnsi(m.Text, width), nil
	}
}

// renderMarkdownRole renders message text as Markdown. No left bar prefix.
func (h *ChatHistory) renderMarkdownRole(text string, width int64, theme ChatHistoryTheme) []string {
	if text == "" {
		return nil
	}
	md := component.NewMarkdown(text)
	md.SetTheme(theme.MarkdownTheme)
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
