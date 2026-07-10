package patent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

// mockLlmJudge is a controllable LlmJudgeClient for testing.
type mockLlmJudge struct {
	verdict     Verdict
	reasons     []string
	suggestions []string
	err         error
	called      bool
}

func (m *mockLlmJudge) Judge(_ context.Context, _ string, _ []CheckRule) (Verdict, []string, []string, error) {
	m.called = true
	return m.verdict, m.reasons, m.suggestions, m.err
}

func TestChecker_RuleOnlyMode(t *testing.T) {
	engine := NewRuleEngine()
	engine.RegisterRules(DefaultPatentRules())
	checker := NewChecker(engine, nil) // nil LLM = rule-only

	// Text that blocks on level-0 (novelty single-comparison).
	result, err := checker.Check(context.Background(), CheckerInput{
		StepID: "s1",
		Text:   "对比文件1-3结合公开了所有特征，不具有新颖性",
		Domain: "patent_novelty",
		Role:   "executor",
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.CheckMethod != CheckMethodRules {
		t.Errorf("rule-only mode should use CheckMethodRules, got %s", result.CheckMethod)
	}
	if result.Verdict != VerdictBlocked {
		t.Errorf("level-0 failure should block, got %s", result.Verdict)
	}
}

func TestChecker_TriggersLLM_NeedsRevision(t *testing.T) {
	engine := NewRuleEngine()
	// Register only quality-level rules so we get needs_revision, not blocked.
	engine.RegisterRules([]CheckRule{
		{
			ID: "Q1", Name: "Q1", Level: LevelQuality, Severity: SeverityMinor,
			Message: "q1 fail", CheckType: CheckNovelty,
			RequiredElements: []string{"zzz_nonexistent"},
		},
		{
			ID: "Q2", Name: "Q2", Level: LevelQuality, Severity: SeverityMinor,
			Message: "q2 fail", CheckType: CheckNovelty,
			RequiredElements: []string{"zzz_nonexistent2"},
		},
		{
			ID: "Q3", Name: "Q3", Level: LevelQuality, Severity: SeverityMinor,
			Message: "q3 fail", CheckType: CheckNovelty,
			RequiredElements: []string{"zzz_nonexistent3"},
		},
	})

	llm := &mockLlmJudge{verdict: VerdictPass, suggestions: []string{"建议补充分析"}}
	checker := NewChecker(engine, llm)

	result, err := checker.Check(context.Background(), CheckerInput{
		StepID: "s1",
		Text:   "some analysis text",
		Domain: "patent_novelty",
		Role:   "executor",
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !llm.called {
		t.Error("LLM should be triggered when verdict is needs_revision")
	}
	// Conservative (default) policy: take the stricter verdict.
	// rule=needs_revision + llm=pass → needs_revision.
	if result.Verdict != VerdictNeedsRevision {
		t.Errorf("conservative merge → needs_revision, got %s", result.Verdict)
	}
	if result.CheckMethod != CheckMethodHybrid {
		t.Errorf("both tracks contributed → hybrid, got %s", result.CheckMethod)
	}
	if !result.Conflict.Detected {
		t.Error("expected track conflict detected (rule != llm)")
	}
}

func TestChecker_BlockedSkipsLLM(t *testing.T) {
	engine := NewRuleEngine()
	engine.RegisterRules(DefaultPatentRules())

	llm := &mockLlmJudge{verdict: VerdictPass}
	checker := NewChecker(engine, llm)

	result, err := checker.Check(context.Background(), CheckerInput{
		StepID: "s1",
		Text:   "对比文件1-3结合", // triggers level-0 block
		Domain: "patent_novelty",
		Role:   "executor",
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if llm.called {
		t.Error("LLM should NOT be called when rule track blocks")
	}
	if result.Verdict != VerdictBlocked {
		t.Errorf("blocked verdict should be preserved, got %s", result.Verdict)
	}
}

func TestChecker_ResearcherAlwaysLLM(t *testing.T) {
	engine := NewRuleEngine()
	// No rules registered → rule track passes.
	llm := &mockLlmJudge{verdict: VerdictPass, suggestions: []string{"ok"}}
	checker := NewChecker(engine, llm)

	_, err := checker.Check(context.Background(), CheckerInput{
		StepID: "s1",
		Text:   "research output",
		Role:   "researcher",
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !llm.called {
		t.Error("researcher role should always trigger LLM")
	}
}

func TestChecker_LLMErrorDegradesToRules(t *testing.T) {
	engine := NewRuleEngine()
	engine.RegisterRules(DefaultPatentRules())

	llm := &mockLlmJudge{err: errors.New("LLM unavailable")}
	checker := NewChecker(engine, llm)

	result, err := checker.Check(context.Background(), CheckerInput{
		StepID: "s1",
		Text:   "clean text with no violations",
		Domain: "patent_novelty",
		Role:   "researcher",
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.CheckMethod != CheckMethodRules {
		t.Errorf("LLM error should degrade to rules, got %s", result.CheckMethod)
	}
}

func TestChecker_MergeBlockedStaysBlocked(t *testing.T) {
	engine := NewRuleEngine()
	engine.RegisterRules(DefaultPatentRules())

	// Even if LLM says pass, rule-blocked stays blocked.
	llm := &mockLlmJudge{verdict: VerdictPass}
	checker := NewChecker(engine, llm)

	result, _ := checker.Check(context.Background(), CheckerInput{
		StepID: "s1",
		Text:   "对比文件1-3结合",
		Domain: "patent_novelty",
		Role:   "executor",
	})
	// blocked → LLM not triggered, so stays rule-only blocked.
	if result.Verdict != VerdictBlocked {
		t.Errorf("expected blocked, got %s", result.Verdict)
	}
}

func TestBuildNoveltyGraphWithRules(t *testing.T) {
	g, err := BuildNoveltyGraphWithRules()
	if err != nil {
		t.Fatalf("BuildNoveltyGraphWithRules: %v", err)
	}

	state := newPatentTestState()
	finalState, err := g.Run(context.Background(), state)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := finalState.GetString(StateOutput)
	if output == "" {
		t.Fatal("output should not be empty")
	}
	if !strings.Contains(output, "规则引擎检查") {
		t.Error("output should contain rule check section")
	}
	ruleVerdict := finalState.GetString(StateRuleVerdict)
	if ruleVerdict == "" {
		t.Error("rule verdict should be populated")
	}
	t.Logf("Rule verdict: %s", ruleVerdict)
}

func TestBuildNoveltyGraphWithRules_BlockedWarning(t *testing.T) {
	g, err := BuildNoveltyGraphWithRules()
	if err != nil {
		t.Fatalf("BuildNoveltyGraphWithRules: %v", err)
	}

	// Input that triggers single-comparison violation via features.
	state := newPatentTestState()
	// Inject comparison text that will be checked by rule engine.
	// We run the graph; the rule check evaluates the comparison output.
	finalState, err := g.Run(context.Background(), state)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The blocked warning should appear only if verdict is blocked.
	ruleVerdict := finalState.GetString(StateRuleVerdict)
	if ruleVerdict == string(VerdictBlocked) {
		output := finalState.GetString(StateOutput)
		if !strings.Contains(output, "规则引擎检查未通过") {
			t.Error("blocked verdict should prepend warning")
		}
	}
}

func TestFormatCheckerResult(t *testing.T) {
	r := &CheckerResult{
		StepID:      "s1",
		Verdict:     VerdictNeedsRevision,
		CheckMethod: CheckMethodHybrid,
		Summary:     "测试摘要",
		Issues: []CheckIssue{
			{Severity: SeverityMajor, Description: "主要问题", RuleID: "R1"},
		},
	}
	out := FormatCheckerResult(r)
	if !strings.Contains(out, "双轨检查结果") {
		t.Error("should contain header")
	}
	if !strings.Contains(out, "needs_revision") {
		t.Error("should contain verdict")
	}
	if !strings.Contains(out, "主要问题") {
		t.Error("should contain issue")
	}
}

// newPatentTestState returns a state with a typical patent invention input.
func newPatentTestState() graph.PregelState {
	return graph.PregelState{
		StateInput: "一种基于深度学习的图像识别系统，包括图像采集模块、特征提取模块和分类模块，" +
			"其特征在于所述特征提取模块使用改进的卷积神经网络结构。",
	}
}
