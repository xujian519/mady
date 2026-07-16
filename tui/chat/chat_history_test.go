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
	cm, _ = h.msgCache[id]
	h.mu.Unlock()
	if cm.blockCache.Entries() < firstEntries {
		t.Errorf("block cache shrank after delta: first=%d now=%d (earlier blocks should be reused)",
			firstEntries, cm.blockCache.Entries())
	}

	// Add a new block — entry count must grow to cover it.
	h.AppendDelta(id, "\n\n- new bullet")
	_ = h.Render(60)
	h.mu.Lock()
	cm, _ = h.msgCache[id]
	h.mu.Unlock()
	if cm.blockCache.Entries() <= firstEntries {
		t.Errorf("block cache did not grow after adding a block: first=%d now=%d",
			firstEntries, cm.blockCache.Entries())
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
