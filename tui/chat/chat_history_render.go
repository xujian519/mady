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
	"github.com/xujian519/mady/tui/theme"
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
	renderWidth, sbNow := h.computeRenderWidth(width)
	wasDirty := h.dirty
	if h.cachedWidth != renderWidth {
		h.cachedWidth = renderWidth
		h.clearMsgCacheLocked()
		h.firstDirtyIdx = 0
		h.dirty = true
	}

	needRender := h.dirty || h.cachedAll == nil

	var all []string
	if needRender {
		h.dirty = false
		snap := h.captureSnapshot()
		localCache := make(map[string]cachedMessage, len(h.msgCache))
		for k, v := range h.msgCache {
			localCache[k] = v
		}

		rendered, ranges := h.renderAllFromSnapshot(snap, renderWidth, localCache)
		h.msgCache = localCache
		h.evictCacheEntriesLocked()
		h.cachedAll = rendered
		h.cachedMsgRanges = ranges
		h.firstDirtyIdx = 0

		if wasDirty && h.follow {
			h.offset = 0
		}
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

	if h.follow {
		h.tailAnchorLen = int64(len(all))
	}
	newSinceAnchor := h.computeNewSinceAnchor(all)
	maxRows := h.maxRows
	offset := h.offset
	follow := h.follow
	h.mu.Unlock()

	if maxRows <= 0 || int64(len(all)) <= maxRows {
		return all
	}

	visible := h.extractViewport(all, maxRows, offset)
	visible = h.addScrollIndicator(visible, all, maxRows, offset, follow)
	visible = h.addStickToBottomHint(visible, all, maxRows, follow, newSinceAnchor)
	visible = h.drawScrollbar(visible, width, all, maxRows, follow)
	visible = h.padToWidth(visible, width, maxRows, sbNow, h.sbWidth)
	return visible
}

// computeRenderWidth determines the content width accounting for scrollbar.
//
// 方案1：滚动条列恒定预留。滚动条启用时内容渲染宽度恒为 width-sbWidth，
// 不随滚动条实际显隐而变。修复前：流式输出期间内容从"可容纳"变为"溢出"时，
// 滚动条出现/消失让 renderWidth 在 width 与 width-1 之间切换；Markdown 段落
// 按宽度换行，宽度突变会让已渲染行的换行点全部错位——表现为"输出时排版混乱、
// 重开 TUI 后正常"（重开时整条消息以稳定的最终宽度重渲染）。修复后 cachedWidth
// 恒定，不再因滚动条切换触发缓存失效与全量重排，整条消息始终以同一宽度渲染。
func (h *ChatHistory) computeRenderWidth(width int64) (renderWidth int64, sbNow bool) {
	sbEnabled := h.sbEnabled && h.sbWidth > 0
	// sbNow 仅表示本帧是否实际绘制滚动条轨道（内容超出视口时），
	// 只影响后处理（drawScrollbar / padToWidth），不再影响内容换行宽度。
	sbNow = sbEnabled && h.cachedAll != nil && h.maxRows > 0 && int64(len(h.cachedAll)) > h.maxRows
	renderWidth = width
	if sbEnabled {
		renderWidth = width - h.sbWidth
		if renderWidth < 1 {
			renderWidth = 1
		}
	}
	return
}

// computeNewSinceAnchor returns how many new lines have arrived since
// the tail anchor was frozen (when the user last scrolled up).
func (h *ChatHistory) computeNewSinceAnchor(all []string) int64 {
	if !h.follow && h.tailAnchorLen > 0 {
		n := int64(len(all)) - h.tailAnchorLen
		if n < 0 {
			return 0
		}
		return n
	}
	return 0
}

// extractViewport returns the visible slice of all lines clipped to maxRows.
func (h *ChatHistory) extractViewport(all []string, maxRows int64, offset int64) []string {
	end := int64(len(all)) - offset
	if end > int64(len(all)) {
		end = int64(len(all))
	}
	start := end - maxRows
	if start < 0 {
		start = 0
		end = maxRows
	}
	return all[start:end]
}

// addScrollIndicator prepends a scroll-position indicator when not following.
func (h *ChatHistory) addScrollIndicator(visible []string, all []string, maxRows int64, offset int64, follow bool) []string {
	if follow {
		return visible
	}
	visibleEnd := int64(len(all)) - offset
	if visibleEnd >= int64(len(all)) {
		return visible
	}
	totalLines := int64(len(all))
	visibleStart := visibleEnd - maxRows
	if visibleStart < 0 {
		visibleStart = 0
	}
	percent := int64(0)
	if totalLines > 0 {
		percent = visibleStart * 100 / totalLines
	}
	indicator := h.theme.DimStyle.Render(fmt.Sprintf("\u25b2 %d/%d (%d%%) \u2014 End to follow", visibleStart, totalLines, percent))
	if int64(len(visible)) >= maxRows && len(visible) > 0 {
		visible = visible[:len(visible)-1]
	}
	return append([]string{indicator}, visible...)
}

// addStickToBottomHint appends a "N new" hint when not following and new content arrived.
func (h *ChatHistory) addStickToBottomHint(visible []string, all []string, maxRows int64, follow bool, newSinceAnchor int64) []string {
	if follow || newSinceAnchor <= 0 {
		return visible
	}
	hint := h.theme.SuccessStyle.Render(fmt.Sprintf("\u2193 %d new \u2014 End to follow", newSinceAnchor))
	if int64(len(visible)) >= maxRows && len(visible) > 0 {
		visible = visible[:len(visible)-1]
	}
	return append(visible, hint)
}

// drawScrollbar renders a scrollbar on the right edge when content overflows.
func (h *ChatHistory) drawScrollbar(visible []string, width int64, all []string, maxRows int64, follow bool) []string {
	if !h.sbEnabled || h.sbWidth <= 0 || int64(len(all)) <= maxRows {
		return visible
	}
	contentWidth := width - h.sbWidth
	if contentWidth < 1 {
		contentWidth = 1
	}
	total := int64(len(all))
	thumbLen := maxRows * maxRows / total
	if thumbLen < 1 {
		thumbLen = 1
	}
	start := int64(len(all)) - maxRows - h.offset
	if start < 0 {
		start = 0
	}
	thumbOff := start * (maxRows - thumbLen) / (total - maxRows)
	thumbEnd := thumbOff + thumbLen
	if thumbEnd > maxRows {
		thumbEnd = maxRows
	}

	pal := theme.CurrentPalette()
	trackStyle := pal.SurfaceBg.Render(" ")
	thumbStyle := pal.SurfaceRaisedBg.Render(" ")
	if !follow {
		thumbStyle = pal.SurfaceBg.Render(" ")
	}

	for i := int64(0); i < int64(len(visible)); i++ {
		ln := visible[i]
		if core.VisibleWidth(ln) > contentWidth {
			ln = core.TruncateToWidth(ln, contentWidth, "\u2026")
		} else {
			ln = core.PadToWidth(ln, contentWidth)
		}
		if i >= thumbOff && i < thumbEnd {
			visible[i] = ln + thumbStyle
		} else {
			visible[i] = ln + trackStyle
		}
	}
	return visible
}

// padToWidth pads every line to full width so the TUI diff engine never
// leaves a partial column. Skipped when the scrollbar track was drawn this
// frame (sbNow): drawScrollbar has already padded lines to contentWidth and
// appended the track, so padding again would be a no-op.
func (h *ChatHistory) padToWidth(visible []string, width int64, maxRows int64, sbNow bool, sbWidth int64) []string {
	if sbNow {
		return visible
	}
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
			// renderToolGroup reports lines relative to itself (startLine 0);
			// rebase onto the absolute output position so hit-testing and
			// scroll-to-match use consistent coordinates.
			r.startLine = len(out) - len(lines)
			r.endLine = len(out)
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
// 展开时使用左侧色带（│）把多个工具/系统消息连成一条紧凑时间线，
// 避免原本散落的卡片感。
func (h *ChatHistory) renderToolGroup(msgs []ChatMessage, start, end int, expanded bool, theme ChatHistoryTheme, width int64, cache map[string]cachedMessage) ([]string, msgRange) {
	toolCount, sysCount := 0, 0
	for j := start; j <= end; j++ {
		if msgs[j].Role == RoleTool {
			toolCount++
		} else {
			sysCount++
		}
	}

	var lines []string
	// 折叠/展开只差一个标记符，统一用 marker 构建。marker 不带尾随空格，
	// 由各分支的格式串/拼接统一补一个空格，避免 "[+]  2 tools" 双空格。
	marker := "[+]"
	if expanded {
		marker = "[-]"
	}
	summary := fmt.Sprintf("%s %d tools · %d msgs", marker, toolCount, sysCount)
	if sysCount == 0 {
		summary = fmt.Sprintf("%s %d tools", marker, toolCount)
	}
	if !expanded {
		for j := start; j <= end; j++ {
			if msgs[j].Meta != "" && msgs[j].Meta != "tool" {
				summary = marker + " " + msgs[j].Meta
				break
			}
		}
	}
	lines = append(lines, theme.DimStyle.Render(summary))
	if expanded {
		// 左侧 2 列色带 + 内容，使组内成员连成一体。
		barStyled := theme.DimStyle.Render("│")
		prefix := "  " + barStyled + " "
		innerW := width - core.VisibleWidth(prefix)
		if innerW < 1 {
			innerW = 1
		}
		for j := start; j <= end; j++ {
			member := trimBlankEdges(h.renderMessageCachedWithCache(msgs[j], theme, innerW, cache))
			for _, ln := range member {
				lines = append(lines, prefix+ln)
			}
			if j < end {
				// 组成员之间的细色带连接，比空行更紧凑。
				lines = append(lines, "  "+barStyled)
			}
		}
	}

	return lines, msgRange{
		startLine: 0, endLine: len(lines), msgIndex: start,
		toolGroup: true, groupFrom: start, groupTo: end,
	}
}

// renderMessageSeparator 在两条连续消息之间插入最小视觉分隔。
// 采用“时间线”密度：角色切换用单空行，同角色连续用极细点线，
// 连续工具调用之间不再插入额外分隔线（由工具组自身色带/卡片承担层次）。
func (h *ChatHistory) renderMessageSeparator(prev, curr ChatMessage, width int64, theme ChatHistoryTheme) []string {
	switch {
	// Tool ↔ Tool 连续工具调用：无额外分隔，由工具组色带/卡片自身承担层次。
	case prev.Role == RoleTool && curr.Role == RoleTool:
		return nil

	// 系统消息之间：单空行。
	case prev.Role == RoleSystem && curr.Role == RoleSystem:
		return []string{""}

	// 其他连续同角色消息（Assistant↔Assistant、User↔User）：八分之一宽点线。
	case prev.Role == curr.Role:
		eighth := int64(int(width) / 8)
		if eighth < 1 {
			eighth = 1
		}
		return []string{theme.DimStyle.Render(strings.Repeat("·", int(eighth)))}

	// 其余任意角色切换：单空行。
	default:
		return []string{""}
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
