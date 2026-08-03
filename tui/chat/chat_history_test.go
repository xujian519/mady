package chat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestChatHistoryAppendAndRender(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "hello"})
	h.Append(ChatMessage{Role: RoleAssistant, Text: "world"})

	lines := h.Render(40)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "hello") {
		t.Fatalf("user message missing: %q", joined)
	}
	if !strings.Contains(joined, "world") {
		t.Fatalf("assistant message missing: %q", joined)
	}
}

func TestChatHistoryAppendDelta(t *testing.T) {
	h := NewChatHistory()
	id := h.AppendDelta("", "Hello, ")
	if id == "" {
		t.Fatalf("no id returned")
	}
	h.AppendDelta(id, "world!")
	msgs := h.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 msg, got %d", len(msgs))
	}
	if msgs[0].Text != "Hello, world!" {
		t.Fatalf("text=%q", msgs[0].Text)
	}
	if !msgs[0].Pending {
		t.Fatalf("expected pending")
	}
	h.Finalize(id)
	msgs = h.Messages()
	if msgs[0].Pending {
		t.Fatalf("finalize should clear pending")
	}
}

func TestChatHistoryViewportClipping(t *testing.T) {
	h := NewChatHistory()
	for i := 0; i < 20; i++ {
		h.Append(ChatMessage{Role: RoleSystem, Text: "line"})
	}
	h.SetMaxRows(5)
	lines := h.Render(20)
	if int64(len(lines)) != 5 {
		t.Fatalf("viewport should clip to 5 rows; got %d", len(lines))
	}
}

// TestChatHistoryScrollbarNoEllipsisTruncation 是 scrollbar 宽度错配的回归测试。
// 修复前：Markdown 按全宽 width 换行，但 scrollbar 占 1 列后视口可用宽度为
// width-1，导致每行末尾被截断为省略号（长文本尤甚）。
// 修复后：渲染预留 sbWidth，行宽不再超出可用宽度。
func TestChatHistoryScrollbarNoEllipsisTruncation(t *testing.T) {
	h := NewChatHistory() // 默认 sbEnabled=true, sbWidth=1

	// 构造超过 maxRows 的长中文消息，确保触发 scrollbar 截断分支。
	longText := strings.Repeat("中文测试内容", 20) // 120 个汉字
	h.Append(ChatMessage{Role: RoleAssistant, Text: longText})
	h.SetMaxRows(3)

	lines := h.Render(20)
	if int64(len(lines)) != 3 {
		t.Fatalf("expected 3 visible rows, got %d", len(lines))
	}
	for i, ln := range lines {
		if strings.Contains(ln, "…") {
			t.Errorf("line %d truncated with ellipsis (scrollbar width mismatch): %q", i, ln)
		}
	}
}

// TestChatHistoryScrollbarTransition 是 scrollbar 显隐切换的回归测试。
// 修复前：内容从未超出 maxRows 变为超出时，cachedAll 在 width 宽度下渲染，
// 但 scrollbar 后处理截断到 contentWidth=width-sbWidth，导致每行末尾出现 "…"。
// 修复后：renderWidth 根据实际 scrollbar 显隐动态决定，切换时 cachedWidth
// 自然触发缓存失效，行宽始终匹配。
func TestChatHistoryScrollbarTransition(t *testing.T) {
	h := NewChatHistory() // 默认 sbEnabled=true, sbWidth=1
	h.SetMaxRows(3)

	// 第一阶段：内容可容纳，无 scrollbar
	h.Append(ChatMessage{Role: RoleAssistant, Text: "短文本"})
	lines1 := h.Render(20)
	for i, ln := range lines1 {
		if strings.Contains(ln, "…") {
			t.Errorf("phase 1 line %d should not be truncated: %q", i, ln)
		}
	}

	// 第二阶段：追加长文本，内容超出 maxRows → scrollbar 出现
	longText := strings.Repeat("中文测试内容", 20)
	h.Append(ChatMessage{Role: RoleAssistant, Text: longText})
	lines2 := h.Render(20)
	if int64(len(lines2)) != 3 {
		t.Fatalf("expected 3 visible rows after transition, got %d", len(lines2))
	}
	for i, ln := range lines2 {
		if strings.Contains(ln, "…") {
			t.Errorf("phase 2 line %d truncated after scrollbar appears: %q", i, ln)
		}
	}
}

// TestChatHistoryReservedGutterWidthStable 是方案1（滚动条列恒定预留）的回归测试。
// 修复前：流式输出期间内容从"可容纳"变为"溢出"时，renderWidth 在 width 与
// width-1 之间切换，Markdown 换行点全部错位（输出时排版混乱、重开 TUI 后正常）。
// 修复后：滚动条启用时内容渲染宽度恒为 width-sbWidth-gutter（gutter=1），
// cachedWidth 不随滚动条显隐变化，整条消息始终以同一宽度渲染。
func TestChatHistoryReservedGutterWidthStable(t *testing.T) {
	h := NewChatHistory() // 默认 sbEnabled=true, sbWidth=1
	h.SetMaxRows(3)
	const width = int64(20)
	wantWidth := width - h.sbWidth - 1 // 额外 1 列内容-滚动条内边距

	// 第一阶段：内容可容纳，无滚动条轨道。渲染宽度必须已预留滚动条列+内边距。
	h.Append(ChatMessage{Role: RoleAssistant, Text: "短文本"})
	_ = h.Render(width)
	if w := h.cachedWidth; w != wantWidth {
		t.Fatalf("phase 1 cachedWidth = %d, want %d (width - sbWidth - gutter)", w, wantWidth)
	}

	// 第二阶段：内容溢出 → 滚动条出现，渲染宽度不得变化。
	h.Append(ChatMessage{Role: RoleAssistant, Text: strings.Repeat("中文测试内容", 20)})
	_ = h.Render(width)
	if w := h.cachedWidth; w != wantWidth {
		t.Fatalf("phase 2 cachedWidth = %d, want %d (width must be stable)", w, wantWidth)
	}

	// 第三阶段：模拟流式 delta 继续到达，宽度仍然稳定。
	h.Append(ChatMessage{Role: RoleAssistant, Text: "更多内容"})
	_ = h.Render(width)
	if w := h.cachedWidth; w != wantWidth {
		t.Fatalf("phase 3 cachedWidth = %d, want %d (width must be stable)", w, wantWidth)
	}
}

// TestChatHistoryCJKReplyNoDroppedRunes 是 CJK 换行丢字的端到端回归测试。
// 修复前：findBreakColumn 在 2 列宽汉字放不下时返回裸 width（落在字符中间），
// SliceByColumn 随后把跨边界汉字整个丢弃——长中文回复每行末尾都丢字，
// 表现为"输出总是被截断"。修复后 wrap 断点始终落在完整字形边界。
func TestChatHistoryCJKReplyNoDroppedRunes(t *testing.T) {
	h := NewChatHistory()
	reply := strings.Join([]string{
		"根据专利法第26条第3款的规定，权利要求书应当以说明书为依据，清楚、简要地限定要求专利保护的范围。",
		"",
		"本发明的技术方案包括以下步骤：",
		"1. 获取用户输入的技术特征；",
		"2. 基于所述技术特征构建检索式；",
		"3. 将检索结果与权利要求进行要素级比对。",
		"",
		"**有益效果**：通过上述方案，能够提高检索准确率，`claim 1` 的技术特征得到完整覆盖。",
	}, "\n")
	h.Append(ChatMessage{Role: RoleAssistant, Text: reply})
	h.SetMaxRows(30)

	width := int64(60)
	lines := h.Render(width)
	joined := strings.Join(lines, "\n")

	// 所有渲染行不得超过视口宽度（scrollbar 占 1 列，内容宽度实际为 width-1）。
	for i, ln := range lines {
		if w := core.VisibleWidth(ln); w > width {
			t.Errorf("line %d width %d > %d: %q", i, w, width, ln)
		}
	}
	// 段落不得被截断成省略号。
	if strings.Contains(core.StripAnsi(joined), "…") {
		t.Errorf("unexpected ellipsis truncation:\n%s", joined)
	}
	// 关键内容必须完整出现（markdown 标记会被样式替换，仅校验纯文本片段）。
	for _, frag := range []string{
		"根据专利法第26条第3款的规定",
		"获取用户输入的技术特征",
		"构建检索式",
		"要素级比对",
		"提高检索准确率",
		"claim 1",
	} {
		if !strings.Contains(core.StripAnsi(joined), frag) {
			t.Errorf("reply fragment missing: %q\nframe:\n%s", frag, joined)
		}
	}
}

func TestChatHistoryScroll(t *testing.T) {
	h := NewChatHistory()
	for i := 0; i < 30; i++ {
		h.Append(ChatMessage{Role: RoleSystem, Text: "row"})
	}
	h.SetMaxRows(5)
	_ = h.Render(20)
	h.ScrollBy(10)
	if h.follow {
		t.Fatalf("scroll-up should stop following tail")
	}
	h.FollowTail()
	if !h.follow {
		t.Fatalf("FollowTail should re-enable following")
	}
}

func TestSelectionHighlightKeepsVisibleWidthStable(t *testing.T) {
	h := NewChatHistory()
	h.maxRows = 10
	h.selActive = true
	h.selStart = selectionPos{line: 0, col: 1}

	origLine := "\x1b[38;5;245m▌\x1b[0m assistant: hello world"
	origWidth := core.VisibleWidth(origLine)

	for endCol := int64(1); endCol <= origWidth; endCol++ {
		h.selEnd = selectionPos{line: 0, col: endCol}
		lines := []string{origLine}
		h.applySelectionHighlightLocked(lines, 120)
		gotWidth := core.VisibleWidth(lines[0])
		if gotWidth != origWidth {
			t.Fatalf("visible width changed at endCol=%d: got=%d want=%d", endCol, gotWidth, origWidth)
		}
	}
}

func TestSelectionHighlightWidthStableOnCJKAndEmoji(t *testing.T) {
	h := NewChatHistory()
	h.maxRows = 10
	h.selActive = true
	h.selStart = selectionPos{line: 0, col: 0}

	line := "中🙂文 abc"
	origWidth := core.VisibleWidth(line)

	for endCol := int64(0); endCol <= origWidth; endCol++ {
		h.selEnd = selectionPos{line: 0, col: endCol}
		lines := []string{line}
		h.applySelectionHighlightLocked(lines, 120)
		gotWidth := core.VisibleWidth(lines[0])
		if gotWidth != origWidth {
			t.Fatalf("cjk/emoji width changed at endCol=%d: got=%d want=%d", endCol, gotWidth, origWidth)
		}
	}
}

func TestSelectionHighlightWidthStableWhenBoundaryMovesBackAndForth(t *testing.T) {
	h := NewChatHistory()
	h.maxRows = 10
	h.selActive = true
	h.selStart = selectionPos{line: 0, col: 0}

	line := "\x1b[38;5;245m彩色\x1b[0m mixed 中🙂 text"
	origWidth := core.VisibleWidth(line)

	sequence := []int64{0, 2, 5, 9, 6, 3, 8, 1, origWidth, 0, 4, 7, 2}
	for _, endCol := range sequence {
		h.selEnd = selectionPos{line: 0, col: endCol}
		lines := []string{line}
		h.applySelectionHighlightLocked(lines, 120)
		gotWidth := core.VisibleWidth(lines[0])
		if gotWidth != origWidth {
			t.Fatalf("boundary move changed width at endCol=%d: got=%d want=%d", endCol, gotWidth, origWidth)
		}
	}
}

func TestMapMouseColToVisibleColSnapsWideContinuation(t *testing.T) {
	h := NewChatHistory()
	h.cachedAll = []string{"中a"}

	if got := h.mapMouseColToVisibleColLocked(0, 1); got != 0 {
		t.Fatalf("continuation col should snap to wide rune start: got=%d want=0", got)
	}
}

func TestSelectionHighlightUsesUniformStyleOverStyledText(t *testing.T) {
	h := NewChatHistory()
	h.maxRows = 10
	h.selActive = true
	h.selStart = selectionPos{line: 0, col: 0}
	h.selEnd = selectionPos{line: 0, col: 5}

	line := "\x1b[31mAB\x1b[0m\x1b[32mCD\x1b[0mE"
	lines := []string{line}
	h.applySelectionHighlightLocked(lines, 80)

	row := core.ParseLine(lines[0])
	if row.IsRaw() {
		t.Fatalf("expected parsed row, got raw")
	}
	if len(row.Cells) < 5 {
		t.Fatalf("unexpected rendered cell count: %d", len(row.Cells))
	}
	base := row.Cells[0].Style
	for i := 1; i < 5; i++ {
		if !row.Cells[i].Style.Equal(base) {
			t.Fatalf("selected styles are not uniform at col=%d", i)
		}
	}
}

// TestViewportRowToAbsoluteWithScrollIndicator verifies that when the history
// is scrolled up (!follow, offset > 0), Render inserts a "^ N more lines"
// indicator at viewport row 0, and viewportRowToAbsoluteLocked correctly
// skips it so mouse selections map to the content actually displayed.
//
// Without the fix, every row is off by one: clicking the first visible
// content line selects the second, etc.
func TestViewportRowToAbsoluteWithScrollIndicator(t *testing.T) {
	h := NewChatHistory()
	for i := 0; i < 20; i++ {
		h.Append(ChatMessage{Role: RoleSystem, Text: "row"})
	}
	h.SetMaxRows(5)

	// Populate cachedAll.
	_ = h.Render(40)

	// Scroll up so the indicator row appears.
	h.ScrollBy(3)
	if h.follow || h.offset == 0 {
		t.Fatalf("precondition: expected !follow and offset>0; follow=%v offset=%d", h.follow, h.offset)
	}

	// Row 0 is the indicator row — not selectable.
	if got := h.viewportRowToAbsoluteLocked(0); got != -1 {
		t.Fatalf("indicator row (0) should be unselectable; got absLine=%d", got)
	}

	// Row 1 maps to the first visible content line. Compute expected via
	// the same formula Render uses (minus the indicator skip).
	total := int64(len(h.cachedAll))
	end := total - h.offset
	start := end - h.maxRows
	if start < 0 {
		start = 0
	}
	wantFirst := start
	if got := h.viewportRowToAbsoluteLocked(1); got != wantFirst {
		t.Fatalf("row 1 should map to first content line %d; got %d", wantFirst, got)
	}

	// Row 2 maps to the second visible content line.
	if got := h.viewportRowToAbsoluteLocked(2); got != wantFirst+1 {
		t.Fatalf("row 2 should map to content line %d; got %d", wantFirst+1, got)
	}
}

// TestViewportRowToAbsoluteNoIndicatorWhenFollowingTail verifies that when
// following the tail (offset == 0, no indicator row), the mapping is direct
// with no row-skip.
func TestViewportRowToAbsoluteNoIndicatorWhenFollowingTail(t *testing.T) {
	h := NewChatHistory()
	for i := 0; i < 20; i++ {
		h.Append(ChatMessage{Role: RoleSystem, Text: "row"})
	}
	h.SetMaxRows(5)
	_ = h.Render(40)

	// No scroll — following tail, no indicator row.
	if !h.follow || h.offset != 0 {
		t.Fatalf("precondition: expected follow=true offset=0; follow=%v offset=%d", h.follow, h.offset)
	}

	total := int64(len(h.cachedAll))
	end := total - h.offset
	start := end - h.maxRows
	if start < 0 {
		start = 0
	}

	// Row 0 maps directly to the first visible content line (no indicator).
	if got := h.viewportRowToAbsoluteLocked(0); got != start {
		t.Fatalf("row 0 should map to content line %d; got %d", start, got)
	}
}

// TestChatHistoryIncrementalCache verifies that ChatHistory only renders
// messages that have changed, not the full transcript on every update.
func TestChatHistoryIncrementalCache(t *testing.T) {
	h := NewChatHistory()

	// Append 5 messages and render once.
	for i := 0; i < 5; i++ {
		h.Append(ChatMessage{Role: RoleSystem, Text: "row"})
	}
	_ = h.Render(40)
	if h.renderCount != 5 {
		t.Fatalf("expected 5 render calls for initial render, got %d", h.renderCount)
	}
	if len(h.msgCache) != 5 {
		t.Fatalf("expected 5 cached messages, got %d", len(h.msgCache))
	}

	// Render again without changes: no new render calls.
	_ = h.Render(40)
	if h.renderCount != 5 {
		t.Fatalf("expected no new renders on unchanged history, got %d", h.renderCount)
	}

	// Append one more message: only the new message is rendered.
	h.Append(ChatMessage{Role: RoleSystem, Text: "new"})
	_ = h.Render(40)
	if h.renderCount != 6 {
		t.Fatalf("expected 1 new render call for appended message, got %d", h.renderCount)
	}
	if len(h.msgCache) != 6 {
		t.Fatalf("expected 6 cached messages after append, got %d", len(h.msgCache))
	}

	// Patch an existing message: only that message is re-rendered.
	firstID := h.messages[0].ID
	h.PatchMessage(firstID, func(m *ChatMessage) { m.Text = "patched" })
	_ = h.Render(40)
	if h.renderCount != 7 {
		t.Fatalf("expected 1 new render call for patched message, got %d", h.renderCount)
	}

	// Changing width clears the cache and re-renders everything.
	prevCount := h.renderCount
	_ = h.Render(80)
	// With 6 cached messages, width change should re-render all 6.
	if h.renderCount != prevCount+6 {
		t.Fatalf("width change should re-render 6 messages (prev=%d got=%d)", prevCount, h.renderCount)
	}
}

// TestChatHistoryAppendDeltaGeneratesUniqueIDs verifies that streaming deltas
// create distinct messages when no id is supplied, exercising the monotonic
// msgIDSeq generator.
func TestChatHistoryAppendDeltaGeneratesUniqueIDs(t *testing.T) {
	h := NewChatHistory()
	id1 := h.AppendDelta("", "first")
	id2 := h.AppendDelta("", "second")
	if id1 == "" || id2 == "" {
		t.Fatalf("expected non-empty IDs, got %q and %q", id1, id2)
	}
	if id1 == id2 {
		t.Fatalf("AppendDelta with empty id should generate unique IDs, got %q twice", id1)
	}
}

// TestChatHistoryCacheProducesIdenticalOutput verifies that the incremental
// cache does not change the rendered output compared to a full re-render.
func TestChatHistoryCacheProducesIdenticalOutput(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "hello"})
	h.Append(ChatMessage{Role: RoleAssistant, Text: "world"})

	first := h.Render(40)
	second := h.Render(40)
	if len(first) != len(second) {
		t.Fatalf("output length changed: first=%d second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("line %d differs:\nfirst:  %q\nsecond: %q", i, first[i], second[i])
		}
	}

	// Append a message and confirm output still contains all prior content.
	h.Append(ChatMessage{Role: RoleUser, Text: "next"})
	third := h.Render(40)
	joined := strings.Join(third, "\n")
	for _, want := range []string{"hello", "world", "next"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in output:\n%s", want, joined)
		}
	}
}

// TestChatHistoryStreamingDeltaReusesBlockCache verifies that streaming deltas
// to a Pending assistant message reuse the per-block cache rather than
// re-rendering the whole message. The cache entry count for that message must
// grow as blocks accumulate, but each delta must NOT reset it to zero — the
// already-closed blocks are preserved across deltas.
func TestChatHistoryStreamingDeltaReusesBlockCache(t *testing.T) {
	h := NewChatHistory()
	id := h.AppendDelta("", "# Title\n\nFirst paragraph.")

	// First render: builds the block cache for the Pending message.
	_ = h.Render(60)
	h.mu.Lock()
	cm, ok := h.msgCache[id]
	h.mu.Unlock()
	if !ok || cm.blockCache == nil {
		t.Fatalf("Pending assistant message should have a block cache, got ok=%v", ok)
	}
	firstEntries := cm.blockCache.Entries()

	// Append more text to the trailing paragraph — the cache must NOT shrink
	// (earlier blocks are reused); the entry count stays equal to block count.
	h.AppendDelta(id, " More text.")
	_ = h.Render(60)
	h.mu.Lock()
	cm = h.msgCache[id]
	h.mu.Unlock()
	if cm.blockCache.Entries() < firstEntries {
		t.Errorf("block cache shrank after delta: first=%d now=%d (earlier blocks should be reused)",
			firstEntries, cm.blockCache.Entries())
	}

	// Add a new block — entry count must grow to cover it.
	h.AppendDelta(id, "\n\n- new bullet")
	_ = h.Render(60)
	h.mu.Lock()
	cm = h.msgCache[id]
	h.mu.Unlock()
	if cm.blockCache.Entries() <= firstEntries {
		t.Errorf("block cache did not grow after adding a block: first=%d now=%d",
			firstEntries, cm.blockCache.Entries())
	}
}

// TestChatHistoryAppendDeltaDeduplicatesNonConsecutiveRepeats verifies that
// identical deltas are suppressed even when they are not back-to-back. Some
// providers (or proxy/buffer layers) re-emit the same sentence after a different
// delta; without this guard the assistant text appears duplicated in the UI.
func TestChatHistoryAppendDeltaDeduplicatesNonConsecutiveRepeats(t *testing.T) {
	h := NewChatHistory()
	id := h.AppendDelta("", "Hello, ")
	h.AppendDelta(id, "world!")
	h.AppendDelta(id, "Hello, ") // non-consecutive repeat of an earlier delta
	h.AppendDelta(id, "world!")  // non-consecutive repeat

	msgs := h.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 streaming msg, got %d", len(msgs))
	}
	if got, want := msgs[0].Text, "Hello, world!"; got != want {
		t.Fatalf("dedup failed: text=%q want=%q", got, want)
	}
}

// TestChatHistoryAppendDeltaRejectsCumulativeContent verifies that if a provider
// mistakenly sends the full generated text so far as each "delta", we do not
// keep appending it. This prevents the exponential/cumulative duplication seen
// when providers stream whole sentences instead of incremental tokens.
func TestChatHistoryAppendDeltaRejectsCumulativeContent(t *testing.T) {
	h := NewChatHistory()
	id := h.AppendDelta("", "Hello")
	h.AppendDelta(id, "Hello, world") // cumulative: contains prefix already in text
	h.AppendDelta(id, "Hello, world!")

	msgs := h.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 streaming msg, got %d", len(msgs))
	}
	if got, want := msgs[0].Text, "Hello, world!"; got != want {
		t.Fatalf("cumulative dedup failed: text=%q want=%q", got, want)
	}
}

// TestChatHistoryAppendDeltaKeepsLegitSuffixIncrement verifies that an
// incremental delta which happens to equal the tail of the current text is
// appended, not dropped. Providers stream true increments (each chunk is new
// content), so a suffix match means the model genuinely produced that text
// again (repeated words, repeated symbols, closing brackets) — suppressing it
// would silently truncate the visible answer.
func TestChatHistoryAppendDeltaKeepsLegitSuffixIncrement(t *testing.T) {
	h := NewChatHistory()
	id := h.AppendDelta("", "Hello, world")
	h.AppendDelta(id, "world") // legit new content that happens to be a suffix
	h.AppendDelta(id, "!")

	msgs := h.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 streaming msg, got %d", len(msgs))
	}
	// Both deltas are real new output — the text must keep them, duplicated.
	if got, want := msgs[0].Text, "Hello, worldworld!"; got != want {
		t.Fatalf("suffix increment dropped: text=%q want=%q", got, want)
	}
}

// TestChatHistoryAppendDeltaWithKindRoutesThinkingToSegments verifies that
// a "thinking" delta is appended to ThinkingSegments, never to the visible
// Text. This is the regression guard for the DeepSeek v4 text-garbling bug:
// onMessageDelta previously dropped the Kind field, so reasoning_content
// chunks were concatenated into m.Text, scrambling word order.
func TestChatHistoryAppendDeltaWithKindRoutesThinkingToSegments(t *testing.T) {
	h := NewChatHistory()
	id := h.AppendDeltaWithKind("", "可见正文", "text")
	h.AppendDeltaWithKind(id, "内部思考过程", "thinking")
	h.AppendDeltaWithKind(id, "更多正文", "text")

	msgs := h.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 msg, got %d", len(msgs))
	}
	if got, want := msgs[0].Text, "可见正文更多正文"; got != want {
		t.Fatalf("text polluted by thinking: text=%q want=%q", got, want)
	}
	if len(msgs[0].ThinkingSegments) != 1 {
		t.Fatalf("expected 1 thinking segment, got %d", len(msgs[0].ThinkingSegments))
	}
	if got, want := msgs[0].ThinkingSegments[0].Text, "内部思考过程"; got != want {
		t.Fatalf("thinking segment=%q want=%q", got, want)
	}
}

// TestChatHistoryAppendDeltaWithKindMixedStream simulates the real DeepSeek v4
// streaming pattern (testified in example/provider-compat/compat_test.go):
// reasoning_content chunks alternate with content chunks. After the fix, the
// text field must read exactly as the model's visible output, with thinking
// fragments quarantined in separate segments.
func TestChatHistoryAppendDeltaWithKindMixedStream(t *testing.T) {
	h := NewChatHistory()
	id := h.AppendDeltaWithKind("", "知识产权法庭裁判要旨", "text")
	h.AppendDeltaWithKind(id, "需要检索改进动机相关案例", "thinking")
	h.AppendDeltaWithKind(id, "3典型案例 / 关于区别特征", "text")
	h.AppendDeltaWithKind(id, "隧道高清全息成像装置案发明构思", "thinking")
	h.AppendDeltaWithKind(id, "与协调配合关系对改进动机判断的影响", "text")

	msgs := h.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 msg, got %d", len(msgs))
	}
	wantText := "知识产权法庭裁判要旨3典型案例 / 关于区别特征与协调配合关系对改进动机判断的影响"
	if got := msgs[0].Text; got != wantText {
		t.Fatalf("visible text garbled: %q want %q", got, wantText)
	}
	if len(msgs[0].ThinkingSegments) != 2 {
		t.Fatalf("expected 2 thinking segments, got %d", len(msgs[0].ThinkingSegments))
	}
	if got := msgs[0].ThinkingSegments[0].Text; got != "需要检索改进动机相关案例" {
		t.Fatalf("segment 0=%q", got)
	}
	if got := msgs[0].ThinkingSegments[1].Text; got != "隧道高清全息成像装置案发明构思" {
		t.Fatalf("segment 1=%q", got)
	}
}

// TestChatHistoryAppendDeltaWithKindDedupAcrossKinds verifies exact-match
// dedup still holds independently per storage target: a repeated text delta
// is suppressed without touching the thinking stream, and vice versa.
func TestChatHistoryAppendDeltaWithKindDedupAcrossKinds(t *testing.T) {
	h := NewChatHistory()
	id := h.AppendDeltaWithKind("", "abc", "text")
	h.AppendDeltaWithKind(id, "abc", "text") // exact duplicate → suppressed
	h.AppendDeltaWithKind(id, "def", "thinking")
	h.AppendDeltaWithKind(id, "def", "thinking") // exact duplicate → suppressed
	h.AppendDeltaWithKind(id, "ghi", "text")

	msgs := h.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 msg, got %d", len(msgs))
	}
	if got, want := msgs[0].Text, "abcghi"; got != want {
		t.Fatalf("text dedup broken: %q want %q", got, want)
	}
	if len(msgs[0].ThinkingSegments) != 1 || msgs[0].ThinkingSegments[0].Text != "def" {
		t.Fatalf("thinking segments unexpected: %+v", msgs[0].ThinkingSegments)
	}
}

// TestChatHistoryAppendDeltaWithKindCumulativeInText verifies the cumulative
// chunk replacement (HasPrefix) still works on the text stream when thinking
// deltas are interleaved — the scenario where the old bug's contamination
// broke the prefix match and produced duplicated content.
func TestChatHistoryAppendDeltaWithKindCumulativeInText(t *testing.T) {
	h := NewChatHistory()
	id := h.AppendDeltaWithKind("", "Hello", "text")
	h.AppendDeltaWithKind(id, "thinking part", "thinking")
	h.AppendDeltaWithKind(id, "Hello, world", "text") // cumulative text → replace

	msgs := h.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 msg, got %d", len(msgs))
	}
	if got, want := msgs[0].Text, "Hello, world"; got != want {
		t.Fatalf("cumulative replacement broken with interleaved thinking: %q want %q", got, want)
	}
	if got := msgs[0].ThinkingSegments[0].Text; got != "thinking part" {
		t.Fatalf("thinking segment=%q", got)
	}
}

// TestChatHistoryStickToBottomHint verifies the "↓ N new — End to follow"
// hint appears when the user scrolls up and new content arrives, and that
// returning to the tail clears it.
func TestChatHistoryStickToBottomHint(t *testing.T) {
	h := NewChatHistory()
	h.SetMaxRows(3)
	// Fill enough to scroll.
	for i := 0; i < 5; i++ {
		h.Append(ChatMessage{Role: RoleSystem, Text: fmt.Sprintf("line %d", i)})
	}
	_ = h.Render(40)

	// User scrolls up: follow becomes false, tailAnchorLen freezes at 5.
	h.ScrollBy(2)
	if h.follow {
		t.Fatalf("ScrollBy should set follow=false")
	}

	// New content arrives while scrolled up.
	h.Append(ChatMessage{Role: RoleSystem, Text: "new content"})
	withNew := h.Render(40)
	joined := strings.Join(withNew, "\n")
	if !strings.Contains(joined, "↓") {
		t.Errorf("expected '↓ N new' hint after new content while scrolled up, got:\n%s", joined)
	}

	// Return to tail: hint must clear.
	h.FollowTail()
	atTail := h.Render(40)
	if strings.Contains(strings.Join(atTail, "\n"), "↓") {
		t.Errorf("hint should clear after FollowTail, got:\n%s", strings.Join(atTail, "\n"))
	}
}

// BenchmarkChatHistoryStreamAppend models the real streaming workload: many
// small deltas appended to a single Pending assistant message, with a render
// after each delta. This is the path P0-1 optimizes; the block cache should
// keep it near-linear in the message length rather than quadratic.
func BenchmarkChatHistoryStreamAppend(b *testing.B) {
	const deltaCount = 200
	for n := 0; n < b.N; n++ {
		h := NewChatHistory()
		id := ""
		for i := 0; i < deltaCount; i++ {
			if i%20 == 0 {
				if id != "" {
					id = h.AppendDelta(id, "\n\n")
				}
				id = h.AppendDelta(id, fmt.Sprintf("Paragraph %d with some words to render. ", i/20+1))
			} else {
				id = h.AppendDelta(id, "word ")
			}
			_ = h.Render(80)
		}
	}
}

// BenchmarkChatHistoryStreamAppendNoCache is the comparison baseline: it
// renders the pending message with mdCache=nil on every delta, which is
// exactly the pre-P0-1 code path (full NewMarkdown+Render per delta). The
// delta loop matches BenchmarkChatHistoryStreamAppend so the two numbers are
// directly comparable.
func BenchmarkChatHistoryStreamAppendNoCache(b *testing.B) {
	const deltaCount = 200
	for n := 0; n < b.N; n++ {
		h := NewChatHistory()
		id := ""
		for i := 0; i < deltaCount; i++ {
			if i%20 == 0 {
				if id != "" {
					id = h.AppendDelta(id, "\n\n")
				}
				id = h.AppendDelta(id, fmt.Sprintf("Paragraph %d with some words to render. ", i/20+1))
			} else {
				id = h.AppendDelta(id, "word ")
			}
			// Render the pending message with no block cache: full re-parse
			// and re-render every delta, as before P0-1.
			h.mu.Lock()
			if len(h.messages) > 0 {
				msg := h.messages[len(h.messages)-1]
				_ = h.renderMessage(msg, h.theme, 80, nil)
			}
			h.mu.Unlock()
		}
	}
}

// TestAppendStreamCursorFitsWidth verifies the streaming cursor never pushes a
// rendered line past width. Before the fix the cursor was appended blindly, so
// full-width lines (padded code fences, tables, hard-wrapped paragraphs) grew
// to width+1 and got hard-truncated by the scrollbar/engine layer — dropping
// the trailing real character together with the cursor.
func TestAppendStreamCursorFitsWidth(t *testing.T) {
	cursor := "\x1b[1m▊\x1b[0m" // styled cursor, 1 cell wide
	tests := []struct {
		name   string
		line   string
		width  int64
		append bool // expect the cursor appended without trimming
	}{
		{"roomy", "abc", 10, true},
		{"exact fit", "abcdefghi", 10, true}, // 9 + cursor = 10
		{"one over", "abcdefghij", 10, false},
		{"ansi over", "\x1b[32mabcdefghij\x1b[0m", 10, false},
		{"wide zero", "中文中文中文", 8, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := appendStreamCursor(tc.line, tc.width, cursor)
			if w := core.VisibleWidth(out); w > tc.width {
				t.Fatalf("overflows width %d: %q (w=%d)", tc.width, out, w)
			}
			if tc.append && out != tc.line+cursor {
				t.Fatalf("expected direct append, got %q", out)
			}
			if !strings.HasSuffix(out, cursor) {
				t.Fatalf("cursor missing at end: %q", out)
			}
		})
	}
}

// TestChatHistoryPendingCursorNoTruncation is the end-to-end regression test
// for the streaming cursor truncation bug. A Pending assistant message whose
// text is a padded code-fence block is rendered in a narrow terminal with the
// scrollbar active; before the fix the trailing "▊" overflowed the last line
// by one cell and the scrollbar layer truncated it (dropping real characters).
func TestChatHistoryPendingCursorNoTruncation(t *testing.T) {
	h := NewChatHistory() // 默认 sbEnabled=true, sbWidth=1
	h.SetMaxRows(8)
	for i := 0; i < 6; i++ {
		h.Append(ChatMessage{Role: RoleUser, Text: fmt.Sprintf("filler %d", i)})
	}
	id := h.AppendDelta("", "```go\nABCDEFGHIJKLMNOPQRSTUVWXYZ\n```")

	lines := h.Render(20)
	for i, ln := range lines {
		if core.VisibleWidth(ln) > 20 {
			t.Errorf("line %d overflows width 20: %q", i, ln)
		}
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "TUVWXYZ") {
		t.Errorf("streamed fence content truncated:\n%s", joined)
	}
	if !strings.Contains(joined, "▊") {
		t.Errorf("streaming cursor missing:\n%s", joined)
	}
	if strings.Contains(joined, "…") {
		t.Errorf("unexpected ellipsis truncation:\n%s", joined)
	}
	h.Finalize(id)
}
