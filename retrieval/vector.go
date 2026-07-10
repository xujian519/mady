package retrieval

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"math"
	"sort"
)

// VectorSearcher implements Searcher using cosine similarity over
// pre-computed embedding vectors stored in Chunk.Metadata.
//
// Embeddings are stored as base64-encoded little-endian float32 arrays
// under the metadata key "embedding". The VectorSearcher extracts the
// query embedding (typically via an Embedder), computes cosine similarity
// against every chunk's embedding, and returns top-K results.
type VectorSearcher struct {
	// Embedder computes the query embedding at search time.
	Embedder Embedder

	// MinScore filters out results below this similarity threshold.
	// Default: 0.3. Cosine similarity ranges from -1 to 1; typical
	// useful matches are > 0.5.
	MinScore float64
}

// NewVectorSearcher creates a VectorSearcher with sensible defaults.
func NewVectorSearcher(embedder Embedder) *VectorSearcher {
	return &VectorSearcher{
		Embedder: embedder,
		MinScore: 0.3,
	}
}

// Search implements Searcher.Search using vector similarity.
func (vs *VectorSearcher) Search(query string, chunks []Chunk, topK int) []ScoredChunk {
	if vs.Embedder == nil || len(chunks) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = 5
	}

	// Get query embedding.
	vectors, err := vs.Embedder.Embed(context.Background(), []string{query})
	if err != nil || len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil
	}
	queryVec := vectors[0]

	// Compute similarity against each chunk with an embedding.
	var results []ScoredChunk
	for _, chunk := range chunks {
		chunkVec := decodeEmbedding(chunk.Metadata)
		if len(chunkVec) == 0 {
			continue
		}
		sim := CosineSimilarity(queryVec, chunkVec)
		if sim >= vs.MinScore {
			results = append(results, ScoredChunk{
				Chunk:   chunk,
				Score:   sim,
				Matches: []string{},
			})
		}
	}

	// Sort by score descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

// --- HybridSearcher ---

// HybridSearcher combines keyword and vector search with a weighted
// linear combination. Weight = 0.0 means pure keyword; 1.0 means pure
// vector. A weight around 0.7 typically works well for patent/legal text
// where precise terminology matters but semantic similarity helps recall.
type HybridSearcher struct {
	KeywordSearcher Searcher
	VectorSearcher  Searcher

	// Weight controls the balance between keyword and vector scores.
	// 0.0 = all keyword, 1.0 = all vector. Default: 0.7.
	Weight float64
}

// NewHybridSearcher creates a HybridSearcher combining keyword and vector search.
func NewHybridSearcher(keyword, vector Searcher) *HybridSearcher {
	return &HybridSearcher{
		KeywordSearcher: keyword,
		VectorSearcher:  vector,
		Weight:          0.7,
	}
}

// Search implements Searcher.Search by merging keyword and vector results.
// Each chunk's final score is: Weight * vectorScore + (1-Weight) * keywordScore.
func (hs *HybridSearcher) Search(query string, chunks []Chunk, topK int) []ScoredChunk {
	if topK <= 0 {
		topK = 5
	}

	// Run both searches. Keyword search is fast (no API call), vector
	// search may call the embedding API.
	kwResults := hs.KeywordSearcher.Search(query, chunks, len(chunks))
	vecResults := hs.VectorSearcher.Search(query, chunks, len(chunks))

	// Build score maps.
	kwScores := make(map[string]float64)
	for _, r := range kwResults {
		kwScores[r.ID] = r.Score
	}
	vecScores := make(map[string]float64)
	for _, r := range vecResults {
		vecScores[r.ID] = r.Score
	}

	// Merge scores: every chunk that appears in at least one result gets
	// a combined score.
	allIDs := make(map[string]Chunk)
	for _, r := range kwResults {
		allIDs[r.ID] = r.Chunk
	}
	for _, r := range vecResults {
		allIDs[r.ID] = r.Chunk
	}

	// Normalize scores within each result set for fair comparison.
	kwScores = normalizeScores(kwScores)
	vecScores = normalizeScores(vecScores)

	w := hs.Weight
	if w < 0 {
		w = 0
	}
	if w > 1 {
		w = 1
	}

	var merged []ScoredChunk
	for id, chunk := range allIDs {
		score := w*vecScores[id] + (1-w)*kwScores[id]
		if score > 0 {
			merged = append(merged, ScoredChunk{
				Chunk:   chunk,
				Score:   score,
				Matches: []string{},
			})
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	if len(merged) > topK {
		merged = merged[:topK]
	}
	return merged
}

// normalizeScores min-max normalizes scores to [0, 1].
func normalizeScores(scores map[string]float64) map[string]float64 {
	if len(scores) == 0 {
		return scores
	}
	minVal, maxVal := math.MaxFloat64, -math.MaxFloat64
	for _, s := range scores {
		if s < minVal {
			minVal = s
		}
		if s > maxVal {
			maxVal = s
		}
	}
	if maxVal == minVal {
		// All scores equal; set everything to 1.0.
		for k := range scores {
			scores[k] = 1.0
		}
		return scores
	}
	normalized := make(map[string]float64)
	for k, s := range scores {
		normalized[k] = (s - minVal) / (maxVal - minVal)
	}
	return normalized
}

// --- Embedding serialization ---

const embeddingMetaKey = "embedding"

// encodeEmbedding serializes a float32 slice to a base64 string for storage
// in Chunk.Metadata.
func encodeEmbedding(vec []float32) string {
	if len(vec) == 0 {
		return ""
	}
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// DecodeEmbedding deserializes a base64-encoded embedding from Chunk.Metadata.
// Returns nil if no embedding is found or decoding fails.
func DecodeEmbedding(meta map[string]string) []float32 { return decodeEmbedding(meta) }

// decodeEmbedding deserializes a base64-encoded embedding from Chunk.Metadata.
// Returns nil if no embedding is found or decoding fails.
func decodeEmbedding(meta map[string]string) []float32 {
	if meta == nil {
		return nil
	}
	encoded, ok := meta[embeddingMetaKey]
	if !ok || encoded == "" {
		return nil
	}
	buf, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(buf)%4 != 0 {
		return nil
	}
	vec := make([]float32, len(buf)/4)
	for i := range vec {
		bits := binary.LittleEndian.Uint32(buf[i*4:])
		vec[i] = math.Float32frombits(bits)
	}
	return vec
}

// StoreEmbedding encodes an embedding vector and stores it in chunk metadata.
func StoreEmbedding(chunk *Chunk, vec []float32) {
	if chunk.Metadata == nil {
		chunk.Metadata = make(map[string]string)
	}
	chunk.Metadata[embeddingMetaKey] = encodeEmbedding(vec)
}
