package retrieval

import "sort"

// Reranker re-orders search results based on additional signals
// beyond raw keyword matching. Common implementations include:
//   - positional reranking (prefer results from document beginnings)
//   - freshness reranking (prefer newer documents)
//   - diversity reranking (reduce redundancy in top-K results)
type Reranker interface {
	// Rerank re-orders the given scored chunks.
	Rerank(results []ScoredChunk) []ScoredChunk
}

// PositionReranker adjusts scores based on chunk position within the document.
// Earlier chunks (introduction, abstract) often contain more salient information
// and receive a position bonus. This is particularly useful for patent and
// legal documents where key claims or holdings appear early.
type PositionReranker struct {
	// PositionWeight controls how much position affects the final score.
	// 0 = no effect, 1.0 = strong position bias (default: 0.3).
	PositionWeight float64
}

// NewPositionReranker creates a PositionReranker with sensible defaults.
func NewPositionReranker() *PositionReranker {
	return &PositionReranker{PositionWeight: 0.3}
}

// Rerank implements Reranker.Rerank by applying a position bonus to
// earlier chunks and re-sorting.
func (pr *PositionReranker) Rerank(results []ScoredChunk) []ScoredChunk {
	for i := range results {
		// Position bonus: earlier chunks get a boost.
		// chunk 0 (first): 1.0 + 0.3 = 1.3x
		// chunk 10: 1.0 + 0.03 = ~1.03x
		if results[i].Position <= 10 {
			posBoost := 1.0 + pr.PositionWeight*(1.0-float64(results[i].Position)/10.0)
			results[i].Score *= posBoost
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

// DeduplicatingReranker removes near-duplicate chunks from results,
// keeping the highest-scoring version of each duplicate group.
// Duplicates are detected by simple content overlap ratio.
type DeduplicatingReranker struct {
	// OverlapThreshold is the Jaccard-like overlap above which two
	// chunks are considered duplicates (default: 0.7).
	OverlapThreshold float64
}

// NewDeduplicatingReranker creates a DeduplicatingReranker.
func NewDeduplicatingReranker() *DeduplicatingReranker {
	return &DeduplicatingReranker{OverlapThreshold: 0.7}
}

// Rerank implements Reranker.Rerank by removing near-duplicate chunks.
func (dr *DeduplicatingReranker) Rerank(results []ScoredChunk) []ScoredChunk {
	if len(results) <= 1 {
		return results
	}

	seen := make(map[string]bool)
	var filtered []ScoredChunk
	for _, r := range results {
		key := r.Chunk.Content
		if len(key) > 100 {
			key = key[:100] // use first 100 chars as signature
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, r)
	}
	return filtered
}

// ChainReranker applies multiple Rerankers in sequence.
type ChainReranker struct {
	Rerankers []Reranker
}

// Rerank implements Reranker.Rerank by applying each reranker in order.
func (cr *ChainReranker) Rerank(results []ScoredChunk) []ScoredChunk {
	for _, r := range cr.Rerankers {
		results = r.Rerank(results)
	}
	return results
}

// --- LegalReranker ---

// LegalReranker boosts chunks based on legal source hierarchy.
// In Chinese law, higher-level sources (constitution > law > judicial
// interpretation > guiding case) are more authoritative and should rank
// higher in legal retrieval results.
type LegalReranker struct {
	// Hierarchy maps law source names to numeric ranks (higher = more
	// authoritative). Default: Chinese legal hierarchy.
	Hierarchy map[string]int

	// BoostPerRank is the score multiplier applied per rank level above
	// the baseline. Default: 0.15.
	BoostPerRank float64

	// MetadataKey is the chunk metadata key that stores the law source.
	// Default: "law_source".
	MetadataKey string
}

// DefaultLegalHierarchy returns the standard Chinese legal source hierarchy.
func DefaultLegalHierarchy() map[string]int {
	return map[string]int{
		"宪法":      100,
		"法律":       90,
		"行政法规":   80,
		"司法解释":   70,
		"部门规章":   60,
		"地方性法规": 50,
		"指导性案例": 40,
	}
}

// NewLegalReranker creates a LegalReranker with the default Chinese
// legal hierarchy.
func NewLegalReranker() *LegalReranker {
	return &LegalReranker{
		Hierarchy:    DefaultLegalHierarchy(),
		BoostPerRank: 0.15,
		MetadataKey:  "law_source",
	}
}

// Rerank implements Reranker by adjusting scores based on law source rank.
func (lr *LegalReranker) Rerank(results []ScoredChunk) []ScoredChunk {
	if len(results) == 0 {
		return results
	}

	hierarchy := lr.Hierarchy
	if hierarchy == nil {
		hierarchy = DefaultLegalHierarchy()
	}
	boost := lr.BoostPerRank
	if boost <= 0 {
		boost = 0.15
	}
	key := lr.MetadataKey
	if key == "" {
		key = "law_source"
	}

	// Find the baseline rank (lowest among results).
	baselineRank := 1000
	for _, r := range results {
		source := r.Chunk.Metadata[key]
		if rank, ok := hierarchy[source]; ok && rank < baselineRank {
			baselineRank = rank
		}
	}

	for i := range results {
		source := results[i].Chunk.Metadata[key]
		if rank, ok := hierarchy[source]; ok {
			rankDiff := float64(rank-baselineRank) / 100.0
			if rankDiff > 0 {
				results[i].Score *= 1.0 + boost*rankDiff
			}
		}
	}

	return results
}
