package chat

// This file contains the ChatHistory rendering pipeline: the public Render
// (viewport clipping + scroll indicator + width padding), renderAll (lays out
// messages with separators and collapses consecutive tool groups), the
// per-message renderMessageCached/renderMessage (role-specific styling), the
// text-selection highlight pass, and trimBlankEdges.
//
// cachedMsgRanges (built in renderAll) records the absolute line span of each
// message/group so chat_history_input.go can map mouse clicks back to a
// message for click-to-toggle behavior.

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
)

// renderMessageCached returns the rendered lines for a message, using a
// per-message cache keyed on message ID. Caller must hold mu.
//
// For Pending assistant messages the cache also carries a per-block render
// cache so streaming deltas only re-render the tail block (see
// component.BlockCache). Non-Pending messages cache the whole rendered output.
func (h *ChatHistory) renderMessageCached(m ChatMessage, theme ChatHistoryTheme, width int64) []string {
	if m.ID == "" {
		return h.renderMessage(m, theme, width, nil)
	}
	if cached, ok := h.msgCache[m.ID]; ok {
		// Pending messages must re-render on every delta (their text grew),
		// but they reuse the block cache so only the tail block re-renders.
		if !m.Pending {
			return cached.lines
		}
		lines := h.renderMessage(m, theme, width, cached.blockCache)
		trimmed := trimBlankEdges(lines)
		cachedLines := make([]string, len(trimmed))
		copy(cachedLines, trimmed)
		h.msgCache[m.ID] = cachedMessage{lines: cachedLines, blockCache: cached.blockCache}
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
	h.msgCache[m.ID] = cachedMessage{lines: cachedLines, blockCache: bc}
	return cachedLines
}

// Render draws the transcript, clipping to MaxRows when set.
func (h *ChatHistory) Render(width int64) []string {
	if width < 1 {
		width = 1
	}
	h.mu.Lock()
	wasDirty := h.dirty
	if h.cachedWidth != width {
		h.cachedWidth = width
		h.clearMsgCacheLocked()
		h.dirty = true
	}
	if h.dirty || h.cachedAll == nil {
		h.cachedAll = h.renderAll(width)
		h.dirty = false
		// If content changed (was dirty), reset scroll if following tail.
		if wasDirty && h.follow {
			h.offset = 0
		}
		// Also clamp if old offset is now past the content end.
		if len(h.cachedAll) > 0 && h.offset > 0 {
			maxLines := int64(len(h.cachedAll))
			if maxLines > h.maxRows && h.maxRows > 0 && h.offset > maxLines-h.maxRows {
				h.offset = maxLines - h.maxRows
				if h.offset < 0 {
					h.offset = 0
				}
			} else if h.maxRows <= 0 || maxLines <= h.maxRows {
				h.offset = 0
			}
		}
	}
	// Refresh the tail anchor whenever the viewport is at the tail so that,
	// once the user scrolls up, tailAnchorLen freezes and Render can compute
	// how many new lines have arrived since.
	if h.follow {
		h.tailAnchorLen = int64(len(h.cachedAll))
	}
	newSinceAnchor := int64(0)
	if !h.follow && h.tailAnchorLen > 0 {
		newSinceAnchor = int64(len(h.cachedAll)) - h.tailAnchorLen
		if newSinceAnchor < 0 {
			newSinceAnchor = 0
		}
	}
	all := h.cachedAll
	maxRows := h.maxRows
	offset := h.offset
	follow := h.follow
	h.mu.Unlock()

	if maxRows <= 0 || int64(len(all)) <= maxRows {
		return all
	}
	end := int64(len(all)) - offset
	if end > int64(len(all)) {
		end = int64(len(all))
	}
	start := end - maxRows
	if start < 0 {
		start = 0
		end = maxRows
	}
	visible := all[start:end]

	// Add scroll indicator when not auto-following
	if !follow && end < int64(len(all)) {
		indicator := h.theme.DimStyle.Render(fmt.Sprintf("^ %d more lines — End to follow", int64(len(all))-end))
		// Drop last visible line to keep within maxRows, prevent pushing
		// status bar off-screen.
		if int64(len(visible)) >= maxRows && len(visible) > 0 {
			visible = visible[:len(visible)-1]
		}
		visible = append([]string{indicator}, visible...)
	}

	// Stick-to-bottom hint: when the user scrolled up and new streaming
	// content arrived since, surface a "↓ N new" footer so they know there's
	// fresh output to jump to. Placed at the bottom of the visible window.
	if !follow && newSinceAnchor > 0 {
		hint := h.theme.SuccessStyle.Render(fmt.Sprintf("↓ %d new — End to follow", newSinceAnchor))
		if int64(len(visible)) >= maxRows && len(visible) > 0 {
			visible = visible[:len(visible)-1]
		}
		visible = append(visible, hint)
	}

	// Pad every line to full width so the TUI diff engine's \x1b[2K
	// never leaves a partial column that could bleed into the next line.
	for i, ln := range visible {
		if core.VisibleWidth(ln) < width {
			visible[i] = core.PadToWidth(ln, width)
		}
	}

	return visible
}

// Invalidate drops the render cache.
func (h *ChatHistory) Invalidate() {
	h.mu.Lock()
	h.dirty = true
	h.mu.Unlock()
	h.invalidate()
}

func (h *ChatHistory) isSelectionEmptyLocked() bool {
	return h.selStart.line == h.selEnd.line && h.selStart.col == h.selEnd.col
}

// GetSelectedText returns the currently selected text, or empty string if no selection.
func (h *ChatHistory) GetSelectedText() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.getSelectedTextLocked()
}

// ClearSelection clears the current text selection.
func (h *ChatHistory) ClearSelection() {
	h.mu.Lock()
	h.selActive = false
	h.selDragging = false
	h.mu.Unlock()
	h.invalidate()
}

func (h *ChatHistory) getSelectedTextLocked() string {
	if !h.selActive || h.isSelectionEmptyLocked() {
		return ""
	}

	total := int64(len(h.cachedAll))
	if total == 0 {
		return ""
	}

	topLine := h.selStart.line
	botLine := h.selEnd.line
	topCol := h.selStart.col
	botCol := h.selEnd.col

	// Normalize so topLine <= botLine, and when equal topCol <= botCol
	if topLine > botLine || (topLine == botLine && topCol > botCol) {
		topLine, botLine = botLine, topLine
		topCol, botCol = botCol, topCol
	}

	if topLine < 0 || topLine >= total || botLine < 0 || botLine >= total {
		return ""
	}

	var result []string
	for i := topLine; i <= botLine && i < total; i++ {
		line := h.cachedAll[i]
		var part string
		switch {
		case topLine == botLine:
			part = core.SliceByColumn(line, topCol, botCol)
		case i == topLine:
			part = core.SliceByColumn(line, topCol, core.VisibleWidth(line))
		case i == botLine:
			part = core.SliceByColumn(line, 0, botCol)
		default:
			part = line
		}
		result = append(result, core.StripAnsi(part))
	}

	return strings.Join(result, "\n")
}

func (h *ChatHistory) renderAll(width int64) []string {
	theme := h.theme
	var out []string
	var ranges []msgRange

	if len(h.messages) == 0 {
		// 品牌启动屏：引导用户开始对话或使用命令
		dim := h.theme.DimStyle
		accent := h.theme.UserStyle
		sys := h.theme.SystemStyle

		return []string{
			"",
			accent.Render("  Mady") + dim.Render(" — 证据驱动的专利案件工作台"),
			"",
			sys.Render("  输入消息开始对话，输入 / 查看可用命令"),
			dim.Render("  Ctrl+C 退出  ·  Ctrl+P 命令面板  ·  ? 帮助"),
			"",
		}
	}

	for i := 0; i < len(h.messages); i++ {
		m := h.messages[i]

		// Detect a run of consecutive tool/system messages starting at i.
		if m.Role == RoleTool || m.Role == RoleSystem {
			groupEnd := i
			for j := i + 1; j < len(h.messages); j++ {
				r := h.messages[j].Role
				if r == RoleTool || r == RoleSystem {
					groupEnd = j
				} else {
					break
				}
			}
			// Group 2+ consecutive tools UNLESS we're still mid-turn:
			// the group extends to the end AND the last non-tool msg is pending.
			midTurn := groupEnd == len(h.messages)-1
			if midTurn {
				// Check if the last non-tool message is still streaming.
				for j := i - 1; j >= 0; j-- {
					if h.messages[j].Role != RoleTool && h.messages[j].Role != RoleSystem {
						midTurn = h.messages[j].Pending
						break
					}
				}
			}
			if groupEnd > i && !midTurn {
				// Render collapsed or expanded group
				bar := theme.ToolBorder.Render("▌ ")
				toolCount, sysCount := 0, 0
				for j := i; j <= groupEnd; j++ {
					if h.messages[j].Role == RoleTool {
						toolCount++
					} else {
						sysCount++
					}
				}

				if h.expandedGroups[i] {
					start := len(out)
					summary := fmt.Sprintf("[-] %d tools · %d msgs", toolCount, sysCount)
					if sysCount == 0 {
						summary = fmt.Sprintf("[-] %d tools", toolCount)
					}
					out = append(out, core.PadToWidth(bar+theme.DimStyle.Render(summary), width))
					for j := i; j <= groupEnd; j++ {
						out = append(out, trimBlankEdges(h.renderMessageCached(h.messages[j], theme, width))...)
					}
					ranges = append(ranges, msgRange{
						startLine: start, endLine: len(out), msgIndex: i,
						toolGroup: true, groupFrom: i, groupTo: groupEnd,
					})
				} else {
					start := len(out)
					summary := fmt.Sprintf("[+] %d tools · %d msgs", toolCount, sysCount)
					if sysCount == 0 {
						summary = fmt.Sprintf("[+] %d tools", toolCount)
					}
					for j := i; j <= groupEnd; j++ {
						if h.messages[j].Meta != "" && h.messages[j].Meta != "tool" {
							summary = "[+] " + h.messages[j].Meta
							break
						}
					}
					out = append(out, bar+theme.DimStyle.Render(summary))
					ranges = append(ranges, msgRange{
						startLine: start, endLine: len(out), msgIndex: i,
						toolGroup: true, groupFrom: i, groupTo: groupEnd,
					})
				}
				i = groupEnd
				continue
			}
		}

		if i > 0 {
			prev := h.messages[i-1]
			switch {
			case (prev.Role == RoleUser && m.Role == RoleAssistant) ||
				(prev.Role == RoleAssistant && m.Role == RoleUser):
				sep := theme.DimStyle.Render(strings.Repeat("─", int(width)))
				out = append(out, "", sep, "")
			case prev.Role == RoleTool || m.Role == RoleTool:
			default:
				out = append(out, "", "")
			}
		}
		start := len(out)
		out = append(out, trimBlankEdges(h.renderMessageCached(m, theme, width))...)
		ranges = append(ranges, msgRange{startLine: start, endLine: len(out), msgIndex: i})
	}
	h.cachedMsgRanges = ranges

	// Apply selection highlight
	if h.selActive && !h.isSelectionEmptyLocked() {
		h.applySelectionHighlightLocked(out, width)
	}

	return out
}

func (h *ChatHistory) applySelectionHighlightLocked(lines []string, width int64) {
	total := int64(len(lines))
	if total == 0 || h.maxRows <= 0 {
		return
	}

	// Compute the visible viewport range in absolute indices
	end := total - h.offset
	start := end - h.maxRows
	if start < 0 {
		start = 0
	}

	topLine := h.selStart.line
	botLine := h.selEnd.line
	topCol := h.selStart.col
	botCol := h.selEnd.col

	// Normalize so topLine <= botLine, and when equal topCol <= botCol
	if topLine > botLine || (topLine == botLine && topCol > botCol) {
		topLine, botLine = botLine, topLine
		topCol, botCol = botCol, topCol
	}

	// Clamp selection to visible viewport
	if botLine < start || topLine >= end {
		return
	}
	if topLine < start {
		topLine = start
		topCol = 0
	}
	if botLine >= end {
		botLine = end - 1
		botCol = core.VisibleWidth(lines[botLine])
	}

	selBg := h.theme.SelectedBg
	if selBg == "" {
		selBg = "\x1b[48;5;33m"
	}
	selStyle := core.ParseLine(selBg + " " + "\x1b[0m")
	if selStyle.IsRaw() || len(selStyle.Cells) == 0 {
		return
	}
	selectedStyle := selStyle.Cells[0].Style
	if selectedStyle.Bg.IsDefault() {
		return
	}

	for i := topLine; i <= botLine && i < end; i++ {
		if i < 0 || i >= total {
			continue
		}
		line := lines[i]
		row := core.ParseLine(line)
		if row.IsRaw() {
			continue
		}
		lineWidth := row.VisibleWidth()
		if lineWidth <= 0 {
			continue
		}

		fromCol := int64(0)
		toCol := lineWidth
		switch {
		case topLine == botLine:
			fromCol = topCol
			toCol = botCol
		case i == topLine:
			fromCol = topCol
		case i == botLine:
			toCol = botCol
		}

		if fromCol < 0 {
			fromCol = 0
		}
		if toCol < 0 {
			toCol = 0
		}
		if fromCol > lineWidth {
			fromCol = lineWidth
		}
		if toCol > lineWidth {
			toCol = lineWidth
		}
		if toCol < fromCol {
			toCol = fromCol
		}

		if fromCol != toCol {
			start := int(fromCol)
			endCol := int(toCol)
			if start < 0 {
				start = 0
			}
			if endCol > len(row.Cells) {
				endCol = len(row.Cells)
			}
			for c := start; c < endCol; c++ {
				row.Cells[c].Style = selectedStyle
			}
		}

		highlighted := core.SerializeRow(row)
		// Keep each highlighted row width-stable across drag updates.
		lines[i] = core.PadToWidth(core.TruncateToWidth(highlighted, lineWidth, ""), lineWidth)
	}
}

// trimBlankEdges removes leading and trailing blank (whitespace-only) lines
// from a rendered message block. Streamed assistant text often carries stray
// leading/trailing newlines which the markdown renderer turns into padded
// blank lines, inflating the vertical gap between turns. Internal blank lines
// (e.g. inside code blocks) are preserved.
func trimBlankEdges(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(core.StripAnsi(lines[start])) == "" {
		start++
	}
	for end > start && strings.TrimSpace(core.StripAnsi(lines[end-1])) == "" {
		end--
	}
	return lines[start:end]
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
