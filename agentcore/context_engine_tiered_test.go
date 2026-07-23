package agentcore

import (
	"context"
	"strings"
	"testing"
)

func TestTieredEngine_Name(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{ContextWindow: 128000})
	if e.Name() != "tiered" {
		t.Errorf("Name()=%q want %q", e.Name(), "tiered")
	}
}

func TestTieredEngine_TierLevel(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{ContextWindow: 10000}).(*TieredEngine)

	// Build messages of known approximate token size
	smallMsgs := []Message{{Role: RoleUser, Content: "hello"}}
	largeMsgs := make([]Message, 100)
	for i := range largeMsgs {
		largeMsgs[i] = Message{Role: RoleTool, Content: strings.Repeat("x", 200)}
	}

	if level := e.TierLevel(smallMsgs, 10000); level != "none" {
		t.Errorf("small msgs level=%s want none", level)
	}

	if level := e.TierLevel(largeMsgs, 10000); level == "none" {
		t.Errorf("large msgs should trigger at least snip, got none")
	}
}

func TestTieredEngine_SnipToolResults(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{
		ContextWindow:    10000,
		KeepRecentTokens: 500,
	}).(*TieredEngine)

	longContent := strings.Repeat("A", 2000)
	msgs := []Message{
		{Role: RoleUser, Content: "do something"},
		{Role: RoleAssistant, Content: "ok", ToolCalls: []ToolCall{{ID: "1", Name: "test"}}},
		{Role: RoleTool, Content: longContent, ToolCallID: "1"},
		{Role: RoleAssistant, Content: "done"},
		{Role: RoleUser, Content: "thanks"},
	}

	result := e.snipMessages(msgs)

	// The tool result should be truncated
	if len(result[2].Content) >= len(longContent) {
		t.Errorf("expected tool result to be snipped, got len=%d", len(result[2].Content))
	}
	if !strings.Contains(result[2].Content, "已截断") {
		t.Error("snipped content should contain truncation marker")
	}
}

func TestTieredEngine_PruneToolResults(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{
		ContextWindow:    10000,
		KeepRecentTokens: 50, // small tail to leave old msgs outside
	}).(*TieredEngine)

	msgs := []Message{
		{Role: RoleUser, Content: "do something"},
		{Role: RoleAssistant, Content: "ok", ToolCalls: []ToolCall{{ID: "1", Name: "test"}}},
		{Role: RoleTool, Content: strings.Repeat("A", 500), ToolCallID: "1"},
		{Role: RoleAssistant, Content: "done"},
		{Role: RoleUser, Content: "thanks"},
	}

	result := e.pruneToolResults(msgs)

	// The tool result should be replaced with placeholder
	found := false
	for _, m := range result {
		if m.Role == RoleTool && strings.Contains(m.Content, "已清除") {
			found = true
		}
	}
	if !found {
		t.Error("expected at least one tool result to be pruned")
	}
}

func TestTieredEngine_PreserveTail(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{
		ContextWindow:    10000,
		KeepRecentTokens: 5000, // large enough to protect everything
	}).(*TieredEngine)

	longContent := strings.Repeat("A", 2000)
	msgs := []Message{
		{Role: RoleAssistant, Content: "ok", ToolCalls: []ToolCall{{ID: "1", Name: "test"}}},
		{Role: RoleTool, Content: longContent, ToolCallID: "1"},
	}

	result := e.snipMessages(msgs)
	// Find the tool result in the result
	for _, m := range result {
		if m.Role == RoleTool && m.Content != longContent {
			t.Error("tool result in tail zone should not be snipped")
		}
	}
}

func TestTieredEngine_ShortContentNotSnipped(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{
		ContextWindow:    10000,
		KeepRecentTokens: 100,
	}).(*TieredEngine)

	msgs := []Message{
		{Role: RoleAssistant, Content: "ok", ToolCalls: []ToolCall{{ID: "1", Name: "test"}}},
		{Role: RoleTool, Content: "short", ToolCallID: "1"},
	}

	result := e.snipMessages(msgs)
	for _, m := range result {
		if m.Role == RoleTool && m.Content != "short" {
			t.Error("short tool result should not be snipped")
		}
	}
}

func TestTieredEngine_Compress_NoOp(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{ContextWindow: 1000000})
	msgs := []Message{{Role: RoleUser, Content: "hello"}}

	result, saved, err := e.Compress(context.Background(), msgs, "")
	if err != nil {
		t.Fatal(err)
	}
	if saved != 0 {
		t.Errorf("expected 0 saved for tiny messages, got %d", saved)
	}
	if len(result) != len(msgs) {
		t.Error("should not modify messages when context is fine")
	}
}

func TestTieredEngine_ShouldCompact(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{ContextWindow: 1000})

	// Small messages should not trigger
	small := []Message{{Role: RoleUser, Content: "hi"}}
	if e.ShouldCompact(small, nil, 1000) {
		t.Error("small messages should not trigger compaction")
	}

	// Large messages should trigger
	large := make([]Message, 50)
	for i := range large {
		large[i] = Message{Role: RoleUser, Content: strings.Repeat("x", 100)}
	}
	if !e.ShouldCompact(large, nil, 1000) {
		t.Error("large messages should trigger compaction")
	}
}

func TestTieredEngine_OnSessionLifecycle(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{ContextWindow: 10000})

	e.OnSessionStart(context.Background(), "test-model", 50000)
	if e.ContextLength() != 50000 {
		t.Errorf("ContextLength()=%d want 50000", e.ContextLength())
	}

	e.OnSessionReset()
	if e.CompressionCount() != 0 {
		t.Error("compression count should reset")
	}

	e.OnSessionEnd()
}

func TestSnipMessageContent(t *testing.T) {
	content := strings.Repeat("X", 2000)
	result := SnipMessageContent(content, 100, 50)
	if !strings.Contains(result, "已截断") {
		t.Error("should contain truncation marker")
	}
	if len(result) >= len(content) {
		t.Error("snipped content should be shorter")
	}
}

func TestPruneMessageContent(t *testing.T) {
	result := PruneMessageContent("some long content here")
	if !strings.Contains(result, "已清除") {
		t.Error("should contain clearing marker")
	}
}

func TestTieredEngine_FindTailBoundary(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{
		ContextWindow:    100000,
		KeepRecentTokens: 100, // small tail
	}).(*TieredEngine)

	msgs := []Message{
		{Role: RoleUser, Content: strings.Repeat("x", 400)}, // old, outside tail
		{Role: RoleTool, Content: strings.Repeat("x", 400)}, // old, outside tail
		{Role: RoleUser, Content: "recent"},                 // in tail
	}

	boundary := e.findTailBoundary(msgs)
	if boundary < 1 {
		t.Errorf("tail boundary should be > 0, got %d", boundary)
	}
}

func TestEngineRegistry_HasTiered(t *testing.T) {
	reg := NewEngineRegistry()
	names := reg.List()
	found := false
	for _, n := range names {
		if n == "tiered" {
			found = true
		}
	}
	if !found {
		t.Error("EngineRegistry should contain 'tiered' engine")
	}

	e, err := reg.Create("tiered", ContextEngineConfig{ContextWindow: 10000})
	if err != nil {
		t.Fatal(err)
	}
	if e.Name() != "tiered" {
		t.Errorf("created engine name=%q want tiered", e.Name())
	}
}

func TestTieredEngine_SnipLargeUserMessages(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{
		ContextWindow:    10000,
		KeepRecentTokens: 500, // protect recent tail
	}).(*TieredEngine)

	// Large user message (simulates pasted file content)
	largeUserContent := strings.Repeat("这是用户粘贴的大段内容", 300) // ~3600 chars, all CJK
	msgs := []Message{
		{Role: RoleUser, Content: largeUserContent},
		{Role: RoleAssistant, Content: "ok"},
		{Role: RoleUser, Content: "do something with that"},
		{Role: RoleAssistant, Content: "done"},
		{Role: RoleUser, Content: "thanks"},
	}

	result := e.snipMessages(msgs)

	// The large user message should be snipped
	if result[0].Content == largeUserContent {
		t.Error("large user message should be snipped")
	}
	if !strings.Contains(result[0].Content, "已截断") {
		t.Error("snipped user message should contain truncation marker")
	}

	// Small messages should be untouched
	if result[4].Content != "thanks" {
		t.Error("small user message in tail should not be modified")
	}
}

func TestTieredEngine_SnipLargeAssistantMessages(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{
		ContextWindow:    10000,
		KeepRecentTokens: 100,
	}).(*TieredEngine)

	largeAssistantContent := strings.Repeat("assistant output line\n", 500) // ~10000 chars
	msgs := []Message{
		{Role: RoleUser, Content: "do something"},
		{Role: RoleAssistant, Content: largeAssistantContent},
		{Role: RoleUser, Content: "ok"},
	}

	result := e.snipMessages(msgs)

	// Large assistant message should be snipped
	if result[1].Content == largeAssistantContent {
		t.Error("large assistant message should be snipped")
	}
	if !strings.Contains(result[1].Content, "已截断") {
		t.Error("snipped assistant message should contain truncation marker")
	}
}

func TestTieredEngine_SnipBothToolAndNonToolInOnePass(t *testing.T) {
	e := NewTieredEngine(ContextEngineConfig{
		ContextWindow:    10000,
		KeepRecentTokens: 100,
	}).(*TieredEngine)

	largeToolContent := strings.Repeat("T", 2000)
	largeUserContent := strings.Repeat("U", 3000)
	msgs := []Message{
		{Role: RoleUser, Content: largeUserContent},
		{Role: RoleAssistant, Content: "ok", ToolCalls: []ToolCall{{ID: "1", Name: "test"}}},
		{Role: RoleTool, Content: largeToolContent, ToolCallID: "1"},
		{Role: RoleUser, Content: "recent"},
	}

	result := e.snipMessages(msgs)

	// Both large messages should be snipped in a single pass
	if result[0].Content == largeUserContent {
		t.Error("large user message should be snipped")
	}
	if result[2].Content == largeToolContent {
		t.Error("large tool message should be snipped")
	}

	// Non-tool should get a larger head window than tool
	userHead := e.snipHeadChars * snipNonToolHeadMultiplier
	toolHead := e.snipHeadChars
	if userHead <= toolHead {
		t.Fatalf("non-tool head (%d) should be larger than tool head (%d)", userHead, toolHead)
	}
}

func TestTruncateToTokenBudget(t *testing.T) {
	// Content within budget → unchanged
	short := "hello world"
	result := truncateToTokenBudget(short, 5, 100, "...[marker]")
	if result != short {
		t.Fatalf("content within budget should be unchanged: got %q", result)
	}

	// Content over budget → truncated with marker
	long := strings.Repeat("x", 10000) // ~2500 tokens
	result = truncateToTokenBudget(long, 2500, 500, "...[marker]")
	if result == long {
		t.Fatal("over-budget content should be truncated")
	}
	if !strings.Contains(result, "...[marker]") {
		t.Error("truncated content should contain marker")
	}

	// CJK content: token density is higher, so fewer runes are kept
	cjkContent := strings.Repeat("你", 1000) // ~1500 tokens (with CJK correction)
	result = truncateToTokenBudget(cjkContent, EstimateTokens(cjkContent), 500, "...[marker]")
	if result == cjkContent {
		t.Fatal("CJK over-budget content should be truncated")
	}
	// CJK should keep fewer runes than ASCII at the same token budget
	asciiResult := truncateToTokenBudget(
		strings.Repeat("x", 3000), EstimateTokens(strings.Repeat("x", 3000)), 500, "...[marker]")
	cjkRunes := len([]rune(result))
	asciiRunes := len([]rune(asciiResult))
	if cjkRunes >= asciiRunes {
		t.Fatalf("CJK should keep fewer runes than ASCII at same token budget: cjk=%d ascii=%d",
			cjkRunes, asciiRunes)
	}
}
