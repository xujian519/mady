package doomloop

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// ============================================================================
// ToolCallLoopDetector tests
// ============================================================================

func TestToolCallLoopDetector_ID(t *testing.T) {
	d := toolCallLoopDetector{max: 3}
	if id := d.ID(); id != DetectorToolCallLoop {
		t.Errorf("ID() = %q, want %q", id, DetectorToolCallLoop)
	}
}

func TestToolCallLoopDetector_NoSignal(t *testing.T) {
	d := toolCallLoopDetector{max: 3}
	for i := 0; i < 2; i++ {
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				ToolCalls: []agentcore.ToolCall{
					{Name: "search", Arguments: `{"q":"test"}`},
				},
			},
		}
		if sig := d.RecordModelCall(mcc); sig != nil {
			t.Fatalf("unexpected signal on iteration %d", i)
		}
	}
}

func TestToolCallLoopDetector_Triggers(t *testing.T) {
	d := toolCallLoopDetector{max: 3}
	tc := agentcore.ToolCall{Name: "search", Arguments: `{"q":"test"}`}
	for i := 0; i < 3; i++ {
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				ToolCalls: []agentcore.ToolCall{tc},
			},
		}
		sig := d.RecordModelCall(mcc)
		if i < 2 && sig != nil {
			t.Fatalf("unexpected signal on iteration %d", i)
		}
		if i == 2 {
			if sig == nil {
				t.Fatal("expected signal on 3rd iteration")
			}
			if sig.Detector != DetectorToolCallLoop {
				t.Errorf("detector = %q, want %q", sig.Detector, DetectorToolCallLoop)
			}
			if !sig.Fatal {
				t.Error("expected fatal signal")
			}
		}
	}
}

func TestToolCallLoopDetector_Reset(t *testing.T) {
	d := toolCallLoopDetector{max: 2}
	tc := agentcore.ToolCall{Name: "test", Arguments: "{}"}
	mcc := &agentcore.ModelCallContext{
		Response: &agentcore.ProviderResponse{ToolCalls: []agentcore.ToolCall{tc}},
	}
	d.RecordModelCall(mcc)
	d.Reset()
	if sig := d.RecordModelCall(mcc); sig != nil {
		t.Error("expected no signal after reset")
	}
}

// ============================================================================
// TextRepetitionDetector tests
// ============================================================================

func TestTextRepetitionDetector_ID(t *testing.T) {
	d := textRepetitionDetector{minRepeat: 3}
	if id := d.ID(); id != DetectorTextRepetition {
		t.Errorf("ID() = %q", id)
	}
}

func TestTextRepetitionDetector_Triggers(t *testing.T) {
	d := textRepetitionDetector{minRepeat: 3}
	for i := 0; i < 3; i++ {
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				Content: "Same line repeated",
			},
		}
		sig := d.RecordModelCall(mcc)
		if i == 2 {
			if sig == nil {
				t.Fatal("expected signal on 3rd repetition")
			}
			if sig.Detector != DetectorTextRepetition {
				t.Errorf("detector = %q", sig.Detector)
			}
		}
	}
}

func TestTextRepetitionDetector_DifferentText(t *testing.T) {
	d := textRepetitionDetector{minRepeat: 3}
	for i := 0; i < 3; i++ {
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				Content: "Different line " + string(rune('A'+i)),
			},
		}
		if sig := d.RecordModelCall(mcc); sig != nil {
			t.Fatal("expected no signal for different text")
		}
	}
}

// ============================================================================
// CycleDetector tests
// ============================================================================

func TestCycleDetector_ID(t *testing.T) {
	d := cycleDetector{cycleLen: 2}
	if id := d.ID(); id != DetectorCycle {
		t.Errorf("ID() = %q", id)
	}
}

func TestCycleDetector_Triggers(t *testing.T) {
	d := cycleDetector{cycleLen: 2}
	// Pattern: A, B, A, B → detect A→B cycle
	for _, name := range []string{"A", "B", "A", "B"} {
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				ToolCalls: []agentcore.ToolCall{{Name: name}},
			},
		}
		sig := d.RecordModelCall(mcc)
		if name == "B" && len(d.history) == 4 {
			if sig == nil {
				t.Fatal("expected cycle signal on 4th call")
			}
		}
	}
}

func TestCycleDetector_NoCycle(t *testing.T) {
	d := cycleDetector{cycleLen: 2}
	for _, name := range []string{"A", "B", "C", "D"} {
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				ToolCalls: []agentcore.ToolCall{{Name: name}},
			},
		}
		if sig := d.RecordModelCall(mcc); sig != nil {
			t.Fatal("expected no signal for linear sequence")
		}
	}
}

// ============================================================================
// EmptyResultDetector tests
// ============================================================================

func TestEmptyResultDetector_ID(t *testing.T) {
	d := emptyResultDetector{max: 3}
	if id := d.ID(); id != DetectorEmptyResult {
		t.Errorf("ID() = %q", id)
	}
}

func TestEmptyResultDetector_Triggers(t *testing.T) {
	d := emptyResultDetector{max: 3}
	for i := 0; i < 3; i++ {
		tec := &agentcore.ToolExecutionContext{
			Results: []agentcore.ToolResult{
				{Result: ""},
			},
		}
		sig := d.RecordToolResult(tec)
		if i == 2 {
			if sig == nil {
				t.Fatal("expected signal on 3rd empty result")
			}
		}
	}
}

func TestEmptyResultDetector_WithContent(t *testing.T) {
	d := emptyResultDetector{max: 3}
	// Having a non-empty result should reset the counter.
	tec := &agentcore.ToolExecutionContext{
		Results: []agentcore.ToolResult{
			{Result: "some data"},
		},
	}
	if sig := d.RecordToolResult(tec); sig != nil {
		t.Fatal("unexpected signal for non-empty result")
	}
	// Now empty after content should restart counting.
	emptyTEC := &agentcore.ToolExecutionContext{
		Results: []agentcore.ToolResult{{Result: ""}},
	}
	for i := 0; i < 2; i++ {
		if sig := d.RecordToolResult(emptyTEC); sig != nil {
			t.Fatal("unexpected signal on first empty after content")
		}
	}
}

// ============================================================================
// CircuitBreaker tests
// ============================================================================

func TestCircuitBreaker_ID(t *testing.T) {
	d := circuitBreaker{max: 5}
	if id := d.ID(); id != DetectorCircuitBreaker {
		t.Errorf("ID() = %q", id)
	}
}

func TestCircuitBreaker_Triggers(t *testing.T) {
	d := circuitBreaker{max: 5}
	for i := 0; i < 3; i++ {
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				ToolCalls: []agentcore.ToolCall{
					{Name: "tool1"},
					{Name: "tool2"},
				},
			},
		}
		sig := d.RecordModelCall(mcc)
		// Each call adds 2 tools. After 3 calls: 6 tools > 5.
		if i == 2 {
			if sig == nil {
				t.Fatal("expected signal on 6th tool call (max 5)")
			}
		}
	}
}

func TestCircuitBreaker_UnderLimit(t *testing.T) {
	d := circuitBreaker{max: 10}
	for i := 0; i < 3; i++ {
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				ToolCalls: []agentcore.ToolCall{{Name: "tool"}},
			},
		}
		if sig := d.RecordModelCall(mcc); sig != nil {
			t.Fatal("unexpected signal under limit")
		}
	}
}

// ============================================================================
// CompactionBreaker tests
// ============================================================================

func TestCompactionBreaker_ID(t *testing.T) {
	d := compactionBreaker{max: 3}
	if id := d.ID(); id != DetectorCompactionBreaker {
		t.Errorf("ID() = %q", id)
	}
}

func TestCompactionBreaker_Triggers(t *testing.T) {
	d := compactionBreaker{max: 3}
	for i := 0; i < 3; i++ {
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				Content: "【总结】这是前面讨论的摘要",
			},
		}
		sig := d.RecordModelCall(mcc)
		if i == 2 {
			if sig == nil {
				t.Fatal("expected signal on 3rd compaction")
			}
		}
	}
}

func TestCompactionBreaker_WithToolCall(t *testing.T) {
	d := compactionBreaker{max: 3}
	// Compaction with tool call should not count as compaction-only.
	mcc := &agentcore.ModelCallContext{
		Response: &agentcore.ProviderResponse{
			Content:   "【总结】摘要",
			ToolCalls: []agentcore.ToolCall{{Name: "search"}},
		},
	}
	if sig := d.RecordModelCall(mcc); sig != nil {
		t.Fatal("unexpected signal when tool calls are present")
	}
}

// ============================================================================
// DoomLoop coordinator tests
// ============================================================================

func TestNew_Defaults(t *testing.T) {
	dl := New()
	if len(dl.detectors) != 6 {
		t.Errorf("expected 6 detectors with defaults, got %d", len(dl.detectors))
	}
}

func TestNew_WithOptions(t *testing.T) {
	dl := New(WithToolCallLoop(2), WithCircuitBreaker(10))
	// All 6 detectors are always enabled (defaults) — options just tweak limits.
	if len(dl.detectors) != 6 {
		t.Errorf("expected 6 detectors, got %d", len(dl.detectors))
	}
}

func TestDoomLoop_Signals(t *testing.T) {
	var signals []Signal
	dl := New(
		WithToolCallLoop(2),
		WithOnSignal(func(s Signal) {
			signals = append(signals, s)
		}),
	)

	// Trigger tool call loop (2 identical calls).
	tc := agentcore.ToolCall{Name: "tool_x", Arguments: "{}"}
	for i := 0; i < 2; i++ {
		mcc := &agentcore.ModelCallContext{
			Response: &agentcore.ProviderResponse{
				ToolCalls: []agentcore.ToolCall{tc},
			},
		}
		hook := dl.AsHook()
		hook.AfterModelCall(context.TODO(), nil, mcc)
	}

	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Detector != DetectorToolCallLoop {
		t.Errorf("detector = %q", signals[0].Detector)
	}
}

func TestDoomLoop_Reset(t *testing.T) {
	dl := New(WithToolCallLoop(2))
	tc := agentcore.ToolCall{Name: "x", Arguments: "{}"}
	mcc := &agentcore.ModelCallContext{
		Response: &agentcore.ProviderResponse{ToolCalls: []agentcore.ToolCall{tc}},
	}
	dl.AsHook().AfterModelCall(context.TODO(), nil, mcc)
	dl.Reset()
	if len(dl.Signals()) != 0 {
		t.Error("expected no signals after reset")
	}
}

func TestDoomLoopHook_BeforeAgentRun(t *testing.T) {
	dl := New(WithToolCallLoop(2))
	hook := dl.AsHook()

	// Record a tool call.
	tc := agentcore.ToolCall{Name: "x", Arguments: "{}"}
	mcc := &agentcore.ModelCallContext{
		Response: &agentcore.ProviderResponse{ToolCalls: []agentcore.ToolCall{tc}},
	}
	hook.AfterModelCall(context.TODO(), nil, mcc)

	// Reset via BeforeAgentRun.
	hook.BeforeAgentRun(context.TODO(), &agentcore.AgentRunContext{})
	if len(dl.Signals()) != 0 {
		t.Error("expected reset after BeforeAgentRun")
	}
}

func TestIsDoomLoopFatal(t *testing.T) {
	err := &agentcore.NodeError{Message: "test tool_call_loop detected"}
	sig := IsDoomLoopFatal(err)
	if sig == nil {
		t.Fatal("expected signal")
	}
	if sig.Detector != DetectorToolCallLoop {
		t.Errorf("detector = %q", sig.Detector)
	}
}

func TestIsDoomLoopFatal_NoMatch(t *testing.T) {
	err := &agentcore.NodeError{Message: "some other error"}
	if sig := IsDoomLoopFatal(err); sig != nil {
		t.Errorf("unexpected signal: %v", sig)
	}
}
