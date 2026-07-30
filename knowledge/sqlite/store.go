package sqlite

import (
	"context"

	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/xujian519/mady/pkg/vecbytes"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver
)

// SQLiteStore provides read-only access to the XiaoNuo knowledge databases
// (knowledge.db, laws-full.db, patent_kg.db). It exposes FTS5 full-text
// search, vector similarity search, and knowledge-graph loading — all backed
// by pre-built SQLite databases that share the same data model as Mady's
// in-memory Store and GraphStore.
type SQLiteStore struct {
	mu        sync.Mutex
	db        *sql.DB      // knowledge.db — documents, chunks, FTS, embeddings, KG
	lawsDB    *sql.DB      // laws-full.db — 9 121 laws with full text
	kgDB      *sql.DB      // patent_kg.db — 116 K nodes / 484 K edges
	dim       int          // embedding dimension (default 1024 for BGE-M3)
	vecIndex  *VectorIndex // pre-loaded in-memory vector index (nil until PreloadVectors)
	hasLawFTS bool         // true when laws-full-local.db has FTS5 index (law_fts table)
	baseCtx   context.Context
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

	baseCtx := context.Background()
	if err := db.PingContext(baseCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping knowledge.db: %w", err)
	}

	// Detect embedding dimension from stored vectors.
	dim := 1024
	var vecLen int
	row := db.QueryRowContext(baseCtx, "SELECT length(vector) FROM embeddings LIMIT 1")
	if err := row.Scan(&vecLen); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("detect embedding dimension: %w", err)
	}
	if vecLen > 0 {
		dim = vecLen / 4 // float32 = 4 bytes
	}

	return &SQLiteStore{db: db, dim: dim, baseCtx: baseCtx}, nil
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
	err := s.db.QueryRowContext(s.baseCtx, "SELECT vector FROM embeddings LIMIT 1").Scan(&blob)
	if err != nil || len(blob) == 0 {
		return nil
	}
	return vecbytes.BytesToFloats(blob)
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
	_ = s.db.QueryRowContext(s.baseCtx, "SELECT COUNT(*) FROM documents").Scan(&st.Documents)
	_ = s.db.QueryRowContext(s.baseCtx, "SELECT COUNT(*) FROM chunks").Scan(&st.Chunks)
	_ = s.db.QueryRowContext(s.baseCtx, "SELECT COUNT(*) FROM embeddings").Scan(&st.Embeddings)
	var blobLen int
	_ = s.db.QueryRowContext(s.baseCtx, "SELECT LENGTH(vector) FROM embeddings LIMIT 1").Scan(&blobLen)
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
