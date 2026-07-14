package sqlite

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

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
	db       *sql.DB      // knowledge.db — documents, chunks, FTS, embeddings, KG
	lawsDB   *sql.DB      // laws-full.db — 9 121 laws with full text
	kgDB     *sql.DB      // patent_kg.db — 116 K nodes / 484 K edges
	dim      int          // embedding dimension (default 1024 for BGE-M3)
	vecIndex *VectorIndex // pre-loaded in-memory vector index (nil until PreloadVectors)
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
	return nil
}

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

// VectorSearch performs brute-force cosine-similarity search against stored
// BGE-M3 embeddings. If the in-memory vector index is loaded (via
// PreloadVectors), it uses parallel in-memory computation (~50-200ms for
// 144K vectors). Otherwise it falls back to SQL batch reads (seconds).
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

	// Slow path: SQL batch reads (fallback).

	// Pre-compute query norm.
	qNorm := float64(0)
	for _, v := range queryVec {
		qNorm += float64(v) * float64(v)
	}
	qNorm = math.Sqrt(qNorm)
	if qNorm == 0 {
		return nil, fmt.Errorf("query vector is zero")
	}

	type candidate struct {
		chunkID    int
		documentID string
		score      float64
	}
	top := make([]candidate, 0, topK+1)
	batchSize := 2000
	offset := 0

	for {
		rows, err := s.db.Query(`
			SELECT e.chunk_id, e.document_id, e.vector, e.norm
			FROM embeddings e
			ORDER BY e.id
			LIMIT ? OFFSET ?`, batchSize, offset)
		if err != nil {
			return nil, fmt.Errorf("vector query: %w", err)
		}

		count := 0
		for rows.Next() {
			count++
			var chunkID int
			var documentID string
			var vecBlob []byte
			var norm float64
			if err := rows.Scan(&chunkID, &documentID, &vecBlob, &norm); err != nil {
				rows.Close()
				return nil, fmt.Errorf("vector scan: %w", err)
			}

			// Compute dot product.
			vec := bytesToFloat32(vecBlob)
			if len(vec) != s.dim {
				continue
			}
			dot := float64(0)
			for i := 0; i < s.dim; i++ {
				dot += float64(queryVec[i]) * float64(vec[i])
			}
			cosine := dot / (qNorm * norm)

			// Insert into top-K.
			if len(top) < topK {
				top = append(top, candidate{chunkID, documentID, cosine})
				// Sort descending by score (simple insertion sort for small K).
				for j := len(top) - 1; j > 0; j-- {
					if top[j].score > top[j-1].score {
						top[j], top[j-1] = top[j-1], top[j]
					} else {
						break
					}
				}
			} else if cosine > top[len(top)-1].score {
				top[len(top)-1] = candidate{chunkID, documentID, cosine}
				for j := len(top) - 1; j > 0; j-- {
					if top[j].score > top[j-1].score {
						top[j], top[j-1] = top[j-1], top[j]
					} else {
						break
					}
				}
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("vector rows iteration: %w", err)
		}
		if count < batchSize {
			break
		}
		offset += batchSize
	}

	// Fetch chunk content for the top results with a single batch query,
	// falling back to per-chunk fetch if the batch query fails.
	chunkIDs := make([]int, len(top))
	for i, c := range top {
		chunkIDs[i] = c.chunkID
	}
	chunkMap := s.getChunksBatch(chunkIDs)
	results := make([]retrieval.ScoredChunk, 0, len(top))
	for _, c := range top {
		chunk, ok := chunkMap[c.chunkID]
		if !ok || chunk == nil {
			continue
		}
		results = append(results, retrieval.ScoredChunk{
			Chunk:   *chunk,
			Score:   c.score,
			Matches: []string{},
		})
	}
	return results, nil
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

// SearchLaws searches the laws-full.db by law name or content keyword.
// It returns matching law records with their full text.
func (s *SQLiteStore) SearchLaws(keyword string, topK int) ([]LawRecord, error) {
	if s.lawsDB == nil {
		return nil, fmt.Errorf("laws-full.db not opened")
	}
	if topK <= 0 {
		topK = 10
	}
	// Escape SQL LIKE wildcards so the keyword is matched literally.
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
		return nil, fmt.Errorf("search laws: %w", err)
	}
	defer rows.Close()

	var results []LawRecord
	for rows.Next() {
		var r LawRecord
		var subtitle, content sql.NullString
		if err := rows.Scan(&r.ID, &r.Level, &r.Name, &subtitle, &content, &r.Category); err != nil {
			return nil, fmt.Errorf("scan law: %w", err)
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
