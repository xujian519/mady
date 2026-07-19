package risk

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockSearcher returns predefined results for testing.
type mockSearcher struct {
	results []CaseResult
}

func (m *mockSearcher) SearchCases(_ context.Context, features []string, maxResults int) ([]CaseResult, error) {
	if len(m.results) > maxResults {
		return m.results[:maxResults], nil
	}
	return m.results, nil
}

func TestScanner_EmptyFeatures(t *testing.T) {
	s := NewScanner(&mockSearcher{}, DefaultScannerConfig())
	result, err := s.ScanByFeatures(context.Background(), nil)
	if err != nil {
		t.Fatalf("ScanByFeatures() error = %v", err)
	}
	if result.HasSignals() {
		t.Error("expected no signals for empty features")
	}
}

func TestScanner_NoResults(t *testing.T) {
	s := NewScanner(&mockSearcher{}, DefaultScannerConfig())
	result, err := s.ScanByFeatures(context.Background(), []string{"nonexistent_feature"})
	if err != nil {
		t.Fatalf("ScanByFeatures() error = %v", err)
	}
	if result.HasSignals() {
		t.Error("expected no signals when no results found")
	}
}

func TestScanner_SingleFeature(t *testing.T) {
	searcher := &mockSearcher{
		results: []CaseResult{
			{DocID: "reexam/001", Title: "无效宣告请求审查决定", DocType: "reexam",
				Metadata: map[string]string{"tags": "功能性限定"}},
			{DocID: "reexam/002", Title: "无效宣告请求审查决定", DocType: "reexam",
				Metadata: map[string]string{"tags": "功能性限定"}},
			{DocID: "reexam/003", Title: "无效宣告请求审查决定", DocType: "reexam",
				Metadata: map[string]string{"tags": "功能性限定"}},
			{DocID: "judgment/001", Title: "侵权判定案例", DocType: "judgment",
				Metadata: map[string]string{"tags": "功能性限定"}},
		},
	}
	cfg := DefaultScannerConfig()
	cfg.MinCaseCount = 2
	s := NewScanner(searcher, cfg)

	result, err := s.ScanByFeatures(context.Background(), []string{"功能性限定"})
	if err != nil {
		t.Fatalf("ScanByFeatures() error = %v", err)
	}
	if !result.HasSignals() {
		t.Fatal("expected signals")
	}
	if result.TotalCases != 4 {
		t.Errorf("TotalCases = %d, want 4", result.TotalCases)
	}

	// Should have at least one signal for "功能性限定".
	found := false
	for _, sig := range result.Signals {
		if sig.Title == "特征「功能性限定」的无效风险" {
			found = true
			if sig.CaseCount < 2 {
				t.Errorf("CaseCount = %d, want >= 2", sig.CaseCount)
			}
			if sig.Severity == "" {
				t.Error("Severity should not be empty")
			}
			break
		}
	}
	if !found {
		t.Errorf("expected signal for '功能性限定', got signals: %v", result.Signals)
	}
}

func TestScanner_FeaturePair(t *testing.T) {
	results := make([]CaseResult, 6)
	for i := 0; i < 6; i++ {
		results[i] = CaseResult{
			DocID:    fmt.Sprintf("reexam/%03d", i+1),
			Title:    "无效宣告请求审查决定",
			DocType:  "reexam",
			Metadata: map[string]string{"tags": "功能性限定; 参数限定"},
		}
	}
	cfg := DefaultScannerConfig()
	cfg.MinCaseCount = 2
	s := NewScanner(&mockSearcher{results: results}, cfg)

	result, err := s.ScanByFeatures(context.Background(), []string{"功能性限定", "参数限定"})
	if err != nil {
		t.Fatalf("ScanByFeatures() error = %v", err)
	}
	if !result.HasSignals() {
		t.Fatal("expected signals")
	}
	// Should have pair signal "功能性限定 + 参数限定".
	found := false
	for _, sig := range result.Signals {
		if sig.Title == "特征组合「功能性限定 + 参数限定」的无效风险" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected pair signal, got: %v", result.Signals)
	}
}

func TestCalculateConfidence(t *testing.T) {
	tests := []struct {
		count int
		want  float64
	}{
		{100, 0.9},
		{30, 0.75},
		{15, 0.6},
		{7, 0.4},
		{2, 0.25},
	}
	for _, tt := range tests {
		got := calculateConfidence(tt.count)
		if got != tt.want {
			t.Errorf("calculateConfidence(%d) = %.2f, want %.2f", tt.count, got, tt.want)
		}
	}
}

func TestRenderMarkdown_NoSignals(t *testing.T) {
	r := &ScanResult{}
	md := r.RenderMarkdown()
	if md != "✅ 未发现显著风险信号。" {
		t.Errorf("unexpected output: %q", md)
	}
}

func TestRenderMarkdown_WithSignals(t *testing.T) {
	r := &ScanResult{
		Signals: []RiskSignal{
			{
				ID:           "risk-1",
				Type:         RiskFeatureCombination,
				Severity:     SeverityHigh,
				Title:        "特征「功能性限定」的无效风险",
				CaseCount:    15,
				InvalidRate:  0.65,
				FeatureTags:  []string{"功能性限定"},
				RelatedCases: []string{"第566088号决定", "第580287号决定"},
				Description:  "包含功能性限定的案件无效率较高",
			},
		},
		TotalCases: 20,
		Confidence: 0.75,
	}
	md := r.RenderMarkdown()
	if !strings.Contains(md, "🔴") {
		t.Error("expected high severity emoji")
	}
	if !strings.Contains(md, "65%") {
		t.Error("expected invalidation rate")
	}
	if !strings.Contains(md, "特征「功能性限定」的无效风险") {
		t.Error("expected signal title in output")
	}
}

func TestStoreCaseSearcher_NilStore(t *testing.T) {
	searcher := NewStoreCaseSearcher(nil)
	results, err := searcher.SearchCases(context.Background(), []string{"test"}, 10)
	if err != nil {
		t.Fatalf("SearchCases() error = %v", err)
	}
	if len(results) > 0 {
		t.Error("expected empty results for nil store")
	}
}
