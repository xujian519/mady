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

	result := e.snipToolResults(msgs)

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

	result := e.snipToolResults(msgs)
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

	result := e.snipToolResults(msgs)
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
