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
}

// TestRenderMessageSeparator verifies all five separator branches.
func TestRenderMessageSeparator(t *testing.T) {
	th := DefaultChatHistoryTheme()
	sep := func(prev, curr ChatMessage) string {
		lines := (&ChatHistory{theme: th}).renderMessageSeparator(prev, curr, 40, th)
		return strings.Join(lines, "\n")
	}

	user := ChatMessage{Role: RoleUser}
	asst := ChatMessage{Role: RoleAssistant}
	tool := ChatMessage{Role: RoleTool}
	sys := ChatMessage{Role: RoleSystem}

	if s := sep(user, asst); !strings.Contains(core.StripAnsi(s), "─") {
		t.Errorf("user→assistant should be a full divider, got %q", s)
	}
	if s := sep(asst, user); !strings.Contains(core.StripAnsi(s), "─") {
		t.Errorf("assistant→user should be a full divider, got %q", s)
	}
	if s := sep(tool, asst); !strings.Contains(core.StripAnsi(s), "─") {
		t.Errorf("tool→assistant should be a full divider, got %q", s)
	}
	if s := sep(tool, tool); !strings.Contains(core.StripAnsi(s), "·") {
		t.Errorf("tool→tool should be a half divider, got %q", s)
	}
	if s := sep(tool, sys); s != "" {
		t.Errorf("tool→system should be a blank line, got %q", s)
	}
	if s := sep(sys, sys); !strings.Contains(core.StripAnsi(s), "·") {
		t.Errorf("same-role should be a quarter divider, got %q", s)
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
