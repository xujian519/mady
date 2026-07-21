package benchmark

import (
	"os"
	"strings"
	"testing"

	"github.com/xujian519/mady/evaluate"
)

// TestEvalSuite_GoldenPerfect is the primary CI gate: when predictions match
// the reference answers exactly, every case must pass with a perfect PassRate.
// This validates that the metric chain (F1 + KeywordRecall + Citation +
// JudgeConsistency) is wired correctly and that all benchmark cases are
// well-formed. A failure here means either a metric is broken or a benchmark
// case has an inconsistent Expected answer.
func TestEvalSuite_GoldenPerfect(t *testing.T) {
	cases := AllCases()
	if len(cases) == 0 {
		t.Fatal("AllCases() returned 0 cases — benchmark dataset is empty")
	}

	predictions := make(map[string]string, len(cases))
	for _, c := range cases {
		predictions[c.ID] = c.Expected
	}

	report := RunStatic(predictions)
	if report.PassRate != 1.0 {
		t.Errorf("perfect predictions should yield PassRate=1.0, got %.4f", report.PassRate)
		for _, r := range report.Results {
			if !r.Passed {
				t.Errorf("  case %s FAILED (avg=%.3f): %+v", r.CaseID, r.Average, r.Scores)
			}
		}
	}

	for name, score := range report.AggregateScores {
		if score < 0.99 {
			t.Errorf("metric %s aggregate score should be ~1.0 for perfect predictions, got %.4f", name, score)
		}
	}
}

// TestEvalSuite_Degraded verifies that empty predictions fail the gate.
// This is the negative control: if the suite passes with garbage input, the
// metrics are meaningless.
func TestEvalSuite_Degraded(t *testing.T) {
	cases := AllCases()
	predictions := make(map[string]string, len(cases))
	for _, c := range cases {
		predictions[c.ID] = ""
	}

	report := RunStatic(predictions)
	if report.PassRate > 0 {
		t.Errorf("empty predictions should yield PassRate=0, got %.4f", report.PassRate)
	}
}

// TestEvalSuite_CaseIntegrity validates that every benchmark case has the
// required fields populated. A case with an empty ID, Input, or Expected would
// silently produce misleading results.
func TestEvalSuite_CaseIntegrity(t *testing.T) {
	seen := make(map[string]bool)
	for _, c := range AllCases() {
		if c.ID == "" {
			t.Error("found case with empty ID")
			continue
		}
		if seen[c.ID] {
			t.Errorf("duplicate case ID: %s", c.ID)
		}
		seen[c.ID] = true

		if strings.TrimSpace(c.Input) == "" {
			t.Errorf("case %s has empty Input", c.ID)
		}
		if strings.TrimSpace(c.Expected) == "" {
			t.Errorf("case %s has empty Expected", c.ID)
		}
		if c.Domain == "" {
			t.Errorf("case %s has empty Domain", c.ID)
		}
	}
}

// TestEvalSuite_DefaultEvaluator confirms the default evaluator is configured
// with the expected metric set.
func TestEvalSuite_DefaultEvaluator(t *testing.T) {
	e := DefaultEvaluator()
	if e == nil {
		t.Fatal("DefaultEvaluator() returned nil")
	}

	report := e.EvaluateStatic(
		[]evaluate.TestCase{{ID: "x", Expected: "hello world"}},
		map[string]string{"x": "hello world"},
	)
	if report.PassRate != 1.0 {
		t.Errorf("perfect match should pass, got PassRate=%.4f", report.PassRate)
	}
}

// TestP2AEvalCases_SuiteEnv 是 MADY_EVAL_SUITE=p2a 筛选机制的离线门禁：
// 设置后必须返回全量 31 道 P2A 真题（稳定顺序、与数据集一致），
// 保证"P2A 全量 31 题 live 基线"随时可以一键触发。
func TestP2AEvalCases_SuiteEnv(t *testing.T) {
	t.Setenv("MADY_EVAL_SUITE", "p2a")

	cases := p2aEvalCases(t)
	if len(cases) != 31 {
		t.Fatalf("MADY_EVAL_SUITE=p2a should yield all 31 P2A cases, got %d", len(cases))
	}
	for i, c := range cases {
		if c.ID != PatentExamRealCases()[i].ID {
			t.Fatalf("suite mode should preserve dataset order: case %d = %q, want %q",
				i, c.ID, PatentExamRealCases()[i].ID)
		}
		if !strings.HasPrefix(c.ID, "patent_exam_") {
			t.Errorf("case %q is not a P2A real exam case", c.ID)
		}
	}
}

// TestP2AEvalCases_DefaultSample 验证默认（无 suite 环境变量）时保持
// 原有的固定种子随机抽样行为（默认 3 题），避免影响既有基线对比。
func TestP2AEvalCases_DefaultSample(t *testing.T) {
	os.Unsetenv("MADY_EVAL_SUITE")
	os.Unsetenv("MADY_EVAL_CASES")

	cases := p2aEvalCases(t)
	if len(cases) != 3 {
		t.Fatalf("default sample should be 3 cases, got %d", len(cases))
	}
}

// TestP2AEvalCases_CaseCountEnv 验证 MADY_EVAL_CASES 的题量控制仍生效。
func TestP2AEvalCases_CaseCountEnv(t *testing.T) {
	t.Setenv("MADY_EVAL_CASES", "10")

	cases := p2aEvalCases(t)
	if len(cases) != 10 {
		t.Fatalf("MADY_EVAL_CASES=10 should yield 10 cases, got %d", len(cases))
	}
}
