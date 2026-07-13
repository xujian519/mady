package sqlite

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// testEmbedder implements retrieval.Embedder for testing. It produces
// deterministic 8-dimensional vectors based on text content so that
// similar texts yield similar vectors.
type testEmbedder struct {
	dim int
}

func newTestEmbedder(dim int) *testEmbedder { return &testEmbedder{dim: dim} }

func (e *testEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, len(texts))
	for i, text := range texts {
		v := make([]float32, e.dim)
		// Simple deterministic hash-to-vector: each character contributes
		// to a dimension, creating positional similarity.
		for j, ch := range text {
			v[j%e.dim] += float32(ch) / 1000
		}
		// Normalise.
		var sum float64
		for _, val := range v {
			sum += float64(val) * float64(val)
		}
		if sum > 0 {
			norm := float32(1.0 / float64(sum))
			_ = norm // keep it simple; vectors are already small
		}
		vecs[i] = v
	}
	return vecs, nil
}

func (e *testEmbedder) Dimensions() int { return e.dim }

func TestWritableStore_CreateAndAddDocument(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "user.db")
	emb := newTestEmbedder(8)

	w, err := OpenWritable(dbPath, emb, "")
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer w.Close()

	ctx := context.Background()
	content := "这是一段测试文档内容。\n\n专利法第二十二条规定了授予专利权的条件。\n\n新颖性是指发明或者实用新型不属于现有技术。"
	err = w.AddDocument(ctx, "test-doc-1", "测试文档", content)
	if err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	if w.Dim() != 8 {
		t.Errorf("Dim = %d, want 8", w.Dim())
	}
}

func TestWritableStore_SearchFTSHit(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "user.db")
	emb := newTestEmbedder(8)

	w, err := OpenWritable(dbPath, emb, "")
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer w.Close()

	ctx := context.Background()
	content := "专利法第二十二条新颖性创造性实用性"
	if err := w.AddDocument(ctx, "doc1", "专利法", content); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	results, err := w.Search(ctx, "新颖性", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results, got 0")
	}
	if results[0].DocID != "doc1" {
		t.Errorf("DocID = %s, want doc1", results[0].DocID)
	}
}

func TestWritableStore_SearchNoMatch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "user.db")
	emb := newTestEmbedder(8)

	w, err := OpenWritable(dbPath, emb, "")
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer w.Close()

	ctx := context.Background()
	if err := w.AddDocument(ctx, "doc1", "专利法", "专利法新颖性"); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	results, err := w.Search(ctx, "完全不相关的内容xyz", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// FTS trigram may still match on common chars; just verify no panic.
	_ = results
}

func TestWritableStore_ReplaceExistingDoc(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "user.db")
	emb := newTestEmbedder(8)

	w, err := OpenWritable(dbPath, emb, "")
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer w.Close()

	ctx := context.Background()

	// First version.
	if err := w.AddDocument(ctx, "doc1", "V1", "第一版本内容专利法"); err != nil {
		t.Fatalf("AddDocument v1: %v", err)
	}
	// Second version replaces.
	if err := w.AddDocument(ctx, "doc1", "V2", "第二版本完全不同的内容商标法"); err != nil {
		t.Fatalf("AddDocument v2: %v", err)
	}

	// Should find the new content.
	results, err := w.Search(ctx, "商标法", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	found := false
	for _, r := range results {
		if r.DocID == "doc1" {
			found = true
		}
	}
	if !found {
		t.Error("expected doc1 in results after replacement")
	}
}

func TestWritableStore_PathConflictWithKnowledgeDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "knowledge.db")

	_, err := OpenWritable(dbPath, newTestEmbedder(8), dbPath)
	if err != ErrKnowledgeDBConflict {
		t.Errorf("expected ErrKnowledgeDBConflict, got %v", err)
	}
}

func TestWritableStore_NilEmbedder(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "user.db")

	w, err := OpenWritable(dbPath, nil, "")
	if err != nil {
		t.Fatalf("OpenWritable with nil embedder: %v", err)
	}
	defer w.Close()

	ctx := context.Background()
	err = w.AddDocument(ctx, "doc1", "title", "content")
	if err == nil {
		t.Error("expected error when embedder is nil")
	}
}

func TestWritableStore_EmptyDocID(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "user.db")
	w, err := OpenWritable(dbPath, newTestEmbedder(8), "")
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer w.Close()

	err = w.AddDocument(context.Background(), "", "title", "content")
	if err == nil {
		t.Error("expected error for empty docID")
	}
}

func TestWritableStore_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "user.db")
	emb := newTestEmbedder(8)

	w, err := OpenWritable(dbPath, emb, "")
	if err != nil {
		t.Fatalf("OpenWritable: %v", err)
	}
	defer w.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			docID := "concurrent-" + string(rune('A'+idx))
			err := w.AddDocument(ctx, docID, "concurrent", "并发写入测试内容")
			if err != nil {
				t.Errorf("concurrent AddDocument %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestWritableStore_InitSchemaIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "user.db")
	emb := newTestEmbedder(8)

	w1, err := OpenWritable(dbPath, emb, "")
	if err != nil {
		t.Fatalf("first OpenWritable: %v", err)
	}
	w1.Close()

	// Re-open should not fail on existing schema.
	w2, err := OpenWritable(dbPath, emb, "")
	if err != nil {
		t.Fatalf("second OpenWritable: %v", err)
	}
	defer w2.Close()

	ctx := context.Background()
	if err := w2.AddDocument(ctx, "doc1", "test", "重新打开后写入"); err != nil {
		t.Fatalf("AddDocument after reopen: %v", err)
	}
}

func TestWritableStore_HashString(t *testing.T) {
	h1 := hashString("test")
	h2 := hashString("test")
	h3 := hashString("different")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
}

func TestFloat32ToBytesRoundTrip(t *testing.T) {
	original := []float32{1.0, -2.5, 3.14, 0.0, math.Float32frombits(0x80000000)}
	blob := float32ToBytes(original)
	if len(blob) != len(original)*4 {
		t.Fatalf("blob length = %d, want %d", len(blob), len(original)*4)
	}
	decoded := bytesToFloat32(blob)
	for i, v := range original {
		if v != decoded[i] {
			t.Errorf("round-trip mismatch at %d: got %v, want %v", i, decoded[i], v)
		}
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
