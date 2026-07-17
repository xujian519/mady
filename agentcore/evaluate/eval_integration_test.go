package evaluate

import (
	"strings"
	"testing"
)

// TestEvalIntegration_ToolAccuracyFullPipeline tests the full evaluation pipeline
// with ToolAccuracy metrics: TestCase → Evaluator → BatchReport → EnhancedReport.
func TestEvalIntegration_ToolAccuracyFullPipeline(t *testing.T) {
	cases := []TestCase{
		{
			ID:       "tool_correct",
			Domain:   "patent",
			Input:    "搜索AI相关专利",
			Expected: `[{"name":"search_patents","arguments":{"query":"人工智能"}}]`,
		},
		{
			ID:       "tool_wrong_name",
			Domain:   "patent",
			Input:    "读取文件",
			Expected: `[{"name":"read_document","arguments":{"path":"test.txt"}}]`,
		},
	}

	predictions := map[string]string{
		"tool_correct":    `[{"name":"search_patents","arguments":{"query":"人工智能"}}]`,
		"tool_wrong_name": `[{"name":"wrong_tool","arguments":{"path":"test.txt"}}]`,
	}

	evaluator := NewEvaluator(ToolAccuracy{})
	report := evaluator.EvaluateStatic(cases, predictions)

	if report.TotalCases != 2 {
		t.Fatalf("TotalCases = %d, want 2", report.TotalCases)
	}
	if report.PassedCases != 1 {
		t.Errorf("PassedCases = %d, want 1", report.PassedCases)
	}

	// Build enhanced report.
	enh := BuildEnhancedReport(report, nil)
	if enh == nil {
		t.Fatal("expected non-nil EnhancedReport")
	}

	// Verify metric breakdown exists.
	if _, ok := enh.MetricBreakdown["tool_accuracy"]; !ok {
		t.Fatal("expected tool_accuracy metric in breakdown")
	}

	// Format and check output.
	output := FormatEnhancedReport(enh)
	if !strings.Contains(output, "增强评估报告") {
		t.Errorf("output missing title")
	}
	if !strings.Contains(output, "tool_correct") {
		t.Errorf("output missing tool_correct case")
	}
	if !strings.Contains(output, "tool_wrong_name") {
		t.Errorf("output missing tool_wrong_name case")
	}

	// Also verify FormatReport still works.
	baseOutput := FormatReport(report)
	if !strings.Contains(baseOutput, "评估报告") {
		t.Errorf("base report missing title")
	}
}

// TestEvalIntegration_WorkflowQualityFullPipeline tests the full pipeline
// with WorkflowQuality metrics.
func TestEvalIntegration_WorkflowQualityFullPipeline(t *testing.T) {
	cases := []TestCase{
		{
			ID:       "wf_correct",
			Domain:   "patent",
			Input:    "complete workflow",
			Expected: `[{"step":"search","status":"completed"},{"step":"analyze","status":"completed"}]`,
		},
	}

	predictions := map[string]string{
		"wf_correct": `[{"step":"search","status":"completed"},{"step":"analyze","status":"completed"}]`,
	}

	evaluator := NewEvaluator(WorkflowQuality{})
	report := evaluator.EvaluateStatic(cases, predictions)

	if report.TotalCases != 1 {
		t.Fatalf("TotalCases = %d, want 1", report.TotalCases)
	}
	if !report.Results[0].Passed {
		t.Error("expected case to pass")
	}

	// Enhanced report with trend.
	prevReport := &BatchReport{
		TotalCases:  1,
		PassedCases: 0,
		PassRate:    0.0,
		Results: []CaseResult{
			{CaseID: "wf_correct", Passed: false, Average: 0.3, Scores: map[string]float64{"workflow_quality": 0.3}},
		},
	}

	enh := BuildEnhancedReport(report, prevReport)
	if enh.Trend == nil {
		t.Fatal("expected trend")
	}
	if enh.Trend.Delta <= 0 {
		t.Errorf("expected positive trend delta, got %v", enh.Trend.Delta)
	}
	if len(enh.Trend.ImprovedCases) != 1 {
		t.Errorf("expected 1 improved case, got %v", enh.Trend.ImprovedCases)
	}
}

// TestEvalIntegration_MultiMetricReport tests multi-metric evaluation with
// the new metrics combined.
func TestEvalIntegration_MultiMetricReport(t *testing.T) {
	cases := []TestCase{
		{
			ID:       "multi_1",
			Domain:   "patent",
			Input:    "multi test",
			Expected: `[{"name":"search","arguments":{"q":"test"}}]`,
		},
	}

	predictions := map[string]string{
		"multi_1": `[{"name":"search","arguments":{"q":"test"}}]`,
	}

	evaluator := NewEvaluator(
		ToolAccuracy{},
		WorkflowQuality{},
		F1Score{},
	)
	report := evaluator.EvaluateStatic(cases, predictions)

	if len(report.AggregateScores) != 3 {
		t.Errorf("expected 3 aggregate scores, got %d", len(report.AggregateScores))
	}

	for _, name := range []string{"tool_accuracy", "workflow_quality", "f1"} {
		if _, ok := report.AggregateScores[name]; !ok {
			t.Errorf("missing aggregate metric %q", name)
		}
	}
}

// TestEvalIntegration_LoaderToolAccuracy tests loading fixture files and
// evaluating them with ToolAccuracy.
func TestEvalIntegration_LoaderToolAccuracy(t *testing.T) {
	loader := &Loader{}
	result, err := loader.LoadDir("testdata")
	if err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}

	cases, ok := result["tool_accuracy"]
	if !ok || len(cases) == 0 {
		t.Fatal("expected tool_accuracy cases from testdata")
	}

	// Evaluate with ToolAccuracy metric.
	// Each fixture's Expected field contains the correct tool call JSON for
	// the specified input. For static evaluation, use the Expected as prediction.
	predictions := make(map[string]string, len(cases))
	for _, c := range cases {
		predictions[c.ID] = c.Expected
	}

	evaluator := NewEvaluator(ToolAccuracy{})
	report := evaluator.EvaluateStatic(cases, predictions)

	if report.TotalCases != len(cases) {
		t.Errorf("TotalCases = %d, want %d", report.TotalCases, len(cases))
	}
	// All predictions should match expectations exactly.
	if report.PassRate != 1.0 {
		t.Errorf("PassRate = %v, want 1.0 (all predictions match expectations)", report.PassRate)
	}

	// Verify enhanced report output.
	enh := BuildEnhancedReport(report, nil)
	output := FormatEnhancedReport(enh)
	if !strings.Contains(output, "tool_accuracy") {
		t.Errorf("enhanced report missing metric name")
	}
}
