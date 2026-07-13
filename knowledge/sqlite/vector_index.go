package sqlite

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"unsafe"
)

// VectorIndex holds all embeddings in memory for fast brute-force search.
// It eliminates per-query SQL round-trips and enables parallel computation.
// For 144K BGE-M3 vectors (1024-dim float32), this uses ~562 MB of memory.
//
// Vectors are stored in a single flat []float32 slice (row-major, contiguous)
// to maximize CPU cache utilization during dot-product computation.
type VectorIndex struct {
	vectors  []float32 // flat N×dim, row-major
	chunkIDs []int     // N chunk IDs
	docIDs   []string  // N document IDs
	dim      int
	count    int
}

// vectorMatch is an internal type for parallel search results.
type vectorMatch struct {
	chunkID int
	docID   string
	score   float32
}

// PreloadVectorIndex loads all embeddings from the database into memory.
// This should be called once at startup; subsequent VectorSearch calls
// will use the in-memory index instead of querying SQLite per-batch.
func (s *SQLiteStore) PreloadVectorIndex() (*VectorIndex, error) {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM embeddings").Scan(&count); err != nil {
		return nil, fmt.Errorf("count embeddings: %w", err)
	}
	if count == 0 {
		return nil, fmt.Errorf("no embeddings in database")
	}

	rows, err := s.db.Query(`SELECT chunk_id, document_id, vector FROM embeddings ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("preload vectors query: %w", err)
	}
	defer rows.Close()

	idx := &VectorIndex{
		vectors:  make([]float32, 0, count*s.dim),
		chunkIDs: make([]int, 0, count),
		docIDs:   make([]string, 0, count),
		dim:      s.dim,
		count:    count,
	}

	loaded := 0
	for rows.Next() {
		var chunkID int
		var docID string
		var vecBlob []byte
		if err := rows.Scan(&chunkID, &docID, &vecBlob); err != nil {
			return nil, fmt.Errorf("preload scan: %w", err)
		}
		floatCount := len(vecBlob) / 4
		if floatCount != s.dim || len(vecBlob) == 0 {
			continue
		}
		// Zero-copy BLOB→[]float32 via unsafe, then append (which copies
		// into the pre-allocated flat slice via runtime.memmove).
		vec := unsafe.Slice((*float32)(unsafe.Pointer(&vecBlob[0])), floatCount)
		idx.vectors = append(idx.vectors, vec...)
		idx.chunkIDs = append(idx.chunkIDs, chunkID)
		idx.docIDs = append(idx.docIDs, docID)
		loaded++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("preload rows iteration: %w", err)
	}

	idx.count = loaded
	return idx, nil
}

// Search performs parallel brute-force cosine similarity search.
// Vectors are pre-normalized (norm≈1.0), so dot product ≈ cosine similarity.
// The query vector is assumed to be normalized by the caller.
//
// Results are sorted by descending score.
func (idx *VectorIndex) Search(queryVec []float32, topK int) []vectorMatch {
	if topK <= 0 {
		topK = 10
	}
	if idx.count == 0 || len(queryVec) != idx.dim {
		return nil
	}

	numWorkers := runtime.GOMAXPROCS(0)
	if numWorkers > idx.count {
		numWorkers = idx.count
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	// Each worker computes scores for its shard, then sorts and keeps top-K.
	workerResults := make([][]vectorMatch, numWorkers)

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		start := w * idx.count / numWorkers
		end := (w + 1) * idx.count / numWorkers
		wg.Add(1)
		go func(workerID, s, e int) {
			defer wg.Done()
			n := e - s
			matches := make([]vectorMatch, n)
			for i := 0; i < n; i++ {
				vecStart := (s + i) * idx.dim
				var dot float32
				for d := 0; d < idx.dim; d++ {
					dot += queryVec[d] * idx.vectors[vecStart+d]
				}
				matches[i] = vectorMatch{idx.chunkIDs[s+i], idx.docIDs[s+i], dot}
			}
			// Sort descending by score, keep only top-K from this shard.
			sort.Slice(matches, func(a, b int) bool { return matches[a].score > matches[b].score })
			if len(matches) > topK {
				matches = matches[:topK]
			}
			workerResults[workerID] = matches
		}(w, start, end)
	}
	wg.Wait()

	// Merge per-worker top-K into global top-K.
	total := 0
	for _, wr := range workerResults {
		total += len(wr)
	}
	all := make([]vectorMatch, 0, total)
	for _, wr := range workerResults {
		all = append(all, wr...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })
	if len(all) > topK {
		all = all[:topK]
	}
	return all
}

// Count returns the number of vectors in the index.
func (idx *VectorIndex) Count() int { return idx.count }

// Dim returns the embedding dimension.
func (idx *VectorIndex) Dim() int { return idx.dim }
