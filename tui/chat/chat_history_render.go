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
	cachedLinks       [][]core.LinkSpan // 与 cachedAll 行对齐的链接元数据
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
	cl := h.cachedLinks
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
		cachedLinks:       cl,
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
	var allLinks [][]core.LinkSpan
	if needRender {
		h.dirty = false
		snap := h.captureSnapshot()
		localCache := make(map[string]cachedMessage, len(h.msgCache))
		for k, v := range h.msgCache {
			localCache[k] = v
		}

		rendered, ranges, links := h.renderAllFromSnapshot(snap, renderWidth, localCache)
		h.msgCache = localCache
		h.evictCacheEntriesLocked()
		h.cachedAll = rendered
		h.cachedMsgRanges = ranges
		h.cachedLinks = links
		h.firstDirtyIdx = 0
		allLinks = links

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
		allLinks = h.cachedLinks
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
		// No viewport clipping: still clamp every line to the render width.
		// This is the only path that previously skipped padToWidth — a buggy
		// upstream component or miscounted width could emit an over-width
		// line straight to a DECAWM-off terminal.
		for i, ln := range all {
			if vw := core.VisibleWidth(ln); vw > width {
				all[i] = core.TruncateToWidth(ln, width, "")
			}
		}
		h.lastLinks = allLinks
		return all
	}

	visible := h.extractViewport(all, maxRows, offset)
	// 链接元数据与可见行同步裁剪（extractViewport 的切片边界）。
	// allLinks 与 all 恒等长（renderAll 各路径按行同步产出元数据），
	// 直接切片、无需长度钳制；若未来不变式被破坏，越界 panic 会立即
	// 暴露，而不是静默错位。
	vEnd := int64(len(all)) - offset
	vStart := vEnd - maxRows
	if vStart < 0 {
		vStart = 0
		vEnd = maxRows
	}
	var visibleLinks [][]core.LinkSpan
	if allLinks != nil {
		visibleLinks = allLinks[vStart:vEnd]
	}
	visible, visibleLinks = h.addScrollIndicator(visible, all, maxRows, offset, follow, visibleLinks)
	visible, visibleLinks = h.addStickToBottomHint(visible, all, maxRows, follow, newSinceAnchor, visibleLinks)
	visible = h.drawScrollbar(visible, width, all, maxRows, follow)
	visible = h.padToWidth(visible, width, maxRows, sbNow, h.sbWidth)
	h.lastLinks = visibleLinks
	return visible
}

// RenderLinks 实现 core.LinkProvider：返回最近一次 Render 输出的可见行
// 对应的链接元数据（与渲染行一一对应，无链接行为 nil）。
//
// 与 Render 在事件循环同一线程串行调用（引擎先 Render 后 RenderLinks），
// 无锁安全。宽度变化帧的链接可能来自上一宽度——引擎在行数不匹配时
// 忽略（见 tui/tui_render.go 的 j < len(links) 保护），下一帧自动恢复。
func (h *ChatHistory) RenderLinks(width int64) [][]core.LinkSpan {
	return h.lastLinks
}

// computeRenderWidth determines the content width accounting for scrollbar.
//
// 方案1：滚动条列恒定预留。滚动条启用时内容渲染宽度恒为 width-sbWidth-gutter，
// 其中 gutter=1 是内容与滚动条之间的最小内边距。不随滚动条实际显隐而变。
// 修复前：流式输出期间内容从"可容纳"变为"溢出"时，滚动条出现/消失让 renderWidth
// 在 width 与 width-1 之间切换；Markdown 段落按宽度换行，宽度突变会让已渲染行的
// 换行点全部错位——表现为"输出时排版混乱、重开 TUI 后正常"。
// 修复后 cachedWidth 恒定，不再因滚动条切换触发缓存失效与全量重排。
func (h *ChatHistory) computeRenderWidth(width int64) (renderWidth int64, sbNow bool) {
	sbEnabled := h.sbEnabled && h.sbWidth > 0
	// sbNow 仅表示本帧是否实际绘制滚动条轨道（内容超出视口时）。
	sbNow = sbEnabled && h.cachedAll != nil && h.maxRows > 0 && int64(len(h.cachedAll)) > h.maxRows
	renderWidth = width
	if sbEnabled {
		// 预留滚动条列 + 1 列内边距，避免文字紧贴/侵入滚动条区域。
		renderWidth = width - h.sbWidth - 1
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
// links 与 visible 行同步裁剪（indicator 行无链接）。
func (h *ChatHistory) addScrollIndicator(visible []string, all []string, maxRows int64, offset int64, follow bool, links [][]core.LinkSpan) ([]string, [][]core.LinkSpan) {
	if follow {
		return visible, links
	}
	visibleEnd := int64(len(all)) - offset
	if visibleEnd >= int64(len(all)) {
		return visible, links
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
		if len(links) > 0 {
			links = links[:len(links)-1]
		}
	}
	// indicator 行无链接；links 为空（无链接）时保持 nil，不为空对话
	// 制造长度 1 的元数据（否则 lastLinks 与可见行不等长）。
	if len(links) > 0 {
		links = append([][]core.LinkSpan{nil}, links...)
	}
	return append([]string{indicator}, visible...), links
}

// addStickToBottomHint appends a "N new" hint when not following and new content arrived.
// links 与 visible 行同步追加（hint 行无链接）。
func (h *ChatHistory) addStickToBottomHint(visible []string, all []string, maxRows int64, follow bool, newSinceAnchor int64, links [][]core.LinkSpan) ([]string, [][]core.LinkSpan) {
	if follow || newSinceAnchor <= 0 {
		return visible, links
	}
	hint := h.theme.SuccessStyle.Render(fmt.Sprintf("\u2193 %d new \u2014 End to follow", newSinceAnchor))
	if int64(len(visible)) >= maxRows && len(visible) > 0 {
		visible = visible[:len(visible)-1]
		if len(links) > 0 {
			links = links[:len(links)-1]
		}
	}
	// hint 行无链接；links 为空时保持 nil（与 addScrollIndicator 同规则）。
	if len(links) > 0 {
		links = append(links, nil)
	}
	return append(visible, hint), links
}

// drawScrollbar renders a scrollbar on the right edge when content overflows,
// leaving a 1-column gutter between content and the scrollbar track so text
// never touches or underlaps the track.
func (h *ChatHistory) drawScrollbar(visible []string, width int64, all []string, maxRows int64, follow bool) []string {
	if !h.sbEnabled || h.sbWidth <= 0 || int64(len(all)) <= maxRows {
		return visible
	}
	contentWidth := width - h.sbWidth - 1
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
	gapStyle := pal.SurfaceBg.Render(" ") // 1-column gutter
	trackStyle := pal.SurfaceBg.Render(" ")
	thumbStyle := pal.SurfaceRaisedBg.Render(" ")
	if !follow {
		thumbStyle = pal.SurfaceBg.Render(" ")
	}

	for i := int64(0); i < int64(len(visible)); i++ {
		ln := visible[i]
		// Defensive clamp: never let a miscounted-width line overflow the
		// content area. This is the last safety net before DECAWM-off output.
		if core.VisibleWidth(ln) > contentWidth {
			ln = core.TruncateToWidth(ln, contentWidth, "")
		}
		ln = core.PadToWidth(ln, contentWidth)
		if i >= thumbOff && i < thumbEnd {
			visible[i] = ln + gapStyle + thumbStyle
		} else {
			visible[i] = ln + gapStyle + trackStyle
		}
	}
	return visible
}

// padToWidth pads every line to the target width so the TUI diff engine never
// leaves a partial column. When the scrollbar is enabled but not drawn this
// frame, we keep the 1-column right gutter for visual consistency.
// Skipped when the scrollbar track was drawn this frame (sbNow): drawScrollbar
// has already padded lines to contentWidth and appended gutter + track.
func (h *ChatHistory) padToWidth(visible []string, width int64, maxRows int64, sbNow bool, sbWidth int64) []string {
	if sbNow {
		return visible
	}
	target := width
	if sbWidth > 0 {
		// Keep the same right gutter used by drawScrollbar.
		target = width - 1
		if target < 1 {
			target = 1
		}
	}
	for i, ln := range visible {
		vw := core.VisibleWidth(ln)
		if vw > target {
			// Defensive clamp: a buggy component or miscounted width must not
			// emit an over-width line to a DECAWM-off terminal.
			visible[i] = core.TruncateToWidth(ln, target, "")
		} else if vw < target {
			visible[i] = core.PadToWidth(ln, target)
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
// and returns the rendered lines + msgRanges (+ links) so the caller can merge
// them back under the lock. This is the Phase 2 rendering path — it runs
// without h.mu.
func (h *ChatHistory) renderAllFromSnapshot(snap renderSnapshot, width int64, localCache map[string]cachedMessage) ([]string, []msgRange, [][]core.LinkSpan) {
	return h.renderAllWithState(snap, width, localCache)
}

// renderAllWithState is the unified rendering core. It renders from the
// snapshot struct directly (the same one renderAllFromSnapshot receives),
// so callers never unpack fields individually.
func (h *ChatHistory) renderAllWithState(snap renderSnapshot, width int64, cache map[string]cachedMessage) ([]string, []msgRange, [][]core.LinkSpan) {
	// 解包快照字段为局部变量；函数体其余部分与旧签名一致。
	msgs := snap.msgs
	theme := snap.theme
	expandedGroups := snap.expandedGroups
	selActive := snap.selActive
	selStart, selEnd := snap.selStart, snap.selEnd
	rr := snap.reasoningRenderer
	firstDirtyIdx := snap.firstDirtyIdx
	cachedAll := snap.cachedAll
	cachedMsgRanges := snap.cachedMsgRanges
	cachedLinks := snap.cachedLinks

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
		}, nil, nil
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
		renderFrom := firstDirtyIdx
		for _, r := range cachedMsgRanges {
			if r.msgIndex >= firstDirtyIdx {
				break
			}
			// For tool groups, ensure the entire group is clean
			if r.toolGroup && r.groupTo >= firstDirtyIdx {
				// P1-11: firstDirtyIdx lands INSIDE a rendered tool group.
				// The group's earlier members exist only in the cached
				// prefix; starting the re-render at firstDirtyIdx would
				// drop them from both the prefix AND the new render (a
				// visible tool-card loss). Clamp the render start back to
				// the group's first message so the whole group is
				// re-rendered together.
				if r.msgIndex < renderFrom {
					renderFrom = r.msgIndex
				}
				break
			}
			cleanEnd = r.endLine
		}
		if cleanEnd > 0 && cleanEnd <= len(cachedAll) {
			// Splice: keep clean prefix, re-render only dirty suffix.
			out := make([]string, 0, cleanEnd+len(cachedAll)-cleanEnd)
			out = append(out, cachedAll[:cleanEnd]...)
			var ranges []msgRange
			// 前缀链接复用：与 cachedAll 前缀行一一对应。
			var outLinks [][]core.LinkSpan
			if cachedLinks != nil && cleanEnd <= len(cachedLinks) {
				outLinks = append(outLinks, cachedLinks[:cleanEnd]...)
			}

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

			// Re-render only dirty messages (from renderFrom, which may be
			// earlier than firstDirtyIdx when a tool group was affected).
			out, ranges, tailLinks := h.renderMessagesRange(msgs, renderFrom, theme, expandedGroups, width, cache, out, ranges)
			outLinks = append(outLinks, tailLinks...)

			// Apply selection highlight
			selEmpty := selStart.line == selEnd.line && selStart.col == selEnd.col
			if selActive && !selEmpty {
				h.applySelectionHighlightSnapshot(out, width, selStart, selEnd)
			}
			return out, ranges, outLinks
		}
	}

	// Full rebuild path
	out, ranges, outLinks := h.renderMessagesRange(msgs, 0, theme, expandedGroups, width, cache, out, ranges)

	// Apply selection highlight
	selEmpty := selStart.line == selEnd.line && selStart.col == selEnd.col
	if selActive && !selEmpty {
		h.applySelectionHighlightSnapshot(out, width, selStart, selEnd)
	}

	return out, ranges, outLinks
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
) ([]string, []msgRange, [][]core.LinkSpan) {
	var outLinks [][]core.LinkSpan
	for i := start; i < len(msgs); i++ {
		m := msgs[i]
		if groupEnd, ok := h.detectToolGroup(msgs, i); ok {
			lines, r, links := h.renderToolGroup(msgs, i, groupEnd, expandedGroups[i], theme, width, cache)
			out = append(out, lines...)
			outLinks = append(outLinks, links...)
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
			sep := h.renderMessageSeparator(msgs[i-1], m, width, theme)
			out = append(out, sep...)
			// 分隔行无链接；按实际行数补齐，保持 outLinks 与 out 等长
			// （renderMessageSeparator 对 Tool↔Tool 返回 nil，无行）。
			if len(sep) > 0 {
				outLinks = append(outLinks, nilLinks(len(sep))...)
			}
		}
		startLine := len(out)
		msgLines, msgLinks := h.renderMessageCachedWithCache(m, theme, width, cache)
		trimmed, _ := trimBlankEdges(msgLines)
		out = append(out, trimmed...)
		if msgLinks != nil {
			outLinks = append(outLinks, msgLinks...)
		} else if len(trimmed) > 0 {
			outLinks = append(outLinks, nilLinks(len(trimmed))...)
		}
		ranges = append(ranges, msgRange{startLine: startLine, endLine: len(out), msgIndex: i})
	}
	return out, ranges, outLinks
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
// 返回渲染行、行区间与链接元数据（展开时成员行加色带前缀，链接列偏移）。
func (h *ChatHistory) renderToolGroup(msgs []ChatMessage, start, end int, expanded bool, theme ChatHistoryTheme, width int64, cache map[string]cachedMessage) ([]string, msgRange, [][]core.LinkSpan) {
	toolCount, sysCount := 0, 0
	for j := start; j <= end; j++ {
		if msgs[j].Role == RoleTool {
			toolCount++
		} else {
			sysCount++
		}
	}

	var lines []string
	var links [][]core.LinkSpan
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
	links = append(links, nil)
	if expanded {
		// 左侧 2 列色带 + 内容，使组内成员连成一体。
		barStyled := theme.DimStyle.Render("│")
		prefix := "  " + barStyled + " "
		prefixW := core.VisibleWidth(prefix)
		innerW := width - prefixW
		if innerW < 1 {
			innerW = 1
		}
		for j := start; j <= end; j++ {
			memberLines, memberLinks := h.renderMessageCachedWithCache(msgs[j], theme, innerW, cache)
			trimmed, _ := trimBlankEdges(memberLines)
			for k, ln := range trimmed {
				lines = append(lines, prefix+ln)
				if memberLinks != nil && k < len(memberLinks) && memberLinks[k] != nil {
					// 成员行加色带前缀：链接列区间整体右移 prefixW。
					shifted := make([]core.LinkSpan, len(memberLinks[k]))
					for si, ls := range memberLinks[k] {
						shifted[si] = core.LinkSpan{StartCol: ls.StartCol + prefixW, EndCol: ls.EndCol + prefixW, URL: ls.URL}
					}
					links = append(links, shifted)
				} else {
					links = append(links, nil)
				}
			}
			if j < end {
				// 组成员之间的细色带连接，比空行更紧凑。
				lines = append(lines, "  "+barStyled)
				links = append(links, nil)
			}
		}
	}

	return lines, msgRange{
		startLine: 0, endLine: len(lines), msgIndex: start,
		toolGroup: true, groupFrom: start, groupTo: end,
	}, links
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
//
// 返回裁剪后的行与起始偏移（供链接元数据同步裁剪）。
func trimBlankEdges(lines []string) ([]string, int) {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(core.StripAnsi(lines[start])) == "" {
		start++
	}
	for end > start && strings.TrimSpace(core.StripAnsi(lines[end-1])) == "" {
		end--
	}
	return lines[start:end], start
}
