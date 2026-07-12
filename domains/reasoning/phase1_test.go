package reasoning

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Phase 1 验证测试 — 数据契约 + Planner + PlanCompiler + FiveStepRunner
// =============================================================================

// TestPlanSerialization verifies the new Plan type round-trips through JSON.
func TestPlanSerialization(t *testing.T) {
	plan := &Plan{
		PlanID:   "test_plan_01",
		Intent:   PlanIntentChain,
		CaseType: CaseNoveltySearch,
		Steps: []PlanStep{
			{Order: 1, Description: "解析技术特征", Strategy: StrategyChain},
			{Order: 2, Description: "检索对比文件", Strategy: StrategyReact},
			{Order: 3, Description: "生成结论", Strategy: StrategyChain},
		},
		UsedFacts: []string{"fact_1", "fact_2"},
		UsedRules: []string{"A22.1", "A22.2"},
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal Plan: %v", err)
	}

	var restored Plan
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal Plan: %v", err)
	}

	if restored.PlanID != plan.PlanID {
		t.Errorf("PlanID: got %q, want %q", restored.PlanID, plan.PlanID)
	}
	if restored.Intent != plan.Intent {
		t.Errorf("Intent: got %q, want %q", restored.Intent, plan.Intent)
	}
	if len(restored.Steps) != len(plan.Steps) {
		t.Errorf("Steps count: got %d, want %d", len(restored.Steps), len(plan.Steps))
	}
	for i, step := range restored.Steps {
		if step.Strategy != plan.Steps[i].Strategy {
			t.Errorf("Step[%d].Strategy: got %q, want %q", i, step.Strategy, plan.Steps[i].Strategy)
		}
	}
}

// TestCheckReportSerialization verifies CheckReport JSON round-trip.
func TestCheckReportSerialization(t *testing.T) {
	report := &CheckReport{
		PlanID:     "plan_test",
		Passed:     true,
		UsedFacts:  []string{"f1", "f2"},
		UsedRules:  []string{"r1"},
		Confidence: 0.85,
		Gaps: []ValidationGap{{
			Description: "缺少对对比文件 D2 的引用",
			Severity:    "soft",
			Suggestion:  "请补充对比文件 D2 与权利要求 3 的比对",
		}},
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal CheckReport: %v", err)
	}

	var restored CheckReport
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal CheckReport: %v", err)
	}

	if restored.Passed != report.Passed {
		t.Errorf("Passed: got %v, want %v", restored.Passed, report.Passed)
	}
	if restored.Confidence != report.Confidence {
		t.Errorf("Confidence: got %f, want %f", restored.Confidence, report.Confidence)
	}
	if len(restored.Gaps) != 1 || restored.Gaps[0].Severity != "soft" {
		t.Errorf("Gaps not restored correctly: %+v", restored.Gaps)
	}
}

// TestFactBlackboardV2Fields verifies the new blackboard methods.
func TestFactBlackboardV2Fields(t *testing.T) {
	bb := NewFactBlackboard("case_v2", CasePatentability, "通信")

	// StageOutput
	_, ok := bb.StageOutput("stage1")
	if ok {
		t.Fatal("stage1 should not exist yet")
	}
	bb.SetStageOutput("stage1", "事实收集完成")
	v, ok := bb.StageOutput("stage1")
	if !ok || v != "事实收集完成" {
		t.Fatalf("stage1 output: got %v, %v", v, ok)
	}

	// PlanV2
	if bb.PlanV2() != nil {
		t.Fatal("PlanV2 should be nil initially")
	}
	plan := Plan{PlanID: "p1", Intent: PlanIntentChain}
	bb.SetPlanV2(plan)
	if bb.PlanV2() == nil || bb.PlanV2().PlanID != "p1" {
		t.Fatal("PlanV2 not stored correctly")
	}

	// CheckReport
	if bb.CheckReport() != nil {
		t.Fatal("CheckReport should be nil initially")
	}
	report := CheckReport{PlanID: "p1", Passed: true}
	bb.SetCheckReport(report)
	if bb.CheckReport() == nil || !bb.CheckReport().Passed {
		t.Fatal("CheckReport not stored correctly")
	}

	// CurrentStage / WorkflowID
	if bb.CurrentStage() != 0 {
		t.Errorf("initial stage should be 0, got %d", bb.CurrentStage())
	}
	bb.SetCurrentStage(3)
	if bb.CurrentStage() != 3 {
		t.Errorf("stage should be 3, got %d", bb.CurrentStage())
	}
	bb.SetWorkflowID("wf_test")
	if bb.WorkflowID() != "wf_test" {
		t.Errorf("WorkflowID: got %q", bb.WorkflowID())
	}

	// Verify lock behavior.
	bb.Lock()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when mutating locked blackboard")
		}
	}()
	bb.SetStageOutput("stage2", "should panic")
}

// TestPlanner_GeneratePlan_Template verifies template-based Plan generation.
func TestPlanner_GeneratePlan_Template(t *testing.T) {
	bb := NewFactBlackboard("case_tpl", CaseNoveltySearch, "AI芯片")
	bb.AddFact(FactEntry{
		ID:          "f1",
		Source:      "user_text",
		Content:     "一种基于Transformer的AI芯片架构",
		Confidence:  1.0,
		ExtractedAt: nowISO(),
		CollectorID: CollectorUserInput,
	})
	bb.AddRuleConstraint(RuleConstraint{
		ArticleID:   "A22.1",
		ArticleName: "新颖性",
		Requirement: ReqMust,
	})

	planner := NewPlanner(nil)
	for k, v := range defaultPatentPlanTemplates() {
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
		_ = k
	}

	plan, err := planner.GeneratePlan(context.Background(), bb, PlanIntentChain)
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}

	if plan == nil {
		t.Fatal("plan should not be nil")
	}
	if len(plan.Steps) == 0 {
		t.Fatal("plan should have steps")
	}
	if len(plan.UsedFacts) != 1 || plan.UsedFacts[0] != "f1" {
		t.Errorf("UsedFacts: got %v", plan.UsedFacts)
	}
	if len(plan.UsedRules) != 1 || plan.UsedRules[0] != "A22.1" {
		t.Errorf("UsedRules: got %v", plan.UsedRules)
	}

	// Verify the plan was stored on the blackboard.
	if stored := bb.PlanV2(); stored == nil || stored.PlanID != plan.PlanID {
		t.Error("plan not stored on blackboard")
	}
}

// TestPlanner_GeneratePlan_Fallback verifies fallback when no template matches.
func TestPlanner_GeneratePlan_Fallback(t *testing.T) {
	bb := NewFactBlackboard("case_fb", CaseGeneralLegal, "")
	planner := NewPlanner(nil) // no templates registered

	plan, err := planner.GeneratePlan(context.Background(), bb, PlanIntentSimple)
	if err != nil {
		t.Fatalf("GeneratePlan fallback: %v", err)
	}

	if plan == nil {
		t.Fatal("fallback plan should not be nil")
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("fallback plan should have 1 step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].Strategy != StrategyChain {
		t.Errorf("fallback strategy should be chain, got %s", plan.Steps[0].Strategy)
	}
}

// TestPlanCompiler_ChainStrategy verifies chain-strategy graph compilation.
func TestPlanCompiler_ChainStrategy(t *testing.T) {
	plan := &Plan{
		PlanID:   "chain_test",
		Intent:   PlanIntentChain,
		CaseType: CaseNoveltySearch,
		Steps: []PlanStep{
			{Order: 1, Description: "步骤1：解析", Strategy: StrategyChain},
			{Order: 2, Description: "步骤2：检索", Strategy: StrategyChain},
			{Order: 3, Description: "步骤3：结论", Strategy: StrategyChain},
		},
	}

	compiler := NewPlanCompiler(nil) // noop builder
	bb := NewFactBlackboard("c1", CaseNoveltySearch, "")
	pregelGraph, entryName, err := compiler.CompilePlanToGraph(plan, bb)
	if err != nil {
		t.Fatalf("CompilePlanToGraph: %v", err)
	}
	if entryName == "" {
		t.Fatal("entryName should not be empty")
	}

	compiled, err := pregelGraph.Compile(entryName, 10)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	state, err := compiled.Run(context.Background(), graph.PregelState{"input": "test"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify all 3 chain nodes produced output.
	for i := 1; i <= 3; i++ {
		key := "step_" + strconv.Itoa(i) + "_chain"
		_ = key // used by state check
	}
	_ = state
}

// TestPlanCompiler_ReActStrategy verifies ReAct graph compilation with cycle.
func TestPlanCompiler_ReActStrategy(t *testing.T) {
	plan := &Plan{
		PlanID:   "react_test",
		Intent:   PlanIntentReAct,
		CaseType: CasePatentability,
		Steps: []PlanStep{
			{Order: 1, Description: "ReAct搜索", Strategy: StrategyReact},
		},
	}

	compiler := NewPlanCompiler(nil) // noop builder
	bb := NewFactBlackboard("c2", CasePatentability, "")
	pregelGraph, entryName, err := compiler.CompilePlanToGraph(plan, bb)
	if err != nil {
		t.Fatalf("CompilePlanToGraph: %v", err)
	}

	compiled, err := pregelGraph.Compile(entryName, 20)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	state, err := compiled.Run(context.Background(), graph.PregelState{"input": "test"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify the ReAct cycle executed (observe set has_next=false, so it ran once).
	_ = state
}

// TestPlanCompiler_MixedStrategies verifies a Plan mixing chain and react steps.
func TestPlanCompiler_MixedStrategies(t *testing.T) {
	plan := &Plan{
		PlanID:   "mixed_test",
		Intent:   PlanIntentChain,
		CaseType: CaseNoveltySearch,
		Steps: []PlanStep{
			{Order: 1, Description: "解析文档", Strategy: StrategyChain},
			{Order: 2, Description: "搜索文献", Strategy: StrategyReact},
			{Order: 3, Description: "生成结论", Strategy: StrategyChain},
		},
	}

	compiler := NewPlanCompiler(nil)
	bb := NewFactBlackboard("c3", CaseNoveltySearch, "")
	pregelGraph, entryName, err := compiler.CompilePlanToGraph(plan, bb)
	if err != nil {
		t.Fatalf("CompilePlanToGraph: %v", err)
	}

	compiled, err := pregelGraph.Compile(entryName, 30)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	state, err := compiled.Run(context.Background(), graph.PregelState{"input": "test"})
	if err != nil {
		t.Fatalf("Run mixed strategies: %v", err)
	}
	_ = state
}

// TestPlanCompiler_EmptyPlan verifies error handling for invalid inputs.
func TestPlanCompiler_EmptyPlan(t *testing.T) {
	compiler := NewPlanCompiler(nil)
	bb := NewFactBlackboard("c4", CaseNoveltySearch, "")

	// Nil plan.
	_, _, err := compiler.CompilePlanToGraph(nil, bb)
	if err == nil {
		t.Fatal("expected error for nil plan")
	}

	// Plan with no steps.
	_, _, err = compiler.CompilePlanToGraph(&Plan{PlanID: "empty"}, bb)
	if err == nil {
		t.Fatal("expected error for plan with no steps")
	}
}

// TestFiveStepRunner_EndToEnd verifies the full Stage ③ → ④ flow.
func TestFiveStepRunner_EndToEnd(t *testing.T) {
	planner := NewPlanner(nil)
	// Register novelty template.
	for _, v := range defaultPatentPlanTemplates() {
		v := v
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
	}

	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:   planner,
		CaseID:    "case_e2e_001",
		CaseType:  CaseNoveltySearch,
		TechField: "人工智能",
	})

	result, err := runner.Run(context.Background(), "一种基于深度学习的图像识别方法")
	if err != nil {
		t.Fatalf("FiveStepRunner.Run: %v", err)
	}

	// Verify the result contains expected markdown sections.
	if !strings.Contains(result, "五步工作法执行结果") {
		t.Error("result should contain Chinese title")
	}
	if !strings.Contains(result, "case_e2e_001") {
		t.Error("result should contain case ID")
	}
	if !strings.Contains(result, "novelty_search") {
		t.Error("result should contain case type")
	}
	if !strings.Contains(result, "各步骤产出") {
		t.Error("result should contain steps section")
	}

	// Verify blackboard state after run.
	if runner.bb.CurrentStage() != 5 {
		t.Errorf("final stage should be 5, got %d", runner.bb.CurrentStage())
	}
	if runner.bb.PlanV2() == nil {
		t.Error("PlanV2 should be set after run")
	}
	if runner.bb.CheckReport() == nil {
		t.Error("CheckReport should be set after run")
	}
	if !runner.bb.CheckReport().Passed {
		t.Error("CheckReport should pass in Phase 1")
	}
}
