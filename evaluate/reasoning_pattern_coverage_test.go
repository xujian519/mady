package evaluate

import (
	"context"
	"strings"
	"testing"
)

func TestReasoningPatternCoverage_Name(t *testing.T) {
	r := ReasoningPatternCoverage{}
	if got := r.Name(); got != "reasoning_pattern_coverage" {
		t.Errorf("Name() = %q, want %q", got, "reasoning_pattern_coverage")
	}
}

func TestReasoningPatternCoverage_Evaluate(t *testing.T) {
	r := ReasoningPatternCoverage{}
	report, err := r.Evaluate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Evaluate() returned error: %v", err)
	}
	if report == nil {
		t.Fatal("Evaluate() returned nil report")
	}

	// Should have 18 reasoning patterns.
	if report.TotalPatterns != 18 {
		t.Errorf("TotalPatterns = %d, want 18", report.TotalPatterns)
	}

	// All patterns should have at least 1 CheckRule.
	if report.CoveredPatterns != report.TotalPatterns {
		t.Errorf("CoveredPatterns = %d, want %d (all covered)", report.CoveredPatterns, report.TotalPatterns)
	}

	// Coverage rate should be 1.0 (100%).
	if report.CoverageRate != 1.0 {
		t.Errorf("CoverageRate = %f, want 1.0", report.CoverageRate)
	}

	// Should have all 4 categories.
	expectedCategories := []string{"creativity", "novelty", "claims", "other"}
	for _, cat := range expectedCategories {
		if _, ok := report.PatternsByCategory[cat]; !ok {
			t.Errorf("missing category %q", cat)
		}
	}

	// TotalRules should be >= 18 (at least 1 per pattern).
	if report.TotalRules < 18 {
		t.Errorf("TotalRules = %d, want >= 18", report.TotalRules)
	}
}

func TestFormatCoverageReport(t *testing.T) {
	r := ReasoningPatternCoverage{}
	report, err := r.Evaluate(context.Background(), map[string]any{"detail": true})
	if err != nil {
		t.Fatalf("Evaluate() returned error: %v", err)
	}

	output := FormatCoverageReport(report)
	if output == "" {
		t.Fatal("FormatCoverageReport() returned empty string")
	}
	if !strings.Contains(output, "推理模式规则覆盖率") {
		t.Error("output should contain header")
	}
	if !strings.Contains(output, "分类覆盖率") {
		t.Error("output should contain category section")
	}
	if !strings.Contains(output, "创造性") {
		t.Error("output should contain creativity category")
	}
	if !strings.Contains(output, "新颖性") {
		t.Error("output should contain novelty category")
	}
}

func TestFormatCoverageReport_Nil(t *testing.T) {
	output := FormatCoverageReport(nil)
	if output == "" {
		t.Error("FormatCoverageReport(nil) should not be empty")
	}
	if !strings.Contains(output, "无覆盖率数据") {
		t.Error("nil report should indicate no data")
	}
}
