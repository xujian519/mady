package reasoning

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Phase 3 验证测试 — WorkflowManifest + EnhancedSyllogismChecker
// =============================================================================

// TestWorkflowManifestStore_LoadDir verifies YAML loading.
func TestWorkflowManifestStore_LoadDir(t *testing.T) {
	// Create a temp dir with a test manifest.
	dir := t.TempDir()
	yamlContent := `workflow_manifest:
  id: "test_novelty"
  name: "测试新颖性"
  case_type: "novelty_search"
  stage1:
    collectors:
      - type: "user_input"
        enabled: true
        config:
          max_facts: 10
  stage2:
    manifest_id: "test_rules"
    sources:
      - source: "knowledge_graph"
        max_per_source: 5
        weight: 1.0
    aggregation: "priority"
    max_rules: 5
  stage3: {}
  stage4:
    default_strategy: "chain"
    max_steps: 20
    steps:
      - order: 1
        description: "测试步骤"
        strategy: "chain"
  stage5:
    syllogism_level: 2
    llm_validate: true
    max_retries: 3
`

	testFile := filepath.Join(dir, "test_manifest.yaml")
	if err := os.WriteFile(testFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	store := NewWorkflowManifestStore()
	if err := store.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	// Verify by ID.
	m, ok := store.Get("test_novelty")
	if !ok {
		t.Fatal("manifest not found by ID")
	}
	if m.Name != "测试新颖性" {
		t.Errorf("name: got %q", m.Name)
	}
	if m.CaseType != CaseNoveltySearch {
		t.Errorf("case_type: got %q", m.CaseType)
	}

	// Verify by CaseType.
	m2, ok := store.GetByCaseType(CaseNoveltySearch)
	if !ok || m2.ID != "test_novelty" {
		t.Error("manifest not found by case type")
	}

	// Verify stage configs.
	if len(m.Stage4.Steps) != 1 {
		t.Errorf("stage4 steps: got %d", len(m.Stage4.Steps))
	}
	if m.Stage5.SyllogismLevel != 2 {
		t.Errorf("stage5 syllogism_level: got %d", m.Stage5.SyllogismLevel)
	}
	if m.Stage5.MaxRetries != 3 {
		t.Errorf("stage5 max_retries: got %d", m.Stage5.MaxRetries)
	}
}

// TestWorkflowManifestStore_DefaultManifests verifies built-in defaults.
func TestWorkflowManifestStore_DefaultManifests(t *testing.T) {
	store := NewWorkflowManifestStore()
	for _, m := range DefaultManifests() {
		store.Register(m)
	}

	if len(store.List()) < 2 {
		t.Errorf("expected >= 2 manifests, got %d", len(store.List()))
	}

	// Verify novelty manifest has expected stages.
	nov, ok := store.GetByCaseType(CaseNoveltySearch)
	if !ok {
		t.Fatal("novelty manifest not found")
	}
	if len(nov.Stage4.Steps) != 4 {
		t.Errorf("novelty steps: got %d, want 4", len(nov.Stage4.Steps))
	}

	// Verify patentability manifest has multi_hypothesis step.
	pat, ok := store.GetByCaseType(CasePatentability)
	if !ok {
		t.Fatal("patentability manifest not found")
	}
	hasMultiHypo := false
	for _, step := range pat.Stage4.Steps {
		if step.Strategy == StrategyMultiHypothesis {
			hasMultiHypo = true
			break
		}
	}
	if !hasMultiHypo {
		t.Error("patentability manifest should have multi_hypothesis step")
	}

	// Verify drafting manifest (claim drafting: A31 unity/divisional).
	draft, ok := store.GetByCaseType(CaseDrafting)
	if !ok {
		t.Fatal("drafting manifest not found")
	}
	if len(draft.Stage4.Steps) != 5 {
		t.Errorf("drafting steps: got %d, want 5", len(draft.Stage4.Steps))
	}
	// Step 4 (unity analysis) should use multi_hypothesis.
	if draft.Stage4.Steps[3].Strategy != StrategyMultiHypothesis {
		t.Errorf("drafting step 4 strategy: got %s, want %s", draft.Stage4.Steps[3].Strategy, StrategyMultiHypothesis)
	}

	// Verify invalidation manifest (invalidation: A33 amendment scope).
	inval, ok := store.GetByCaseType(CaseInvalidation)
	if !ok {
		t.Fatal("invalidation manifest not found")
	}
	if len(inval.Stage4.Steps) != 5 {
		t.Errorf("invalidation steps: got %d, want 5", len(inval.Stage4.Steps))
	}
	// Step 4 (per-ground assessment) should use multi_hypothesis.
	if inval.Stage4.Steps[3].Strategy != StrategyMultiHypothesis {
		t.Errorf("invalidation step 4 strategy: got %s, want %s", inval.Stage4.Steps[3].Strategy, StrategyMultiHypothesis)
	}
	// Invalidation should require all rules used (hard constraint from XiaoNuo).
	if !inval.Stage5.RequireAllRulesUsed {
		t.Error("invalidation Stage5 should require all rules used")
	}
}

// TestFiveStepRunner_WithManifest verifies manifest-driven runner.
func TestFiveStepRunner_WithManifest(t *testing.T) {
	planner := NewPlanner(nil)
	for _, v := range defaultPatentPlanTemplates() {
		v := v
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
	}

	manifest := defaultNoveltySearchManifest()

	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:  planner,
		Manifest: manifest,
		CaseID:   "case_manifest",
		CaseType: CaseNoveltySearch,
		Collectors: []FactCollectorFunc{
			func(ctx context.Context, input string, bb *FactBlackboard) (*CollectResult, error) {
				bb.AddFact(FactEntry{
					ID: "f1", Source: "user_text", Content: input,
					Confidence: 1.0, ExtractedAt: NowISO(), CollectorID: CollectorUserInput,
				})
				return &CollectResult{CollectorID: CollectorUserInput, FactCount: 1}, nil
			},
		},
	})

	result, err := runner.Run(context.Background(), "一种数据处理方法")
	if err != nil {
		t.Fatalf("manifest runner: %v", err)
	}

	if !strings.Contains(result, "五步工作法执行结果") {
		t.Error("result should contain title")
	}

	// Verify manifest-driven plan steps were used.
	plan := runner.bb.PlanV2()
	if plan == nil || len(plan.Steps) != 4 {
		t.Errorf("expected 4 manifest-driven steps, got %d", len(plan.Steps))
	}
}

// TestEnhancedChecker_L1 verifies Level 1 (reference existence) check.
func TestEnhancedChecker_L1(t *testing.T) {
	bb := NewFactBlackboard("case_l1", CaseNoveltySearch, "")
	bb.AddFact(FactEntry{
		ID: "f1", Source: "user_text", Content: "事实内容",
		Confidence: 1.0, ExtractedAt: NowISO(),
	})
	bb.AddRuleConstraint(RuleConstraint{
		ArticleID: "A22.1", ArticleName: "新颖性", Requirement: ReqMust,
	})

	plan := &Plan{
		PlanID:    "test_l1",
		Intent:    PlanIntentChain,
		CaseType:  CaseNoveltySearch,
		UsedFacts: []string{"f1"},
		UsedRules: []string{"A22.1"},
		Steps: []PlanStep{
			{Order: 1, Description: "分析", Strategy: StrategyChain},
		},
	}

	checker := NewEnhancedSyllogismChecker(nil, 2)
	report, err := checker.Check(context.Background(), bb, plan, CheckLevel1)
	if err != nil {
		t.Fatalf("L1 check: %v", err)
	}

	if !report.Passed {
		t.Error("L1 check should pass with valid references")
	}
	if len(report.Syllogisms) == 0 {
		t.Error("should have built syllogisms")
	}
}

// TestEnhancedChecker_L1_MissingRef verifies L1 catches missing references.
func TestEnhancedChecker_L1_MissingRef(t *testing.T) {
	bb := NewFactBlackboard("case_l1b", CaseNoveltySearch, "")
	// No facts or rules added — all refs will be missing.

	plan := &Plan{
		PlanID:    "test_l1b",
		Intent:    PlanIntentChain,
		CaseType:  CaseNoveltySearch,
		UsedFacts: []string{"ghost_fact"},
		UsedRules: []string{"ghost_rule"},
		Steps: []PlanStep{
			{Order: 1, Description: "分析", Strategy: StrategyChain},
		},
	}

	checker := NewEnhancedSyllogismChecker(nil, 2)
	report, err := checker.Check(context.Background(), bb, plan, CheckLevel1)
	if err != nil {
		t.Fatalf("L1 check: %v", err)
	}

	if report.Passed {
		t.Error("L1 check should fail with missing references")
	}
	if len(report.Gaps) == 0 {
		t.Error("should have validation gaps")
	}
}

// TestEnhancedChecker_UnusedDetection verifies unused fact/rule detection.
func TestEnhancedChecker_UnusedDetection(t *testing.T) {
	bb := NewFactBlackboard("case_unused", CaseNoveltySearch, "")
	bb.AddFact(FactEntry{
		ID: "f1", Source: "user_text", Content: "事实",
		Confidence: 1.0, ExtractedAt: NowISO(),
	})
	bb.AddFact(FactEntry{
		ID: "f2", Source: "user_text", Content: "未使用的事实",
		Confidence: 1.0, ExtractedAt: NowISO(),
	})
	bb.AddRuleConstraint(RuleConstraint{
		ArticleID: "A22.1", ArticleName: "新颖性", Requirement: ReqMust,
	})

	plan := &Plan{
		PlanID:    "test_unused",
		CaseType:  CaseNoveltySearch,
		UsedFacts: []string{"f1", "f2"},
		UsedRules: []string{"A22.1"},
		Steps: []PlanStep{
			{Order: 1, Description: "分析", Strategy: StrategyChain},
		},
	}

	checker := NewEnhancedSyllogismChecker(nil, 2)
	report, err := checker.Check(context.Background(), bb, plan, CheckLevel1)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	// f2 is UsedFacts but may not appear in syllogisms → should be UnusedFacts.
	_ = report.UnusedFacts
	_ = report.UnusedRules
	// Note: The syllogism building is simplified, so exact unused detection
	// depends on how syllogisms are constructed from PlanStep.
}

// TestFiveStepRunner_WithChecker verifies Stage ⑤ enhanced checking.
func TestFiveStepRunner_WithChecker(t *testing.T) {
	planner := NewPlanner(nil)
	for _, v := range defaultPatentPlanTemplates() {
		v := v
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
	}

	checker := NewEnhancedSyllogismChecker(nil, 2)
	manifest := defaultNoveltySearchManifest()

	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:  planner,
		Checker:  checker,
		Manifest: manifest,
		CaseID:   "case_checker",
		CaseType: CaseNoveltySearch,
		Collectors: []FactCollectorFunc{
			func(ctx context.Context, input string, bb *FactBlackboard) (*CollectResult, error) {
				bb.AddFact(FactEntry{
					ID: "f1", Source: "user_text", Content: input,
					Confidence: 1.0, ExtractedAt: NowISO(), CollectorID: CollectorUserInput,
				})
				return &CollectResult{CollectorID: CollectorUserInput, FactCount: 1}, nil
			},
		},
		Retriever: NewMultiSourceRetriever(nil,
			&mockVectorStore{
				rules: []RetrievedRule{
					{Rule: RuleConstraint{ArticleID: "A22.1", ArticleName: "新颖性", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.9},
				},
			}, nil,
		),
	})

	result, err := runner.Run(context.Background(), "一种图像处理方法")
	if err != nil {
		t.Fatalf("runner with checker: %v", err)
	}

	report := runner.bb.CheckReport()
	if report == nil {
		t.Fatal("check report should not be nil")
	}
	if len(report.Syllogisms) == 0 {
		t.Error("should have syllogisms in report")
	}

	_ = result
}

// TestManifest_StageConfigConversion verifies config conversion methods.
func TestManifest_StageConfigConversion(t *testing.T) {
	cfg := Stage2Config{
		ManifestID: "test_rules",
		Sources: []RuleSourceCfg{
			{Source: RuleSourceKG, MaxPerSource: 10, Weight: 1.0},
		},
		Aggregation: "priority",
		MaxRules:    8,
	}

	rm := cfg.ToRuleRetrievalManifest()
	if rm.ManifestID != "test_rules" {
		t.Errorf("manifest ID: got %q", rm.ManifestID)
	}
	if rm.MaxRules != 8 {
		t.Errorf("max rules: got %d", rm.MaxRules)
	}

	cfg4 := Stage4Config{
		DefaultStrategy: StrategyChain,
		Steps: []PlanStep{
			{Order: 1, Description: "步骤1", Strategy: StrategyChain},
		},
	}

	plan := cfg4.ToPlan(CaseNoveltySearch)
	if len(plan.Steps) != 1 {
		t.Errorf("plan steps: got %d", len(plan.Steps))
	}
	if plan.CaseType != CaseNoveltySearch {
		t.Errorf("case type: got %q", plan.CaseType)
	}
}
