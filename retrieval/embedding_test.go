package retrieval

import (
	"context"
	"math"
	"testing"
)

// mockEmbedder implements Embedder for testing without API calls.
type mockEmbedder struct {
	dims   int
	embed  func(text string) []float32
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, t := range texts {
		result[i] = m.embed(t)
	}
	return result, nil
}

func (m *mockEmbedder) Dimensions() int { return m.dims }

// simpleHashEmbed creates a deterministic pseudo-embedding from text.
// Not suitable for production, but useful for tests.
func simpleHashEmbed(text string, dims int) []float32 {
	vec := make([]float32, dims)
	for i := 0; i < len(text) && i < dims; i++ {
		// Map each character to a small float.
		vec[i] = float32(text[i]) / 255.0
	}
	// Normalize to unit length.
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		scale := float32(1.0 / math.Sqrt(norm))
		for i := range vec {
			vec[i] *= scale
		}
	}
	return vec
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	if sim := CosineSimilarity(a, b); sim < 0.99 {
		t.Errorf("identical vectors: sim = %f, want ~1.0", sim)
	}

	c := []float32{0, 1, 0}
	if sim := CosineSimilarity(a, c); sim > 0.01 {
		t.Errorf("orthogonal vectors: sim = %f, want ~0.0", sim)
	}

	d := []float32{-1, 0, 0}
	if sim := CosineSimilarity(a, d); sim > -0.99 {
		t.Errorf("opposite vectors: sim = %f, want ~-1.0", sim)
	}
}

func TestCosineSimilarity_MismatchedLength(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{1, 0, 0}
	if sim := CosineSimilarity(a, b); sim != 0 {
		t.Errorf("mismatched length: sim = %f, want 0", sim)
	}
}

func TestVectorSearcher_Search(t *testing.T) {
	emb := &mockEmbedder{
		dims: 4,
		embed: func(text string) []float32 {
			return simpleHashEmbed(text, 4)
		},
	}

	// Create chunks with pre-computed embeddings.
	chunks := []Chunk{
		{ID: "c1", Content: "专利侵权判定中的全面覆盖原则"},
		{ID: "c2", Content: "合同法关于违约责任的规定"},
		{ID: "c3", Content: "篮球比赛的基本规则介绍"},
	}

	for i := range chunks {
		vec, _ := emb.Embed(context.Background(), []string{chunks[i].Content})
		StoreEmbedding(&chunks[i], vec[0])
	}

	searcher := NewVectorSearcher(emb)

	// Query about patent infringement.
	results := searcher.Search("全面覆盖原则侵权判断", chunks, 3)
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	// Verify all results have scores in valid range.
	for _, r := range results {
		if r.Score < searcher.MinScore {
			t.Errorf("result %s score %f below min %f", r.ID, r.Score, searcher.MinScore)
		}
	}
}

func TestVectorSearcher_NoEmbedding(t *testing.T) {
	emb := &mockEmbedder{dims: 4, embed: func(_ string) []float32 { return []float32{1, 0, 0, 0} }}
	chunks := []Chunk{
		{ID: "c1", Content: "no embedding stored"},
	}
	searcher := NewVectorSearcher(emb)
	results := searcher.Search("test", chunks, 3)
	if len(results) != 0 {
		t.Errorf("chunks without embedding should not match: got %d results", len(results))
	}
}

func TestHybridSearcher(t *testing.T) {
	emb := &mockEmbedder{
		dims: 4,
		embed: func(text string) []float32 {
			return simpleHashEmbed(text, 4)
		},
	}

	keyword := NewKeywordSearcher()
	vector := NewVectorSearcher(emb)
	hybrid := NewHybridSearcher(keyword, vector)
	hybrid.Weight = 0.5

	chunks := []Chunk{
		{ID: "c1", Content: "专利侵权判定的全面覆盖原则要求逐一比对技术特征"},
		{ID: "c2", Content: "关于篮球比赛的规则说明和裁判标准"},
		{ID: "c3", Content: "专利权利要求应当以说明书为依据清楚限定保护范围"},
	}

	// Pre-compute embeddings.
	for i := range chunks {
		vec, _ := emb.Embed(context.Background(), []string{chunks[i].Content})
		StoreEmbedding(&chunks[i], vec[0])
	}

	results := hybrid.Search("专利侵权判定方法", chunks, 2)
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// Top result should be patent-related.
	topID := results[0].ID
	if topID != "c1" && topID != "c3" {
		t.Errorf("top result = %s, expected patent-related chunk", topID)
	}
}

func TestNormalizeScores(t *testing.T) {
	scores := map[string]float64{
		"a": 0.1,
		"b": 0.5,
		"c": 0.9,
	}
	norm := normalizeScores(scores)
	if norm["a"] != 0.0 {
		t.Errorf("min score should be 0.0, got %f", norm["a"])
	}
	if norm["c"] != 1.0 {
		t.Errorf("max score should be 1.0, got %f", norm["c"])
	}
}

func TestNormalizeScores_AllEqual(t *testing.T) {
	scores := map[string]float64{"a": 0.5, "b": 0.5}
	norm := normalizeScores(scores)
	if norm["a"] != 1.0 || norm["b"] != 1.0 {
		t.Errorf("all-equal scores should normalize to 1.0: %v", norm)
	}
}

func TestEncodeDecodeEmbedding(t *testing.T) {
	vec := []float32{0.1, 0.2, 0.3, -0.4, 0.5}
	encoded := encodeEmbedding(vec)
	decoded := decodeEmbedding(map[string]string{"embedding": encoded})

	if len(decoded) != len(vec) {
		t.Fatalf("decoded length = %d, want %d", len(decoded), len(vec))
	}
	for i, v := range vec {
		if math.Abs(float64(decoded[i]-v)) > 0.0001 {
			t.Errorf("decoded[%d] = %f, want %f", i, decoded[i], v)
		}
	}
}

func TestStoreEmbedding(t *testing.T) {
	chunk := &Chunk{ID: "test"}
	vec := []float32{0.5, -0.3}
	StoreEmbedding(chunk, vec)

	if chunk.Metadata == nil {
		t.Fatal("Metadata should be initialized")
	}
	decoded := decodeEmbedding(chunk.Metadata)
	if len(decoded) != 2 {
		t.Errorf("decoded = %v", decoded)
	}
}

func TestDecodeEmbedding_NilMetadata(t *testing.T) {
	if decoded := decodeEmbedding(nil); decoded != nil {
		t.Errorf("nil metadata should return nil, got %v", decoded)
	}
}

func TestDecodeEmbedding_EmptyString(t *testing.T) {
	if decoded := decodeEmbedding(map[string]string{}); decoded != nil {
		t.Errorf("empty metadata should return nil, got %v", decoded)
	}
}

func TestAPIEmbedder_Dimensions(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"text-embedding-3-small", 1536},
		{"text-embedding-3-large", 3072},
		{"text-embedding-ada-002", 1536},
		{"unknown-model", 1536},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			e := NewAPIEmbedder("", "", tt.model)
			if dims := e.Dimensions(); dims != tt.want {
				t.Errorf("Dimensions() = %d, want %d", dims, tt.want)
			}
		})
	}
}
