package sqlite

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver

	"github.com/xujian519/mady/retrieval"
)

// GetChunksByDocID retrieves all chunks for a given document ID, using
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
	rows, err := s.db.QueryContext(s.baseCtx, `
		SELECT id, document_id, chunk_index, heading, content
		FROM chunks
		WHERE document_id = ?
		ORDER BY chunk_index
		LIMIT ?`, docID, limit)
	if err != nil {
		return nil, fmt.Errorf("get chunks by docID: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
	rows, err := s.db.QueryContext(s.baseCtx, sqlQuery, ftsQuery, topK)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
