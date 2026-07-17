package evaluate

import (
	"strings"
	"testing"
)

func TestBuildEnhancedReport_Nil(t *testing.T) {
	if got := BuildEnhancedReport(nil, nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestBuildEnhancedReport_Basic(t *testing.T) {
	report := &BatchReport{
		TotalCases:  2,
		PassedCases: 1,
		PassRate:    0.5,
		AggregateScores: map[string]float64{
			"f1": 0.65,
		},
		Results: []CaseResult{
			{CaseID: "case1", Average: 0.3, Passed: false, Scores: map[string]float64{"f1": 0.3}},
			{CaseID: "case2", Average: 1.0, Passed: true, Scores: map[string]float64{"f1": 1.0}},
		},
	}

	enh := BuildEnhancedReport(report, nil)
	if enh == nil {
		t.Fatal("expected non-nil")
	}
	if enh.TotalCases != 2 {
		t.Errorf("TotalCases = %d, want 2", enh.TotalCases)
	}
	if enh.PassRate != 0.5 {
		t.Errorf("PassRate = %v, want 0.5", enh.PassRate)
	}

	// Check metric breakdown.
	mb, ok := enh.MetricBreakdown["f1"]
	if !ok {
		t.Fatal("expected metric 'f1' in breakdown")
	}
	if !approxEq(mb.Mean, 0.65, 0.001) {
		t.Errorf("Mean = %v, want 0.65", mb.Mean)
	}
	if mb.PassCount != 1 {
		t.Errorf("PassCount = %d, want 1", mb.PassCount)
	}
	if mb.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1", mb.FailCount)
	}

	// Check percentiles (2 data points: [0.3, 1.0] → P50 = interpolated 0.65).
	if !approxEq(enh.Percentiles.P50, 0.65, 0.001) {
		t.Errorf("P50 = %v, want 0.65", enh.Percentiles.P50)
	}

	// Check worst/best cases (≤5 cases: both return the full sorted list).
	if len(enh.WorstCases) != 2 {
		t.Errorf("WorstCases=%d, want 2", len(enh.WorstCases))
	}
	if len(enh.BestCases) != 2 {
		t.Errorf("BestCases=%d, want 2", len(enh.BestCases))
	}
}

func TestBuildEnhancedReport_Trend(t *testing.T) {
	current := &BatchReport{
		TotalCases:  2,
		PassedCases: 2,
		PassRate:    1.0,
		Results: []CaseResult{
			{CaseID: "a", Passed: true, Average: 0.9, Scores: map[string]float64{"f1": 0.9}},
			{CaseID: "b", Passed: true, Average: 0.9, Scores: map[string]float64{"f1": 0.9}},
		},
	}
	previous := &BatchReport{
		TotalCases:  2,
		PassedCases: 1,
		PassRate:    0.5,
		Results: []CaseResult{
			{CaseID: "a", Passed: true, Average: 0.9, Scores: map[string]float64{"f1": 0.9}},
			{CaseID: "b", Passed: false, Average: 0.3, Scores: map[string]float64{"f1": 0.3}},
		},
	}

	enh := BuildEnhancedReport(current, previous)
	if enh.Trend == nil {
		t.Fatal("expected trend")
	}
	if enh.Trend.Delta != 0.5 {
		t.Errorf("Delta = %v, want 0.5", enh.Trend.Delta)
	}
	if len(enh.Trend.RegressedCases) != 0 {
		t.Errorf("RegressedCases = %v, want none", enh.Trend.RegressedCases)
	}
	if len(enh.Trend.ImprovedCases) != 1 || enh.Trend.ImprovedCases[0] != "b" {
		t.Errorf("ImprovedCases = %v, want [b]", enh.Trend.ImprovedCases)
	}
}

func TestBuildEnhancedReport_Regressions(t *testing.T) {
	current := &BatchReport{
		PassedCases: 1,
		TotalCases:  2,
		PassRate:    0.5,
		Results: []CaseResult{
			{CaseID: "a", Passed: false, Average: 0.3, Scores: map[string]float64{"f1": 0.3}},
			{CaseID: "b", Passed: true, Average: 0.9, Scores: map[string]float64{"f1": 0.9}},
		},
	}
	previous := &BatchReport{
		PassedCases: 2,
		TotalCases:  2,
		PassRate:    1.0,
		Results: []CaseResult{
			{CaseID: "a", Passed: true, Average: 0.9, Scores: map[string]float64{"f1": 0.9}},
			{CaseID: "b", Passed: true, Average: 0.9, Scores: map[string]float64{"f1": 0.9}},
		},
	}

	enh := BuildEnhancedReport(current, previous)
	if len(enh.Trend.RegressedCases) != 1 || enh.Trend.RegressedCases[0] != "a" {
		t.Errorf("RegressedCases = %v, want [a]", enh.Trend.RegressedCases)
	}
}

func TestFormatEnhancedReport_Nil(t *testing.T) {
	got := FormatEnhancedReport(nil)
	if !strings.Contains(got, "无数据") {
		t.Errorf("expected '无数据' in output")
	}
}

func TestFormatEnhancedReport_BasicOutput(t *testing.T) {
	report := &BatchReport{
		TotalCases:  1,
		PassedCases: 1,
		PassRate:    1.0,
		AggregateScores: map[string]float64{
			"f1": 0.95,
		},
		Results: []CaseResult{
			{CaseID: "test1", Average: 0.95, Passed: true, Scores: map[string]float64{"f1": 0.95}},
		},
	}
	enh := BuildEnhancedReport(report, nil)
	output := FormatEnhancedReport(enh)

	if !strings.Contains(output, "增强评估报告") {
		t.Errorf("output missing title")
	}
	if !strings.Contains(output, "100.0%") {
		t.Errorf("output missing pass rate")
	}
	if !strings.Contains(output, "test1") {
		t.Errorf("output missing case ID")
	}
}

func TestSortedByAverage(t *testing.T) {
	results := []CaseResult{
		{CaseID: "high", Average: 0.9},
		{CaseID: "low", Average: 0.1},
		{CaseID: "mid", Average: 0.5},
	}
	sorted := sortedByAverage(results)
	if sorted[0].CaseID != "low" {
		t.Errorf("first = %q, want %q", sorted[0].CaseID, "low")
	}
	if sorted[2].CaseID != "high" {
		t.Errorf("last = %q, want %q", sorted[2].CaseID, "high")
	}
}

func TestPercentile(t *testing.T) {
	sorted := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}
	if !approxEq(percentile(sorted, 0.5), 0.55, 0.001) {
		t.Errorf("P50 = %v, want 0.55", percentile(sorted, 0.5))
	}
	if !approxEq(percentile(sorted, 0.1), 0.19, 0.001) {
		t.Errorf("P10 = %v, want 0.19", percentile(sorted, 0.1))
	}
	if !approxEq(percentile(sorted, 0.9), 0.91, 0.001) {
		t.Errorf("P90 = %v, want 0.91", percentile(sorted, 0.9))
	}
}

func TestPercentile_Empty(t *testing.T) {
	if p := percentile(nil, 0.5); p != 0 {
		t.Errorf("empty: got %v, want 0", p)
	}
}

func TestFloatHelpers(t *testing.T) {
	if m := meanFloat64([]float64{1, 2, 3}); m != 2 {
		t.Errorf("mean = %v, want 2", m)
	}
	if m := meanFloat64(nil); m != 0 {
		t.Errorf("empty mean = %v, want 0", m)
	}
	if m := minFloat64([]float64{3, 0.5, 1}); m != 0.5 {
		t.Errorf("min = %v, want 0.5", m)
	}
	if m := maxFloat64([]float64{3, 0.5, 1}); m != 3 {
		t.Errorf("max = %v, want 3", m)
	}
	sd := stdDevFloat64([]float64{1, 2, 3}, 2)
	if !approxEq(sd, 1.0, 0.001) {
		t.Errorf("std dev = %v, want 1.0", sd)
	}
}
