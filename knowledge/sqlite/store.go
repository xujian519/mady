package sqlite

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver

	"github.com/xujian519/mady/knowledge/graph"
	"github.com/xujian519/mady/retrieval"
)

// SQLiteStore provides read-only access to the XiaoNuo knowledge databases
// (knowledge.db, laws-full.db, patent_kg.db). It exposes FTS5 full-text
// search, vector similarity search, and knowledge-graph loading — all backed
// by pre-built SQLite databases that share the same data model as Mady's
// in-memory Store and GraphStore.
type SQLiteStore struct {
	db        *sql.DB      // knowledge.db — documents, chunks, FTS, embeddings, KG
	lawsDB    *sql.DB      // laws-full.db — 9 121 laws with full text
	kgDB      *sql.DB      // patent_kg.db — 116 K nodes / 484 K edges
	dim       int          // embedding dimension (default 1024 for BGE-M3)
	vecIndex  *VectorIndex // pre-loaded in-memory vector index (nil until PreloadVectors)
	hasLawFTS bool         // true when laws-full-local.db has FTS5 index (law_fts table)
}

// NewSQLiteStore opens knowledge.db in read-only mode. The database is
// expected at the given path (typically resolved via util.ResolveDataDir).
func NewSQLiteStore(knowledgeDBPath string) (*SQLiteStore, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", knowledgeDBPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open knowledge.db: %w", err)
	}
	db.SetMaxOpenConns(2) // read-only; limit connections

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping knowledge.db: %w", err)
	}

	// Detect embedding dimension from stored vectors.
	dim := 1024
	var vecLen int
	row := db.QueryRow("SELECT length(vector) FROM embeddings LIMIT 1")
	if err := row.Scan(&vecLen); err != nil {
		db.Close()
		return nil, fmt.Errorf("detect embedding dimension: %w", err)
	}
	if vecLen > 0 {
		dim = vecLen / 4 // float32 = 4 bytes
	}

	return &SQLiteStore{db: db, dim: dim}, nil
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

// HasVectorIndex returns true if the in-memory vector index is loaded.
func (s *SQLiteStore) HasVectorIndex() bool {
	return s.vecIndex != nil
}

// OpenLawsDB opens laws-full.db for law full-text search.
func (s *SQLiteStore) OpenLawsDB(path string) error {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", path)
	lawsDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open laws-full.db: %w", err)
	}
	lawsDB.SetMaxOpenConns(1)
	s.lawsDB = lawsDB

	// Detect whether the law_fts (FTS5) table exists.
	row := lawsDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='law_fts'")
	var ftsCount int
	if err := row.Scan(&ftsCount); err == nil && ftsCount > 0 {
		s.hasLawFTS = true
	}
	return nil
}

// HasLawFTS returns true when the open laws database includes a law_fts
// FTS5 virtual table for fast BM25-ranked search.
func (s *SQLiteStore) HasLawFTS() bool { return s.hasLawFTS }

// OpenPatentKGDB opens patent_kg.db for supplementary graph queries.
func (s *SQLiteStore) OpenPatentKGdb(path string) error {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", path)
	kgDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open patent_kg.db: %w", err)
	}
	kgDB.SetMaxOpenConns(1)
	s.kgDB = kgDB
	return nil
}

// Close closes all opened database connections.
func (s *SQLiteStore) Close() error {
	var errs []error
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close db: %w", err))
		}
	}
	if s.lawsDB != nil {
		if err := s.lawsDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close lawsDB: %w", err))
		}
	}
	if s.kgDB != nil {
		if err := s.kgDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close kgDB: %w", err))
		}
	}
	return errors.Join(errs...)
}

// FTSSearch performs full-text search against the docs_fts trigram index.
// It returns chunks ranked by BM25, with content retrieved from the chunks
// table via rowid join.
func (s *SQLiteStore) FTSSearch(query string, topK int) ([]retrieval.ScoredChunk, error) {
	if topK <= 0 {
		topK = 10
	}
	// Wrap query in double quotes for FTS5 phrase matching. Internal double
	// quotes are escaped by doubling (FTS5 convention). Trigram tokenizer
	// handles CJK text naturally — no manual segmentation needed.
	ftsQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	sqlQuery := `
		SELECT c.id, c.document_id, c.chunk_index, c.heading, c.content,
		       bm25(docs_fts) AS score
		FROM docs_fts
		JOIN chunks c ON c.id = docs_fts.rowid
		WHERE docs_fts MATCH ?
		ORDER BY score
		LIMIT ?`
	rows, err := s.db.Query(sqlQuery, ftsQuery, topK)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer rows.Close()

	var results []retrieval.ScoredChunk
	for rows.Next() {
		var id int
		var docID, content string
		var heading sql.NullString
		var chunkIdx int
		var score float64
		if err := rows.Scan(&id, &docID, &chunkIdx, &heading, &content, &score); err != nil {
			return nil, fmt.Errorf("fts scan: %w", err)
		}
		meta := map[string]string{
			"heading":    heading.String,
			"chunk_type": "section",
		}
		results = append(results, retrieval.ScoredChunk{
			Chunk: retrieval.Chunk{
				ID:       strconv.Itoa(id),
				DocID:    docID,
				Content:  content,
				Position: chunkIdx,
				Metadata: meta,
			},
			Score:   -score, // bm25 returns negative values; negate for higher=better
			Matches: []string{query},
		})
	}
	return results, rows.Err()
}

// GetChunksByDocID returns up to limit chunks belonging to the given document
// ID, ordered by chunk_index. It is the doc-level fetch counterpart to the
// query-level FTSSearch/VectorSearch, enabling DomainRetriever.GetDocument to
// reconstruct a document's text without exposing the underlying *sql.DB.
//
// limit <= 0 defaults to 10. Returns an empty slice (no error) when the
// document ID has no chunks in the store.
func (s *SQLiteStore) GetChunksByDocID(docID string, limit int) ([]retrieval.ScoredChunk, error) {
	if docID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT id, document_id, chunk_index, heading, content
		FROM chunks
		WHERE document_id = ?
		ORDER BY chunk_index
		LIMIT ?`, docID, limit)
	if err != nil {
		return nil, fmt.Errorf("get chunks by docID: %w", err)
	}
	defer rows.Close()

	var results []retrieval.ScoredChunk
	for rows.Next() {
		var id int
		var did, content string
		var heading sql.NullString
		var chunkIdx int
		if err := rows.Scan(&id, &did, &chunkIdx, &heading, &content); err != nil {
			return nil, fmt.Errorf("get chunks scan: %w", err)
		}
		results = append(results, retrieval.ScoredChunk{
			Chunk: retrieval.Chunk{
				ID:       strconv.Itoa(id),
				DocID:    did,
				Content:  content,
				Position: chunkIdx,
				Metadata: map[string]string{
					"heading":    heading.String,
					"chunk_type": "section",
				},
			},
		})
	}
	return results, rows.Err()
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

// vectorSearchSQLParallel is the fallback path when the in-memory vector
// index is not available. It uses parallel goroutines to scan the embeddings
// table in ranges, each maintaining a min-heap of top-K candidates.
// This reduces query time from ~14s (sequential) to ~1-2s on M4 Pro.
func (s *SQLiteStore) vectorSearchSQLParallel(queryVec []float32, topK int) ([]retrieval.ScoredChunk, error) {
	// Determine max ID for range partitioning — using MAX(id) instead of
	// COUNT(*) so that gaps from deletions don't cause tail rows to be missed.
	var maxID int
	if err := s.db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM embeddings").Scan(&maxID); err != nil {
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

			rows, err := s.db.Query(`
				SELECT e.chunk_id, e.vector, e.norm
				FROM embeddings e
				WHERE e.id >= ? AND e.id < ?
				ORDER BY e.id`, sID, eID)
			if err != nil {
				results <- workerResult{err: fmt.Errorf("worker [%d,%d): %w", sID, eID, err)}
				return
			}
			defer rows.Close()

			for rows.Next() {
				var chunkID int
				var vecBlob []byte
				var norm float64
				if err := rows.Scan(&chunkID, &vecBlob, &norm); err != nil {
					continue
				}
				vec := bytesToFloat32(vecBlob)
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

// getChunk retrieves a single chunk by its integer ID.
func (s *SQLiteStore) getChunk(chunkID int) (*retrieval.Chunk, error) {
	var id int
	var docID, heading, content string
	var chunkIdx int
	err := s.db.QueryRow(`
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

	query := fmt.Sprintf(
		"SELECT id, document_id, chunk_index, heading, content FROM chunks WHERE id IN (%s)",
		strings.Join(placeholders, ","),
	)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("getChunksBatch: %w", err)
	}
	defer rows.Close()

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

// LoadGraph loads all nodes and edges from kg_nodes/kg_edges into a new
// GraphStore. The SQLite schema mirrors Mady's GraphNode/GraphEdge types
// exactly, so mapping is a direct field-to-column translation.
func (s *SQLiteStore) LoadGraph() (*graph.GraphStore, error) {
	gs := graph.NewGraphStore()

	// Load nodes.
	nodeRows, err := s.db.Query(`
		SELECT id, node_type, name, title, content, domain, source,
		       full_ref, chapter, article_number, law_refs,
		       priority, authority_weight, level_in_hierarchy
		FROM kg_nodes`)
	if err != nil {
		return nil, fmt.Errorf("load graph nodes: %w", err)
	}
	defer nodeRows.Close()

	nodeCount := 0
	for nodeRows.Next() {
		var n graph.GraphNode
		var title, content, source, fullRef, chapter, articleNumber, lawRefs sql.NullString
		var priority, levelInHierarchy sql.NullInt64
		var authorityWeight sql.NullFloat64

		if err := nodeRows.Scan(
			&n.ID, &n.NodeType, &n.Name, &title, &content, &n.Domain,
			&source, &fullRef, &chapter, &articleNumber, &lawRefs,
			&priority, &authorityWeight, &levelInHierarchy,
		); err != nil {
			return nil, fmt.Errorf("scan graph node: %w", err)
		}

		n.Title = title.String
		n.Content = content.String
		n.Source = source.String
		n.FullRef = fullRef.String
		n.Chapter = chapter.String
		n.ArticleNumber = articleNumber.String
		if lawRefs.String != "" {
			n.LawRefs = strings.Split(lawRefs.String, ";")
		}
		n.Priority = int(priority.Int64)
		n.AuthorityWeight = authorityWeight.Float64
		n.LevelInHierarchy = int(levelInHierarchy.Int64)

		gs.AddNode(&n)
		nodeCount++
	}
	if err := nodeRows.Err(); err != nil {
		return nil, err
	}

	// Load edges.
	edgeRows, err := s.db.Query(`
		SELECT source_id, target_id, relation, weight, evidence
		FROM kg_edges`)
	if err != nil {
		return nil, fmt.Errorf("load graph edges: %w", err)
	}
	defer edgeRows.Close()

	edgeCount := 0
	for edgeRows.Next() {
		var e graph.GraphEdge
		var weight sql.NullFloat64
		var evidence sql.NullString
		if err := edgeRows.Scan(&e.SourceID, &e.TargetID, &e.Relation, &weight, &evidence); err != nil {
			return nil, fmt.Errorf("scan graph edge: %w", err)
		}
		e.Weight = weight.Float64
		e.Evidence = evidence.String
		if gs.HasNode(e.SourceID) && gs.HasNode(e.TargetID) {
			gs.AddEdge(e)
			edgeCount++
		}
	}

	return gs, edgeRows.Err()
}

// SearchLaws searches the laws db by law name or content keyword.
// When the law_fts FTS5 table is present (detected in OpenLawsDB), it
// uses BM25-ranked FTS5 search for better relevance. Otherwise it falls
// back to LIKE pattern matching with order-based sort.
//
// For short queries (< 3 CJK characters) the LIKE fallback is used even
// when FTS5 is available, since the trigram tokenizer requires 3+ chars.
func (s *SQLiteStore) SearchLaws(keyword string, topK int) ([]LawRecord, error) {
	if s.lawsDB == nil {
		return nil, fmt.Errorf("laws-full.db not opened")
	}
	if topK <= 0 {
		topK = 10
	}

	// Use FTS5 path when available and query is long enough (> 2 runes).
	if s.hasLawFTS && len([]rune(keyword)) >= 3 {
		return s.searchLawsFTS(keyword, topK)
	}

	return s.searchLawsLike(keyword, topK)
}

// searchLawsFTS uses the law_fts FTS5 virtual table with BM25 ranking.
func (s *SQLiteStore) searchLawsFTS(keyword string, topK int) ([]LawRecord, error) {
	// Wrap query in double quotes for FTS5 phrase matching. The trigram
	// tokenizer handles CJK text by splitting into 3-character n-grams.
	ftsQuery := `"` + strings.ReplaceAll(keyword, `"`, `""`) + `"`
	rows, err := s.lawsDB.Query(`
		SELECT l.id, l.level, l.name, l.subtitle, l.content,
		       c.name AS category_name
		FROM law_fts
		JOIN law l ON l.id = law_fts.rowid
		JOIN category c ON c.id = l.category_id
		WHERE law_fts MATCH ?
		ORDER BY bm25(law_fts)
		LIMIT ?`, ftsQuery, topK)
	if err != nil {
		return nil, fmt.Errorf("laws fts search: %w", err)
	}
	defer rows.Close()

	var results []LawRecord
	for rows.Next() {
		var r LawRecord
		var subtitle, content sql.NullString
		if err := rows.Scan(&r.ID, &r.Level, &r.Name, &subtitle, &content, &r.Category); err != nil {
			return nil, fmt.Errorf("laws fts scan: %w", err)
		}
		r.Subtitle = subtitle.String
		r.Content = content.String
		results = append(results, r)
	}
	return results, rows.Err()
}

// searchLawsLike falls back to LIKE pattern matching across name and
// content columns. Results are ordered by the law.order field.
func (s *SQLiteStore) searchLawsLike(keyword string, topK int) ([]LawRecord, error) {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(keyword)
	pattern := "%" + escaped + "%"
	rows, err := s.lawsDB.Query(`
		SELECT l.id, l.level, l.name, l.subtitle, l.content,
		       c.name AS category_name
		FROM law l
		JOIN category c ON c.id = l.category_id
		WHERE l.name LIKE ? ESCAPE '\' OR l.content LIKE ? ESCAPE '\'
		ORDER BY l."order"
		LIMIT ?`, pattern, pattern, topK)
	if err != nil {
		return nil, fmt.Errorf("laws like search: %w", err)
	}
	defer rows.Close()

	var results []LawRecord
	for rows.Next() {
		var r LawRecord
		var subtitle, content sql.NullString
		if err := rows.Scan(&r.ID, &r.Level, &r.Name, &subtitle, &content, &r.Category); err != nil {
			return nil, fmt.Errorf("laws like scan: %w", err)
		}
		r.Subtitle = subtitle.String
		r.Content = content.String
		results = append(results, r)
	}
	return results, rows.Err()
}

// LawRecord represents a single law from laws-full.db.
type LawRecord struct {
	ID       string
	Level    string // 法律/行政法规/司法解释/部门规章
	Name     string
	Subtitle string
	Content  string
	Category string
}

// SampleVector returns a single vector from the embeddings table.
// Useful for benchmarks that need a realistic query vector without
// depending on an external embedding service.
func (s *SQLiteStore) SampleVector() []float32 {
	var blob []byte
	err := s.db.QueryRow("SELECT vector FROM embeddings LIMIT 1").Scan(&blob)
	if err != nil || len(blob) == 0 {
		return nil
	}
	return bytesToFloat32(blob)
}

// EmbeddingDim returns the detected embedding dimension.
func (s *SQLiteStore) EmbeddingDim() int { return s.dim }

// StoreStats holds aggregate statistics for startup diagnostics.
type StoreStats struct {
	Documents      int
	Chunks         int
	Embeddings     int
	Dim            int
	VectorMemoryMB float64
}

// Stats queries the database for aggregate statistics.
func (s *SQLiteStore) Stats() StoreStats {
	var st StoreStats
	_ = s.db.QueryRow("SELECT COUNT(*) FROM documents").Scan(&st.Documents)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&st.Chunks)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM embeddings").Scan(&st.Embeddings)
	var blobLen int
	_ = s.db.QueryRow("SELECT LENGTH(vector) FROM embeddings LIMIT 1").Scan(&blobLen)
	if blobLen > 0 {
		st.Dim = blobLen / 4
	} else {
		st.Dim = s.dim
	}
	// Cast to float64 before multiplication to avoid int overflow on 32-bit
	// platforms when embeddings exceed ~500K (int32 max ≈ 2.1B).
	st.VectorMemoryMB = float64(st.Embeddings) * float64(st.Dim) * 4 / 1024 / 1024
	return st
}

// bytesToFloat32 decodes a little-endian float32 BLOB into a slice.
func bytesToFloat32(b []byte) []float32 {
	count := len(b) / 4
	vec := make([]float32, count)
	for i := 0; i < count; i++ {
		bits := binary.LittleEndian.Uint32(b[i*4 : i*4+4])
		vec[i] = math.Float32frombits(bits)
	}
	return vec
}
