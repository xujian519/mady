package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

// --- toolCallSignature tests ---

func TestToolCallSignature_StableOrder(t *testing.T) {
	t.Parallel()
	// Same tools in different orders must produce the same signature.
	calls1 := []ToolCall{
		{ID: "c1", Name: "search", Arguments: `{"q":"a"}`},
		{ID: "c2", Name: "read", Arguments: `{"path":"x"}`},
		{ID: "c3", Name: "summarize", Arguments: `{"doc":"y"}`},
	}
	calls2 := []ToolCall{
		{ID: "c3", Name: "summarize", Arguments: `{"doc":"y"}`},
		{ID: "c1", Name: "search", Arguments: `{"q":"a"}`},
		{ID: "c2", Name: "read", Arguments: `{"path":"x"}`},
	}

	sig1 := toolCallSignature(calls1)
	sig2 := toolCallSignature(calls2)

	if sig1 != sig2 {
		t.Fatalf("signatures differ for same tools: %q vs %q", sig1, sig2)
	}

	// Verify order is alphabetical.
	parts := strings.Split(sig1, ",")
	if len(parts) != 3 {
		t.Fatalf("expected 3 tool names, got %d", len(parts))
	}
	if parts[0] != "read" || parts[1] != "search" || parts[2] != "summarize" {
		t.Fatalf("expected sorted order 'read,search,summarize', got %q", sig1)
	}
}

func TestToolCallSignature_DifferentTools(t *testing.T) {
	t.Parallel()
	calls1 := []ToolCall{{ID: "c1", Name: "search", Arguments: `{}`}}
	calls2 := []ToolCall{{ID: "c2", Name: "read", Arguments: `{}`}}
	calls3 := []ToolCall{
		{ID: "c3", Name: "search", Arguments: `{}`},
		{ID: "c4", Name: "read", Arguments: `{}`},
	}

	sig1 := toolCallSignature(calls1)
	sig2 := toolCallSignature(calls2)
	sig3 := toolCallSignature(calls3)

	if sig1 == sig2 {
		t.Fatal("expected different signatures for different tools, but got same")
	}
	if sig1 == sig3 {
		t.Fatal("expected different signature for different tool count")
	}
	if sig2 == sig3 {
		t.Fatal("expected different signature for different tool count")
	}
}

func TestToolCallSignature_Empty(t *testing.T) {
	t.Parallel()
	sig := toolCallSignature(nil)
	if sig != "" {
		t.Fatalf("expected empty signature for nil calls, got %q", sig)
	}
	sig = toolCallSignature([]ToolCall{})
	if sig != "" {
		t.Fatalf("expected empty signature for empty calls, got %q", sig)
	}
}

func TestToolCallSignature_SingleTool(t *testing.T) {
	t.Parallel()
	calls := []ToolCall{{ID: "c1", Name: "search", Arguments: `{}`}}
	sig := toolCallSignature(calls)
	if sig != "search" {
		t.Fatalf("expected 'search', got %q", sig)
	}
}

// --- isToolPermanentlyUnavailable tests ---

func TestIsToolPermanentlyUnavailable_MCPClosed(t *testing.T) {
	t.Parallel()
	if !isToolPermanentlyUnavailable(errors.New("mcp client closed: connection reset")) {
		t.Fatal("expected true for 'mcp client closed' error")
	}
}

func TestIsToolPermanentlyUnavailable_MCPClientClosed(t *testing.T) {
	t.Parallel()
	if !isToolPermanentlyUnavailable(errors.New("MCP client is closed")) {
		t.Fatal("expected true for 'MCP client is closed' error")
	}
}

func TestIsToolPermanentlyUnavailable_Other(t *testing.T) {
	t.Parallel()
	if isToolPermanentlyUnavailable(errors.New("rate limit exceeded")) {
		t.Fatal("expected false for non-MCP error")
	}
}

func TestIsToolPermanentlyUnavailable_Nil(t *testing.T) {
	t.Parallel()
	if isToolPermanentlyUnavailable(nil) {
		t.Fatal("expected false for nil error")
	}
}

// --- executeToolCalls tests ---

func TestExecuteToolCalls_Basic(t *testing.T) {
	t.Parallel()
	var executed int32

	tool := &Tool{
		Name:        "greet",
		Description: "Greets the user",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		},
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			executed++
			return "hello world", nil
		},
	}

	cfg := stubAgentConfig("exec_tool_basic", []*Tool{tool})
	cfg.MaxTurns = 5
	a := New(cfg)
	a.state.SetStatus(StatusRunning)

	earlyExit, err := a.executeToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Name: "greet", Arguments: `{"name":"test"}`},
	})
	if err != nil {
		t.Fatalf("executeToolCalls: unexpected error: %v", err)
	}
	if earlyExit != "" {
		t.Fatalf("expected no early exit, got: %q", earlyExit)
	}
	if executed != 1 {
		t.Fatalf("tool executed %d times, want 1", executed)
	}

	// Verify result was persisted.
	msgs := a.state.Messages()
	if len(msgs) == 0 {
		t.Fatal("expected persisted messages after tool execution")
	}
	last := msgs[len(msgs)-1]
	if last.Role != RoleTool {
		t.Fatalf("last message role = %q, want %q", last.Role, RoleTool)
	}
	if last.ToolCallID != "call_1" {
		t.Fatalf("ToolCallID = %q, want %q", last.ToolCallID, "call_1")
	}
	if last.Content != "hello world" {
		t.Fatalf("content = %q, want %q", last.Content, "hello world")
	}
}

func TestExecuteToolCalls_AllFail(t *testing.T) {
	t.Parallel()
	var executedFirst, executedSecond int32

	tool1 := &Tool{
		Name:        "fail_tool",
		Description: "Tool that always fails",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string"},
			},
		},
		Func: func(_ context.Context, _ json.RawMessage) (any, error) {
			executedFirst++
			return "", fmt.Errorf("boom")
		},
	}

	tool2 := &Tool{
		Name:        "also_fail",
		Description: "Another failing tool",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string"},
			},
		},
		Func: func(_ context.Context, _ json.RawMessage) (any, error) {
			executedSecond++
			return "", fmt.Errorf("kaboom")
		},
	}

	cfg := stubAgentConfig("exec_tool_fail_all", []*Tool{tool1, tool2})
	cfg.MaxTurns = 5
	a := New(cfg)
	a.state.SetStatus(StatusRunning)

	earlyExit, err := a.executeToolCalls(context.Background(), []ToolCall{
		{ID: "call_1", Name: "fail_tool", Arguments: `{"input":"x"}`},
		{ID: "call_2", Name: "also_fail", Arguments: `{"input":"y"}`},
	})
	if err != nil {
		t.Fatalf("executeToolCalls: unexpected error: %v", err)
	}
	if earlyExit != "" {
		t.Fatalf("expected no early exit, got: %q", earlyExit)
	}

	// Both tools should have been executed.
	if executedFirst != 1 {
		t.Fatalf("fail_tool executed %d times, want 1", executedFirst)
	}
	if executedSecond != 1 {
		t.Fatalf("also_fail executed %d times, want 1", executedSecond)
	}

	// Both error results should be persisted.
	msgs := a.state.Messages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 persisted error messages, got %d", len(msgs))
	}

	// First persisted: fail_tool error
	m1 := msgs[len(msgs)-2]
	if m1.Role != RoleTool || m1.ToolCallID != "call_1" {
		t.Fatalf("msg[%d]: expected tool role with call_1, got role=%q id=%q",
			len(msgs)-2, m1.Role, m1.ToolCallID)
	}

	// Second persisted: also_fail error
	m2 := msgs[len(msgs)-1]
	if m2.Role != RoleTool || m2.ToolCallID != "call_2" {
		t.Fatalf("msg[%d]: expected tool role with call_2, got role=%q id=%q",
			len(msgs)-1, m2.Role, m2.ToolCallID)
	}
}

func TestExecuteToolCalls_MultipleSerial(t *testing.T) {
	t.Parallel()
	var count atomic.Int32

	tool := &Tool{
		Name:        "counter",
		Description: "Increments counter",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string"},
			},
		},
		Func: func(_ context.Context, _ json.RawMessage) (any, error) {
			count.Add(1)
			return "ok", nil
		},
	}

	cfg := stubAgentConfig("exec_tool_serial", []*Tool{tool})
	cfg.MaxTurns = 5
	cfg.ExecutionMode = ModeSerial
	a := New(cfg)
	a.state.SetStatus(StatusRunning)

	calls := []ToolCall{
		{ID: "c1", Name: "counter", Arguments: `{"input":"a"}`},
		{ID: "c2", Name: "counter", Arguments: `{"input":"b"}`},
		{ID: "c3", Name: "counter", Arguments: `{"input":"c"}`},
	}
	earlyExit, err := a.executeToolCalls(context.Background(), calls)
	if err != nil {
		t.Fatalf("executeToolCalls: unexpected error: %v", err)
	}
	if earlyExit != "" {
		t.Fatalf("expected no early exit, got: %q", earlyExit)
	}
	if count.Load() != 3 {
		t.Fatalf("expected 3 tool executions, got %d", count.Load())
	}
}

// --- applyDefaultReserveTokens tests ---

func TestApplyDefaultReserveTokens_ExplicitValue(t *testing.T) {
	t.Parallel()
	result := applyDefaultReserveTokens(10000, 2000)
	if result != 2000 {
		t.Fatalf("expected 2000, got %d", result)
	}
}

func TestApplyDefaultReserveTokens_ContextWindowQuarter(t *testing.T) {
	t.Parallel()
	result := applyDefaultReserveTokens(10000, 0)
	if result != 2500 {
		t.Fatalf("expected 2500 (10000/4), got %d", result)
	}
}

func TestApplyDefaultReserveTokens_ZeroContextWindow(t *testing.T) {
	t.Parallel()
	result := applyDefaultReserveTokens(0, 0)
	if result != 0 {
		t.Fatalf("expected 0, got %d", result)
	}
}
