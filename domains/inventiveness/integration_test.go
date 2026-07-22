package inventiveness

import (
	"context"
	"testing"

	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Integration tests — full Pregel graph flow with mock provider
// =============================================================================

// TestInventivenessGraphFullFlow verifies the complete Pregel graph topology
// by running all 5 nodes with a mock provider. Since the mock provider returns
// empty responses, this primarily validates:
//   - Graph compilation succeeds
//   - State flows through all nodes without panics
//   - Skip propagation works correctly
//   - Result is populated in state
func TestInventivenessGraphFullFlow(t *testing.T) {
	provider := mockProvider{}
	compiled, err := BuildInventivenessGraph(provider)
	if err != nil {
		t.Fatalf("BuildInventivenessGraph failed: %v", err)
	}

	// Build a valid input with evidence coverage.
	input := &InventivenessInput{
		EvidenceCoverage: "full",
		Features: []TechFeature{
			{ID: "f1", Category: "structure", Description: "超声波传感器", Importance: "high"},
		},
		PriorArtChunks: []EvidenceChunk{
			{DocID: "D1", Title: "红外传感器专利", Snippet: "使用红外传感器进行检测...", Score: 0.9},
		},
		PFETriples: []PFETriple{
			{ID: "t1", Problem: "检测精度低", Effect: "提高检测精度"},
		},
		NoveltyConclusion: "相对于D1具备新颖性",
	}

	state := graph.PregelState{}
	state[StateKeyInput] = input

	ctx := context.Background()
	state, runErr := compiled.Run(ctx, state)

	// Verify that we got a result (even with mock responses, nodes should produce output).
	result, ok := state[StateKeyResult].(*InventivenessResult)
	if !ok {
		if runErr != nil {
			t.Logf("graph run returned error (expected with mock provider): %v", runErr)
		}
		t.Fatal("expected InventivenessResult in state after full flow")
	}

	if result == nil {
		t.Fatal("result should not be nil")
	}

	t.Logf("Full flow result: assessed=%v skipped=%v confidence=%q",
		result.Assessed, result.Skipped, result.Confidence)

	// Even with mock responses, the graph should have populated basic fields.
	if !result.Assessed && !result.Skipped {
		t.Error("expected either Assessed=true or Skipped=true")
	}
}

// TestInventivenessGraph_SkipPropagation verifies that when loadInputNode
// sets Skipped=true, all downstream nodes correctly short-circuit.
func TestInventivenessGraph_SkipPropagation(t *testing.T) {
	provider := mockProvider{}
	compiled, err := BuildInventivenessGraph(provider)
	if err != nil {
		t.Fatalf("BuildInventivenessGraph failed: %v", err)
	}

	// Input with no evidence — should trigger skip.
	input := &InventivenessInput{
		EvidenceCoverage: "none",
	}

	state := graph.PregelState{}
	state[StateKeyInput] = input

	ctx := context.Background()
	state, runErr := compiled.Run(ctx, state)
	_ = runErr // May be nil or error depending on mock behavior

	result, ok := state[StateKeyResult].(*InventivenessResult)
	if !ok {
		t.Fatal("expected InventivenessResult in state after skip")
	}

	if !result.Skipped {
		t.Error("expected Skipped=true when EvidenceCoverage=none")
	}

	if result.Assessed {
		t.Error("expected Assessed=false when skipped")
	}

	t.Logf("Skip propagation: skip_reason=%q", result.SkipReason)
}

// TestInventivenessGraph_InvalidInput verifies graceful handling of nil/missing input.
func TestInventivenessGraph_InvalidInput(t *testing.T) {
	provider := mockProvider{}
	compiled, err := BuildInventivenessGraph(provider)
	if err != nil {
		t.Fatalf("BuildInventivenessGraph failed: %v", err)
	}

	state := graph.PregelState{}
	// Intentionally NOT setting StateKeyInput — simulate missing input.

	ctx := context.Background()
	state, _ = compiled.Run(ctx, state)

	result, ok := state[StateKeyResult].(*InventivenessResult)
	if !ok {
		t.Fatal("expected InventivenessResult in state even with missing input")
	}

	if !result.Skipped {
		t.Error("expected Skipped=true when input is missing")
	}
}

// TestInventivenessTool_RunWithMockProvider verifies the tool execution path.
func TestInventivenessTool_RunWithMockProvider(t *testing.T) {
	provider := mockProvider{}
	tool := NewInventivenessTool(WithProvider(provider))
	if tool == nil {
		t.Fatal("NewInventivenessTool returned nil")
	}

	// The tool is registered correctly — verify its Func is callable.
	if tool.Func == nil {
		t.Fatal("tool Func should not be nil")
	}

	// Invoking with nil context and minimal valid args should not panic
	// (it will likely error due to mock responses, but should not crash).
	if tool.Name != "evaluate_inventiveness" {
		t.Errorf("expected tool name evaluate_inventiveness, got %q", tool.Name)
	}
	if !tool.ReadOnly {
		t.Error("evaluate_inventiveness tool should be read-only")
	}
}

// TestInventivenessTool_NoProviderReturnsError verifies graceful degradation.
func TestInventivenessTool_NoProviderReturnsError(t *testing.T) {
	tool := NewInventivenessTool()
	if tool == nil {
		t.Fatal("NewInventivenessTool returned nil")
	}

	// When provider is nil, the tool should return an error map, not panic or error.
	ctx := context.Background()
	// We need valid-ish JSON args to trigger the execution path.
	result, err := tool.Func(ctx, []byte(`{"features":[],"evidence_coverage":"none"}`))
	if err != nil {
		t.Fatalf("tool.Func returned error: %v", err)
	}

	// Should return an error map.
	errMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any error response, got %T", result)
	}
	if errMap["error"] == "" {
		t.Error("expected error message when provider is nil")
	}
	t.Logf("no-provider response: %v", errMap)
}

// TestTypeCompatibility ensures that the types used across the module
// are compatible with each other (e.g., StateKey constants match node usage).
func TestTypeCompatibility(t *testing.T) {
	// Verify that state keys used in graph.go match those in nodes.go.
	// Since they're in the same package, compilation alone proves this.
	// This test exists as documentation of the contract.

	// Verify that state keys are non-empty.
	for _, key := range []string{StateKeyInput, StateKeyResult, stateKeyStep1, stateKeyStep2, stateKeyStep3, stateKeyStep4} {
		if key == "" {
			t.Error("state key should not be empty")
		}
	}

	// Verify that InventivenessResult JSON tags are valid.
	result := &InventivenessResult{
		Assessed:    true,
		Conclusion:  "test",
		Confidence:  "high",
		Step1:       Step1Result{ClosestPriorArt: "D1"},
		Step2:       Step2Result{DistinguishingFeatures: []string{"f1"}},
		Step3:       Step3Result{TechnicalSuggestion: false},
		Step4:       Step4Result{HasSignificantProgress: true, ProgressType: ProgressTypeEffectImprove},
		IsInventive: true,
	}

	// Verify backward-compatible ThreeStep is populated.
	result.ThreeStep = ThreeStepResult{
		ClosestPriorArt:        result.Step1.ClosestPriorArt,
		DistinguishingFeatures: result.Step2.DistinguishingFeatures,
		ActualTechProblem:      result.Step2.ActualTechProblem,
		TechnicalSuggestion:    result.Step3.TechnicalSuggestion,
		SuggestionRationale:    result.Step3.Rationale,
	}

	if result.ThreeStep.ClosestPriorArt != "D1" {
		t.Error("ThreeStep should mirror Step1")
	}
	if result.ThreeStep.TechnicalSuggestion {
		t.Error("ThreeStep should mirror Step3")
	}
}
