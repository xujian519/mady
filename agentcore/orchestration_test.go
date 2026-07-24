package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestOrchestrationManifest(t *testing.T) {
	m := &OrchestrationManifest{
		ID:          "test_flow",
		Name:        "测试工作流",
		Description: "用于测试的编排",
		Steps: []OrchestrationStep{
			{ToolName: "step1", Description: "第一步"},
		},
	}

	if m.ID != "test_flow" {
		t.Errorf("expected ID 'test_flow', got %q", m.ID)
	}
	if len(m.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(m.Steps))
	}
}

func TestOrchestrationStep_Condition(t *testing.T) {
	state := OrchestrationState{"flag": true}

	called := false
	step := OrchestrationStep{
		ToolName:    "cond_step",
		Description: "条件步骤",
		Condition: func(s map[string]any) bool {
			called = true
			v, ok := s["flag"]
			return ok && v.(bool)
		},
	}

	if !step.Condition(state) {
		t.Error("condition should return true when flag is true")
	}
	if !called {
		t.Error("condition should have been called")
	}

	state["flag"] = false
	if step.Condition(state) {
		t.Error("condition should return false when flag is false")
	}
}

func TestOrchestrationExecutor_SequentialSteps(t *testing.T) {
	// Create stub tools that record their invocation.
	invocations := make(map[string]int)

	tool1 := &Tool{
		Name:        "step1",
		Description: "First step",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			invocations["step1"]++
			return map[string]any{"result": "step1_done"}, nil
		},
	}

	tool2 := &Tool{
		Name:        "step2",
		Description: "Second step",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			invocations["step2"]++
			return map[string]any{"result": "step2_done"}, nil
		},
	}

	agent := New(stubAgentConfig("test", []*Tool{tool1, tool2}))

	manifest := &OrchestrationManifest{
		ID:   "test_sequential",
		Name: "顺序执行测试",
		Steps: []OrchestrationStep{
			{ToolName: "step1", Description: "步骤1", InputKey: "_input"},
			{ToolName: "step2", Description: "步骤2", InputKey: "step1"},
		},
	}

	executor := NewOrchestrationExecutor(agent)
	state := OrchestrationState{
		"_input": map[string]any{"key": "value"},
	}

	result, err := executor.Run(context.Background(), manifest, state)
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	if !result.Success {
		t.Error("expected successful execution")
	}
	if result.StepsCompleted != 2 {
		t.Errorf("expected 2 completed steps, got %d", result.StepsCompleted)
	}
	if invocations["step1"] != 1 {
		t.Errorf("step1 should be invoked once, got %d", invocations["step1"])
	}
	if invocations["step2"] != 1 {
		t.Errorf("step2 should be invoked once, got %d", invocations["step2"])
	}
}

func TestOrchestrationExecutor_ConditionalSkip(t *testing.T) {
	tool := &Tool{
		Name:        "always_run",
		Description: "Always runs",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return "ran", nil
		},
	}
	skippedTool := &Tool{
		Name:        "skip_me",
		Description: "Should be skipped",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return "should_not_run", nil
		},
	}

	agent := New(stubAgentConfig("test_cond", []*Tool{tool, skippedTool}))

	manifest := &OrchestrationManifest{
		ID:   "test_conditional",
		Name: "条件跳过测试",
		Steps: []OrchestrationStep{
			{ToolName: "always_run", Description: "始终执行"},
			{
				ToolName:    "skip_me",
				Description: "条件不满足时跳过",
				Condition: func(s map[string]any) bool {
					return false // always skip
				},
			},
		},
	}

	executor := NewOrchestrationExecutor(agent)
	result, err := executor.Run(context.Background(), manifest, nil)
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	if !result.Success {
		t.Error("expected successful execution (skip is not an error)")
	}
	if result.StepsCompleted != 1 {
		t.Errorf("expected 1 completed step, got %d", result.StepsCompleted)
	}
	if result.StepsSkipped != 1 {
		t.Errorf("expected 1 skipped step, got %d", result.StepsSkipped)
	}
}

func TestOrchestrationExecutor_OptionalStepFailure(t *testing.T) {
	requiredTool := &Tool{
		Name:        "required_step",
		Description: "Must succeed",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return "ok", nil
		},
	}
	optionalTool := &Tool{
		Name:        "optional_step",
		Description: "Can fail",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return nil, fmt.Errorf("optional tool failure")
		},
	}

	agent := New(stubAgentConfig("test_optional", []*Tool{requiredTool, optionalTool}))

	manifest := &OrchestrationManifest{
		ID:   "test_optional",
		Name: "可选步骤测试",
		Steps: []OrchestrationStep{
			{ToolName: "optional_step", Description: "可能失败", Optional: true},
			{ToolName: "required_step", Description: "必须成功"},
		},
	}

	executor := NewOrchestrationExecutor(agent)
	result, err := executor.Run(context.Background(), manifest, nil)
	if err != nil {
		t.Fatalf("execution should not fail when optional step errors: %v", err)
	}

	if !result.Success {
		t.Error("expected success despite optional step failure")
	}
	if result.StepsCompleted != 1 {
		t.Errorf("expected 1 completed step (required), got %d", result.StepsCompleted)
	}
	if errMsg, ok := result.StepErrors["optional_step"]; !ok {
		t.Error("expected error recorded for optional step")
	} else if errMsg == "" {
		t.Error("expected non-empty error message")
	}
}

func TestConditionFunc_NilIsAlwaysTrue(t *testing.T) {
	// A nil Condition means "always execute".
	step := OrchestrationStep{
		ToolName:    "unconditional",
		Description: "无条件步骤",
		Condition:   nil,
	}
	// Condition == nil is the signal for "always"; no need to call it.
	if step.Condition != nil {
		t.Error("nil condition should remain nil (executor treats nil as always-true)")
	}
}

func TestOrchestrationExecutor_InterruptPausesAndReturnsPartialState(t *testing.T) {
	// Step 1: succeeds normally.
	tool1 := &Tool{
		Name:        "step1",
		Description: "First step",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return map[string]any{"result": "step1_done"}, nil
		},
	}
	// Step 2: returns an interrupt (simulates "waiting for user confirmation").
	tool2 := &Tool{
		Name:        "step2",
		Description: "Second step (interrupts)",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return "step2_partial_result", NewInterruptError("需要用户确认分析结果")
		},
	}
	// Step 3: should not execute (orchestration pauses at step 2).
	tool3 := &Tool{
		Name:        "step3",
		Description: "Third step",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			t.Error("step3 should not be called when interrupted at step2")
			return nil, nil
		},
	}

	agent := New(stubAgentConfig("test_interrupt", []*Tool{tool1, tool2, tool3}))

	manifest := &OrchestrationManifest{
		ID:   "test_interrupt",
		Name: "中断测试",
		Steps: []OrchestrationStep{
			{ToolName: "step1", Description: "第一步"},
			{ToolName: "step2", Description: "第二步（中断）"},
			{ToolName: "step3", Description: "第三步（不应执行）"},
		},
	}

	executor := NewOrchestrationExecutor(agent)
	result, err := executor.Run(context.Background(), manifest, nil)
	if err != nil {
		t.Fatalf("interrupt should not propagate as error: %v", err)
	}

	if !result.Success {
		t.Error("interrupted orchestration should still have Success=true (paused, not failed)")
	}
	if result.InterruptedStep != "step2" {
		t.Errorf("expected InterruptedStep 'step2', got %q", result.InterruptedStep)
	}
	if result.StepsCompleted != 2 {
		t.Errorf("expected 2 completed steps (step1 ran, step2 interrupted after invocation), got %d", result.StepsCompleted)
	}
	if result.PendingReview == "" {
		t.Error("expected non-empty PendingReview with step2's output")
	}
	if result.PartialState == nil {
		t.Error("expected non-nil PartialState for resuming")
	}
	// PartialState should contain step1's output.
	if _, ok := result.PartialState["step1"]; !ok {
		t.Error("PartialState should contain step1's output")
	}
}
