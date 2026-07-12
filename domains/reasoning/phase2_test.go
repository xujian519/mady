package reasoning

import (
	"context"
	"strings"
	"testing"
)

// =============================================================================
// Phase 2 验证测试 — Stage ① Collector + Stage ② Rule Retrieval
// =============================================================================

// TestFiveStepRunner_WithCollectors verifies the Stage ① integration.
func TestFiveStepRunner_WithCollectors(t *testing.T) {
	planner := NewPlanner(nil)
	for _, v := range defaultPatentPlanTemplates() {
		v := v
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
	}

	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:  planner,
		CaseID:   "case_collectors",
		CaseType: CaseNoveltySearch,
		Collectors: []FactCollectorFunc{
			func(ctx context.Context, input string, bb *FactBlackboard) (*CollectResult, error) {
				bb.AddFact(FactEntry{
					ID:          "f_user",
					Source:      "user_text",
					Content:     input,
					Confidence:  1.0,
					ExtractedAt: NowISO(),
					CollectorID: CollectorUserInput,
				})
				return &CollectResult{CollectorID: CollectorUserInput, FactCount: 1}, nil
			},
			func(ctx context.Context, input string, bb *FactBlackboard) (*CollectResult, error) {
				bb.AddFact(FactEntry{
					ID:          "f_doc",
					Source:      "file",
					Content:     "技术特征：多层神经网络架构",
					Confidence:  0.9,
					ExtractedAt: NowISO(),
					CollectorID: CollectorDocuments,
				})
				return &CollectResult{CollectorID: CollectorDocuments, FactCount: 1}, nil
			},
		},
	})

	result, err := runner.Run(context.Background(), "一种图像识别方法")
	if err != nil {
		t.Fatalf("FiveStepRunner with collectors: %v", err)
	}

	// Verify facts were collected.
	if len(runner.bb.ActiveFacts()) < 2 {
		t.Errorf("expected >= 2 facts, got %d", len(runner.bb.ActiveFacts()))
	}

	// Verify stage progression.
	if runner.bb.CurrentStage() != 5 {
		t.Errorf("final stage should be 5, got %d", runner.bb.CurrentStage())
	}

	// Verify result.
	if !strings.Contains(result, "五步工作法执行结果") {
		t.Error("result should contain title")
	}
}

// TestFiveStepRunner_WithRetriever verifies Stage ② integration.
func TestFiveStepRunner_WithRetriever(t *testing.T) {
	planner := NewPlanner(nil)
	for _, v := range defaultPatentPlanTemplates() {
		v := v
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
	}

	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:  planner,
		CaseID:   "case_retriever",
		CaseType: CasePatentability,
		Retriever: NewMultiSourceRetriever(
			nil, // No KG store — will be skipped
			&mockVectorStore{
				rules: []RetrievedRule{
					{Rule: RuleConstraint{ArticleID: "A22.3", ArticleName: "创造性", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.9},
					{Rule: RuleConstraint{ArticleID: "A22.2", ArticleName: "新颖性", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.95},
				},
			},
			nil, // No skill reader
		),
	})

	result, err := runner.Run(context.Background(), "一种基于深度学习的芯片设计方法")
	if err != nil {
		t.Fatalf("FiveStepRunner with retriever: %v", err)
	}

	// Verify rules were stored.
	constraints := runner.bb.RuleConstraints()
	if len(constraints) < 2 {
		t.Errorf("expected >= 2 rule constraints, got %d: %v", len(constraints), constraints)
	}

	if !strings.Contains(result, "五步工作法执行结果") {
		t.Error("result should contain title")
	}
}

// TestFiveStepRunner_Stage1Stage2_Combined verifies both stages together.
func TestFiveStepRunner_Stage1Stage2_Combined(t *testing.T) {
	planner := NewPlanner(nil)
	for _, v := range defaultPatentPlanTemplates() {
		v := v
		planner.RegisterTemplate(v.CaseType, v.Intent, v)
	}

	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:  planner,
		CaseID:   "case_combined",
		CaseType: CasePatentability,
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
					{Rule: RuleConstraint{ArticleID: "A22.3", ArticleName: "创造性", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.9},
				},
			}, nil,
		),
	})

	result, err := runner.Run(context.Background(), "测试输入")
	if err != nil {
		t.Fatalf("combined run: %v", err)
	}

	// Verify full pipeline.
	if len(runner.bb.ActiveFacts()) == 0 {
		t.Error("should have facts")
	}
	if len(runner.bb.RuleConstraints()) == 0 {
		t.Error("should have rules")
	}
	if runner.bb.PlanV2() == nil {
		t.Error("should have plan")
	}
	if runner.bb.CheckReport() == nil {
		t.Error("should have check report")
	}

	_ = result
}

// TestMultiSourceRetriever_Retrieve verifies the retriever with all sources.
func TestMultiSourceRetriever_Retrieve(t *testing.T) {
	vs := &mockVectorStore{
		rules: []RetrievedRule{
			{Rule: RuleConstraint{ArticleID: "A22.1", ArticleName: "授权条件", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.9},
			{Rule: RuleConstraint{ArticleID: "A22.2", ArticleName: "新颖性", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.85},
		},
	}

	retriever := NewMultiSourceRetriever(nil, vs, nil)
	manifest := RuleRetrievalManifest{
		ManifestID: "test",
		CaseType:   CaseNoveltySearch,
		Sources:    []RuleSourceCfg{{Source: RuleSourceVector, MaxPerSource: 10, Weight: 1.0}},
		MaxRules:   5,
	}

	rules, err := retriever.Retrieve(context.Background(), manifest, nil, "")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}

	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(rules))
	}
}

// TestMultiSourceRetriever_Deduplicate verifies deduplication by ArticleID.
func TestMultiSourceRetriever_Deduplicate(t *testing.T) {
	vs := &mockVectorStore{
		rules: []RetrievedRule{
			{Rule: RuleConstraint{ArticleID: "A22.3", ArticleName: "创造性", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 2, Confidence: 0.7},
			{Rule: RuleConstraint{ArticleID: "A22.3", ArticleName: "创造性(详)", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.9},
		},
	}

	retriever := NewMultiSourceRetriever(nil, vs, nil)
	manifest := RuleRetrievalManifest{
		ManifestID: "test_dedup",
		Sources:    []RuleSourceCfg{{Source: RuleSourceVector, MaxPerSource: 10}},
		MaxRules:   5,
	}

	rules, err := retriever.Retrieve(context.Background(), manifest, nil, "")
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}

	// Should have deduplicated to 1 rule with Priority=1 (the higher-priority one).
	if len(rules) != 1 {
		t.Fatalf("expected 1 deduplicated rule, got %d", len(rules))
	}
	if rules[0].Priority != 1 {
		t.Errorf("expected priority 1 (higher), got %d", rules[0].Priority)
	}
}

// mockVectorStore implements RuleVectorStore for testing.
type mockVectorStore struct {
	rules []RetrievedRule
}

func (m *mockVectorStore) SearchRules(ctx context.Context, query string, topK int) ([]RetrievedRule, error) {
	if topK > len(m.rules) {
		topK = len(m.rules)
	}
	return m.rules[:topK], nil
}
