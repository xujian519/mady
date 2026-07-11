package retrieval

import "sort"

// RRFFuser implements Reciprocal Rank Fusion to merge multiple ranked result
// lists. RRF is score-agnostic — it only considers rank position — making it
// robust for fusing results from heterogeneous scoring systems (e.g., FTS BM25
// and vector cosine similarity).
//
// Formula: score(d) = Σ_i 1 / (k + rank_i(d))
// where k is a smoothing constant (typically 60) and rank_i(d) is the
// 1-based rank of document d in list i.
type RRFFuser struct {
	// K is the smoothing constant. Default: 60 (from the original paper).
	K float64
}

// NewRRFFuser creates an RRFFuser with k=60.
func NewRRFFuser() *RRFFuser {
	return &RRFFuser{K: 60}
}

// Fuse merges multiple ranked lists using RRF. Each input list should be
// sorted by relevance descending. Returns topK results sorted by RRF score.
func (f *RRFFuser) Fuse(lists [][]ScoredChunk, topK int) []ScoredChunk {
	if len(lists) == 0 || topK <= 0 {
		return nil
	}
	k := f.K
	if k <= 0 {
		k = 60
	}

	type entry struct {
		chunk    Chunk
		rrfScore float64
		matches  []string
	}

	merged := make(map[string]*entry)

	for _, list := range lists {
		for rank, r := range list {
			id := r.ID
			contribution := 1.0 / (k + float64(rank+1))
			if e, ok := merged[id]; ok {
				e.rrfScore += contribution
			} else {
				matches := r.Matches
				if matches == nil {
					matches = []string{}
				}
				merged[id] = &entry{
					chunk:    r.Chunk,
					rrfScore: contribution,
					matches:  matches,
				}
			}
		}
	}

	results := make([]ScoredChunk, 0, len(merged))
	for _, e := range merged {
		results = append(results, ScoredChunk{
			Chunk:   e.chunk,
			Score:   e.rrfScore,
			Matches: e.matches,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}
	return results
}
