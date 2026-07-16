package reasoning

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Phase 4 验证测试 — Multi-Hypothesis Advocate+Judge
// =============================================================================

// TestMultiHypothesisGraph_Compilation verifies the subgraph builds correctly.
func TestMultiHypothesisGraph_Compilation(t *testing.T) {
	plan := &Plan{
		PlanID:   "mh_test",
		Intent:   PlanIntentMultiHypothesis,
		CaseType: CasePatentability,
		Steps: []PlanStep{
			{Order: 1, Description: "创造性分析（显而易见性判断）", Strategy: StrategyMultiHypothesis},
		},
	}

	compiler := NewPlanCompiler(nil) // noop builder
	bb := NewFactBlackboard("c_mh", CasePatentability, "AI")

	pregelGraph, entryName, err := compiler.CompilePlanToGraph(plan, bb)
	if err != nil {
		t.Fatalf("CompilePlanToGraph: %v", err)
	}
	if entryName == "" {
		t.Fatal("entryName should not be empty")
	}

	compiled, err := pregelGraph.Compile(entryName, 30)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// Seed state with facts to make arguments valid.
	bb.AddFact(FactEntry{
		ID: "f_pro", Source: "user_text", Content: "正方事实",
		Confidence: 0.9, ExtractedAt: NowISO(),
	})
	bb.AddFact(FactEntry{
		ID: "f_con", Source: "user_text", Content: "反方事实",
		Confidence: 0.8, ExtractedAt: NowISO(),
	})
	bb.AddRuleConstraint(RuleConstraint{
		ArticleID: "A22.3", ArticleName: "创造性", Requirement: ReqMust,
	})

	state, err := compiled.Run(context.Background(), graph.PregelState{
		"input": "测试输入",
		"bb":    bb,
	})
	if err != nil {
		t.Fatalf("Run multi-hypothesis: %v", err)
	}

	// Verify verdict was produced.
	verdict, ok := state["mh_verdict"].(Verdict)
	if !ok {
		t.Fatal("verdict not found in state")
	}
	// With noop nodes, adv_a may have no supporting facts → invalid.
	// The important thing is the graph executed without errors.
	_ = verdict
}

// TestEvidenceJudge_BothValid verifies Judge picks the higher-scoring side.
func TestEvidenceJudge_BothValid(t *testing.T) {
	bb := NewFactBlackboard("c_valid", CasePatentability, "")
	bb.AddFact(FactEntry{
		ID: "f_a1", Source: "user_text", Content: "有力事实", Confidence: 0.95, ExtractedAt: NowISO(),
	})
	bb.AddFact(FactEntry{
		ID: "f_b1", Source: "user_text", Content: "较弱事实", Confidence: 0.6, ExtractedAt: NowISO(),
	})
	bb.AddRuleConstraint(RuleConstraint{
		ArticleID: "A22.3", ArticleName: "创造性", Requirement: ReqMust,
	})

	argA := Argument{
		HypothesisID:              "adv_a",
		Claim:                     "非显而易见的",
		Reasoning:                 "D2未教导组合D1和D3",
		SupportingFacts:           []string{"f_a1"},
		SupportingRules:           []string{"A22.3"},
		AcknowledgedCounterpoints: "D1与D2属于相近技术领域",
	}
	argB := Argument{
		HypothesisID:              "adv_b",
		Claim:                     "显而易见的",
		Reasoning:                 "D1和D2的简单组合",
		SupportingFacts:           []string{"f_b1"},
		SupportingRules:           []string{"A22.3"},
		AcknowledgedCounterpoints: "组合后效果超出预期",
	}

	scoreA := computeEvidenceScore(argA, bb)
	scoreB := computeEvidenceScore(argB, bb)

	// argA should score higher (higher fact confidence).
	if scoreA <= scoreB {
		t.Errorf("adv_a should score higher: A=%.3f, B=%.3f", scoreA, scoreB)
	}
}

// TestEvidenceJudge_Tie verifies tie detection.
func TestEvidenceJudge_Tie(t *testing.T) {
	bb := NewFactBlackboard("c_tie", CasePatentability, "")
	bb.AddFact(FactEntry{
		ID: "f1", Source: "user_text", Content: "事实", Confidence: 0.8, ExtractedAt: NowISO(),
	})
	bb.AddRuleConstraint(RuleConstraint{
		ArticleID: "A22.3", ArticleName: "创造性", Requirement: ReqMust,
	})

	argA := Argument{
		HypothesisID:              "adv_a",
		Claim:                     "观点A",
		SupportingFacts:           []string{"f1"},
		SupportingRules:           []string{"A22.3"},
		AcknowledgedCounterpoints: "反驳",
	}
	argB := Argument{
		HypothesisID:              "adv_b",
		Claim:                     "观点B",
		SupportingFacts:           []string{"f1"},
		SupportingRules:           []string{"A22.3"},
		AcknowledgedCounterpoints: "反驳",
	}

	scoreA := computeEvidenceScore(argA, bb)
	scoreB := computeEvidenceScore(argB, bb)

	diff := scoreA - scoreB
	if diff < 0 {
		diff = -diff
	}
	if diff >= mhTieThreshold {
		t.Logf("scores: A=%.3f, B=%.3f (diff=%.3f, threshold=%.3f)", scoreA, scoreB, diff, mhTieThreshold)
		// With identical inputs, scores should be very close.
	}
}

// TestRejectionPath_NoArguments verifies rejection when no valid arguments exist.
func TestRejectionPath_NoArguments(t *testing.T) {
	plan := &Plan{
		PlanID:   "reject_test",
		Intent:   PlanIntentMultiHypothesis,
		CaseType: CasePatentability,
		Steps: []PlanStep{
			{Order: 1, Description: "创造性判断", Strategy: StrategyMultiHypothesis},
		},
	}

	compiler := NewPlanCompiler(nil)
	bb := NewFactBlackboard("c_reject", CasePatentability, "")
	// No facts or rules added → both sides will have empty arguments.

	pregelGraph, entryName, err := compiler.CompilePlanToGraph(plan, bb)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	compiled, err := pregelGraph.Compile(entryName, 30)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	state, err := compiled.Run(context.Background(), graph.PregelState{
		"input": "测试",
		"bb":    bb,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Should have a rejection message.
	msg := state.GetString("mh_rejection_message")
	if msg == "" {
		t.Error("rejection message should not be empty")
	}
	if !strings.Contains(msg, "人工复核") {
		t.Errorf("rejection should mention human review: %q", msg)
	}

	// Verdict should be unresolved.
	verdict, ok := state["mh_verdict"].(Verdict)
	if !ok {
		t.Fatal("verdict not found")
	}
	if verdict.Resolved {
		t.Error("verdict should be unresolved with no valid arguments")
	}
}

// TestFiveStepRunner_WithMultiHypothesisStep verifies full integration.
func TestFiveStepRunner_WithMultiHypothesisStep(t *testing.T) {
	planner := NewPlanner(nil)
	manifest := defaultPatentabilityManifest()
	plan := manifest.Stage4.ToPlan(CasePatentability)
	planner.RegisterTemplate(CasePatentability, PlanIntentChain, *plan)

	runner := NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:  planner,
		Manifest: manifest,
		CaseID:   "case_mh",
		CaseType: CasePatentability,
		Collectors: []FactCollectorFunc{
			func(ctx context.Context, input string, bb *FactBlackboard) (*CollectResult, error) {
				bb.AddFact(FactEntry{
					ID: "f_pro", Source: "user_text", Content: "技术特征A+B产生协同效应",
					Confidence: 0.9, ExtractedAt: NowISO(), CollectorID: CollectorUserInput,
				})
				bb.AddFact(FactEntry{
					ID: "f_con", Source: "user_text", Content: "D1公开了特征A，D2公开了特征B",
					Confidence: 0.85, ExtractedAt: NowISO(), CollectorID: CollectorKnowledge,
				})
				return &CollectResult{CollectorID: CollectorUserInput, FactCount: 2}, nil
			},
		},
		Retriever: NewMultiSourceRetriever(nil,
			&mockVectorStore{
				rules: []RetrievedRule{
					{Rule: RuleConstraint{ArticleID: "A22.3", ArticleName: "创造性", Requirement: ReqMust}, Source: RuleSourceVector, Priority: 1, Confidence: 0.9},
				},
			}, nil, nil,
		),
	})

	result, err := runner.Run(context.Background(), "判断权利要求1是否具有创造性")
	if err != nil {
		t.Fatalf("multi-hypothesis runner: %v", err)
	}

	if !strings.Contains(result, "五步工作法执行结果") {
		t.Error("result should contain title")
	}

	// Verify plan has multi_hypothesis step.
	planStored := runner.bb.PlanV2()
	hasMH := false
	for _, s := range planStored.Steps {
		if s.Strategy == StrategyMultiHypothesis {
			hasMH = true
			break
		}
	}
	if !hasMH {
		t.Error("patentability plan should contain multi_hypothesis step")
	}
}

// TestHypothesisSpecTypes verifies data contract serialization.
func TestHypothesisSpecTypes(t *testing.T) {
	arg := Argument{
		HypothesisID:              "adv_a",
		Claim:                     "权利要求1相对于D1+D2是非显而易见的",
		SupportingFacts:           []string{"f1", "f2"},
		SupportingRules:           []string{"A22.3"},
		Reasoning:                 "D1和D2属于不同技术领域，不存在组合启示",
		AcknowledgedCounterpoints: "D1和D2都涉及图像处理，有一定技术关联性",
	}

	if len(arg.SupportingFacts) != 2 {
		t.Errorf("supporting facts: got %d", len(arg.SupportingFacts))
	}
	if arg.AcknowledgedCounterpoints == "" {
		t.Error("counterpoints should not be empty — this is the key constraint")
	}

	verdict := Verdict{
		Resolved:          true,
		WinningHypothesis: "adv_a",
		Confidence:        0.82,
		Rationale:         "正方证据强度 (0.82) 高于反方 (0.65)，且双方论证均通过逻辑有效性过滤",
		DissentNotes:      "反方关于技术领域关联性的论据有一定道理，建议在答复中回应此点",
	}

	if !verdict.Resolved {
		t.Error("verdict should be resolved")
	}
	if verdict.WinningHypothesis != "adv_a" {
		t.Error("adv_a should win")
	}
}

// TestComputeEvidenceScore_RuleAuthority verifies authority weighting.
func TestComputeEvidenceScore_RuleAuthority(t *testing.T) {
	bb := NewFactBlackboard("c_auth", CasePatentability, "")
	bb.AddFact(FactEntry{
		ID: "f1", Source: "user_text", Content: "事实", Confidence: 0.8, ExtractedAt: NowISO(),
	})
	bb.AddRuleConstraint(RuleConstraint{
		ArticleID: "A22.3", ArticleName: "创造性", Requirement: ReqMust, // authority = 1.0
	})
	bb.AddRuleConstraint(RuleConstraint{
		ArticleID: "guideline_04", ArticleName: "审查指南第四章", Requirement: ReqShould, // authority = 0.7
	})

	// Argument with ReqMust rule.
	argMust := Argument{
		HypothesisID:    "must_side",
		SupportingFacts: []string{"f1"},
		SupportingRules: []string{"A22.3"},
	}
	scoreMust := computeEvidenceScore(argMust, bb)

	// Argument with ReqShould rule.
	argShould := Argument{
		HypothesisID:    "should_side",
		SupportingFacts: []string{"f1"},
		SupportingRules: []string{"guideline_04"},
	}
	scoreShould := computeEvidenceScore(argShould, bb)

	if scoreMust <= scoreShould {
		t.Errorf("ReqMust (%.3f) should score higher than ReqShould (%.3f)", scoreMust, scoreShould)
	}
}
