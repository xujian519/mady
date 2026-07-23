package infringement

import (
	"context"
	"encoding/json"
	"testing"
)

func TestInfringementInput_JSONRoundTrip(t *testing.T) {
	input := &InfringementInput{
		PatentClaims:   "一种图像识别方法，包括：步骤A、步骤B、步骤C",
		AccusedProduct: "智能相机系统，实现步骤A、步骤B'、步骤C、步骤D",
		Perspective:    PerspectivePatentee,
		PatentType:     PatentTypeInvention,
	}
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out InfringementInput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Perspective != PerspectivePatentee {
		t.Errorf("perspective = %s, want patentee", out.Perspective)
	}
}

func TestInfringementOutput_JSONRoundTrip(t *testing.T) {
	output := &InfringementOutput{
		Verdict: InfringementVerdict{
			Conclusion:  "infringed",
			Likelihood:  0.85,
			Basis:       []string{"literal", "equivalence"},
			KeyFindings: []string{"特征A完全匹配", "特征B构成等同"},
			RiskLevel:   "high",
		},
		Confidence: 0.85,
		Disclaimer: "本分析不构成法律意见",
	}
	b, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out InfringementOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Verdict.Conclusion != "infringed" {
		t.Errorf("conclusion = %s, want infringed", out.Verdict.Conclusion)
	}
}

// --- Rule engine tests ---

func TestAllElementsRule_MissingFeature(t *testing.T) {
	rule := &allElementsRule{}
	input := &RuleCheckInput{
		LiteralResult: &LiteralResult{
			AllElementsMet:  false,
			MissingFeatures: []string{"特征B"},
		},
	}
	result, err := rule.Check(context.Background(), input)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Passed {
		t.Error("expected rule to fail when feature is missing")
	}
}

func TestAllElementsRule_AllMatched(t *testing.T) {
	rule := &allElementsRule{}
	input := &RuleCheckInput{
		LiteralResult: &LiteralResult{AllElementsMet: true},
	}
	result, err := rule.Check(context.Background(), input)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !result.Passed {
		t.Error("expected rule to pass when all elements matched")
	}
}

func TestAllElementsRule_AllMatchedWithExtra(t *testing.T) {
	rule := &allElementsRule{}
	input := &RuleCheckInput{
		LiteralResult: &LiteralResult{
			AllElementsMet: true,
			ExtraFeatures:  []string{"额外特征D"},
		},
	}
	result, err := rule.Check(context.Background(), input)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !result.Passed {
		t.Error("extra features should not cause rule to fail")
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected suggestion about extra features")
	}
}

func TestEquivalenceTestRule_LogicalContradiction(t *testing.T) {
	rule := &equivalenceTestRule{}
	input := &RuleCheckInput{
		EquivalenceResult: &EquivalenceResult{
			EquivalentFeatures: []EquivalenceAssessment{
				{
					ClaimFeature:   "特征A",
					ProductFeature: "特征A'",
					SameMeans:      false,
					SameFunction:   false,
					SameEffect:     false,
					IsEquivalent:   true,
				},
			},
		},
	}
	result, err := rule.Check(context.Background(), input)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Passed {
		t.Error("expected rule to detect logical contradiction")
	}
}

func TestEstoppelRule_ProsecutionHistoryProvided(t *testing.T) {
	rule := &estoppelRule{}
	input := &RuleCheckInput{
		ProsecutionHistory: "申请人在OA中限缩了权利要求1的范围",
		EquivalenceResult: &EquivalenceResult{
			EquivalentFeatures: []EquivalenceAssessment{
				{ClaimFeature: "特征A", ProductFeature: "特征A'", IsEquivalent: true},
			},
			EstoppelApplied: false,
			EstoppelDetails: "",
		},
	}
	result, err := rule.Check(context.Background(), input)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Passed {
		t.Error("expected rule to flag missing estoppel check when prosecution history is available")
	}
}

func TestRuleEngine_Check(t *testing.T) {
	engine := NewRuleEngine()
	input := &RuleCheckInput{
		LiteralResult: &LiteralResult{
			AllElementsMet:  false,
			MissingFeatures: []string{"特征B"},
		},
	}
	violations := engine.Check(context.Background(), input)
	if len(violations) == 0 {
		t.Error("expected at least one violation for missing feature")
	}
}

// --- Scorer tests ---

func TestScorer_AllLiteralMatch(t *testing.T) {
	scorer := NewInfringementScorer(nil)
	output := &InfringementOutput{
		LiteralResult: LiteralResult{AllElementsMet: true},
		FeatureMapping: []FeatureComparison{
			{MatchType: "literal"},
			{MatchType: "literal"},
			{MatchType: "literal"},
		},
		EquivalenceResult: EquivalenceResult{},
		DefenseAnalysis:   []DefenseAssessment{},
		RemedyAssessment:  RemedyResult{},
		StrategyAdvice:    StrategyResult{},
	}
	result := scorer.Score(output)
	if result.Dimensions["literal_match"] != 1.0 {
		t.Errorf("literal_match = %f, want 1.0", result.Dimensions["literal_match"])
	}
}

func TestScorer_PartialEquivalence(t *testing.T) {
	scorer := NewInfringementScorer(nil)
	output := &InfringementOutput{
		LiteralResult: LiteralResult{AllElementsMet: false},
		FeatureMapping: []FeatureComparison{
			{MatchType: "literal"},
			{MatchType: "equivalent"},
			{MatchType: "missing"},
		},
		EquivalenceResult: EquivalenceResult{
			EquivalentFeatures: []EquivalenceAssessment{
				{IsEquivalent: true},
				{IsEquivalent: false},
			},
		},
		DefenseAnalysis:  []DefenseAssessment{},
		RemedyAssessment: RemedyResult{},
		StrategyAdvice:   StrategyResult{},
	}
	result := scorer.Score(output)
	if result.Dimensions["equivalence"] != 0.5 {
		t.Errorf("equivalence = %f, want 0.5", result.Dimensions["equivalence"])
	}
}

func TestRiskLevel_High(t *testing.T) {
	if rl := riskLevel(0.75); rl != "high" {
		t.Errorf("riskLevel(0.75) = %s, want high", rl)
	}
}

func TestRiskLevel_Medium(t *testing.T) {
	if rl := riskLevel(0.5); rl != "medium" {
		t.Errorf("riskLevel(0.5) = %s, want medium", rl)
	}
}

func TestRiskLevel_Low(t *testing.T) {
	if rl := riskLevel(0.3); rl != "low" {
		t.Errorf("riskLevel(0.3) = %s, want low", rl)
	}
}

// --- Perspective tests ---

func TestBuildPerspectivePrompt_Patentee(t *testing.T) {
	prompt := buildPerspectivePrompt("中立框架", "原告指令", "被告指令", PerspectivePatentee)
	if !contains(prompt, "专利权人/原告") {
		t.Errorf("prompt should contain 专利权人/原告: %s", prompt)
	}
}

func TestBuildPerspectivePrompt_Defendant(t *testing.T) {
	prompt := buildPerspectivePrompt("中立框架", "原告指令", "被告指令", PerspectiveDefendant)
	if !contains(prompt, "被控侵权人/被告") {
		t.Errorf("prompt should contain 被控侵权人/被告: %s", prompt)
	}
}

// --- Helper tests ---

func TestTruncate_Short(t *testing.T) {
	if s := truncate("hello", 10); s != "hello" {
		t.Errorf("truncate(hello, 10) = %s, want hello", s)
	}
}

func TestTruncate_Long(t *testing.T) {
	s := truncate("hello world", 5)
	if len(s) > 8 {
		t.Errorf("truncate(hello world, 5) too long: %s", s)
	}
}

func TestJoinLines_Empty(t *testing.T) {
	result := joinLines(nil)
	if result != "(无)" {
		t.Errorf("joinLines(nil) = %s, want (无)", result)
	}
}

func TestJoinLines_NonEmpty(t *testing.T) {
	result := joinLines([]string{"A", "B"})
	if !contains(result, "1. A") || !contains(result, "2. B") {
		t.Errorf("joinLines = %s", result)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
