package disclosure

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBuildMarkdownReport_Full(t *testing.T) {
	report := &AnalysisReport{
		ID:          "rpt_001",
		GeneratedAt: time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC),
		Document: &DisclosureDoc{
			Title:       "智能伸缩支架",
			Format:      "txt",
			HasDrawings: true,
			FigureRefs:  []string{"图1", "图2"},
		},
		Extraction: &ExtractionResult{
			Features: []TechFeature{
				{ID: "F1", Description: "伸缩臂", Category: CatStructure, Function: "调节高度", PriorArtStatus: "unknown", Importance: "high"},
				{ID: "F2", Description: "压力传感器", Category: CatParameter, Function: "检测负重", PriorArtStatus: "known", Importance: "medium"},
			},
			Problems: []string{"现有支架调节不便"},
			Effects:  []string{"自动调节高度"},
		},
		Consistency: &ConsistencyResult{
			Pass:         true,
			OverallScore: 0.95,
		},
		SearchKeywords: []string{"伸缩支架", "智能控制"},
		Novelty: &NoveltyResult{
			Assessed:   true,
			Conclusion: "部分特征具有新颖性",
			Notes:      "详细分析内容",
		},
		ReportText:      "## 详细分析\n\n内容",
		ReviewedByHuman: false,
	}

	md := buildMarkdownReport(report)
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}

	// Verify key sections exist
	checks := []string{"技术交底书分析报告", "智能伸缩支架", "伸缩臂", "压力传感器",
		"现有支架调节不便", "95%", "伸缩支架", "部分特征具有新颖性",
		"免责声明", "尚未经人工复核"}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("markdown missing: %s", c)
		}
	}
}

func TestBuildMarkdownReport_Nil(t *testing.T) {
	md := buildMarkdownReport(nil)
	if md != "（空报告）" {
		t.Errorf("expected empty report marker, got %q", md)
	}
}

func TestBuildMarkdownReport_Reviewed(t *testing.T) {
	report := &AnalysisReport{
		ID:              "rpt_002",
		GeneratedAt:     time.Now(),
		ReviewedByHuman: true,
	}
	md := buildMarkdownReport(report)
	if strings.Contains(md, "尚未经人工复核") {
		t.Error("should not show unreviewed warning when reviewed")
	}
}

func TestExportReport_Markdown(t *testing.T) {
	report := &AnalysisReport{
		ID:          "rpt_003",
		GeneratedAt: time.Now(),
	}
	data, err := ExportReport(report, FormatMarkdown)
	if err != nil {
		t.Fatalf("ExportReport markdown: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty data")
	}
}

func TestSaveReport_Markdown(t *testing.T) {
	report := &AnalysisReport{ID: "rpt_004", GeneratedAt: time.Now()}
	tmpFile := os.TempDir() + "/mady_test_export.md"
	defer os.Remove(tmpFile)

	if err := SaveReport(report, tmpFile); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty saved file")
	}
}

func TestSaveReport_DOCX(t *testing.T) {
	// Only run if pandoc is available.
	if _, err := exec.LookPath("pandoc"); err != nil {
		t.Skip("pandoc not available, skipping DOCX test")
	}

	report := &AnalysisReport{
		ID:          "rpt_005",
		GeneratedAt: time.Now(),
		Extraction: &ExtractionResult{
			Features: []TechFeature{
				{ID: "F1", Description: "测试特征"},
			},
		},
	}
	tmpFile := os.TempDir() + "/mady_test_export.docx"
	defer os.Remove(tmpFile)

	if err := SaveReport(report, tmpFile); err != nil {
		t.Fatalf("SaveReport DOCX: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty docx")
	}
}
