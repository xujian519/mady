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
func (h *ChatHistory) renderMessageCachedWithCache(m ChatMessage, theme ChatHistoryTheme, width int64, cache map[string]cachedMessage) []string {
	if m.ID == "" {
		return h.renderMessage(m, theme, width, nil)
	}
	if cached, ok := cache[m.ID]; ok {
		// Pending messages must re-render on every delta (their text grew),
		// but they reuse the block cache so only the tail block re-renders.
		if !m.Pending {
			return cached.lines
		}
		lines := h.renderMessage(m, theme, width, cached.blockCache)
		trimmed := trimBlankEdges(lines)
		cachedLines := make([]string, len(trimmed))
		copy(cachedLines, trimmed)
		cache[m.ID] = cachedMessage{lines: cachedLines, blockCache: cached.blockCache}
		return cachedLines
	}
	var bc *component.BlockCache
	if m.Pending && m.Role == RoleAssistant && m.Text != "" {
		bc = &component.BlockCache{}
	}
	lines := h.renderMessage(m, theme, width, bc)
	if m.ID == "" {
		return lines
	}
	// Trim blank edges before caching so the stored version matches what
	// renderAll callers need (trimBlankEdges is idempotent on already-trimmed).
	trimmed := trimBlankEdges(lines)
	cachedLines := make([]string, len(trimmed))
	copy(cachedLines, trimmed)
	cache[m.ID] = cachedMessage{lines: cachedLines, blockCache: bc}
	return cachedLines
}

// renderDomainCard routes a DomainMessage to the appropriate professional card renderer.
func (h *ChatHistory) renderDomainCard(m ChatMessage, theme ChatHistoryTheme, width int64) []string {
	dm := m.DomainMsg
	if dm == nil {
		return nil
	}
	switch dm.Type {
	case "evidence_card":
		ecTheme := component.DefaultEvidenceCardTheme()
		return component.RenderEvidenceCard(dm, m.Collapsed, ecTheme, width)
	case "conclusion_card":
		ccTheme := component.DefaultConclusionCardTheme()
		return component.RenderConclusionCard(dm, ccTheme, width)
	case "approval_prompt":
		acTheme := component.DefaultApprovalCardTheme()
		return component.RenderApprovalCard(dm, acTheme, width)
	default:
		// Fallback: render body as markdown
		md := component.NewMarkdown(dm.Body)
		md.SetTheme(theme.MarkdownTheme)
		return md.Render(width)
	}
}

func (h *ChatHistory) renderMessage(m ChatMessage, theme ChatHistoryTheme, width int64, mdCache *component.BlockCache) []string {
	h.renderCount++
	// Phase 5: route domain messages to professional card renderers
	if m.DomainMsg != nil {
		return h.renderDomainCard(m, theme, width)
	}

	switch m.Role {
	case RoleUser:
		bar := theme.UserStyle.Render("▌ ")
		body := bar + theme.UserStyle.Render(m.Text)
		return core.WrapAnsi(body, width)
	case RoleAssistant:
		// Collapsed assistant messages (e.g. collapsed diffs)
		if m.Collapsed && m.Text != "" {
			// Show first line as summary + expand hint
			firstLine := m.Text
			if idx := strings.IndexByte(firstLine, '\n'); idx > 0 {
				firstLine = firstLine[:idx]
			}
			if len(firstLine) > 80 {
				firstLine = firstLine[:77] + "..."
			}
			head := theme.ToolBorder.Render("▌") + " " + theme.DimStyle.Render(firstLine)
			lines := core.WrapAnsi(head, width)
			lines = append(lines, theme.DimStyle.Render("  ▸ expand"))
			return lines
		}

		var allLines []string

		// Render thinking segments first — delegated to the injected
		// ReasoningRenderer. The default implementation honors the
		// legacy Show/Mode policy; custom renderers can draw reasoning
		// anywhere (sidebar, overlay, etc.).
		if h.reasoningRenderer != nil {
			if rendered := h.reasoningRenderer.RenderThinking(m, width); len(rendered) > 0 {
				allLines = append(allLines, rendered...)
			}
		}

		// Render text content. When a block cache is supplied (streaming
		// Pending messages), reuse the per-block render output so each delta
		// only re-renders the tail block instead of the whole message.
		if m.Text != "" {
			var lines []string
			if mdCache != nil {
				lines = component.RenderMarkdownIncremental(m.Text, width, theme.MarkdownTheme, mdCache)
			} else {
				md := component.NewMarkdown(m.Text)
				md.SetTheme(theme.MarkdownTheme)
				lines = md.Render(width)
			}
			if m.Pending {
				if len(lines) == 0 {
					lines = []string{theme.DimStyle.Render("…")}
				} else {
					last := lines[len(lines)-1]
					lines[len(lines)-1] = last + theme.UserStyle.Render("▊")
				}
			}
			allLines = append(allLines, lines...)
		} else if len(m.ThinkingSegments) > 0 && m.Pending {
			// Only thinking, no text yet, show cursor
			if len(allLines) == 0 {
				allLines = []string{theme.DimStyle.Render("…")}
			} else {
				last := allLines[len(allLines)-1]
				allLines[len(allLines)-1] = last + theme.ThinkingStyle.Render("▊")
			}
		}

		if len(allLines) == 0 {
			allLines = []string{theme.DimStyle.Render("…")}
		}
		return allLines
	case RoleSystem:
		bar := theme.ToolBorder.Render("▌ ")
		return core.WrapAnsi(bar+theme.SystemStyle.Render(m.Text), width)
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
			Status:    m.Text,
			Duration:  m.Duration,
			Collapsed: m.Collapsed,
		}, tcTheme, width)
	case RoleError:
		bar := theme.ErrorStyle.Render("▌ ")
		return core.WrapAnsi(bar+theme.ErrorStyle.Render(m.Text), width)
	case RoleDivider:
		ch := theme.DividerChar
		if ch == "" {
			ch = "─"
		}
		return []string{theme.DimStyle.Render(strings.Repeat(ch, int(width)))}
	default:
		return core.WrapAnsi(m.Text, width)
	}
}
