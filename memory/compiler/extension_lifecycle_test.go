package compiler

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// ---------------------------------------------------------------------------
// CompilerExtension.LifecycleHook
// ---------------------------------------------------------------------------

func TestCompilerExtension_LifecycleHook(t *testing.T) {
	comp := NewCompiler(Config{})
	ext := NewExtension(comp)
	hook := ext.LifecycleHook()
	if hook == nil {
		t.Fatal("expected non-nil LifecycleHook")
	}
}

func TestCompilerExtension_LifecycleHook_ReturnsCompilerHook(t *testing.T) {
	comp := NewCompiler(Config{})
	ext := NewExtension(comp)
	hook := ext.LifecycleHook()
	if hook == nil {
		t.Fatalf("expected non-nil LifecycleHook from compiler extension")
	}
}

// ---------------------------------------------------------------------------
// compilerHook.BeforeTurn
// ---------------------------------------------------------------------------

func TestCompilerHook_BeforeTurn_ReturnsNil(t *testing.T) {
	comp := NewCompiler(Config{})
	ext := NewExtension(comp)
	hook := ext.LifecycleHook()

	err := hook.BeforeTurn(context.Background(), &agentcore.AgentRunContext{
		Turn:  1,
		Input: "test query",
	})
	if err != nil {
		t.Fatalf("BeforeTurn returned error: %v", err)
	}
}

func TestCompilerHook_BeforeTurn_NilAgent(t *testing.T) {
	comp := NewCompiler(Config{})
	ext := NewExtension(comp)
	// agent is nil by default.
	hook := ext.LifecycleHook()

	err := hook.BeforeTurn(context.Background(), &agentcore.AgentRunContext{
		Turn:  1,
		Input: "test",
	})
	if err != nil {
		t.Fatalf("BeforeTurn should not fail when agent is nil: %v", err)
	}
}

// ---------------------------------------------------------------------------
// compilerHook.AfterTurn
// ---------------------------------------------------------------------------

func TestCompilerHook_AfterTurn_RecordsTrace(t *testing.T) {
	comp := NewCompiler(Config{})
	ext := NewExtension(comp)
	hook := ext.LifecycleHook()

	// Simulate a turn.
	arc := &agentcore.AgentRunContext{
		Turn:  1,
		Input: "test task",
		Messages: []agentcore.Message{
			{Role: "user", Content: "test task"},
			{Role: "assistant", Content: "completed successfully"},
		},
	}

	hook.BeforeTurn(context.Background(), arc)
	hook.AfterTurn(context.Background(), arc, agentcore.TurnInfo{HadToolCalls: false})

	// After turn, the compiler should have recorded statistics.
	stats := comp.Stats()
	if stats.TotalTraces != 1 {
		t.Fatalf("expected 1 trace, got %d", stats.TotalTraces)
	}
}

func TestCompilerHook_AfterTurn_EmptyMessages(t *testing.T) {
	comp := NewCompiler(Config{})
	ext := NewExtension(comp)
	hook := ext.LifecycleHook()

	arc := &agentcore.AgentRunContext{
		Turn:  1,
		Input: "test",
	}

	hook.BeforeTurn(context.Background(), arc)
	// Must not panic with empty Messages — no Stats assertion needed.
	hook.AfterTurn(context.Background(), arc, agentcore.TurnInfo{})
}
