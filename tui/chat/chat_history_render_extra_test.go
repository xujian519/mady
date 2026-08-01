package chat

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
)

// TestRenderToolGroup verifies collapsed/expanded group rendering and the
// tool/system counting in the summary line.
func TestRenderToolGroup(t *testing.T) {
	th := DefaultChatHistoryTheme()
	msgs := []ChatMessage{
		{Role: RoleTool, Meta: "search", Text: "..."},
		{Role: RoleTool, Meta: "read", Text: "..."},
		{Role: RoleSystem, Text: "note"},
	}
	cache := make(map[string]cachedMessage)

	t.Run("collapsed with meta", func(t *testing.T) {
		lines, r := (&ChatHistory{theme: th}).renderToolGroup(msgs, 0, 2, false, th, 60, cache)
		if len(lines) != 1 {
			t.Fatalf("collapsed group should render 1 line, got %d", len(lines))
		}
		plain := core.StripAnsi(lines[0])
		if !strings.Contains(plain, "[+]") || !strings.Contains(plain, "search") {
			t.Fatalf("collapsed summary = %q", plain)
		}
		if !r.toolGroup || r.msgIndex != 0 || r.groupFrom != 0 || r.groupTo != 2 {
			t.Fatalf("group range = %+v", r)
		}
	})

	t.Run("collapsed without meta counts", func(t *testing.T) {
		noMeta := []ChatMessage{
			{Role: RoleTool, Text: "..."},
			{Role: RoleTool, Text: "..."},
		}
		lines, _ := (&ChatHistory{theme: th}).renderToolGroup(noMeta, 0, 1, false, th, 60, cache)
		plain := core.StripAnsi(lines[0])
		if !strings.Contains(plain, "2 tools") {
			t.Fatalf("summary should count tools: %q", plain)
		}
	})

	t.Run("expanded shows all lines", func(t *testing.T) {
		lines, _ := (&ChatHistory{theme: th}).renderToolGroup(msgs, 0, 2, true, th, 60, cache)
		if len(lines) < 4 {
			t.Fatalf("expanded group should render summary + members, got %d lines", len(lines))
		}
		plain := core.StripAnsi(lines[0])
		if !strings.Contains(plain, "[-]") || !strings.Contains(plain, "2 tools") || !strings.Contains(plain, "1 msgs") {
			t.Fatalf("expanded summary = %q", plain)
		}
	})

	t.Run("expanded tools-only summary", func(t *testing.T) {
		lines, _ := (&ChatHistory{theme: th}).renderToolGroup(msgs[:2], 0, 1, true, th, 60, cache)
		plain := core.StripAnsi(lines[0])
		if strings.Contains(plain, "msgs") || !strings.Contains(plain, "2 tools") {
			t.Fatalf("tools-only summary = %q", plain)
		}
	})

	t.Run("summary single-space format", func(t *testing.T) {
		// 回归：marker 与计数之间必须恰好一个空格（曾出现 "[+]  2 tools" 双空格）。
		noMeta := []ChatMessage{
			{Role: RoleTool, Text: "..."},
			{Role: RoleTool, Text: "..."},
		}
		collapsed, _ := (&ChatHistory{theme: th}).renderToolGroup(noMeta, 0, 1, false, th, 60, cache)
		if plain := core.StripAnsi(collapsed[0]); plain != "[+] 2 tools" {
			t.Fatalf("collapsed no-meta summary = %q, want %q", plain, "[+] 2 tools")
		}
		withMeta, _ := (&ChatHistory{theme: th}).renderToolGroup(msgs, 0, 2, false, th, 60, cache)
		if plain := core.StripAnsi(withMeta[0]); plain != "[+] search" {
			t.Fatalf("collapsed meta summary = %q, want %q", plain, "[+] search")
		}
		expanded, _ := (&ChatHistory{theme: th}).renderToolGroup(msgs, 0, 2, true, th, 60, cache)
		if plain := core.StripAnsi(expanded[0]); plain != "[-] 2 tools · 1 msgs" {
			t.Fatalf("expanded summary = %q, want %q", plain, "[-] 2 tools · 1 msgs")
		}
	})

	t.Run("expanded uses left bar timeline", func(t *testing.T) {
		lines, _ := (&ChatHistory{theme: th}).renderToolGroup(msgs[:2], 0, 1, true, th, 60, cache)
		joined := core.StripAnsi(strings.Join(lines, "\n"))
		// 2 个成员 + 1 条组内连接 = 至少 3 处色带。
		barCount := strings.Count(joined, "│")
		if barCount < 3 {
			t.Fatalf("expected left bar markers (2 members + 1 connector), got:\n%s", joined)
		}
		// 组成员之间用细色带连接，不应出现点线或旧的全宽分隔线。
		if strings.Contains(joined, "·") || strings.Contains(joined, "─") {
			t.Fatalf("expanded group should not use dot/full-width dividers, got:\n%s", joined)
		}
	})
}

// TestRenderToolGroupCacheWidthMismatch verifies that a message cached at full
// width (e.g. via the mid-turn non-group path) is re-rendered at the group's
// innerW instead of returning stale full-width lines that would overflow the
// viewport once the left bar prefix is added.
func TestRenderToolGroupCacheWidthMismatch(t *testing.T) {
	th := DefaultChatHistoryTheme()
	const width = int64(60)
	msgs := []ChatMessage{
		{ID: "w-1", Role: RoleTool, Meta: "search", Text: "result"},
		{ID: "w-2", Role: RoleTool, Meta: "read", Text: "..."},
	}
	cache := make(map[string]cachedMessage)
	// 先以全宽渲染第一条并写入缓存，模拟流式中途轮次的非分组路径。
	(&ChatHistory{theme: th}).renderMessageCachedWithCache(msgs[0], th, width, cache)

	// 展开组以 innerW 渲染：缓存宽度不一致，必须重渲染而不是复用全宽行。
	lines, _ := (&ChatHistory{theme: th}).renderToolGroup(msgs, 0, 1, true, th, width, cache)
	for _, ln := range lines {
		if vw := core.VisibleWidth(ln); vw > width {
			t.Fatalf("expanded group line exceeds width %d (got %d): %q", width, vw, core.StripAnsi(ln))
		}
	}
	if c, ok := cache["w-1"]; !ok || c.width != width-4 {
		t.Fatalf("cache should be re-rendered at innerW=%d, got width=%d ok=%v", width-4, c.width, ok)
	}
}

// TestRenderMessageSeparator verifies the compact timeline separator rules:
// role switches produce a single blank line, consecutive tools have no divider,
// and same-role messages use a subtle short divider.
func TestRenderMessageSeparator(t *testing.T) {
	th := DefaultChatHistoryTheme()
	sep := func(prev, curr ChatMessage) []string {
		return (&ChatHistory{theme: th}).renderMessageSeparator(prev, curr, 80, th)
	}

	user := ChatMessage{Role: RoleUser}
	asst := ChatMessage{Role: RoleAssistant}
	tool := ChatMessage{Role: RoleTool}
	sys := ChatMessage{Role: RoleSystem}

	// Role switches: single blank line.
	for _, tc := range []struct {
		prev, curr ChatMessage
		label      string
	}{
		{user, asst, "user→assistant"},
		{asst, user, "assistant→user"},
		{tool, asst, "tool→assistant"},
		{tool, sys, "tool→system"},
		{sys, tool, "system→tool"},
	} {
		lines := sep(tc.prev, tc.curr)
		if len(lines) != 1 || lines[0] != "" {
			t.Errorf("%s should be a single blank line, got %q", tc.label, lines)
		}
	}

	// Consecutive tools: no separator (grouped or ungrouped, the card itself is enough).
	if lines := sep(tool, tool); len(lines) != 0 {
		t.Errorf("tool→tool should have no separator, got %q", lines)
	}

	// System↔System: single blank line.
	if lines := sep(sys, sys); len(lines) != 1 || lines[0] != "" {
		t.Errorf("system→system should be a single blank line, got %q", lines)
	}

	// Same-role assistant/assistant: subtle eighth-width dot divider.
	lines := sep(asst, asst)
	if len(lines) != 1 || !strings.Contains(core.StripAnsi(lines[0]), "·") {
		t.Errorf("assistant→assistant should be a short dot divider, got %q", lines)
	}
}

// TestRenderDomainCard verifies the four domain-card routing branches.
func TestRenderDomainCard(t *testing.T) {
	th := DefaultChatHistoryTheme()
	h := NewChatHistory()

	t.Run("evidence card", func(t *testing.T) {
		lines := h.renderDomainCard(ChatMessage{
			DomainMsg: &component.DomainMessage{Type: component.DomainMsgTypeEvidenceCard, Body: "证据"},
		}, th, 60)
		if len(lines) == 0 {
			t.Fatal("evidence card should render lines")
		}
	})

	t.Run("conclusion card", func(t *testing.T) {
		lines := h.renderDomainCard(ChatMessage{
			DomainMsg: &component.DomainMessage{Type: component.DomainMsgTypeConclusionCard, Body: "结论"},
		}, th, 60)
		if len(lines) == 0 {
			t.Fatal("conclusion card should render lines")
		}
	})

	t.Run("approval card", func(t *testing.T) {
		lines := h.renderDomainCard(ChatMessage{
			DomainMsg: &component.DomainMessage{Type: component.DomainMsgTypeApprovalPrompt, Body: "确认?"},
		}, th, 60)
		if len(lines) == 0 {
			t.Fatal("approval card should render lines")
		}
	})

	t.Run("fallback markdown", func(t *testing.T) {
		lines := h.renderDomainCard(ChatMessage{
			DomainMsg: &component.DomainMessage{Type: "custom_type", Body: "**bold body**"},
		}, th, 60)
		if len(lines) == 0 {
			t.Fatal("fallback should render markdown")
		}
	})

	t.Run("nil domain msg", func(t *testing.T) {
		if lines := h.renderDomainCard(ChatMessage{}, th, 60); lines != nil {
			t.Fatalf("nil DomainMsg should return nil, got %v", lines)
		}
	})
}

// TestRenderMessageExtraRoles verifies remaining renderMessage branches:
// collapsed assistant, error, divider, unknown role, and pending-with-thinking.
func TestRenderMessageExtraRoles(t *testing.T) {
	th := DefaultChatHistoryTheme()
	h := NewChatHistory()

	t.Run("collapsed assistant", func(t *testing.T) {
		long := strings.Repeat("x", 300) + "\nsecond line"
		lines := h.renderMessage(ChatMessage{Role: RoleAssistant, Collapsed: true, Text: long}, th, 60, nil)
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "expand") {
			t.Fatalf("collapsed assistant should show expand hint, got %q", joined)
		}
		if len(joined) > 250 {
			t.Fatalf("collapsed first line should be truncated, len=%d", len(joined))
		}
	})

	t.Run("error role", func(t *testing.T) {
		lines := h.renderMessage(ChatMessage{Role: RoleError, Text: "**boom**"}, th, 60, nil)
		if len(lines) == 0 {
			t.Fatal("error role should render markdown")
		}
	})

	t.Run("divider role", func(t *testing.T) {
		lines := h.renderMessage(ChatMessage{Role: RoleDivider}, th, 60, nil)
		if len(lines) != 1 || lines[0] != "" {
			t.Fatalf("divider should render one blank line, got %v", lines)
		}
	})

	t.Run("unknown role falls back to plain wrap", func(t *testing.T) {
		lines := h.renderMessage(ChatMessage{Role: ChatRole(99), Text: "raw"}, th, 20, nil)
		if len(lines) != 1 || !strings.Contains(lines[0], "raw") {
			t.Fatalf("unknown role should wrap raw text, got %v", lines)
		}
	})

	t.Run("pending thinking without text", func(t *testing.T) {
		lines := h.renderMessage(ChatMessage{
			Role:             RoleAssistant,
			Pending:          true,
			ThinkingSegments: []ThinkingSegment{{Text: "thinking..."}},
		}, th, 60, nil)
		if len(lines) == 0 {
			t.Fatal("pending thinking should render at least a cursor line")
		}
	})

	t.Run("empty assistant renders blank", func(t *testing.T) {
		lines := h.renderMessage(ChatMessage{Role: RoleAssistant}, th, 60, nil)
		if len(lines) != 1 || lines[0] != "" {
			t.Fatalf("empty assistant should render one blank line, got %v", lines)
		}
	})
}

// TestPadToWidth verifies padding behavior with and without the scrollbar.
func TestPadToWidth(t *testing.T) {
	h := NewChatHistory()
	in := []string{"short", "longer line"}

	out := h.padToWidth(append([]string(nil), in...), 20, 2, false, 0)
	for _, ln := range out {
		if core.VisibleWidth(ln) != 20 {
			t.Fatalf("padded width = %d, want 20: %q", core.VisibleWidth(ln), ln)
		}
	}

	// With scrollbar active (sbNow=true) lines pass through unchanged.
	out = h.padToWidth(append([]string(nil), in...), 20, 2, true, 1)
	if len(out) != 2 || out[0] != "short" {
		t.Fatalf("sbNow should skip padding, got %v", out)
	}
}

// TestChatHistoryRenderEmptyBootScreen verifies the empty-transcript boot
// screen via the snapshot path.
func TestChatHistoryRenderEmptyBootScreen(t *testing.T) {
	h := NewChatHistory()
	out := h.Render(60)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "Mady") {
		t.Fatalf("boot screen should mention Mady, got %q", joined)
	}
}

// TestRenderAllWithStateSpliceFastPath verifies the firstDirtyIdx splice path
// preserves the clean prefix.
func TestRenderAllWithStateSpliceFastPath(t *testing.T) {
	h := NewChatHistory()
	h.Append(ChatMessage{Role: RoleUser, Text: "one"})
	h.Append(ChatMessage{Role: RoleUser, Text: "two"})
	h.Append(ChatMessage{Role: RoleUser, Text: "three"})
	before := h.Render(60)

	// Patch the last message: firstDirtyIdx = 2 → splice path.
	h.PatchMessage(h.Messages()[2].ID, func(m *ChatMessage) { m.Text = "three!" })
	after := h.Render(60)
	joined := strings.Join(after, "\n")
	if !strings.Contains(joined, "three!") {
		t.Fatalf("splice path should reflect the patch, got %q", joined)
	}
	if len(before) == 0 {
		t.Fatal("setup: before render should not be empty")
	}
}
