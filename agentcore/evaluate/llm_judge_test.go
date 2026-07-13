package evaluate

import (
	"errors"
	"testing"
)

// mockLLMJudge implements LLMJudgeCaller for testing.
type mockLLMJudge struct {
	agree bool
	err   error
}

func (m *mockLLMJudge) JudgeConsistency(prediction, reference string) (bool, error) {
	return m.agree, m.err
}

func TestNewLLMJudgeFunc_Agree(t *testing.T) {
	caller := &mockLLMJudge{agree: true}
	judge := NewLLMJudgeFunc(caller)
	if !judge("pred", "ref") {
		t.Error("expected true for agree")
	}
}

func TestNewLLMJudgeFunc_Disagree(t *testing.T) {
	caller := &mockLLMJudge{agree: false}
	judge := NewLLMJudgeFunc(caller)
	if judge("pred", "ref") {
		t.Error("expected false for disagree")
	}
}

func TestNewLLMJudgeFunc_Error(t *testing.T) {
	caller := &mockLLMJudge{err: errors.New("LLM unavailable")}
	judge := NewLLMJudgeFunc(caller)
	if judge("pred", "ref") {
		t.Error("expected false on LLM error (conservative fallback)")
	}
}

func TestLLMJudgeConsistency_WithCaller(t *testing.T) {
	caller := &mockLLMJudge{agree: true}
	metric := JudgeConsistency{Judge: NewLLMJudgeFunc(caller)}
	if metric.Compute("pred", "ref") != 1.0 {
		t.Error("expected 1.0 for agree")
	}

	caller2 := &mockLLMJudge{agree: false}
	metric2 := JudgeConsistency{Judge: NewLLMJudgeFunc(caller2)}
	if metric2.Compute("pred", "ref") != 0.0 {
		t.Error("expected 0.0 for disagree")
	}
}

func TestCollectCalibrationSamples_FailedCase(t *testing.T) {
	cases := []TestCase{
		{ID: "c1", Expected: "ref1"},
		{ID: "c2", Expected: "ref2"},
	}
	report := &BatchReport{
		Results: []CaseResult{
			{CaseID: "c1", Passed: false, Average: 0.3},
			{CaseID: "c2", Passed: true, Average: 0.95},
		},
	}
	predictions := map[string]string{"c1": "pred1", "c2": "pred2"}

	samples := CollectCalibrationSamples(report, predictions, cases, 0.5, 0.7)
	if len(samples) == 0 {
		t.Fatal("expected at least 1 sample")
	}

	// c1 failed → must be in samples
	found := false
	for _, s := range samples {
		if s.CaseID == "c1" {
			found = true
			if s.Score != 0.3 {
				t.Errorf("c1 score = %.2f, want 0.3", s.Score)
			}
		}
	}
	if !found {
		t.Error("failed case c1 not in calibration samples")
	}
}

func TestCollectCalibrationSamples_Borderline(t *testing.T) {
	cases := []TestCase{
		{ID: "c1", Expected: "ref1"},
	}
	report := &BatchReport{
		Results: []CaseResult{
			{CaseID: "c1", Passed: true, Average: 0.65}, // borderline (0.7-0.1=0.6 ... 0.7+0.1=0.8)
		},
	}
	predictions := map[string]string{"c1": "pred1"}

	samples := CollectCalibrationSamples(report, predictions, cases, 0.0, 0.7)
	if len(samples) == 0 {
		t.Fatal("expected borderline case to be sampled")
	}
	if samples[0].Reason != "borderline — near threshold" {
		t.Errorf("reason = %s, want borderline", samples[0].Reason)
	}
}

func TestCollectCalibrationSamples_NilReport(t *testing.T) {
	samples := CollectCalibrationSamples(nil, nil, nil, 0.5, 0.7)
	if len(samples) != 0 {
		t.Error("expected 0 samples for nil report")
	}
}

func TestCollectCalibrationSamples_ZeroRate(t *testing.T) {
	cases := []TestCase{
		{ID: "c1", Expected: "ref1"},
	}
	report := &BatchReport{
		Results: []CaseResult{
			{CaseID: "c1", Passed: true, Average: 0.95},
		},
	}
	predictions := map[string]string{"c1": "pred1"}

	// rate=0 but case passes with high score → should not appear
	// (not failed, not borderline, and rate=0 means no random sampling)
	samples := CollectCalibrationSamples(report, predictions, cases, 0.0, 0.7)
	if len(samples) != 0 {
		t.Errorf("expected 0 samples, got %d", len(samples))
	}
}
