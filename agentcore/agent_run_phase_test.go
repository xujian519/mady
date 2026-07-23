package agentcore

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
)

// runPreTurn helper: we set the agent to running state so runPreTurn can proceed.
func setupRunningAgent(t *testing.T, cfg Config) *Agent {
	t.Helper()
	a := New(cfg)
	a.state.SetStatus(StatusRunning)
	return a
}

func TestRunPreTurn_ExceedsMaxTurns(t *testing.T) {
	t.Parallel()
	a := setupRunningAgent(t, Config{
		ModelConfig: ModelConfig{
			Name:     "max_turns_test",
			Model:    "stub",
			Provider: &echoProvider{},
		},
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 2,
		},
	})

	// loopStartTurn=0, turn=3 → 3-0 > 2 → should exceed max turns
	err := a.runPreTurn(context.Background(), 0, 3)
	if err == nil {
		t.Fatal("expected error for exceeding max turns, got nil")
	}
	ne, ok := err.(*NodeError)
	if !ok {
		t.Fatalf("expected *NodeError, got %T: %v", err, err)
	}
	if ne.Err != ErrExceedMaxSteps {
		t.Fatalf("expected ErrExceedMaxSteps, got: %v", ne.Err)
	}
	if a.state.Status() != StatusError {
		t.Fatalf("expected StatusError after max turns exceeded, got: %s", a.state.Status())
	}
}

func TestRunPreTurn_BeforeTurnHook(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	hook := &testBeforeTurnHook{
		fn: func(ctx context.Context, arc *AgentRunContext) error {
			called.Add(1)
			if arc.Turn != 1 {
				t.Errorf("expected turn=1, got %d", arc.Turn)
			}
			return nil
		},
	}

	a := setupRunningAgent(t, Config{
		ModelConfig: ModelConfig{
			Name:     "before_turn_test",
			Model:    "stub",
			Provider: &echoProvider{},
		},
		Lifecycle: hook,
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 5,
		},
	})

	// First persist user input so arc.Messages is non-empty (optional, but realistic).
	_ = a.persistMessage(context.Background(), Message{Role: RoleUser, Content: "hello"})

	err := a.runPreTurn(context.Background(), 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called.Load() != 1 {
		t.Fatalf("BeforeTurn called %d times, want 1", called.Load())
	}
}

// testBeforeTurnHook is a minimal lifecycle hook that delegates BeforeTurn.
type testBeforeTurnHook struct {
	BaseLifecycleHook
	fn func(ctx context.Context, arc *AgentRunContext) error
}

func (h *testBeforeTurnHook) BeforeTurn(ctx context.Context, arc *AgentRunContext) error {
	return h.fn(ctx, arc)
}

func TestGuardTruncation_EmptyToolCallID(t *testing.T) {
	t.Parallel()
	a := setupRunningAgent(t, Config{
		ModelConfig: ModelConfig{
			Name:     "trunc_empty_id",
			Model:    "stub",
			Provider: &echoProvider{},
		},
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 5,
		},
	})

	// Invalid JSON args (truncated) with empty ID.
	resp := &ProviderResponse{
		FinishReason: "length",
		ToolCalls: []ToolCall{
			{ID: "", Name: "test_tool", Arguments: `{"input": "truncated`}, // invalid JSON
		},
	}

	handled, err := a.guardTruncation(context.Background(), 1, resp)
	if err != nil {
		t.Fatalf("guardTruncation should handle empty ID without error, got: %v", err)
	}
	if !handled {
		t.Fatal("guardTruncation should have handled (handled=true) for FinishReason=length + invalid args")
	}

	// Verify the tool result was persisted (message count should include it).
	msgs := a.state.Messages()
	if len(msgs) == 0 {
		t.Fatal("expected at least one persisted message after guardTruncation")
	}
	last := msgs[len(msgs)-1]
	if last.Role != RoleTool {
		t.Fatalf("last message role = %q, want %q", last.Role, RoleTool)
	}
	if last.ToolCallID != "" {
		t.Fatalf("expected empty ToolCallID for empty-ID tool call, got: %q", last.ToolCallID)
	}
}

func TestGuardTruncation_ValidToolCalls(t *testing.T) {
	t.Parallel()
	a := setupRunningAgent(t, Config{
		ModelConfig: ModelConfig{
			Name:     "trunc_valid",
			Model:    "stub",
			Provider: &echoProvider{},
		},
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 5,
		},
	})

	// Valid tool calls with FinishReason="stop" (not "length") → should NOT truncate.
	resp := &ProviderResponse{
		FinishReason: "stop",
		ToolCalls: []ToolCall{
			{ID: "call_1", Name: "test_tool", Arguments: `{"input":"valid"}`},
		},
	}

	handled, err := a.guardTruncation(context.Background(), 1, resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("guardTruncation should NOT handle valid tool calls (handled=false)")
	}
}

// TestGuardTruncation_FinishReasonStop_InvalidArgs also should not trigger
// because FinishReason != "length" is the dominant condition.
func TestGuardTruncation_FinishReasonNotLength(t *testing.T) {
	t.Parallel()
	a := setupRunningAgent(t, Config{
		ModelConfig: ModelConfig{
			Name:     "trunc_stop",
			Model:    "stub",
			Provider: &echoProvider{},
		},
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 5,
		},
	})

	resp := &ProviderResponse{
		FinishReason: "stop",
		ToolCalls: []ToolCall{
			{ID: "call_1", Name: "test_tool", Arguments: `{"input": "truncated`}, // invalid but FinishReason != "length"
		},
	}

	handled, err := a.guardTruncation(context.Background(), 1, resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("guardTruncation should NOT handle when FinishReason != 'length'")
	}
}

// TestGuardTruncation_InvalidJSON_FinishReasonLength triggers guard truncation
// with valid (non-empty) tool call ID to cover the normal truncation path.
func TestGuardTruncation_InvalidJSON_FinishReasonLength(t *testing.T) {
	t.Parallel()
	a := setupRunningAgent(t, Config{
		ModelConfig: ModelConfig{
			Name:     "trunc_invalid_json",
			Model:    "stub",
			Provider: &echoProvider{},
		},
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 5,
		},
	})

	resp := &ProviderResponse{
		FinishReason: "length",
		ToolCalls: []ToolCall{
			{ID: "call_1", Name: "test_tool", Arguments: `{"input": "truncated`}, // invalid JSON
		},
	}

	handled, err := a.guardTruncation(context.Background(), 1, resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("guardTruncation should handle FinishReason=length + invalid JSON args")
	}
}

// --- hasInvalidToolCallArgs edge cases ---

func TestHasInvalidToolCallArgs_ValidJSON(t *testing.T) {
	calls := []ToolCall{
		{ID: "c1", Name: "t1", Arguments: `{"a":1}`},
		{ID: "c2", Name: "t2", Arguments: `{"b":2}`},
	}
	if hasInvalidToolCallArgs(calls) {
		t.Fatal("expected false for valid JSON args")
	}
}

func TestHasInvalidToolCallArgs_EmptyArgs(t *testing.T) {
	calls := []ToolCall{
		{ID: "c1", Name: "t1", Arguments: ""},
	}
	if hasInvalidToolCallArgs(calls) {
		t.Fatal("expected false for empty args (considered valid)")
	}
}

func TestHasInvalidToolCallArgs_InvalidJSON(t *testing.T) {
	calls := []ToolCall{
		{ID: "c1", Name: "t1", Arguments: `{"a":1}`},
		{ID: "c2", Name: "t2", Arguments: `{"b":`}, // truncated JSON
	}
	if !hasInvalidToolCallArgs(calls) {
		t.Fatal("expected true when any tool call has invalid JSON args")
	}
}

func TestMarshalJSON_AgentErrorEvent_WithStackTrace(t *testing.T) {
	orig := NewAgentErrorEvent(ErrExceedMaxSteps)
	data, err := orig.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var restored AgentErrorEvent
	if err := restored.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if restored.EventKind() != orig.EventKind() {
		t.Fatalf("kind = %v, want %v", restored.EventKind(), orig.EventKind())
	}
}

// --- ToolCall JSON marshaling ---

func TestToolCall_MarshalUnmarshal(t *testing.T) {
	t.Parallel()
	orig := ToolCall{
		ID:        "call_abc123",
		Name:      "get_weather",
		Arguments: `{"city":"Beijing"}`,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var restored ToolCall
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if restored.ID != orig.ID {
		t.Fatalf("ID = %q, want %q", restored.ID, orig.ID)
	}
	if restored.Name != orig.Name {
		t.Fatalf("Name = %q, want %q", restored.Name, orig.Name)
	}
}
