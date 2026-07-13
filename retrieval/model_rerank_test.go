package retrieval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// rerankResult pairs an original index with a relevance score.
type rerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// rerankJSON builds a Cohere-compatible /v1/rerank response.
func rerankJSON(results ...rerankResult) []byte {
	resp := struct {
		Results []rerankResult `json:"results"`
		Model   string         `json:"model"`
		ID      string         `json:"id"`
	}{
		Results: results,
		Model:   "test-reranker",
		ID:      "test-id",
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestModelReranker_RerankNoOp(t *testing.T) {
	r := NewModelReranker("http://localhost:0", "key", "model")
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "1", Content: "foo"}, Score: 0.9},
		{Chunk: Chunk{ID: "2", Content: "bar"}, Score: 0.5},
	}
	got := r.Rerank(results)
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "2" {
		t.Fatalf("Rerank without query should be a no-op, got %v", got)
	}
}

func TestModelReranker_EmptyResults(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	mr := NewModelReranker(srv.URL, "key", "model")
	got := mr.RerankWithQuery(context.Background(), "query", nil)
	if len(got) != 0 {
		t.Fatalf("expected empty results, got %v", got)
	}
	if called {
		t.Fatal("API should not be called for empty results")
	}
}

func TestModelReranker_EmptyQuery(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	mr := NewModelReranker(srv.URL, "key", "model")
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "1", Content: "foo"}, Score: 0.9},
	}
	got := mr.RerankWithQuery(context.Background(), "", results)
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("expected original results, got %v", got)
	}
	if called {
		t.Fatal("API should not be called for empty query")
	}
}

func TestModelReranker_RerankWithQuery_Reorders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rerankRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode error: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Query != "专利新颖性" {
			t.Errorf("unexpected query: %s", req.Query)
		}
		if len(req.Documents) != 3 {
			t.Errorf("expected 3 docs, got %d", len(req.Documents))
		}
		if req.Documents[0] != "doc-zero" || req.Documents[1] != "doc-one" || req.Documents[2] != "doc-two" {
			t.Errorf("unexpected documents: %v", req.Documents)
		}

		// Reverse the order: index 2 gets highest score, 0 gets lowest.
		w.Write(rerankJSON(
			rerankResult{Index: 2, RelevanceScore: 0.95},
			rerankResult{Index: 1, RelevanceScore: 0.72},
			rerankResult{Index: 0, RelevanceScore: 0.31},
		))
	}))
	defer srv.Close()

	mr := NewModelReranker(srv.URL, "key", "test-reranker")
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "a", Content: "doc-zero"}, Score: 0.9},
		{Chunk: Chunk{ID: "b", Content: "doc-one"}, Score: 0.7},
		{Chunk: Chunk{ID: "c", Content: "doc-two"}, Score: 0.5},
	}

	got := mr.RerankWithQuery(context.Background(), "专利新颖性", results)
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}
	if got[0].ID != "c" || got[0].Score != 0.95 {
		t.Errorf("expected first=c(0.95), got %s(%.2f)", got[0].ID, got[0].Score)
	}
	if got[1].ID != "b" || got[1].Score != 0.72 {
		t.Errorf("expected second=b(0.72), got %s(%.2f)", got[1].ID, got[1].Score)
	}
	if got[2].ID != "a" || got[2].Score != 0.31 {
		t.Errorf("expected third=a(0.31), got %s(%.2f)", got[2].ID, got[2].Score)
	}
}

func TestModelReranker_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	mr := NewModelReranker(srv.URL, "key", "model")
	results := []ScoredChunk{
		{Chunk: Chunk{ID: "a", Content: "foo"}, Score: 0.9},
		{Chunk: Chunk{ID: "b", Content: "bar"}, Score: 0.5},
	}

	got := mr.RerankWithQuery(context.Background(), "query", results)
	if len(got) != 2 {
		t.Fatalf("expected 2 results on API error, got %d", len(got))
	}
	// Original order preserved.
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("expected original order on API error, got %v", got)
	}
}

func TestModelReranker_MaxDocuments(t *testing.T) {
	var receivedDocs int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rerankRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedDocs = len(req.Documents)

		// Return first 2 with high scores.
		w.Write(rerankJSON(
			rerankResult{Index: 0, RelevanceScore: 0.99},
			rerankResult{Index: 1, RelevanceScore: 0.88},
		))
	}))
	defer srv.Close()

	mr := NewModelReranker(srv.URL, "key", "model")
	mr.MaxDocuments = 3

	// 5 results, only first 3 sent to reranker, last 2 appended as-is.
	results := make([]ScoredChunk, 5)
	for i := range results {
		results[i] = ScoredChunk{
			Chunk: Chunk{ID: string(rune('a' + i)), Content: "content"},
			Score: float64(5-i) / 10.0,
		}
	}

	got := mr.RerankWithQuery(context.Background(), "query", results)
	if receivedDocs != 3 {
		t.Fatalf("expected 3 docs sent to API, got %d", receivedDocs)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 total results, got %d", len(got))
	}
	// First 2 are reranked (high scores), last 3 are tail (original order).
	if got[0].ID != "a" || got[0].Score != 0.99 {
		t.Errorf("expected first=a(0.99), got %s(%.2f)", got[0].ID, got[0].Score)
	}
	if got[1].ID != "b" || got[1].Score != 0.88 {
		t.Errorf("expected second=b(0.88), got %s(%.2f)", got[1].ID, got[1].Score)
	}
	// Tail: c, d, e in original order.
	if got[2].ID != "c" || got[3].ID != "d" || got[4].ID != "e" {
		t.Errorf("expected tail [c,d,e], got %s,%s,%s", got[2].ID, got[3].ID, got[4].ID)
	}
}

func TestModelReranker_TopN(t *testing.T) {
	var receivedTopN int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rerankRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedTopN = req.TopN

		// Return only 2 results (top_n=2).
		w.Write(rerankJSON(
			rerankResult{Index: 1, RelevanceScore: 0.95},
			rerankResult{Index: 0, RelevanceScore: 0.80},
		))
	}))
	defer srv.Close()

	mr := NewModelReranker(srv.URL, "key", "model")
	mr.TopN = 2

	results := []ScoredChunk{
		{Chunk: Chunk{ID: "a", Content: "foo"}, Score: 0.9},
		{Chunk: Chunk{ID: "b", Content: "bar"}, Score: 0.7},
	}

	got := mr.RerankWithQuery(context.Background(), "query", results)
	if receivedTopN != 2 {
		t.Fatalf("expected top_n=2 in request, got %d", receivedTopN)
	}
	// API returned 2 results, but no tail (len(results)=2 <= MaxDocuments=20).
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].ID != "b" || got[0].Score != 0.95 {
		t.Errorf("expected first=b(0.95), got %s(%.2f)", got[0].ID, got[0].Score)
	}
}

func TestModelReranker_ImplementsInterfaces(t *testing.T) {
	mr := NewModelReranker("http://localhost", "key", "model")

	var _ Reranker = mr
	var _ QueryReranker = mr

	if _, ok := interface{}(mr).(QueryReranker); !ok {
		t.Fatal("ModelReranker must implement QueryReranker")
	}
}
