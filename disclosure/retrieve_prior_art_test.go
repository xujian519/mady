package disclosure

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/retrieval/domain"
)

// fakeRetriever implements domain.DomainRetriever for testing without knowledge.db.
type fakeRetriever struct {
	docs []domain.DomainDocument
	err  error
	last string
}

func (f *fakeRetriever) Search(_ context.Context, q domain.DomainQuery) (*domain.DomainResults, error) {
	f.last = q.Text
	if f.err != nil {
		return nil, f.err
	}
	return &domain.DomainResults{Documents: f.docs, Source: "fake"}, nil
}
func (f *fakeRetriever) GetDocument(_ context.Context, _ string) (*domain.DomainDocument, error) {
	return nil, nil
}
func (f *fakeRetriever) SourceName() string { return "fake" }

func TestRetrievePriorArt_NilRetriever(t *testing.T) {
	node := retrievePriorArtNode(nil)
	state, err := node(context.Background(), graph.PregelState{
		StateKeySearchKeywords: []string{"深度学习"},
	})
	if err != nil {
		t.Fatalf("nil retriever: %v", err)
	}
	if state[StateKeyEvidenceCoverage] != "none" {
		t.Errorf("coverage = %v, want 'none'", state[StateKeyEvidenceCoverage])
	}
}

func TestRetrievePriorArt_WithEvidence(t *testing.T) {
	r := &fakeRetriever{docs: []domain.DomainDocument{
		{ID: "CN001", Title: "现有技术A", Snippet: "一种图像识别方法…", Score: 0.9},
		{ID: "CN002", Title: "现有技术B", Snippet: "卷积神经网络架构…", Score: 0.75},
	}}
	node := retrievePriorArtNode(r)

	state, err := node(context.Background(), graph.PregelState{
		StateKeySearchKeywords: []string{"图像识别", "深度学习"},
	})
	if err != nil {
		t.Fatalf("RetrievePriorArt: %v", err)
	}
	if state[StateKeyEvidenceCoverage] != "partial" {
		t.Errorf("coverage = %v, want 'partial' (2 docs)", state[StateKeyEvidenceCoverage])
	}
	chunks, ok := state[StateKeyEvidence].([]EvidenceChunk)
	if !ok || len(chunks) != 2 {
		t.Fatalf("got %v evidence chunks, want 2", state[StateKeyEvidence])
	}
	if chunks[0].DocID != "CN001" || chunks[0].Score != 0.9 {
		t.Errorf("first chunk = %+v", chunks[0])
	}
}

func TestRetrievePriorArt_SearchError_NoCrash(t *testing.T) {
	r := &fakeRetriever{err: errors.New("fake search error")}
	node := retrievePriorArtNode(r)
	state, err := node(context.Background(), graph.PregelState{
		StateKeySearchKeywords: []string{"x"},
	})
	if err != nil {
		t.Fatalf("search error should not propagate: %v", err)
	}
	if state[StateKeyEvidenceCoverage] != "none" {
		t.Errorf("on error coverage = %v, want 'none'", state[StateKeyEvidenceCoverage])
	}
}

func TestRetrievePriorArt_EmptyQuery(t *testing.T) {
	r := &fakeRetriever{}
	node := retrievePriorArtNode(r)
	state, _ := node(context.Background(), graph.PregelState{})
	if state[StateKeyEvidenceCoverage] != "none" {
		t.Errorf("empty query coverage = %v, want 'none'", state[StateKeyEvidenceCoverage])
	}
}

func TestRetrievePriorArt_FullCoverage(t *testing.T) {
	// 5+ docs → "full" coverage.
	docs := make([]domain.DomainDocument, 5)
	for i := range docs {
		docs[i] = domain.DomainDocument{ID: "doc", Snippet: "snippet"}
	}
	r := &fakeRetriever{docs: docs}
	node := retrievePriorArtNode(r)
	state, _ := node(context.Background(), graph.PregelState{
		StateKeySearchKeywords: []string{"x"},
	})
	if state[StateKeyEvidenceCoverage] != "full" {
		t.Errorf("5 docs coverage = %v, want 'full'", state[StateKeyEvidenceCoverage])
	}
}

func TestBuildNoveltyInput_IncludesEvidence(t *testing.T) {
	state := graph.PregelState{
		StateKeyExtraction: &ExtractionResult{
			Features: []TechFeature{{ID: "F1", Description: "特征A"}},
		},
		StateKeyEvidence: []EvidenceChunk{
			{DocID: "CN001", Title: "现有技术", Snippet: "原文片段", Score: 0.8},
		},
		StateKeyEvidenceCoverage: "partial",
	}
	input := buildNoveltyInput(state)
	if input == "" {
		t.Fatal("expected non-empty novelty input")
	}
	if !strings.Contains(input, "CN001") {
		t.Error("novelty input should include evidence doc_id CN001")
	}
	if !strings.Contains(input, "cited_evidence_ids") {
		t.Error("novelty input should instruct LLM to fill cited_evidence_ids")
	}
}

func TestBuildNoveltyInput_EvidenceCoverageNone(t *testing.T) {
	state := graph.PregelState{
		StateKeyExtraction: &ExtractionResult{
			Features: []TechFeature{{ID: "F1", Description: "特征A"}},
		},
		StateKeyEvidenceCoverage: "none",
	}
	input := buildNoveltyInput(state)
	if !strings.Contains(input, "无可用现有技术证据") {
		t.Error("coverage=none should produce a no-evidence warning in prompt")
	}
}

func TestCoverageLevel(t *testing.T) {
	if got := coverageLevel(0); got != "none" {
		t.Errorf("coverageLevel(0) = %s, want none", got)
	}
	if got := coverageLevel(3); got != "partial" {
		t.Errorf("coverageLevel(3) = %s, want partial", got)
	}
	if got := coverageLevel(5); got != "full" {
		t.Errorf("coverageLevel(5) = %s, want full", got)
	}
}
