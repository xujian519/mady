package sqlite

import (
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/xujian519/mady/pkg/vecbytes"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver

	"github.com/xujian519/mady/retrieval"
)

// getChunksBatch retrieves multiple chunks by their integer IDs in a single
// SQL query. Returns a map from chunk ID to chunk, skipping any IDs that
// don't exist (no error for missing chunks). If the batch query fails, it
// falls back to fetching chunks individually — this preserves the
// fault-tolerance of the old per-chunk code path.
func (s *SQLiteStore) getChunksBatch(ids []int) map[int]*retrieval.Chunk {
	if len(ids) == 0 {
		return nil
	}

	// Try batch query first.
	chunkMap, err := s.getChunksBatchImpl(ids)
	if err == nil {
		return chunkMap
	}

	// Fallback: batch failed (e.g. corrupt row), fetch individually.
	// Silently skip individual errors to preserve the old behavior.
	result := make(map[int]*retrieval.Chunk, len(ids))
	for _, id := range ids {
		chunk, err := s.getChunk(id)
		if err != nil || chunk == nil {
			continue
		}
		result[id] = chunk
	}
	return result
}

// VectorSearch performs brute-force cosine-similarity search against stored
// BGE-M3 embeddings. If the in-memory vector index is loaded (via
// PreloadVectors), it uses parallel in-memory computation (~50-200ms for
// 144K vectors). Otherwise it falls back to parallel SQL batch scans (uses
// goroutines for ~1-2s fallback instead of ~14s sequential).
func (s *SQLiteStore) VectorSearch(queryVec []float32, topK int) ([]retrieval.ScoredChunk, error) {
	if topK <= 0 {
		topK = 10
	}
	if len(queryVec) != s.dim {
		return nil, fmt.Errorf("vector dimension mismatch: got %d, want %d", len(queryVec), s.dim)
	}

	// Fast path: in-memory parallel search.
	if s.vecIndex != nil {
		return s.vectorSearchInMemory(queryVec, topK)
	}

	// Slow path: parallel SQL batch scans (fallback).
	return s.vectorSearchSQLParallel(queryVec, topK)
}

// HasVectorIndex returns true if the in-memory vector index is loaded.
func (s *SQLiteStore) HasVectorIndex() bool {
	return s.vecIndex != nil
}

// PreloadVectors loads all embeddings into memory for fast brute-force search.
// This should be called once at startup. After preloading, VectorSearch
// uses the in-memory index instead of per-query SQL batch reads.
// For 144K BGE-M3 vectors (1024-dim) this uses ~562 MB of memory.
func (s *SQLiteStore) PreloadVectors() error {
	idx, err := s.PreloadVectorIndex()
	if err != nil {
		return err
	}
	s.vecIndex = idx
	return nil
}

// vectorSearchInMemory uses the pre-loaded in-memory vector index for
// parallel brute-force search, then fetches chunk content for top results.
func (s *SQLiteStore) vectorSearchInMemory(queryVec []float32, topK int) ([]retrieval.ScoredChunk, error) {
	matches := s.vecIndex.Search(queryVec, topK)
	if len(matches) == 0 {
		return nil, nil
	}

	// Fetch chunk content for the top results with a single batch query,
	// falling back to per-chunk fetch if the batch query fails.
	chunkIDs := make([]int, len(matches))
	for i, m := range matches {
		chunkIDs[i] = m.chunkID
	}
	chunkMap := s.getChunksBatch(chunkIDs)
	results := make([]retrieval.ScoredChunk, 0, len(matches))
	for _, m := range matches {
		chunk, ok := chunkMap[m.chunkID]
		if !ok || chunk == nil {
			continue
		}
		results = append(results, retrieval.ScoredChunk{
			Chunk:   *chunk,
			Score:   float64(m.score),
			Matches: []string{},
		})
	}
	return results, nil
}

// vectorSearchSQLParallel is the fallback path when the in-memory vector
// index is not available. It uses parallel goroutines to scan the embeddings
// table in ranges, each maintaining a min-heap of top-K candidates.
// This reduces query time from ~14s (sequential) to ~1-2s on M4 Pro.
func (s *SQLiteStore) vectorSearchSQLParallel(queryVec []float32, topK int) ([]retrieval.ScoredChunk, error) {
	// Determine max ID for range partitioning — using MAX(id) instead of
	// COUNT(*) so that gaps from deletions don't cause tail rows to be missed.
	var maxID int
	if err := s.db.QueryRowContext(s.baseCtx, "SELECT COALESCE(MAX(id), 0) FROM embeddings").Scan(&maxID); err != nil {
		return nil, fmt.Errorf("vector sql max id: %w", err)
	}
	if maxID == 0 {
		return nil, nil
	}
	if topK <= 0 {
		topK = 10
	}

	qNorm := float64(0)
	for _, v := range queryVec {
		qNorm += float64(v) * float64(v)
	}
	qNorm = math.Sqrt(qNorm)
	if qNorm == 0 {
		return nil, fmt.Errorf("query vector is zero")
	}

	numWorkers := runtime.GOMAXPROCS(0)
	if numWorkers < 1 {
		numWorkers = 1
	}

	type candidate struct {
		chunkID int
		score   float64
	}

	type workerResult struct {
		top []candidate
		err error
	}

	results := make(chan workerResult, numWorkers)
	var wg sync.WaitGroup

	batchSize := maxID / numWorkers
	if batchSize < 1000 {
		batchSize = 1000
	}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startID := w * batchSize
		endID := (w + 1) * batchSize
		if w == numWorkers-1 {
			endID = maxID + 1 // +1 because the query uses id < endID
		}

		go func(sID, eID int) {
			defer wg.Done()
			localTop := make([]candidate, 0, topK)

			rows, err := s.db.QueryContext(s.baseCtx, `
				SELECT e.chunk_id, e.vector, e.norm
				FROM embeddings e
				WHERE e.id >= ? AND e.id < ?
				ORDER BY e.id`, sID, eID)
			if err != nil {
				results <- workerResult{err: fmt.Errorf("worker [%d,%d): %w", sID, eID, err)}
				return
			}
			defer func() { _ = rows.Close() }()

			for rows.Next() {
				var chunkID int
				var vecBlob []byte
				var norm float64
				if err := rows.Scan(&chunkID, &vecBlob, &norm); err != nil {
					continue
				}
				vec := vecbytes.BytesToFloats(vecBlob)
				if len(vec) != len(queryVec) {
					continue
				}
				dot := float64(0)
				for i, v := range queryVec {
					dot += float64(v) * float64(vec[i])
				}
				cosine := dot / (qNorm * norm)

				if len(localTop) < topK {
					localTop = append(localTop, candidate{chunkID, cosine})
					for j := len(localTop) - 1; j > 0; j-- {
						if localTop[j].score > localTop[j-1].score {
							localTop[j], localTop[j-1] = localTop[j-1], localTop[j]
						}
					}
				} else if cosine > localTop[len(localTop)-1].score {
					localTop[len(localTop)-1] = candidate{chunkID, cosine}
					for j := len(localTop) - 1; j > 0; j-- {
						if localTop[j].score > localTop[j-1].score {
							localTop[j], localTop[j-1] = localTop[j-1], localTop[j]
						}
					}
				}
			}
			results <- workerResult{top: localTop}
		}(startID, endID)
	}

	wg.Wait()
	close(results)

	// Merge worker results, collecting any errors.
	var workerErrs []error
	merged := make([]candidate, 0, topK)
	for wr := range results {
		if wr.err != nil {
			workerErrs = append(workerErrs, wr.err)
			continue
		}
		for _, c := range wr.top {
			if len(merged) < topK {
				merged = append(merged, c)
				for j := len(merged) - 1; j > 0; j-- {
					if merged[j].score > merged[j-1].score {
						merged[j], merged[j-1] = merged[j-1], merged[j]
					}
				}
			} else if c.score > merged[len(merged)-1].score {
				merged[len(merged)-1] = c
				for j := len(merged) - 1; j > 0; j-- {
					if merged[j].score > merged[j-1].score {
						merged[j], merged[j-1] = merged[j-1], merged[j]
					}
				}
			}
		}
	}

	// If no results AND no errors from any worker, return empty.
	if len(merged) == 0 && len(workerErrs) == 0 {
		return nil, nil
	}
	// If no results but some workers errored, report the first error.
	if len(merged) == 0 && len(workerErrs) > 0 {
		return nil, fmt.Errorf("vector sql parallel: %w", workerErrs[0])
	}

	// Fetch chunk content.
	chunkIDs := make([]int, len(merged))
	for i, c := range merged {
		chunkIDs[i] = c.chunkID
	}
	chunkMap := s.getChunksBatch(chunkIDs)
	results2 := make([]retrieval.ScoredChunk, 0, len(merged))
	for _, c := range merged {
		chunk, ok := chunkMap[c.chunkID]
		if !ok || chunk == nil {
			continue
		}
		results2 = append(results2, retrieval.ScoredChunk{
			Chunk:   *chunk,
			Score:   c.score,
			Matches: []string{},
		})
	}
	return results2, nil
}

// getChunk retrieves a single chunk by its integer ID.
func (s *SQLiteStore) getChunk(chunkID int) (*retrieval.Chunk, error) {
	var id int
	var docID, heading, content string
	var chunkIdx int
	err := s.db.QueryRowContext(s.baseCtx, `
		SELECT id, document_id, chunk_index, heading, content
		FROM chunks WHERE id = ?`, chunkID).Scan(&id, &docID, &chunkIdx, &heading, &content)
	if err != nil {
		return nil, err
	}
	return &retrieval.Chunk{
		ID:       strconv.Itoa(id),
		DocID:    docID,
		Content:  content,
		Position: chunkIdx,
		Metadata: map[string]string{"heading": heading},
	}, nil
}

// getChunksBatchImpl performs the actual batch SQL query.
func (s *SQLiteStore) getChunksBatchImpl(ids []int) (map[int]*retrieval.Chunk, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Build placeholders: WHERE id IN (?, ?, ...)
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf( //nolint:gosec // safe: only ? placeholders for IN clause
		"SELECT id, document_id, chunk_index, heading, content FROM chunks WHERE id IN (%s)",
		strings.Join(placeholders, ","),
	)

	rows, err := s.db.QueryContext(s.baseCtx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getChunksBatch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int]*retrieval.Chunk, len(ids))
	for rows.Next() {
		var id int
		var docID, heading, content string
		var chunkIdx int
		if err := rows.Scan(&id, &docID, &chunkIdx, &heading, &content); err != nil {
			return nil, fmt.Errorf("getChunksBatch scan: %w", err)
		}
		result[id] = &retrieval.Chunk{
			ID:       strconv.Itoa(id),
			DocID:    docID,
			Content:  content,
			Position: chunkIdx,
			Metadata: map[string]string{"heading": heading},
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getChunksBatch rows: %w", err)
	}
	return result, nil
}
