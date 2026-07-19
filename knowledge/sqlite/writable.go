package sqlite

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver

	"github.com/xujian519/mady/retrieval"
)

// embedBatchSize aligns with oMLX embedding_batch_size to avoid overloading
// the local embedding service in a single request.
const embedBatchSize = 32

// ErrKnowledgeDBConflict is returned when the writable store path resolves to
// the same file as the read-only knowledge.db, which would corrupt the
// authoritative database.
var ErrKnowledgeDBConflict = errors.New("writable: path must not point to knowledge.db")

// WritableStore provides read-write access to a user-owned SQLite database
// (user.db). Documents added via AddDocument are chunked, embedded, and
// persisted so that subsequent searches return them alongside knowledge.db
// results via RRF fusion in the KnowledgeExtension.
//
// The schema mirrors knowledge.db (documents / chunks / embeddings / docs_fts)
// to keep query patterns uniform, but the database is physically separate and
// opened in read-write WAL mode for safe concurrent reads during writes.
type WritableStore struct {
	db       *sql.DB
	dim      int
	embedder retrieval.Embedder
	mu       sync.Mutex // serialize writes
}

// OpenWritable opens or creates the user database at the given path. If the
// file does not exist it is created with the full schema. The embedder is
// used for vectorising documents at write time and query vectors at search
// time; it must return the same dimensionality as stored vectors.
//
// knowledgeDBPath is the path to the read-only knowledge.db; OpenWritable
// rejects paths that resolve to the same file to prevent accidental writes
// to the authoritative database.
func OpenWritable(path string, embedder retrieval.Embedder, knowledgeDBPath string) (*WritableStore, error) {
	if knowledgeDBPath != "" {
		absUser, err1 := filepath.Abs(path)
		absKnow, err2 := filepath.Abs(knowledgeDBPath)
		if err1 == nil && err2 == nil && absUser == absKnow {
			return nil, ErrKnowledgeDBConflict
		}
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open writable db: %w", err)
	}
	db.SetMaxOpenConns(4)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping writable db: %w", err)
	}

	dim := 0
	if embedder != nil {
		dim = embedder.Dimensions()
	}
	if dim <= 0 {
		dim = 1024 // default for BGE-M3
	}

	w := &WritableStore{db: db, dim: dim, embedder: embedder}
	if err := w.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return w, nil
}

// initSchema creates tables if they do not exist. Idempotent — safe to call
// on an already-initialized database.
func (w *WritableStore) initSchema() error {
	_, err := w.db.Exec(`
		CREATE TABLE IF NOT EXISTS documents (
			id            TEXT PRIMARY KEY,
			source        TEXT NOT NULL,
			doc_type      TEXT NOT NULL DEFAULT 'document',
			domain        TEXT DEFAULT 'patent',
			title         TEXT NOT NULL,
			content_hash  TEXT,
			indexed_at    TEXT NOT NULL,
			char_count    INTEGER,
			chunk_count   INTEGER
		);
		CREATE TABLE IF NOT EXISTS chunks (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			document_id   TEXT NOT NULL,
			chunk_index   INTEGER NOT NULL,
			chunk_type    TEXT NOT NULL DEFAULT 'section',
			heading       TEXT,
			content       TEXT NOT NULL,
			char_count    INTEGER
		);
		CREATE TABLE IF NOT EXISTS embeddings (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			chunk_id      INTEGER NOT NULL,
			document_id   TEXT NOT NULL,
			vector        BLOB NOT NULL,
			model         TEXT NOT NULL,
			dim           INTEGER NOT NULL,
			norm          REAL NOT NULL,
			indexed_at    TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_user_embeddings_doc    ON embeddings(document_id);
		CREATE INDEX IF NOT EXISTS idx_user_embeddings_chunk  ON embeddings(chunk_id);
		CREATE VIRTUAL TABLE IF NOT EXISTS docs_fts USING fts5(content, heading, document_id, chunk_id, tokenize='trigram');
	`)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	return nil
}

// AddDocument chunks the content, embeds each chunk via the configured
// embedder, and persists the document, chunks, embeddings, and FTS index
// in a single transaction. If a document with the same docID already exists
// it is replaced (delete + insert).
//
// The call is serialized by a mutex to ensure transaction integrity. Reads
// from other goroutines are not blocked (WAL mode allows concurrent readers).
func (w *WritableStore) AddDocument(ctx context.Context, docID, title, content string) error {
	if docID == "" {
		return errors.New("writable: docID must not be empty")
	}
	if content == "" {
		return errors.New("writable: content must not be empty")
	}
	if w.embedder == nil {
		return errors.New("writable: embedder not configured")
	}

	chunks := retrieval.ChunkDocument(docID, content, retrieval.DefaultChunkOptions())
	if len(chunks) == 0 {
		return errors.New("writable: chunking produced no chunks")
	}

	// Embed in batches of embedBatchSize to respect oMLX limits.
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	allVecs := make([][]float32, 0, len(chunks))
	for i := 0; i < len(texts); i += embedBatchSize {
		end := i + embedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := w.embedder.Embed(ctx, texts[i:end])
		if err != nil {
			return fmt.Errorf("embed batch %d-%d: %w", i, end, err)
		}
		allVecs = append(allVecs, vecs...)
	}

	if len(allVecs) != len(chunks) {
		return fmt.Errorf("embed count mismatch: %d chunks, %d vectors", len(chunks), len(allVecs))
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC().Format(time.RFC3339)

	// Replace existing document with the same ID.
	if _, err := tx.Exec(`DELETE FROM embeddings WHERE document_id = ?`, docID); err != nil {
		return fmt.Errorf("delete old embeddings: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM chunks WHERE document_id = ?`, docID); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM documents WHERE id = ?`, docID); err != nil {
		return fmt.Errorf("delete old document: %w", err)
	}

	// Insert document.
	if _, err := tx.Exec(
		`INSERT INTO documents (id, source, doc_type, domain, title, content_hash, indexed_at, char_count, chunk_count) VALUES (?,?,?,?,?,?,?,?,?)`,
		docID, "user", "document", "patent", title, hashString(content), now, len(content), len(chunks),
	); err != nil {
		return fmt.Errorf("insert document: %w", err)
	}

	// Insert chunks + embeddings + FTS.
	modelName := "bge-m3"
	for i, chunk := range chunks {
		vec := allVecs[i]
		if len(vec) != w.dim {
			return fmt.Errorf("vector dim mismatch at chunk %d: got %d, want %d", i, len(vec), w.dim)
		}
		blob := float32ToBytes(vec)
		norm := vecNorm(vec)

		res, err := tx.Exec(
			`INSERT INTO chunks (document_id, chunk_index, chunk_type, heading, content, char_count) VALUES (?,?,?,?,?,?)`,
			docID, chunk.Position, "section", "", chunk.Content, len(chunk.Content),
		)
		if err != nil {
			return fmt.Errorf("insert chunk %d: %w", i, err)
		}
		chunkID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("last insert id chunk %d: %w", i, err)
		}

		if _, err := tx.Exec(
			`INSERT INTO embeddings (chunk_id, document_id, vector, model, dim, norm, indexed_at) VALUES (?,?,?,?,?,?,?)`,
			chunkID, docID, blob, modelName, w.dim, norm, now,
		); err != nil {
			return fmt.Errorf("insert embedding %d: %w", i, err)
		}

		if _, err := tx.Exec(
			`INSERT INTO docs_fts (content, heading, document_id, chunk_id) VALUES (?,?,?,?)`,
			chunk.Content, "", docID, chunkID,
		); err != nil {
			return fmt.Errorf("insert fts %d: %w", i, err)
		}
	}

	return tx.Commit()
}

// Search performs FTS + vector RRF fusion within the user database. It
// returns scored chunks that can be merged with knowledge.db results by
// the KnowledgeExtension's backendSearch.
func (w *WritableStore) Search(ctx context.Context, query string, topK int) ([]retrieval.ScoredChunk, error) {
	if topK <= 0 {
		topK = 10
	}

	var lists [][]retrieval.ScoredChunk

	// FTS path.
	if ftsResults, err := w.ftsSearch(query, topK); err == nil && len(ftsResults) > 0 {
		lists = append(lists, ftsResults)
	} else if err != nil {
		slog.Error("writable: FTS search error", "err", err)
	}

	// Vector path.
	if w.embedder != nil {
		vecs, err := w.embedder.Embed(ctx, []string{query})
		if err == nil && len(vecs) > 0 && len(vecs[0]) > 0 {
			if vecResults, vErr := w.vectorSearch(vecs[0], topK); vErr == nil && len(vecResults) > 0 {
				lists = append(lists, vecResults)
			} else if vErr != nil {
				slog.Error("writable: vector search error", "err", vErr)
			}
		} else if err != nil {
			slog.Error("writable: embed error", "err", err)
		}
	}

	if len(lists) == 0 {
		return nil, nil
	}

	fuser := retrieval.NewRRFFuser()
	return fuser.Fuse(lists, topK), nil
}

// ftsSearch performs BM25 full-text search against the user database.
func (w *WritableStore) ftsSearch(query string, topK int) ([]retrieval.ScoredChunk, error) {
	ftsQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	rows, err := w.db.Query(`
		SELECT c.id, c.document_id, c.chunk_index, c.content,
		       bm25(docs_fts) AS score
		FROM docs_fts
		JOIN chunks c ON c.id = docs_fts.chunk_id
		WHERE docs_fts MATCH ?
		ORDER BY score
		LIMIT ?`, ftsQuery, topK)
	if err != nil {
		return nil, fmt.Errorf("writable fts: %w", err)
	}
	defer rows.Close()

	var results []retrieval.ScoredChunk
	for rows.Next() {
		var id int
		var docID, content string
		var chunkIdx int
		var score float64
		if err := rows.Scan(&id, &docID, &chunkIdx, &content, &score); err != nil {
			return nil, fmt.Errorf("writable fts scan: %w", err)
		}
		results = append(results, retrieval.ScoredChunk{
			Chunk: retrieval.Chunk{
				ID:       "u:" + strconv.Itoa(id),
				DocID:    docID,
				Content:  content,
				Position: chunkIdx,
				Metadata: map[string]string{"chunk_type": "section", "source": "user"},
			},
			Score:   -score,
			Matches: []string{query},
		})
	}
	return results, rows.Err()
}

// vectorSearch performs brute-force cosine similarity search against the
// user database embeddings. User databases are expected to be small, so
// no in-memory index is built.
func (w *WritableStore) vectorSearch(queryVec []float32, topK int) ([]retrieval.ScoredChunk, error) {
	if len(queryVec) != w.dim {
		return nil, fmt.Errorf("vector dim mismatch: got %d, want %d", len(queryVec), w.dim)
	}

	qNorm := float64(0)
	for _, v := range queryVec {
		qNorm += float64(v) * float64(v)
	}
	qNorm = math.Sqrt(qNorm)
	if qNorm == 0 {
		return nil, errors.New("query vector is zero")
	}

	type candidate struct {
		chunkID int
		docID   string
		score   float64
	}
	top := make([]candidate, 0, topK+1)

	rows, err := w.db.Query(`SELECT chunk_id, document_id, vector, norm FROM embeddings`)
	if err != nil {
		return nil, fmt.Errorf("writable vector query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var chunkID int
		var docID string
		var vecBlob []byte
		var norm float64
		if err := rows.Scan(&chunkID, &docID, &vecBlob, &norm); err != nil {
			return nil, fmt.Errorf("writable vector scan: %w", err)
		}
		vec := bytesToFloat32(vecBlob)
		if len(vec) != w.dim || norm == 0 {
			continue
		}
		dot := float64(0)
		for i := 0; i < w.dim; i++ {
			dot += float64(queryVec[i]) * float64(vec[i])
		}
		cosine := dot / (qNorm * norm)

		if len(top) < topK {
			top = append(top, candidate{chunkID, docID, cosine})
			for j := len(top) - 1; j > 0; j-- {
				if top[j].score > top[j-1].score {
					top[j], top[j-1] = top[j-1], top[j]
				} else {
					break
				}
			}
		} else if cosine > top[len(top)-1].score {
			top[len(top)-1] = candidate{chunkID, docID, cosine}
			for j := len(top) - 1; j > 0; j-- {
				if top[j].score > top[j-1].score {
					top[j], top[j-1] = top[j-1], top[j]
				} else {
					break
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	results := make([]retrieval.ScoredChunk, 0, len(top))
	for _, c := range top {
		chunk, err := w.getChunk(c.chunkID)
		if err != nil || chunk == nil {
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

// getChunk retrieves a single chunk by ID from the user database.
func (w *WritableStore) getChunk(chunkID int) (*retrieval.Chunk, error) {
	var id int
	var docID, content string
	var chunkIdx int
	err := w.db.QueryRow(
		`SELECT id, document_id, chunk_index, content FROM chunks WHERE id = ?`, chunkID,
	).Scan(&id, &docID, &chunkIdx, &content)
	if err != nil {
		return nil, err
	}
	return &retrieval.Chunk{
		ID:       "u:" + strconv.Itoa(id),
		DocID:    docID,
		Content:  content,
		Position: chunkIdx,
		Metadata: map[string]string{"source": "user"},
	}, nil
}

// Dim returns the embedding dimension used by this store.
func (w *WritableStore) Dim() int { return w.dim }

// Close closes the database connection.
func (w *WritableStore) Close() error {
	if w.db == nil {
		return nil
	}
	return w.db.Close()
}

// float32ToBytes encodes a float32 slice as a little-endian BLOB.
func float32ToBytes(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// vecNorm computes the L2 norm of a float32 vector.
func vecNorm(vec []float32) float64 {
	sum := float64(0)
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum)
}

// hashString returns a simple FNV-1a hash hex string for content deduplication.
func hashString(s string) string {
	const (
		offsetBasis uint64 = 14695981039346656037
		prime       uint64 = 1099511628211
	)
	h := offsetBasis
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	return strconv.FormatUint(h, 16)
}
