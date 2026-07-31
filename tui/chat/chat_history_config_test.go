package chat

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// TestChatHistorySetTheme verifies theme replacement invalidates the cache.
func TestChatHistorySetTheme(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "hello"})
	h.Render(40)
	if len(h.msgCache) == 0 {
		t.Fatal("setup: expected render cache entries")
	}

	th := DefaultChatHistoryTheme()
	th.UserPrefix = "» "
	h.SetTheme(th)
	if h.theme.UserPrefix != "» " {
		t.Fatalf("theme not applied: %q", h.theme.UserPrefix)
	}
	if !h.dirty {
		t.Fatal("SetTheme should mark history dirty")
	}
	if len(h.msgCache) != 0 {
		t.Fatal("SetTheme should clear the render cache")
	}
	// A second SetTheme with the same struct still invalidates.
	h.SetTheme(DefaultChatHistoryTheme())
	if !h.dirty {
		t.Fatal("second SetTheme should keep history dirty")
	}
}

// TestChatHistorySetReasoningRenderer verifies renderer swap.
func TestChatHistorySetReasoningRenderer(t *testing.T) {
	h := NewChatHistory()
	h.SetReasoningRenderer(nil) // nil → hidden
	h.SetReasoningRenderer(&DefaultReasoningRenderer{Show: true, Mode: "collapsed"})

	id := h.AppendDeltaWithKind("", "secret thinking", "thinking")
	msgs := h.Messages()
	if len(msgs[0].ThinkingSegments) != 1 {
		t.Fatalf("setup: expected 1 thinking segment")
	}
	out := strings.Join(h.Render(40), "\n")
	if !strings.Contains(out, "Thinking") {
		t.Fatalf("renderer should render thinking, got %q", out)
	}
	h.Finalize(id)
}

// TestChatHistorySetScrollbarEnabled verifies toggling the scrollbar.
func TestChatHistorySetScrollbarEnabled(t *testing.T) {
	h := NewChatHistory()
	h.SetScrollbarEnabled(false)
	if h.sbEnabled {
		t.Fatal("scrollbar should be disabled")
	}
	h.SetScrollbarEnabled(true)
	if !h.sbEnabled {
		t.Fatal("scrollbar should be enabled")
	}
	if h.sbWidth != 1 {
		t.Fatalf("sbWidth = %d, want 1", h.sbWidth)
	}
}

// TestChatHistoryToggleFoldAtViewportCenter verifies the fold toggle helper.
func TestChatHistoryToggleFoldAtViewportCenter(t *testing.T) {
	h := NewChatHistory()
	if h.ToggleFoldAtViewportCenter() {
		t.Fatal("empty history must not toggle folds")
	}
	h.SetMaxRows(10)

	// Assistant message with thinking segments, center of the viewport.
	h.AppendDeltaWithKind("", "thinking one", "thinking")
	h.Render(40)
	if !h.ToggleFoldAtViewportCenter() {
		t.Fatal("fold should toggle for thinking content at center")
	}
	msgs := h.Messages()
	if !msgs[0].ThinkingSegments[0].Collapsed {
		t.Fatal("toggle should collapse the thinking segment")
	}
}

// TestChatHistoryCollapseConsecutiveTools verifies group collapse at turn end.
func TestChatHistoryCollapseConsecutiveTools(t *testing.T) {
	t.Run("collapses run of 2+", func(t *testing.T) {
		h := NewChatHistory()
		h.Append(ChatMessage{Role: RoleAssistant, Text: "answer"})
		h.Append(ChatMessage{Role: RoleTool, Meta: "t1", Text: "..."})
		h.Append(ChatMessage{Role: RoleTool, Meta: "t2", Text: "..."})

		h.CollapseConsecutiveTools()
		h.SetMaxRows(10)
		out := strings.Join(h.Render(40), "\n")
		if !strings.Contains(out, "[+]") {
			t.Fatalf("expected collapsed group header, got %q", out)
		}
	})

	t.Run("single tool no collapse", func(t *testing.T) {
		h := NewChatHistory()
		h.Append(ChatMessage{Role: RoleAssistant, Text: "answer"})
		h.Append(ChatMessage{Role: RoleTool, Meta: "t1", Text: "..."})
		h.CollapseConsecutiveTools()
		h.SetMaxRows(10)
		out := strings.Join(h.Render(40), "\n")
		if strings.Contains(out, "[+]") {
			t.Fatalf("single tool must not collapse, got %q", out)
		}
	})

	t.Run("assistant tail no-op", func(t *testing.T) {
		h := NewChatHistory()
		h.Append(ChatMessage{Role: RoleAssistant, Text: "x"})
		h.CollapseConsecutiveTools()
	})
}

// TestChatHistoryClear verifies the transcript reset.
func TestChatHistoryClear(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "a"})
	h.Append(ChatMessage{Role: RoleAssistant, Text: "b", Pending: true})
	h.Render(40)

	h.Clear()
	if len(h.messages) != 0 {
		t.Fatalf("messages not cleared: %d", len(h.messages))
	}
	if !h.follow || h.offset != 0 {
		t.Fatal("Clear should restore follow-tail")
	}
	if h.pendingCount != 0 {
		t.Fatalf("pendingCount = %d, want 0", h.pendingCount)
	}
	if h.dirty != true {
		t.Fatal("Clear should mark dirty")
	}
	out := strings.Join(h.Render(40), "\n")
	if !strings.Contains(out, "Mady") {
		t.Fatalf("empty history should show the boot screen, got %q", out)
	}
}

// TestChatHistoryMouseConsumed verifies the MouseConsumer contract.
func TestChatHistoryMouseConsumed(t *testing.T) {
	h := NewChatHistory()
	if h.MouseConsumed() {
		t.Fatal("initially not consumed")
	}
	h.mu.Lock()
	h.mouseConsumed = true
	h.mu.Unlock()
	if !h.MouseConsumed() {
		t.Fatal("should report consumed after set")
	}
}

// TestEvictCacheEntriesLocked verifies the cache cap enforcement.
func TestEvictCacheEntriesLocked(t *testing.T) {
	h := NewChatHistory()
	h.mu.Lock()
	defer h.mu.Unlock()

	h.maxCacheEntries = 2
	h.msgCache = make(map[string]cachedMessage)
	h.messages = nil
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		h.msgCache[id] = cachedMessage{lines: []string{"x"}}
		h.messages = append(h.messages, ChatMessage{ID: id})
	}
	h.evictCacheEntriesLocked()
	if len(h.msgCache) > 2 {
		t.Fatalf("cache should be capped at 2, got %d", len(h.msgCache))
	}

	// Pending messages are exempt from eviction.
	h.maxCacheEntries = 0 // disable cap → no-op path
	h.msgCache["z"] = cachedMessage{lines: []string{"y"}}
	h.evictCacheEntriesLocked()
	if _, ok := h.msgCache["z"]; !ok {
		t.Fatal("disabled cap should not evict")
	}
}

// TestChatHistoryInvalidateDirect verifies Invalidate() marks dirty and fires
// the callback.
func TestChatHistoryInvalidateDirect(t *testing.T) {
	h := NewChatHistory()
	invalidated := false
	h.SetOnInvalidate(func() { invalidated = true })
	h.Render(40)
	h.Invalidate()
	if !h.dirty {
		t.Fatal("Invalidate should mark dirty")
	}
	if !invalidated {
		t.Fatal("Invalidate should trigger onInvalidate")
	}
}

// TestChatHistoryWindowSizeInvalidates verifies WindowSizeMsg routes to
// Invalidate via Update.
func TestChatHistoryWindowSizeInvalidates(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "x"})
	h.Render(40)
	h.dirty = false
	h.Update(core.WindowSizeMsg{Width: 40, Height: 20})
	if !h.dirty {
		t.Fatal("WindowSizeMsg should invalidate the history")
	}
}

// TestChatHistoryThemeStyleRoundTrip verifies style fields survive theme
// construction (guards against nil style panics in DefaultChatHistoryTheme).
func TestChatHistoryThemeStyleRoundTrip(t *testing.T) {
	th := DefaultChatHistoryTheme()
	pal := theme.CurrentPalette()
	_ = pal
	if th.DimStyle.Render("x") == "" {
		t.Fatal("DimStyle should render")
	}
}
