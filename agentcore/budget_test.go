package agentcore

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBudgetIsUnlimited(t *testing.T) {
	if !(Budget{}).IsUnlimited() {
		t.Fatal("zero budget should be unlimited")
	}
	if (Budget{MaxTokens: 1}).IsUnlimited() {
		t.Fatal("budget with a limit should not be unlimited")
	}
}

func TestBudgetExceededErrorUnwrap(t *testing.T) {
	wrapped := NewNodeError("lifecycle before_model_call failed",
		&BudgetExceededError{Dimension: BudgetDimTokens, Limit: 10, Used: 12},
		"agent", "turn:1")
	if !IsBudgetExceeded(wrapped) {
		t.Fatal("IsBudgetExceeded should detect wrapped budget error")
	}
	if !errors.Is(wrapped, ErrBudgetExceeded) {
		t.Fatal("errors.Is should match the sentinel through NewNodeError")
	}
	var be *BudgetExceededError
	if !errors.As(wrapped, &be) {
		t.Fatal("errors.As should extract *BudgetExceededError")
	}
	if be.Dimension != BudgetDimTokens {
		t.Fatalf("unexpected dimension %s", be.Dimension)
	}
}

// helper: simulate a successful model call that reports the given total tokens.
func recordModelCall(bc *BudgetController, totalTokens int64) {
	arc := &AgentRunContext{}
	bc.BeforeModelCall(context.Background(), arc, &ModelCallContext{Request: &ProviderRequest{}})
	bc.AfterModelCall(context.Background(), arc, &ModelCallContext{
		Response: &ProviderResponse{Usage: TokenUsage{TotalTokens: totalTokens}},
	})
}

func TestBudgetController_Tokens(t *testing.T) {
	bc := NewBudgetController(Budget{MaxTokens: 100})
	bc.BeforeAgentRun(context.Background(), &AgentRunContext{})

	// First call under budget (40 tokens) → allowed.
	recordModelCall(bc, 40)
	if bc.Usage().Tokens != 40 {
		t.Fatalf("expected 40 tokens, got %d", bc.Usage().Tokens)
	}

	// Second call (60 more = 100) still checked before the call, usage=40<100 → allowed.
	recordModelCall(bc, 60)

	// Third call: usage now 100 >= MaxTokens → exceeded before invoking.
	err := bc.BeforeModelCall(context.Background(), &AgentRunContext{}, &ModelCallContext{Request: &ProviderRequest{}})
	if !IsBudgetExceeded(err) {
		t.Fatalf("expected budget exceeded, got %v", err)
	}
}

func TestBudgetController_Calls(t *testing.T) {
	bc := NewBudgetController(Budget{MaxCalls: 2})
	bc.BeforeAgentRun(context.Background(), &AgentRunContext{})

	recordModelCall(bc, 0) // call 1
	recordModelCall(bc, 0) // call 2

	// Third call → 2 >= MaxCalls → exceeded.
	err := bc.BeforeModelCall(context.Background(), &AgentRunContext{}, &ModelCallContext{})
	if !IsBudgetExceeded(err) {
		t.Fatalf("expected call budget exceeded, got %v", err)
	}
	var be *BudgetExceededError
	errors.As(err, &be)
	if be.Dimension != BudgetDimCalls {
		t.Fatalf("expected calls dimension, got %s", be.Dimension)
	}
}

func TestBudgetController_Duration(t *testing.T) {
	bc := NewBudgetController(Budget{MaxDuration: 5 * time.Millisecond})
	bc.BeforeAgentRun(context.Background(), &AgentRunContext{})

	// Kick off the timer by recording one model call.
	recordModelCall(bc, 0)
	// Wait past the duration limit.
	time.Sleep(12 * time.Millisecond)

	err := bc.BeforeModelCall(context.Background(), &AgentRunContext{}, &ModelCallContext{})
	if !IsBudgetExceeded(err) {
		t.Fatalf("expected duration budget exceeded, got %v", err)
	}
}

func TestBudgetController_ToolCalls(t *testing.T) {
	bc := NewBudgetController(Budget{MaxToolCalls: 2})
	bc.BeforeAgentRun(context.Background(), &AgentRunContext{})

	// Batch of 2 tool calls: allowed (2 <= 2).
	err := bc.BeforeToolExecution(context.Background(), &AgentRunContext{}, &ToolExecutionContext{
		ToolCalls: []ToolCall{{ID: "1", Name: "t1"}, {ID: "2", Name: "t2"}},
	})
	if err != nil {
		t.Fatalf("first batch should be allowed, got %v", err)
	}
	// Simulate completion.
	bc.AfterToolExecution(context.Background(), &AgentRunContext{}, &ToolExecutionContext{
		Results: []ToolResult{{ToolCallID: "1"}, {ToolCallID: "2"}},
	})
	if bc.Usage().ToolCalls != 2 {
		t.Fatalf("expected 2 tool calls used, got %d", bc.Usage().ToolCalls)
	}

	// Next batch of 1 would push to 3 > 2 → exceeded.
	err = bc.BeforeToolExecution(context.Background(), &AgentRunContext{}, &ToolExecutionContext{
		ToolCalls: []ToolCall{{ID: "3", Name: "t3"}},
	})
	if !IsBudgetExceeded(err) {
		t.Fatalf("expected tool-call budget exceeded, got %v", err)
	}
}

func TestBudgetController_UnlimitedNeverExceeds(t *testing.T) {
	bc := NewBudgetController(Budget{}) // unlimited
	bc.BeforeAgentRun(context.Background(), &AgentRunContext{})

	for i := 0; i < 50; i++ {
		recordModelCall(bc, 1000)
	}
	if err := bc.BeforeModelCall(context.Background(), &AgentRunContext{}, &ModelCallContext{}); err != nil {
		t.Fatalf("unlimited budget should never exceed, got %v", err)
	}
}

func TestBudgetController_OnExceedCallback(t *testing.T) {
	var triggered bool
	var seenDim BudgetDimension
	bc := NewBudgetController(Budget{MaxCalls: 1})
	bc.OnExceed = func(dim BudgetDimension, _ Budget, _ BudgetUsage) {
		triggered = true
		seenDim = dim
	}
	bc.BeforeAgentRun(context.Background(), &AgentRunContext{})
	recordModelCall(bc, 0) // call 1

	err := bc.BeforeModelCall(context.Background(), &AgentRunContext{}, &ModelCallContext{})
	if !triggered {
		t.Fatal("OnExceed callback should fire on breach")
	}
	if seenDim != BudgetDimCalls {
		t.Fatalf("expected calls dimension in callback, got %s", seenDim)
	}
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBudgetController_Reset(t *testing.T) {
	bc := NewBudgetController(Budget{MaxCalls: 3})
	bc.BeforeAgentRun(context.Background(), &AgentRunContext{})
	recordModelCall(bc, 0)
	recordModelCall(bc, 0)
	if bc.Usage().Calls != 2 {
		t.Fatalf("expected 2 calls, got %d", bc.Usage().Calls)
	}
	bc.Reset()
	if bc.Usage().Calls != 0 {
		t.Fatalf("expected 0 calls after reset, got %d", bc.Usage().Calls)
	}
}

func TestWithBudgetComposesLifecycle(t *testing.T) {
	existing := &AuditHook{}
	cfg := Config{}
	WithLifecycle(existing)(&cfg)
	WithBudget(Budget{MaxCalls: 1})(&cfg)

	chain, ok := cfg.Lifecycle.(LifecycleChain)
	if !ok {
		t.Fatalf("expected LifecycleChain, got %T", cfg.Lifecycle)
	}
	if len(chain) != 2 {
		t.Fatalf("expected chain of 2, got %d", len(chain))
	}
	if chain[0] != existing {
		t.Fatal("existing hook should be preserved first")
	}
	if _, ok := chain[1].(*BudgetController); !ok {
		t.Fatalf("expected BudgetController second, got %T", chain[1])
	}
}

func TestWithBudgetSetsLifecycleWhenNone(t *testing.T) {
	cfg := Config{}
	WithBudget(Budget{MaxTokens: 100})(&cfg)
	if _, ok := cfg.Lifecycle.(*BudgetController); !ok {
		t.Fatalf("expected BudgetController, got %T", cfg.Lifecycle)
	}
}
