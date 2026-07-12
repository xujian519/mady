package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// Phase 5 全链路验证测试 — Handoff Tool + Checkpoint + 竞态检测
// =============================================================================

// TestWorkflowTool_Integration verifies the FiveStepRunner as an agentcore.Tool.
func TestWorkflowTool_Integration(t *testing.T) {
	// Create a runner with all components.
	retriever := NewMultiSourceRetriever(nil,
		&mockVectorStore{
			rules: []RetrievedRule{
				{Rule: RuleConstraint{ArticleID: "A22.3", ArticleName: "创造性", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.95},
				{Rule: RuleConstraint{ArticleID: "A22.2", ArticleName: "新颖性", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.9},
			},
		}, nil,
	)

	runner := NewWorkflowRunner("case_tool_001", CasePatentability, "量子计算", retriever, nil)

	// Wrap as a tool.
	tool := AsWorkflowTool(runner)
	if tool.Name != WorkflowToolName {
		t.Errorf("tool name: got %q, want %q", tool.Name, WorkflowToolName)
	}
	if !tool.ReadOnly {
		t.Error("workflow tool should be read-only")
	}

	// Invoke via tool Func.
	args, _ := json.Marshal(map[string]string{
		"query":      "判断一种基于量子纠错的芯片架构是否具有创造性",
		"tech_field": "量子计算",
	})

	result, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("tool execution: %v", err)
	}

	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	if !strings.Contains(resultStr, "五步工作法执行结果") {
		t.Error("tool result should contain workflow title")
	}
	if !strings.Contains(resultStr, "case_tool_001") {
		t.Error("tool result should contain case ID")
	}

	// Verify the blackboard has full state.
	if runner.bb.CurrentStage() != 5 {
		t.Errorf("final stage: got %d, want 5", runner.bb.CurrentStage())
	}
	if runner.bb.PlanV2() == nil {
		t.Error("plan should be set")
	}
	if len(runner.bb.RuleConstraints()) < 2 {
		t.Errorf("expected >= 2 rules, got %d", len(runner.bb.RuleConstraints()))
	}
}

// TestCheckpoint_SaveAndResume verifies checkpoint save/restore cycle.
func TestCheckpoint_SaveAndResume(t *testing.T) {
	planner := NewPlanner(nil)
	for _, v := range defaultPatentPlanTemplates() {
		v := v
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
	}

	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:  planner,
		CaseID:   "case_cp",
		CaseType: CaseNoveltySearch,
		Collectors: []FactCollectorFunc{
			func(ctx context.Context, input string, bb *FactBlackboard) (*CollectResult, error) {
				bb.AddFact(FactEntry{
					ID: "f_cp", Source: "user_text", Content: input,
					Confidence: 1.0, ExtractedAt: NowISO(), CollectorID: CollectorUserInput,
				})
				return &CollectResult{CollectorID: CollectorUserInput, FactCount: 1}, nil
			},
		},
	})

	// Run partial workflow (just Stage ①).
	if err := runner.runStage1(context.Background(), "test input"); err != nil {
		t.Fatalf("stage1: %v", err)
	}

	// Save checkpoint.
	store := NewMemoryCheckpointStore()
	if err := runner.SaveCheckpoint(context.Background(), store, "cp_001"); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	// Resume from checkpoint.
	restored, err := ResumeFromCheckpoint(context.Background(), store, "cp_001", FiveStepRunnerConfig{
		Planner:  planner,
		CaseID:   "case_cp",
		CaseType: CaseNoveltySearch,
		Retriever: NewMultiSourceRetriever(nil,
			&mockVectorStore{
				rules: []RetrievedRule{
					{Rule: RuleConstraint{ArticleID: "A22.1", ArticleName: "新颖性", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.9},
				},
			}, nil,
		),
	})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}

	// Verify restored state.
	if restored.bb.CaseID != "case_cp" {
		t.Errorf("case ID: got %q", restored.bb.CaseID)
	}
	if len(restored.bb.ActiveFacts()) < 1 {
		t.Error("restored blackboard should have facts")
	}

	// Continue execution from Stage ②.
	result, err := restored.ContinueFromStage(context.Background(), "test input", 2)
	if err != nil {
		t.Fatalf("continue: %v", err)
	}

	if !strings.Contains(result, "五步工作法执行结果") {
		t.Error("continued result should contain title")
	}
}

// TestCheckpoint_JSONRoundTrip verifies checkpoint serialization.
func TestCheckpoint_JSONRoundTrip(t *testing.T) {
	bb := NewFactBlackboard("case_json", CasePatentability, "AI")
	bb.AddFact(FactEntry{
		ID: "f_json", Source: "user_text", Content: "test",
		Confidence: 1.0, ExtractedAt: NowISO(),
	})
	bb.SetCurrentStage(3)

	plan := &Plan{
		PlanID:   "plan_json",
		Intent:   PlanIntentChain,
		CaseType: CasePatentability,
		Steps:    []PlanStep{{Order: 1, Description: "test", Strategy: StrategyChain}},
	}
	bb.SetPlanV2(*plan)

	cp := &StageCheckpoint{
		CheckpointID: "cp_json",
		CaseID:       "case_json",
		CaseType:     CasePatentability,
		CurrentStage: 3,
		Blackboard:   bb,
		Plan:         plan,
	}

	data, err := MarshalCheckpoint(cp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	restored, err := UnmarshalCheckpoint(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.CheckpointID != "cp_json" {
		t.Errorf("checkpoint ID mismatch")
	}
	if restored.CurrentStage != 3 {
		t.Errorf("current stage: got %d", restored.CurrentStage)
	}
}

// TestRaceCondition_ConcurrentCollectors verifies goroutine safety of collectors.
func TestRaceCondition_ConcurrentCollectors(t *testing.T) {
	bb := NewFactBlackboard("case_race", CaseNoveltySearch, "")

	// Simulate concurrent fact additions from multiple collectors.
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				factID := fmt.Sprintf("f_race_%d_%d", id, j)
				bb.AddFact(FactEntry{
					ID: factID, Source: "user_text", Content: "concurrent fact",
					Confidence: 0.9, ExtractedAt: NowISO(), CollectorID: CollectorUserInput,
				})
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if len(bb.ActiveFacts()) != 100 {
		t.Errorf("expected 100 facts (10 goroutines × 10), got %d", len(bb.ActiveFacts()))
	}
}

// TestRaceCondition_ConcurrentPlanner verifies planner goroutine safety.
func TestRaceCondition_ConcurrentPlanner(t *testing.T) {
	planner := NewPlanner(nil)
	for _, v := range defaultPatentPlanTemplates() {
		v := v
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
	}

	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(id int) {
			bb := NewFactBlackboard(fmt.Sprintf("c%d", id), CaseNoveltySearch, "")
			bb.AddFact(FactEntry{
				ID: fmt.Sprintf("f%d", id), Source: "user_text", Content: "test",
				Confidence: 1.0, ExtractedAt: NowISO(),
			})
			_, err := planner.GeneratePlan(context.Background(), bb, PlanIntentChain)
			if err != nil {
				t.Errorf("goroutine %d: GeneratePlan: %v", id, err)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}
}

// TestRaceCondition_ConcurrentRetriever verifies retriever goroutine safety.
func TestRaceCondition_ConcurrentRetriever(t *testing.T) {
	vs := &mockVectorStore{
		rules: []RetrievedRule{
			{Rule: RuleConstraint{ArticleID: "A22.1", ArticleName: "新颖性", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.9},
		},
	}

	retriever := NewMultiSourceRetriever(nil, vs, nil)
	manifest := RuleRetrievalManifest{
		ManifestID: "race_test",
		Sources:    []RuleSourceCfg{{Source: RuleSourceVector, MaxPerSource: 5}},
		MaxRules:   5,
	}

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := retriever.Retrieve(context.Background(), manifest, nil, "")
			if err != nil {
				t.Errorf("concurrent retrieve: %v", err)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestNewWorkflowRunner_AllCaseTypes verifies runners for all case types.
func TestNewWorkflowRunner_AllCaseTypes(t *testing.T) {
	caseTypes := []CaseType{
		CaseNoveltySearch,
		CasePatentability,
		CaseInvalidation,
		CaseValidity,
	}

	for _, ct := range caseTypes {
		runner := NewWorkflowRunner("case_"+string(ct), ct, "通用技术", nil, nil)

		result, err := runner.Run(context.Background(), "测试输入")
		if err != nil {
			t.Errorf("%s: Run: %v", ct, err)
			continue
		}

		if !strings.Contains(result, "五步工作法执行结果") {
			t.Errorf("%s: missing title in result", ct)
		}
		if runner.bb.CurrentStage() != 5 {
			t.Errorf("%s: final stage = %d", ct, runner.bb.CurrentStage())
		}
	}
}
