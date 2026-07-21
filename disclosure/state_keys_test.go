package disclosure

import (
	"testing"

	"github.com/xujian519/mady/graph"
)

func TestStateKeyAccessors(t *testing.T) {
	state := graph.PregelState{}

	t.Run("Doc", func(t *testing.T) {
		doc := &DisclosureDoc{Title: "测试交底书", Format: "markdown"}
		SetDoc(state, doc)
		got, ok := GetDoc(state)
		if !ok {
			t.Fatal("expected to find Doc")
		}
		if got.Title != "测试交底书" {
			t.Errorf("Title = %q, want %q", got.Title, "测试交底书")
		}
	})

	t.Run("Extraction", func(t *testing.T) {
		ext := &ExtractionResult{
			Problems: []string{"测试问题"},
			Features: []TechFeature{
				{Description: "测试特征", Category: CatStructure},
			},
		}
		SetExtraction(state, ext)
		got, ok := GetExtraction(state)
		if !ok {
			t.Fatal("expected to find Extraction")
		}
		if len(got.Problems) != 1 {
			t.Errorf("got %d problems, want 1", len(got.Problems))
		}
	})

	t.Run("SearchKeywords", func(t *testing.T) {
		kw := []string{"关键词1", "关键词2"}
		SetSearchKeywords(state, kw)
		got, ok := GetSearchKeywords(state)
		if !ok {
			t.Fatal("expected to find SearchKeywords")
		}
		if len(got) != 2 || got[0] != "关键词1" {
			t.Errorf("got %v, want %v", got, kw)
		}
	})

	t.Run("Novelty", func(t *testing.T) {
		nr := &NoveltyResult{Assessed: true, Conclusion: "有新颖性"}
		SetNovelty(state, nr)
		got, ok := GetNovelty(state)
		if !ok {
			t.Fatal("expected to find Novelty")
		}
		if !got.Assessed {
			t.Error("expected Assessed to be true")
		}
	})

	t.Run("Report", func(t *testing.T) {
		rpt := &AnalysisReport{ID: "rpt_test"}
		SetReport(state, rpt)
		got, ok := GetReport(state)
		if !ok {
			t.Fatal("expected to find Report")
		}
		if got.ID != "rpt_test" {
			t.Errorf("ID = %q, want %q", got.ID, "rpt_test")
		}
	})

	t.Run("Output", func(t *testing.T) {
		SetOutput(state, "测试输出")
		got := GetOutput(state)
		if got != "测试输出" {
			t.Errorf("Output = %q, want %q", got, "测试输出")
		}
	})

	t.Run("Evidence", func(t *testing.T) {
		chunks := []EvidenceChunk{
			{DocID: "doc1", Snippet: "测试证据"},
		}
		SetEvidence(state, chunks)
		got, ok := GetEvidence(state)
		if !ok {
			t.Fatal("expected to find Evidence")
		}
		if len(got) != 1 || got[0].DocID != "doc1" {
			t.Errorf("got %+v, want 1 chunk with doc1", got)
		}
	})

	t.Run("EvidenceCoverage", func(t *testing.T) {
		SetEvidenceCoverage(state, "full")
		got := GetEvidenceCoverage(state)
		if got != "full" {
			t.Errorf("Coverage = %q, want %q", got, "full")
		}
	})

	t.Run("DraftClaims", func(t *testing.T) {
		SetDraftClaims(state, "权利要求1：一种装置...")
		got, ok := GetDraftClaims(state)
		if !ok {
			t.Fatal("expected to find DraftClaims")
		}
		if got != "权利要求1：一种装置..." {
			t.Errorf("DraftClaims = %q", got)
		}
	})

	t.Run("RetryCount", func(t *testing.T) {
		SetRetryCount(state, 3)
		got := GetRetryCount(state)
		if got != 3 {
			t.Errorf("RetryCount = %d, want %d", got, 3)
		}
	})

	t.Run("RetryFeedback", func(t *testing.T) {
		SetRetryFeedback(state, "请补充特征描述")
		got := GetRetryFeedback(state)
		if got != "请补充特征描述" {
			t.Errorf("RetryFeedback = %q", got)
		}
	})
}

func TestStateKeyAccessors_MissingKey(t *testing.T) {
	state := graph.PregelState{}

	t.Run("EmptyState", func(t *testing.T) {
		if _, ok := GetExtraction(state); ok {
			t.Error("expected ok=false for missing key")
		}
		if _, ok := GetSearchKeywords(state); ok {
			t.Error("expected ok=false for missing key")
		}
		if _, ok := GetReport(state); ok {
			t.Error("expected ok=false for missing key")
		}
	})

	t.Run("WrongType", func(t *testing.T) {
		state["extraction_result"] = "not an ExtractionResult"
		if _, ok := GetExtraction(state); ok {
			t.Error("expected ok=false for wrong type")
		}
	})

	t.Run("DefaultValues", func(t *testing.T) {
		if got := GetOutput(state); got != "" {
			t.Errorf("empty state Output = %q, want ''", got)
		}
		if got := GetRetryCount(state); got != 0 {
			t.Errorf("empty state RetryCount = %d, want 0", got)
		}
		if got := GetEvidenceCoverage(state); got != "" {
			t.Errorf("empty state EvidenceCoverage = %q, want ''", got)
		}
	})
}
