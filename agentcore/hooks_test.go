package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestLoggingBeforeHook(t *testing.T) {
	hook := LoggingBeforeHook(slog.Default())
	hc := &HookContext{
		ToolName:  "test_tool",
		Arguments: json.RawMessage(`{"key":"value"}`),
		State:     &AgentState{},
	}
	err := hook(context.Background(), hc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRateLimitBeforeHook(t *testing.T) {
	hook := RateLimitBeforeHook(2, time.Minute)

	// First two should succeed
	if err := hook(nil, &HookContext{ToolName: "t1", State: &AgentState{}}); err != nil {
		t.Fatal("expected first call to succeed")
	}
	if err := hook(nil, &HookContext{ToolName: "t2", State: &AgentState{}}); err != nil {
		t.Fatal("expected second call to succeed")
	}

	// Third should fail
	if err := hook(nil, &HookContext{ToolName: "t3", State: &AgentState{}}); err == nil {
		t.Fatal("expected third call to be rate limited")
	}
}

func TestRateLimitBeforeHookRefill(t *testing.T) {
	hook := RateLimitBeforeHook(1, 50*time.Millisecond)

	if err := hook(nil, &HookContext{State: &AgentState{}}); err != nil {
		t.Fatal("expected first call to succeed")
	}
	// Should fail immediately
	if err := hook(nil, &HookContext{State: &AgentState{}}); err == nil {
		t.Fatal("expected second call to fail")
	}

	// Wait for refill
	time.Sleep(60 * time.Millisecond)
	if err := hook(nil, &HookContext{State: &AgentState{}}); err != nil {
		t.Fatal("expected call after refill to succeed")
	}
}

func TestLoggingAfterHook(t *testing.T) {
	hook := LoggingAfterHook(slog.Default())
	hc := &HookContext{
		ToolName: "test_tool",
		State:    &AgentState{},
	}
	// Should not panic
	hook(context.Background(), hc, "result", nil)
	hook(context.Background(), hc, "", errors.New("boom"))
}

func TestTimeoutMiddleware(t *testing.T) {
	mw := TimeoutMiddleware(100 * time.Millisecond)
	executed := false
	fn := mw(func(ctx context.Context, tc ToolCall) (string, error) {
		executed = true
		return "ok", nil
	})
	result, err := fn(context.Background(), ToolCall{Name: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("result = %q", result)
	}
	if !executed {
		t.Fatal("inner function should have been called")
	}
}

func TestTimeoutMiddlewareCancels(t *testing.T) {
	mw := TimeoutMiddleware(10 * time.Millisecond)
	fn := mw(func(ctx context.Context, tc ToolCall) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	_, err := fn(context.Background(), ToolCall{Name: "test"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestRetryMiddlewareSuccess(t *testing.T) {
	mw := RetryMiddleware(3, time.Millisecond)
	callCount := 0
	fn := mw(func(ctx context.Context, tc ToolCall) (string, error) {
		callCount++
		return "ok", nil
	})
	result, err := fn(context.Background(), ToolCall{Name: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("result = %q", result)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}
}

func TestRetryMiddlewareRetries(t *testing.T) {
	mw := RetryMiddleware(3, time.Millisecond)
	callCount := 0
	fn := mw(func(ctx context.Context, tc ToolCall) (string, error) {
		callCount++
		if callCount < 3 {
			return "", errors.New("transient error")
		}
		return "ok", nil
	})
	result, err := fn(context.Background(), ToolCall{Name: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("result = %q", result)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls, got %d", callCount)
	}
}

func TestRetryMiddlewareExhausted(t *testing.T) {
	mw := RetryMiddleware(2, time.Millisecond)
	fn := mw(func(ctx context.Context, tc ToolCall) (string, error) {
		return "", errors.New("persistent error")
	})
	_, err := fn(context.Background(), ToolCall{Name: "test"})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
}

func TestRetryMiddlewareContextCancelled(t *testing.T) {
	mw := RetryMiddleware(5, 100*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	fn := mw(func(ctx context.Context, tc ToolCall) (string, error) {
		return "", errors.New("fail")
	})
	_, err := fn(ctx, ToolCall{Name: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- deprecatedHookAdapter tests ---

func TestDeprecatedHookAdapter_BeforeToolExecution_BlocksTool(t *testing.T) {
	t.Parallel()
	adapter := &deprecatedHookAdapter{
		beforeToolCall: func(ctx context.Context, tc ToolCall) *ToolCallOverride {
			if tc.Name == "blocked_tool" {
				return &ToolCallOverride{Block: true, Result: "custom blocked message"}
			}
			return nil
		},
	}

	toolCalls := []ToolCall{
		{ID: "c1", Name: "allowed_tool", Arguments: `{}`},
		{ID: "c2", Name: "blocked_tool", Arguments: `{}`},
		{ID: "c3", Name: "another_allowed", Arguments: `{}`},
	}

	results := make([]ToolResult, len(toolCalls))
	tec := &ToolExecutionContext{ToolCalls: toolCalls, Results: results}

	err := adapter.BeforeToolExecution(context.Background(), nil, tec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// allowed_tool: should have empty result (not blocked)
	if tec.Results[0].ToolCallID != "" {
		t.Fatalf("allowed_tool should not be pre-populated, got ToolCallID=%q", tec.Results[0].ToolCallID)
	}

	// blocked_tool: should be pre-populated with blocked result
	if tec.Results[1].ToolCallID != "c2" {
		t.Fatalf("blocked_tool should have ToolCallID=%q, got %q", "c2", tec.Results[1].ToolCallID)
	}
	if tec.Results[1].Result != "custom blocked message" {
		t.Fatalf("blocked_tool result = %q, want %q", tec.Results[1].Result, "custom blocked message")
	}
	if tec.Results[1].Err != nil {
		t.Fatalf("blocked_tool should not have error when IsError is false, got: %v", tec.Results[1].Err)
	}

	// another_allowed: should have empty result (not blocked)
	if tec.Results[2].ToolCallID != "" {
		t.Fatalf("another_allowed should not be pre-populated, got ToolCallID=%q", tec.Results[2].ToolCallID)
	}

	// Verify blockedTools map tracks the blocked index.
	if !adapter.blockedTools[1] {
		t.Fatal("expected blockedTools[1] to be true for blocked_tool")
	}
}

func TestDeprecatedHookAdapter_BeforeToolExecution_BlocksTool_IsError(t *testing.T) {
	t.Parallel()
	adapter := &deprecatedHookAdapter{
		beforeToolCall: func(ctx context.Context, tc ToolCall) *ToolCallOverride {
			return &ToolCallOverride{Block: true, Result: "fatal error", IsError: true}
		},
	}

	toolCalls := []ToolCall{{ID: "c1", Name: "err_tool", Arguments: `{}`}}
	results := make([]ToolResult, len(toolCalls))
	tec := &ToolExecutionContext{ToolCalls: toolCalls, Results: results}

	err := adapter.BeforeToolExecution(context.Background(), nil, tec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tec.Results[0].Err == nil {
		t.Fatal("expected error when IsError is true")
	}
	if tec.Results[0].Err.Error() != "fatal error" {
		t.Fatalf("err = %q, want %q", tec.Results[0].Err.Error(), "fatal error")
	}
}

func TestDeprecatedHookAdapter_BeforeToolExecution_NilHook(t *testing.T) {
	t.Parallel()
	adapter := &deprecatedHookAdapter{beforeToolCall: nil}

	toolCalls := []ToolCall{{ID: "c1", Name: "tool1", Arguments: `{}`}}
	results := make([]ToolResult, len(toolCalls))
	tec := &ToolExecutionContext{ToolCalls: toolCalls, Results: results}

	err := adapter.BeforeToolExecution(context.Background(), nil, tec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tec.Results[0].ToolCallID != "" {
		t.Fatal("expected empty result when beforeToolCall is nil")
	}
}

func TestDeprecatedHookAdapter_AfterToolExecution_ModifiesResult(t *testing.T) {
	t.Parallel()
	adapter := &deprecatedHookAdapter{
		afterToolCall: func(ctx context.Context, tc ToolCall, result *ToolResult) *ToolResult {
			if tc.Name == "modify_me" {
				modified := *result
				modified.Result = "modified: " + result.Result
				return &modified
			}
			return nil
		},
	}

	toolCalls := []ToolCall{
		{ID: "c1", Name: "modify_me", Arguments: `{}`},
		{ID: "c2", Name: "leave_alone", Arguments: `{}`},
	}

	results := []ToolResult{
		{ToolCallID: "c1", ToolName: "modify_me", Result: "original"},
		{ToolCallID: "c2", ToolName: "leave_alone", Result: "untouched"},
	}
	adapter.blockedTools = make(map[int]bool)

	tec := &ToolExecutionContext{ToolCalls: toolCalls, Results: results}
	adapter.AfterToolExecution(context.Background(), nil, tec)

	if tec.Results[0].Result != "modified: original" {
		t.Fatalf("result[0] = %q, want %q", tec.Results[0].Result, "modified: original")
	}
	if tec.Results[1].Result != "untouched" {
		t.Fatalf("result[1] = %q, want %q (should be unchanged)", tec.Results[1].Result, "untouched")
	}
}

func TestDeprecatedHookAdapter_AfterToolExecution_SkipsBlocked(t *testing.T) {
	t.Parallel()
	adapter := &deprecatedHookAdapter{
		afterToolCall: func(ctx context.Context, tc ToolCall, result *ToolResult) *ToolResult {
			modified := *result
			modified.Result = "should not happen"
			return &modified
		},
	}
	// Mark index 0 as blocked.
	adapter.blockedTools = map[int]bool{0: true}

	toolCalls := []ToolCall{
		{ID: "c1", Name: "blocked_tool", Arguments: `{}`},
		{ID: "c2", Name: "active_tool", Arguments: `{}`},
	}
	results := []ToolResult{
		{ToolCallID: "c1", ToolName: "blocked_tool", Result: "blocked"},
		{ToolCallID: "c2", ToolName: "active_tool", Result: "real_result"},
	}

	tec := &ToolExecutionContext{ToolCalls: toolCalls, Results: results}
	adapter.AfterToolExecution(context.Background(), nil, tec)

	// Blocked tool must retain its original result.
	if tec.Results[0].Result != "blocked" {
		t.Fatalf("blocked result should not be modified, got %q", tec.Results[0].Result)
	}
	// Active tool should be modified.
	if tec.Results[1].Result != "should not happen" {
		t.Fatalf("active tool result should be modified, got %q", tec.Results[1].Result)
	}
}

func TestDeprecatedHookAdapter_PostProcessResults(t *testing.T) {
	t.Parallel()
	adapter := &deprecatedHookAdapter{
		postProcessResults: func(ctx context.Context, calls []ToolCall, results []ToolResult) []ToolResult {
			// Remove all results for tools named "remove_me".
			filtered := make([]ToolResult, 0, len(results))
			for i, r := range results {
				if calls[i].Name != "remove_me" {
					filtered = append(filtered, r)
				}
			}
			return filtered
		},
	}

	toolCalls := []ToolCall{
		{ID: "c1", Name: "keep", Arguments: `{}`},
		{ID: "c2", Name: "remove_me", Arguments: `{}`},
		{ID: "c3", Name: "keep_too", Arguments: `{}`},
	}
	results := []ToolResult{
		{ToolCallID: "c1", Result: "result1"},
		{ToolCallID: "c2", Result: "result2"},
		{ToolCallID: "c3", Result: "result3"},
	}

	tec := &ToolExecutionContext{ToolCalls: toolCalls, Results: results}
	adapter.AfterToolExecution(context.Background(), nil, tec)

	if len(tec.Results) != 2 {
		t.Fatalf("expected 2 results after post-processing, got %d", len(tec.Results))
	}
	if tec.Results[0].ToolCallID != "c1" || tec.Results[1].ToolCallID != "c3" {
		t.Fatalf("expected remaining results to be c1 and c3, got %q and %q",
			tec.Results[0].ToolCallID, tec.Results[1].ToolCallID)
	}
}
