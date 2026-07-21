package chat

// This file contains the ChatHistory rendering pipeline: the public Render
// (viewport clipping + scroll indicator + width padding), renderAll (lays out
// messages with separators and collapses consecutive tool groups), and
// trimBlankEdges.
//
// Per-message rendering is in chat_history_render_message.go.
// Text-selection highlighting is in chat_history_render_highlight.go.
//
// cachedMsgRanges (built in renderAll) records the absolute line span of each
// message/group so chat_history_input.go can map mouse clicks back to a
// message for click-to-toggle behavior.

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/tui/core"
)

// renderSnapshot holds a point-in-time copy of the mutable state that renderAll
// needs. It is captured under ChatHistory.mu, then the lock is released before
// the expensive renderAll runs. This keeps the critical section short so
// streaming deltas (AppendDelta) are never blocked by Markdown parsing.
type renderSnapshot struct {
	msgs              []ChatMessage
	theme             ChatHistoryTheme
	expandedGroups    map[int]bool
	selActive         bool
	selStart          selectionPos
	selEnd            selectionPos
	reasoningRenderer ReasoningRenderer
	firstDirtyIdx     int      // earliest changed message index (0 = full rebuild)
	cachedAll         []string // previous render output, for splice fast path
	cachedMsgRanges   []msgRange
}

// captureSnapshot copies all mutable render state under h.mu. The returned
// snapshot is safe to use without holding the lock.
func (h *ChatHistory) captureSnapshot() renderSnapshot {
	msgs := make([]ChatMessage, len(h.messages))
	copy(msgs, h.messages)
	eg := make(map[int]bool, len(h.expandedGroups))
	for k, v := range h.expandedGroups {
		eg[k] = v
	}
	// Snapshot cachedAll and cachedMsgRanges for the streaming fast path,
	// avoiding unlocked reads of these fields during Phase 2 rendering.
	cal := h.cachedAll
	cmr := make([]msgRange, len(h.cachedMsgRanges))
	copy(cmr, h.cachedMsgRanges)
	return renderSnapshot{
		msgs:              msgs,
		theme:             h.theme,
		expandedGroups:    eg,
		selActive:         h.selActive,
		selStart:          h.selStart,
		selEnd:            h.selEnd,
		reasoningRenderer: h.reasoningRenderer,
		firstDirtyIdx:     h.firstDirtyIdx,
		cachedAll:         cal,
		cachedMsgRanges:   cmr,
	}
}

// Render draws the transcript, clipping to MaxRows when set.
//
// Phase 2 optimization: the expensive renderAll (which iterates all messages
// and runs Markdown parsing) no longer runs under ChatHistory.mu. Instead we:
//  1. Snapshot mutable state under the lock
//  2. Release the lock and render from the snapshot
//  3. Re-acquire the lock to merge the updated msgCache and write back results
//
// This eliminates the main serialization point between streaming delta
// processing (AppendDelta) and rendering (renderAll). Before this change,
// AppendDelta could block for 5-10ms waiting for renderAll to release the
// lock; now the critical section is ~100µs (snapshot + merge).
func (h *ChatHistory) Render(width int64) []string {
	if width < 1 {
		width = 1
	}
	h.mu.Lock()
	wasDirty := h.dirty
	if h.cachedWidth != width {
		h.cachedWidth = width
		h.clearMsgCacheLocked()
		h.firstDirtyIdx = 0
		h.dirty = true
	}

	needRender := h.dirty || h.cachedAll == nil

	var all []string
	if needRender {
		// Phase 1: snapshot mutable state under lock.
		// Reset dirty BEFORE releasing the lock so Phase 3 can detect
		// whether AppendDelta set it during Phase 2. If h.dirty is still
		// false at merge time, no concurrent mutations happened and we can
		// safely clear firstDirtyIdx. If true, AppendDelta set it and we
		// must preserve both flags for the next render cycle.
		h.dirty = false
		snap := h.captureSnapshot()
		// Shallow-copy the msgCache map so the snapshot render can use
		// existing cached entries and populate new ones locally.
		localCache := make(map[string]cachedMessage, len(h.msgCache))
		for k, v := range h.msgCache {
			localCache[k] = v
		}
		h.mu.Unlock()

		// Phase 2: expensive rendering without holding h.mu.
		// AppendDelta can process new deltas concurrently.
		rendered, ranges := h.renderAllFromSnapshot(snap, width, localCache)

		// Phase 3: merge results back under lock.
		h.mu.Lock()
		// Replace h.msgCache with localCache. localCache started as a
		// shallow copy of h.msgCache (before Phase 2) plus any new entries
		// populated during snapshot rendering. Entries that AppendDelta
		// deleted during Phase 2 may be temporarily restored from the
		// Phase 1 snapshot copy. This is safe because AppendDelta also
		// sets dirty=true and firstDirtyIdx, ensuring the next Render
		// cycle rebuilds those messages with current text.
		h.msgCache = localCache
		h.cachedAll = rendered
		h.cachedMsgRanges = ranges
		// If no concurrent mutation set dirty=true during Phase 2,
		// clear the incremental tracking. Otherwise AppendDelta (or
		// another mutation) already set firstDirtyIdx and we keep it
		// — the next Render call triggered by their RequestRender
		// will process the fresh content.
		if !h.dirty {
			h.firstDirtyIdx = 0
		}

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
		all = h.cachedAll
	} else {
		all = h.cachedAll
	}

	// Refresh the tail anchor whenever the viewport is at the tail so that,
	// once the user scrolls up, tailAnchorLen freezes and Render can compute
	// how many new lines have arrived since.
	if h.follow {
		h.tailAnchorLen = int64(len(all))
	}
	newSinceAnchor := int64(0)
	if !h.follow && h.tailAnchorLen > 0 {
		newSinceAnchor = int64(len(all)) - h.tailAnchorLen
		if newSinceAnchor < 0 {
			newSinceAnchor = 0
		}
	}
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

// renderAllFromSnapshot renders the full transcript from a snapshot captured
// under h.mu. It writes new cache entries to localCache instead of h.msgCache,
// and returns the rendered lines + msgRanges so the caller can merge them back
// under the lock. This is the Phase 2 rendering path — it runs without h.mu.
func (h *ChatHistory) renderAllFromSnapshot(snap renderSnapshot, width int64, localCache map[string]cachedMessage) ([]string, []msgRange) {
	return h.renderAllWithState(snap.msgs, snap.theme, snap.expandedGroups, snap.selActive,
		snap.selStart, snap.selEnd, snap.reasoningRenderer, width, localCache,
		snap.firstDirtyIdx, snap.cachedAll, snap.cachedMsgRanges)
}

// renderAllWithState is the unified rendering core. It takes all mutable state
// as parameters so it can be called both from renderAll (with live h fields)
// and from renderAllFromSnapshot (with snapshot copies).
func (h *ChatHistory) renderAllWithState(
	msgs []ChatMessage,
	theme ChatHistoryTheme,
	expandedGroups map[int]bool,
	selActive bool,
	selStart, selEnd selectionPos,
	rr ReasoningRenderer,
	width int64,
	cache map[string]cachedMessage,
	firstDirtyIdx int,
	cachedAll []string,
	cachedMsgRanges []msgRange,
) ([]string, []msgRange) {
	// Temporarily swap the reasoning renderer so renderMessage (called from
	// renderMessageCachedWithCache) uses the snapshot value. Safe because
	// the event loop is single-threaded and AppendDelta never reads this.
	savedRR := h.reasoningRenderer
	h.reasoningRenderer = rr
	defer func() { h.reasoningRenderer = savedRR }()

	var out []string
	var ranges []msgRange

	if len(msgs) == 0 {
		// 品牌启动屏：引导用户开始对话或使用命令
		dim := theme.DimStyle
		accent := theme.UserStyle
		sys := theme.SystemStyle

		return []string{
			"",
			accent.Render("  Mady") + dim.Render(" — 证据驱动的专利案件工作台"),
			"",
			sys.Render("  输入消息开始对话，输入 / 查看可用命令"),
			dim.Render("  Ctrl+C 退出  ·  Ctrl+P 命令面板  ·  ? 帮助"),
			"",
		}, nil
	}

	// Streaming fast path: when only the tail of the message list changed
	// (the common AppendDelta case), splice the unchanged prefix from the
	// previous cachedAll instead of rebuilding from scratch. This turns
	// renderAll from O(N) into O(1) during streaming.
	if firstDirtyIdx > 0 && firstDirtyIdx < len(msgs) &&
		cachedAll != nil && len(cachedMsgRanges) > 0 {
		// Find the line where clean (unchanged) messages end in the
		// previous cachedAll. We walk cachedMsgRanges backwards from
		// firstDirtyIdx to find the boundary.
		cleanEnd := 0
		for _, r := range cachedMsgRanges {
			if r.msgIndex >= firstDirtyIdx {
				break
			}
			// For tool groups, ensure the entire group is clean
			if r.toolGroup && r.groupTo >= firstDirtyIdx {
				break
			}
			cleanEnd = r.endLine
		}
		if cleanEnd > 0 && cleanEnd <= len(cachedAll) {
			// Splice: keep clean prefix, re-render only dirty suffix.
			out := make([]string, 0, cleanEnd+len(cachedAll)-cleanEnd)
			out = append(out, cachedAll[:cleanEnd]...)
			var ranges []msgRange

			// Copy ranges for clean messages unchanged
			for _, r := range cachedMsgRanges {
				if r.msgIndex >= firstDirtyIdx {
					break
				}
				if r.toolGroup && r.groupTo >= firstDirtyIdx {
					break
				}
				ranges = append(ranges, r)
			}

			// Re-render only dirty messages
			out, ranges = h.renderMessagesRange(msgs, firstDirtyIdx, theme, expandedGroups, width, cache, out, ranges)

			// Apply selection highlight
			selEmpty := selStart.line == selEnd.line && selStart.col == selEnd.col
			if selActive && !selEmpty {
				h.applySelectionHighlightSnapshot(out, width, selStart, selEnd)
			}
			return out, ranges
		}
	}

	// Full rebuild path
	out, ranges = h.renderMessagesRange(msgs, 0, theme, expandedGroups, width, cache, out, ranges)

	// Apply selection highlight
	selEmpty := selStart.line == selEnd.line && selStart.col == selEnd.col
	if selActive && !selEmpty {
		h.applySelectionHighlightSnapshot(out, width, selStart, selEnd)
	}

	return out, ranges
}

// renderMessagesRange 从 start 开始渲染连续消息到 out/ranges。
// 快路径（start > 0，拼接）和慢路径（start = 0，全量）共用此函数。
// renderMessageSeparator 的 i > 0 条件对两路径均成立：快路径保证
// firstDirtyIdx > 0，慢路径从 0 开始自然跳过首次无前任消息。
func (h *ChatHistory) renderMessagesRange(
	msgs []ChatMessage, start int,
	theme ChatHistoryTheme, expandedGroups map[int]bool, width int64,
	cache map[string]cachedMessage,
	out []string, ranges []msgRange,
) ([]string, []msgRange) {
	for i := start; i < len(msgs); i++ {
		m := msgs[i]
		if groupEnd, ok := h.detectToolGroup(msgs, i); ok {
			lines, r := h.renderToolGroup(msgs, i, groupEnd, expandedGroups[i], theme, width, cache)
			out = append(out, lines...)
			ranges = append(ranges, r)
			i = groupEnd
			continue
		}
		if i > 0 {
			out = append(out, h.renderMessageSeparator(msgs[i-1], m, width, theme)...)
		}
		startLine := len(out)
		out = append(out, trimBlankEdges(h.renderMessageCachedWithCache(m, theme, width, cache))...)
		ranges = append(ranges, msgRange{startLine: startLine, endLine: len(out), msgIndex: i})
	}
	return out, ranges
}

// detectToolGroup 检查 msgs[i] 是否为一组连续工具/系统消息的起始。
// 如果是且不在中间轮次（mid-turn，Assistant 仍在 Pending 中），返回
// groupEnd（含）和 ok=true。快速路径和慢速路径共用此检测逻辑。
func (h *ChatHistory) detectToolGroup(msgs []ChatMessage, i int) (groupEnd int, ok bool) {
	if msgs[i].Role != RoleTool && msgs[i].Role != RoleSystem {
		return 0, false
	}
	end := i
	for j := i + 1; j < len(msgs); j++ {
		r := msgs[j].Role
		if r == RoleTool || r == RoleSystem {
			end = j
		} else {
			break
		}
	}
	// 单条工具消息不折叠
	if end == i {
		return 0, false
	}
	// 检查是否为中间轮次（消息在末尾且前一条 Assistant 消息仍在 Pending）
	if end == len(msgs)-1 {
		foundPrev := false
		for j := i - 1; j >= 0; j-- {
			if msgs[j].Role != RoleTool && msgs[j].Role != RoleSystem {
				if msgs[j].Pending {
					return 0, false // mid-turn，不折叠
				}
				foundPrev = true
				break
			}
		}
		// 没有前一条非工具消息（如 i==0 全部为工具消息），
		// 原始逻辑 midTurn 保持 true，不折叠
		if !foundPrev {
			return 0, false
		}
	}
	return end, true
}

// renderToolGroup 渲染一组连续的工具/系统消息为折叠（[+]）或展开（[-]）形式。
// 返回渲染后的行列表及对应的 msgRange。
func (h *ChatHistory) renderToolGroup(msgs []ChatMessage, start, end int, expanded bool, theme ChatHistoryTheme, width int64, cache map[string]cachedMessage) ([]string, msgRange) {
	bar := theme.ToolBorder.Render("▌ ")
	toolCount, sysCount := 0, 0
	for j := start; j <= end; j++ {
		if msgs[j].Role == RoleTool {
			toolCount++
		} else {
			sysCount++
		}
	}

	var lines []string
	if expanded {
		summary := fmt.Sprintf("[-] %d tools · %d msgs", toolCount, sysCount)
		if sysCount == 0 {
			summary = fmt.Sprintf("[-] %d tools", toolCount)
		}
		lines = append(lines, core.PadToWidth(bar+theme.DimStyle.Render(summary), width))
		for j := start; j <= end; j++ {
			lines = append(lines, trimBlankEdges(h.renderMessageCachedWithCache(msgs[j], theme, width, cache))...)
		}
	} else {
		summary := fmt.Sprintf("[+] %d tools · %d msgs", toolCount, sysCount)
		if sysCount == 0 {
			summary = fmt.Sprintf("[+] %d tools", toolCount)
		}
		for j := start; j <= end; j++ {
			if msgs[j].Meta != "" && msgs[j].Meta != "tool" {
				summary = "[+] " + msgs[j].Meta
				break
			}
		}
		lines = append(lines, bar+theme.DimStyle.Render(summary))
	}

	return lines, msgRange{
		startLine: 0, endLine: len(lines), msgIndex: start,
		toolGroup: true, groupFrom: start, groupTo: end,
	}
}

// renderMessageSeparator 在两条连续消息之间插入视觉分隔符。
// 返回空行列表（可能含分隔线），调用方直接 append 到输出 buffer。
func (h *ChatHistory) renderMessageSeparator(prev, curr ChatMessage, width int64, theme ChatHistoryTheme) []string {
	switch {
	case (prev.Role == RoleUser && curr.Role == RoleAssistant) ||
		(prev.Role == RoleAssistant && curr.Role == RoleUser):
		sep := theme.DimStyle.Render(strings.Repeat("─", int(width)))
		return []string{"", sep, ""}
	case prev.Role == RoleTool || curr.Role == RoleTool:
		return nil
	default:
		return []string{"", ""}
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
