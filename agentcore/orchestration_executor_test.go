package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// TestOrchestrationExecutor_DefaultDepthAllowsFlatManifest verifies that a
// single-level orchestration (no nested run_orchestration) succeeds with the
// default depth budget.
func TestOrchestrationExecutor_DefaultDepthAllowsFlatManifest(t *testing.T) {
	tool := &Tool{
		Name:        "leaf",
		Description: "Leaf step",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	}

	agent := New(stubAgentConfig("depth-flat", []*Tool{tool}))
	executor := NewOrchestrationExecutor(agent)

	manifest := &OrchestrationManifest{
		ID:   "flat",
		Name: "单层编排",
		Steps: []OrchestrationStep{
			{ToolName: "leaf", Description: "叶子步骤"},
		},
	}

	result, err := executor.Run(context.Background(), manifest, nil)
	if err != nil {
		t.Fatalf("flat orchestration should not error: %v", err)
	}
	if !result.Success {
		t.Error("expected flat orchestration to succeed")
	}
	if result.StepsCompleted != 1 {
		t.Errorf("expected 1 completed step, got %d", result.StepsCompleted)
	}
}

// TestOrchestrationExecutor_NestedWithinLimit verifies that a nested
// orchestration succeeds when it stays within the configured depth limit.
func TestOrchestrationExecutor_NestedWithinLimit(t *testing.T) {
	leafCalls := 0
	leaf := &Tool{
		Name:        "leaf",
		Description: "Leaf step",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			leafCalls++
			return map[string]any{"ok": true}, nil
		},
	}

	agent := New(stubAgentConfig("depth-nested-ok", []*Tool{leaf}))

	innerManifest := &OrchestrationManifest{
		ID:   "inner",
		Name: "内层编排",
		Steps: []OrchestrationStep{
			{ToolName: "leaf", Description: "内层叶子"},
		},
	}

	nested := &Tool{
		Name:        "nested",
		Description: "Runs inner orchestration",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			exec := NewOrchestrationExecutorWithMaxDepth(agent, 2)
			res, err := exec.Run(ctx, innerManifest, nil)
			if err != nil {
				return nil, err
			}
			return res, nil
		},
	}

	// Re-build agent so the registry knows about the nested tool.
	agent = New(stubAgentConfig("depth-nested-ok", []*Tool{leaf, nested}))

	outerManifest := &OrchestrationManifest{
		ID:   "outer",
		Name: "外层编排",
		Steps: []OrchestrationStep{
			{ToolName: "nested", Description: "调用内层编排"},
		},
	}

	executor := NewOrchestrationExecutorWithMaxDepth(agent, 2)
	result, err := executor.Run(context.Background(), outerManifest, nil)
	if err != nil {
		t.Fatalf("nested orchestration within limit should succeed: %v", err)
	}
	if !result.Success {
		t.Error("expected nested orchestration to succeed")
	}
	if leafCalls != 1 {
		t.Errorf("expected leaf to be invoked once, got %d", leafCalls)
	}
}

// TestOrchestrationExecutor_NestedExceedsLimit verifies that nested
// orchestrations are rejected once the configured depth limit is exceeded.
func TestOrchestrationExecutor_NestedExceedsLimit(t *testing.T) {
	leaf := &Tool{
		Name:        "leaf",
		Description: "Leaf step",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return map[string]any{"ok": true}, nil
		},
	}

	agent := New(stubAgentConfig("depth-nested-fail", []*Tool{leaf}))

	innerManifest := &OrchestrationManifest{
		ID:   "inner",
		Name: "内层编排",
		Steps: []OrchestrationStep{
			{ToolName: "leaf", Description: "内层叶子"},
		},
	}

	nested := &Tool{
		Name:        "nested",
		Description: "Runs inner orchestration",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			// Limit is 1, so the inner Run (depth=2) must fail.
			exec := NewOrchestrationExecutorWithMaxDepth(agent, 1)
			return exec.Run(ctx, innerManifest, nil)
		},
	}

	agent = New(stubAgentConfig("depth-nested-fail", []*Tool{leaf, nested}))

	outerManifest := &OrchestrationManifest{
		ID:   "outer",
		Name: "外层编排",
		Steps: []OrchestrationStep{
			{ToolName: "nested", Description: "调用内层编排"},
		},
	}

	executor := NewOrchestrationExecutorWithMaxDepth(agent, 1)
	result, err := executor.Run(context.Background(), outerManifest, nil)
	if err == nil {
		t.Fatal("expected error when nested orchestration exceeds depth limit")
	}
	if result != nil && result.Success {
		t.Error("expected result.Success to be false when depth exceeded")
	}
	if !errors.Is(err, ErrDepthExceeded) {
		t.Errorf("expected ErrDepthExceeded, got %v", err)
	}
}

// TestOrchestrationExecutor_DefaultLimitBoundsRecursion verifies the default
// depth budget (8) using a self-referencing orchestration. The recursion stops
// cleanly at the 9th nesting attempt and surfaces ErrDepthExceeded.
//
// Note: agent is declared as var before the closure (step ①) because the
// self_referencing tool's Func must capture the agent variable; agent cannot
// be created until after the tool is built (so it can be registered). This
// creates a chicken-and-egg dependency resolved by declaring var first, then
// building the tool closure (capturing the var), then assigning the var.
func TestOrchestrationExecutor_DefaultLimitBoundsRecursion(t *testing.T) {
	var agent *Agent // ① 先声明 — 下面的闭包捕获这个变量

	selfManifest := &OrchestrationManifest{
		ID:   "self",
		Name: "自递归编排",
		Steps: []OrchestrationStep{
			{ToolName: "self_nested", Description: "递归步骤"},
		},
	}

	selfNested := &Tool{
		Name:        "self_nested",
		Description: "Calls run_orchestration recursively",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			exec := NewOrchestrationExecutor(agent)
			return exec.Run(ctx, selfManifest, nil)
		},
	}

	agent = New(stubAgentConfig("depth-recursion", []*Tool{selfNested}))

	executor := NewOrchestrationExecutor(agent)
	result, err := executor.Run(context.Background(), selfManifest, nil)
	if err == nil {
		t.Fatal("expected recursion to be bounded by the default depth limit")
	}
	if result != nil && result.Success {
		t.Error("expected result.Success to be false when recursion bound hit")
	}
	if !errors.Is(err, ErrDepthExceeded) {
		t.Errorf("expected ErrDepthExceeded, got %v", err)
	}
}
