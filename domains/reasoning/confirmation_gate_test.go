package reasoning

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// TestConfirmationGate_Stage2Interrupts verifies that a runner configured with
// RequireRuleConfirmation interrupts after Stage ② retrieval (returns an
// InterruptError carrying the rule count), rather than proceeding to Stage ③.
func TestConfirmationGate_Stage2Interrupts(t *testing.T) {
	planner := NewPlanner(nil)
	for _, v := range defaultPatentPlanTemplates() {
		v := v
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
	}
	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:                 planner,
		CaseID:                  "case_gate",
		CaseType:                CaseNoveltySearch,
		RequireRuleConfirmation: true,
		Retriever: NewMultiSourceRetriever(nil,
			&mockVectorStore{rules: []RetrievedRule{
				{Rule: RuleConstraint{ArticleID: "NOV-001", ArticleName: "新颖性", Requirement: ReqMust}},
			}},
			nil, nil),
	})

	_, err := runner.Run(context.Background(), "一种图像识别方法")
	if err == nil {
		t.Fatal("expected interrupt error from confirmation gate, got nil")
	}
	if !agentcore.IsInterrupt(err) {
		t.Fatalf("expected InterruptError, got: %v", err)
	}
	data := agentcore.InterruptData(err)
	if data["gate"] != "rule_confirmation" {
		t.Errorf("gate = %v, want rule_confirmation", data["gate"])
	}
	if data["total_rules"] != 1 {
		t.Errorf("total_rules = %v, want 1", data["total_rules"])
	}
	// Rules should be on the blackboard despite the interrupt.
	if got := runner.bb.RuleConstraints(); len(got) != 1 {
		t.Errorf("blackboard should have 1 rule, got %d", len(got))
	}
}

// TestConfirmationGate_NoInterruptWithoutRules verifies the gate does not
// interrupt when retrieval yields zero rules (nothing to confirm).
func TestConfirmationGate_NoInterruptWithoutRules(t *testing.T) {
	planner := NewPlanner(nil)
	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:                 planner,
		CaseID:                  "case_empty",
		CaseType:                CaseNoveltySearch,
		RequireRuleConfirmation: true,
		Retriever:               NewMultiSourceRetriever(nil, nil, nil, nil), // all nil → no rules
	})

	_, err := runner.Run(context.Background(), "test")
	// With no rules, no interrupt; runner proceeds (may error on later stages
	// without LLM, but the point is no confirmation interrupt).
	if err != nil && agentcore.IsInterrupt(err) {
		t.Fatalf("should not interrupt with 0 rules, got: %v", err)
	}
}

// TestConfirmationGate_DisabledByDefault verifies the gate is off unless
// explicitly enabled.
func TestConfirmationGate_DisabledByDefault(t *testing.T) {
	planner := NewPlanner(nil)
	for _, v := range defaultPatentPlanTemplates() {
		v := v
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
	}
	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:  planner,
		CaseID:   "case_nogate",
		CaseType: CaseNoveltySearch,
		Retriever: NewMultiSourceRetriever(nil, &mockVectorStore{rules: []RetrievedRule{
			{Rule: RuleConstraint{ArticleID: "X"}},
		}}, nil, nil),
	})

	_, err := runner.Run(context.Background(), "test")
	// Without RequireRuleConfirmation, no interrupt even with rules.
	if err != nil && agentcore.IsInterrupt(err) {
		t.Fatalf("gate should be off by default, got interrupt: %v", err)
	}
}

// TestWorkflowTool_CheckpointSaveOnInterrupt verifies the tool saves a
// checkpoint when the runner interrupts, and the returned message carries the
// checkpoint_id.
func TestWorkflowTool_CheckpointSaveOnInterrupt(t *testing.T) {
	planner := NewPlanner(nil)
	for _, v := range defaultPatentPlanTemplates() {
		v := v
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
	}
	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:                 planner,
		CaseID:                  "case_tool",
		CaseType:                CaseNoveltySearch,
		RequireRuleConfirmation: true,
		Retriever: NewMultiSourceRetriever(nil, &mockVectorStore{rules: []RetrievedRule{
			{Rule: RuleConstraint{ArticleID: "NOV-001", ArticleName: "新颖性"}},
		}}, nil, nil),
	})

	store := NewMemoryCheckpointStore()
	tool := AsWorkflowToolWithCheckpoint(runner, store)

	args, _ := json.Marshal(WorkflowToolInput{Query: "一种图像识别方法"})
	result, err := tool.Func(context.Background(), args)
	// Tool swallows the interrupt and returns a confirmation message.
	if err != nil {
		t.Fatalf("tool should handle interrupt gracefully, got err: %v", err)
	}
	msg, ok := result.(string)
	if !ok {
		t.Fatalf("result type = %T, want string", result)
	}
	if !strings.Contains(msg, "checkpoint_id") {
		t.Errorf("result should contain checkpoint_id:\n%s", msg)
	}

	// Extract checkpoint_id and verify it's in the store.
	// The message format is "checkpoint_id: wf-..."
	idx := strings.Index(msg, "checkpoint_id: wf-")
	if idx < 0 {
		t.Fatal("could not find checkpoint_id in message")
	}
	cpID := msg[idx+len("checkpoint_id: "):]
	if end := strings.IndexAny(cpID, "\n "); end > 0 {
		cpID = cpID[:end]
	}
	cp, err := store.Load(context.Background(), cpID)
	if err != nil {
		t.Fatalf("checkpoint not saved: %v", err)
	}
	if cp.Blackboard == nil {
		t.Error("checkpoint blackboard should be non-nil")
	}
}

// TestWorkflowTool_BackwardCompatible verifies AsWorkflowTool(runner) (no store)
// still works without checkpoint — interrupt propagates as error.
func TestWorkflowTool_BackwardCompatible(t *testing.T) {
	planner := NewPlanner(nil)
	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:                 planner,
		CaseID:                  "case_compat",
		CaseType:                CaseNoveltySearch,
		RequireRuleConfirmation: true,
		Retriever: NewMultiSourceRetriever(nil, &mockVectorStore{rules: []RetrievedRule{
			{Rule: RuleConstraint{ArticleID: "X"}},
		}}, nil, nil),
	})

	tool := AsWorkflowTool(runner) // no store
	args, _ := json.Marshal(WorkflowToolInput{Query: "test"})
	_, err := tool.Func(context.Background(), args)
	// Without store, interrupt propagates as error (not swallowed).
	if err == nil || !strings.Contains(err.Error(), "execution failed") {
		t.Fatalf("expected execution-failed error without store, got: %v", err)
	}
}
