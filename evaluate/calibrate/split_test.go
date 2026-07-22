package calibrate

import (
	"context"
	"testing"

	"github.com/xujian519/mady/evaluate"
)

func TestFilterByEra(t *testing.T) {
	cases := []evaluate.TestCase{
		{ID: "c1", Era: "pre_2020"},
		{ID: "c2", Era: "post_2020"},
		{ID: "c3", Era: "pre_2020"},
	}

	filtered := FilterByEra(cases, "pre_2020")
	if len(filtered) != 2 {
		t.Errorf("expected 2 pre_2020 cases, got %d", len(filtered))
	}

	filtered = FilterByEra(cases, "nonexistent")
	if len(filtered) != 0 {
		t.Errorf("expected 0 cases for nonexistent era, got %d", len(filtered))
	}
}

func TestFilterByDifficulty(t *testing.T) {
	cases := []evaluate.TestCase{
		{ID: "c1", Difficulty: "easy"},
		{ID: "c2", Difficulty: "hard"},
		{ID: "c3", Difficulty: "medium"},
		{ID: "c4", Difficulty: "easy"},
	}

	filtered := FilterByDifficulty(cases, "easy")
	if len(filtered) != 2 {
		t.Errorf("expected 2 easy cases, got %d", len(filtered))
	}

	filtered = FilterByDifficulty(cases, "hard")
	if len(filtered) != 1 {
		t.Errorf("expected 1 hard case, got %d", len(filtered))
	}
}

func TestEvaluateSplit_Empty(t *testing.T) {
	// 空 case 列表应正常返回，无崩溃
	eval := evaluate.NewEvaluator()
	report, err := EvaluateSplit(context.TODO(), nil, nil, eval, SplitConfig{
		TrainEra: "pre_2020",
		TestEras: []string{"post_2020"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Train != nil {
		t.Error("expected nil train report for empty cases")
	}
	if len(report.Test) != 0 {
		t.Errorf("expected 0 test reports, got %d", len(report.Test))
	}
}
