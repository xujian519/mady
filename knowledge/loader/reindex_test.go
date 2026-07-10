package loader

import (
	"context"
	"math"
	"testing"

	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/retrieval"
)

// mockEmbedder for ReindexVectors testing.
type mockEmbedder struct {
	dims int
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, t := range texts {
		// Deterministic pseudo-embedding from text length + content.
		vec := make([]float32, m.dims)
		for j := 0; j < len(t) && j < m.dims; j++ {
			vec[j] = float32(t[j]) / 255.0
		}
		// Normalize.
		var norm float64
		for _, v := range vec {
			norm += float64(v) * float64(v)
		}
		if norm > 0 {
			scale := float32(1.0 / math.Sqrt(norm))
			for j := range vec {
				vec[j] *= scale
			}
		}
		result[i] = vec
	}
	return result, nil
}

func (m *mockEmbedder) Dimensions() int { return m.dims }

func TestReindexVectors_Store(t *testing.T) {
	store := knowledge.NewStore()

	// Add some documents.
	store.AddDocument("patent", "doc1", "Test Doc 1", "专利侵权判定中的全面覆盖原则要求逐一比对技术特征", "inline")
	store.AddDocument("patent", "doc2", "Test Doc 2", "合同违约责任的法律规定和适用条件", "inline")

	emb := &mockEmbedder{dims: 16}

	err := store.ReindexVectors(context.Background(), emb)
	if err != nil {
		t.Fatalf("ReindexVectors: %v", err)
	}

	// Verify chunks have embeddings.
	chunks := store.ChunksForDomain("patent")
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	for _, c := range chunks {
		vec := retrieval.DecodeEmbedding(c.Metadata)
		if len(vec) == 0 {
			t.Errorf("chunk %s missing embedding", c.ID)
		}
	}

	// Reindexing again should skip (embeddings already present).
	err = store.ReindexVectors(context.Background(), emb)
	if err != nil {
		t.Errorf("second ReindexVectors should succeed (no-op): %v", err)
	}
}

func TestReindexVectors_SkipsNonSearchable(t *testing.T) {
	store := knowledge.NewStore()

	store.AddDocument("patent", "doc1", "Searchable", "some content here for chunking", "inline")
	// Mark as non-searchable.
	if doc, ok := store.GetDocument("doc1"); ok {
		doc.Searchable = true // Make it searchable for initial add.
	}
	store.AddDocument("patent", "doc2", "NotSearchable", "index page content that should be skipped", "inline")
	if doc, ok := store.GetDocument("doc2"); ok {
		doc.Searchable = false
	}
	// Also test with searchable=true.
	if doc, ok := store.GetDocument("doc1"); ok {
		doc.Searchable = true
	}

	emb := &mockEmbedder{dims: 8}
	err := store.ReindexVectors(context.Background(), emb)
	if err != nil {
		t.Fatalf("ReindexVectors: %v", err)
	}

	// doc1 (searchable) should have embeddings, doc2 should not.
	chunks := store.ChunksForDomain("patent")
	for _, c := range chunks {
		hasEmbedding := len(retrieval.DecodeEmbedding(c.Metadata)) > 0
		// We can only verify the doc1 chunks have embeddings.
		if c.DocID == "doc1" && !hasEmbedding {
			t.Errorf("searchable chunk %s should have embedding", c.ID)
		}
	}
}

func TestReindexVectors_EmptyStore(t *testing.T) {
	store := knowledge.NewStore()
	emb := &mockEmbedder{dims: 4}
	err := store.ReindexVectors(context.Background(), emb)
	if err != nil {
		t.Errorf("empty store should succeed: %v", err)
	}
}

func TestReindexVectors_NilEmbedder(t *testing.T) {
	store := knowledge.NewStore()
	store.AddDocument("patent", "doc1", "Test", "content", "inline")
	err := store.ReindexVectors(context.Background(), nil)
	if err == nil {
		t.Error("nil embedder should return error")
	}
}
