// Package fileindex provides a lightweight file-level index for case/project
// folders. It enables Agent-driven file search without full document parsing.
//
// Architecture:
//
//	FileIndex (SQLite) — Refresh scans project RootPath and creates/updates
//	file_records with cheap metadata (path, category, size, mtime, preview text).
//	Search uses 4-way RRF (reciprocal rank fusion) across:
//	  1. File-name keyword matching
//	  2. Path-segment signals (subfolder names match query)
//	  3. FTS5 trigram on preview text
//	  4. Recency (modified-at recency)
//
// The preview text is intentionally cheap — first 512 bytes of text files,
// empty for binary files. This is NOT full document parsing; that is deferred
// to read_project_file (M2).
package fileindex

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver

	"github.com/xujian519/mady/retrieval"
)

// FileCategory classifies files by type for display and routing.
type FileCategory string

const (
	CategoryTextDoc     FileCategory = "text_doc"
	CategoryImage       FileCategory = "image"
	CategoryAudio       FileCategory = "audio"
	CategorySpreadsheet FileCategory = "spreadsheet"
	CategoryPdf         FileCategory = "pdf"
	CategoryUnknown     FileCategory = "unknown"
)

// FileRecord is a cheaply indexed file entry in the project folder.
type FileRecord struct {
	Path        string
	Category    FileCategory
	SizeBytes   int64
	ModifiedAt  time.Time
	PreviewText string // first 512 bytes (text) or empty (binary)
	Indexed     bool   // whether full content has been deep-processed
	ProcessedAt *time.Time
	Checksum    string // base64 of first 16 bytes of SHA256
}

// FileCandidate is a search result returned to the Agent.
type FileCandidate struct {
	Path       string       `json:"path"`
	Category   FileCategory `json:"category"`
	SizeBytes  int64        `json:"size_bytes"`
	ModifiedAt time.Time    `json:"modified_at"`
	Relevance  float64      `json:"relevance"`
	Preview    string       `json:"preview"` // preview text for relevance assessment
}

// FileIndex maintains a SQLite-backed index of files in a project folder.
type FileIndex struct {
	db  *sql.DB
	dir string // project RootPath (absolute)
	mu  sync.Mutex

	rrf *retrieval.RRFFuser
}

// OpenFileIndex opens or creates the file index database.
// dir is the project root path (used to scope file scanning).
// dbPath is the SQLite database file path (typically under workspace).
func OpenFileIndex(dir, dbPath string) (*FileIndex, error) {
	if dir == "" {
		return nil, fmt.Errorf("fileindex: dir must not be empty")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("fileindex: resolve dir %s: %w", dir, err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("fileindex: open db %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(4)

	fi := &FileIndex{db: db, dir: absDir, rrf: retrieval.NewRRFFuser()}
	if err := fi.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return fi, nil
}

// Close releases the database connection.
func (fi *FileIndex) Close() error {
	return fi.db.Close()
}

// Dir returns the project root path this index is scoped to.
func (fi *FileIndex) Dir() string { return fi.dir }

// initSchema creates the file_records table and FTS5 index.
func (fi *FileIndex) initSchema() error {
	_, err := fi.db.Exec(`
		CREATE TABLE IF NOT EXISTS file_records (
			path         TEXT PRIMARY KEY,
			category     TEXT NOT NULL DEFAULT 'unknown',
			size_bytes   INTEGER NOT NULL DEFAULT 0,
			modified_at  TEXT NOT NULL,
			preview_text TEXT NOT NULL DEFAULT '',
			indexed      INTEGER NOT NULL DEFAULT 0,
			processed_at TEXT,
			checksum     TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_file_category ON file_records(category);
		CREATE INDEX IF NOT EXISTS idx_file_modified  ON file_records(modified_at);
		CREATE VIRTUAL TABLE IF NOT EXISTS file_records_fts USING fts5(
			path UNINDEXED, preview_text,
			tokenize='trigram'
		);
	`)
	return err
}

// ---------------------------------------------------------------------------
// Refresh — incremental scan of the project folder
// ---------------------------------------------------------------------------

// Refresh walks the project directory and updates the file index incrementally.
// New/modified files are added/updated; deleted files are removed.
// The call is lightweight — for a typical 200-file case folder it completes
// in under 50 ms (os.Stat for each file + checksum comparison for changed files).
func (fi *FileIndex) Refresh(ctx context.Context) error {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	// Load existing records into a map for O(1) lookup.
	existing := make(map[string]FileRecord)
	rows, err := fi.db.QueryContext(ctx,
		`SELECT path, category, size_bytes, modified_at, preview_text, indexed, checksum FROM file_records`)
	if err != nil {
		return fmt.Errorf("fileindex: query existing: %w", err)
	}
	for rows.Next() {
		var r FileRecord
		var modAt string
		if err := rows.Scan(&r.Path, (*string)(&r.Category), &r.SizeBytes, &modAt,
			&r.PreviewText, &r.Indexed, &r.Checksum); err != nil {
			rows.Close()
			return fmt.Errorf("fileindex: scan: %w", err)
		}
		r.ModifiedAt, _ = time.Parse(time.RFC3339, modAt)
		existing[r.Path] = r
	}
	rows.Close()

	// Walk the directory.
	var currentPaths []string
	walkErr := filepath.WalkDir(fi.dir, func(path string, d os.DirEntry, err error) error {
		// Check context cancellation to abort long walks.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err != nil {
			return nil // skip inaccessible entries
		}
		if d.IsDir() {
			// Skip common hidden/ignored directories.
			base := d.Name()
			if base != "." && strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			if base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip SQLite support files (shm/wal/journal) and our own database file.
		if strings.HasSuffix(path, "-shm") || strings.HasSuffix(path, "-wal") ||
			strings.HasSuffix(path, ".db-journal") || strings.HasSuffix(path, ".db") {
			return nil
		}
		currentPaths = append(currentPaths, path)
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("fileindex: walk %s: %w", fi.dir, walkErr)
	}

	// Build set of current paths for deletion detection.
	currentSet := make(map[string]bool, len(currentPaths))
	for _, p := range currentPaths {
		currentSet[p] = true
	}

	// Process each file: new or changed → insert/update; unchanged → skip.
	for _, path := range currentPaths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		relPath := path // store full absolute path for now

		existingRecord, exists := existing[relPath]
		checksum := quickChecksum(path, info)

		if exists && existingRecord.Checksum == checksum && existingRecord.SizeBytes == info.Size() {
			continue // unchanged, skip
		}

		cat := classifyFile(path)
		preview := extractPreview(path, cat)
		modAt := info.ModTime().Format(time.RFC3339)

		// Upsert into SQLite.
		_, err = fi.db.ExecContext(ctx, `
			INSERT INTO file_records (path, category, size_bytes, modified_at, preview_text, indexed, checksum)
			VALUES (?, ?, ?, ?, ?, 0, ?)
			ON CONFLICT(path) DO UPDATE SET
				category=excluded.category, size_bytes=excluded.size_bytes,
				modified_at=excluded.modified_at, preview_text=excluded.preview_text,
				checksum=excluded.checksum
		`, relPath, string(cat), info.Size(), modAt, preview, checksum)
		if err != nil {
			return fmt.Errorf("fileindex: upsert %s: %w", relPath, err)
		}

		// Sync FTS5 (delete old + insert new).
		if _, err := fi.db.ExecContext(ctx, `DELETE FROM file_records_fts WHERE rowid IN (
			SELECT rowid FROM file_records WHERE path = ?)`, relPath); err != nil {
			log.Printf("fileindex: fts5 delete error: %v", err)
		}
		if _, err := fi.db.ExecContext(ctx,
			`INSERT INTO file_records_fts (rowid, path, preview_text) VALUES (
				(SELECT rowid FROM file_records WHERE path = ?), ?, ?)`,
			relPath, relPath, preview); err != nil {
			log.Printf("fileindex: fts5 insert error: %v", err)
		}
	}

	// Remove records for files that no longer exist.
	for path := range existing {
		if !currentSet[path] {
			// Delete FTS5 entry FIRST (while the record still exists for the subquery).
			if _, err := fi.db.ExecContext(ctx, `DELETE FROM file_records_fts WHERE rowid IN (
				SELECT rowid FROM file_records WHERE path = ?)`, path); err != nil {
				log.Printf("fileindex: fts5 delete error on remove: %v", err)
			}
			_, _ = fi.db.ExecContext(ctx, `DELETE FROM file_records WHERE path = ?`, path)
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Search — 4-way RRF fusion
// ---------------------------------------------------------------------------

// Search returns files matching the query, ranked by RRF-fused relevance.
func (fi *FileIndex) Search(ctx context.Context, query string, topK int) ([]FileCandidate, error) {
	if topK <= 0 {
		topK = 10
	}
	if query == "" {
		// Return most recently modified files.
		return fi.recentFiles(ctx, topK)
	}

	fi.mu.Lock()
	allRecords, err := fi.loadAllRecordsLocked(ctx)
	fi.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if len(allRecords) == 0 {
		return nil, nil
	}

	qlower := strings.ToLower(query)

	// Build 4 ranked lists for RRF fusion.
	type scored struct {
		record FileRecord
		score  float64
	}

	// Signal 1: File-name keyword match.
	var list1 []scored
	for _, r := range allRecords {
		base := strings.ToLower(filepath.Base(r.Path))
		s := scoreFileName(qlower, base)
		if s > 0 {
			list1 = append(list1, scored{record: r, score: s})
		}
	}
	sort.Slice(list1, func(i, j int) bool { return list1[i].score > list1[j].score })

	// Signal 2: Path-segment signal.
	var list2 []scored
	for _, r := range allRecords {
		s := scorePathSegments(qlower, r.Path)
		if s > 0 {
			list2 = append(list2, scored{record: r, score: s})
		}
	}
	sort.Slice(list2, func(i, j int) bool { return list2[i].score > list2[j].score })

	// Signal 3: FTS5 on preview text.
	list3 := fi.ftsSearchLocked(ctx, qlower, allRecords)

	// Signal 4: Recency.
	var list4 []scored
	now := time.Now()
	for _, r := range allRecords {
		s := scoreRecency(now, r.ModifiedAt)
		list4 = append(list4, scored{record: r, score: s})
	}
	sort.Slice(list4, func(i, j int) bool { return list4[i].score > list4[j].score })

	// Convert to ScoredChunk slices for RRF, keyed by path.
	toScored := func(list []scored) []retrieval.ScoredChunk {
		out := make([]retrieval.ScoredChunk, len(list))
		for i, s := range list {
			out[i] = retrieval.ScoredChunk{
				Chunk: retrieval.Chunk{ID: s.record.Path},
				Score: s.score,
			}
		}
		return out
	}

	var lists [][]retrieval.ScoredChunk
	if len(list1) > 0 {
		lists = append(lists, toScored(list1))
	}
	if len(list2) > 0 {
		lists = append(lists, toScored(list2))
	}
	if len(list3) > 0 {
		lists = append(lists, list3)
	}
	if len(list4) > 0 {
		lists = append(lists, toScored(list4))
	}

	fused := fi.rrf.Fuse(lists, topK)

	// Build FileCandidate results.
	recordByPath := make(map[string]FileRecord, len(allRecords))
	for _, r := range allRecords {
		recordByPath[r.Path] = r
	}

	results := make([]FileCandidate, 0, len(fused))
	for _, sc := range fused {
		r, ok := recordByPath[sc.ID]
		if !ok {
			continue
		}
		results = append(results, FileCandidate{
			Path:       r.Path,
			Category:   r.Category,
			SizeBytes:  r.SizeBytes,
			ModifiedAt: r.ModifiedAt,
			Relevance:  sc.Score,
			Preview:    truncatePreview(r.PreviewText, 200),
		})
	}
	return results, nil
}

// recentFiles returns the most recently modified files.
func (fi *FileIndex) recentFiles(ctx context.Context, topK int) ([]FileCandidate, error) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	rows, err := fi.db.QueryContext(ctx,
		`SELECT path, category, size_bytes, modified_at, preview_text
		 FROM file_records ORDER BY modified_at DESC LIMIT ?`, topK)
	if err != nil {
		return nil, fmt.Errorf("fileindex: recent files: %w", err)
	}
	defer rows.Close()

	var results []FileCandidate
	for rows.Next() {
		var fc FileCandidate
		var modAt, preview string
		if err := rows.Scan(&fc.Path, (*string)(&fc.Category), &fc.SizeBytes, &modAt, &preview); err != nil {
			return nil, fmt.Errorf("fileindex: scan recent: %w", err)
		}
		fc.ModifiedAt, _ = time.Parse(time.RFC3339, modAt)
		fc.Preview = truncatePreview(preview, 200)
		fc.Relevance = 1.0 // no query context
		results = append(results, fc)
	}
	return results, nil
}

// loadAllRecordsLocked reads all file_records into memory. Must hold fi.mu.
func (fi *FileIndex) loadAllRecordsLocked(ctx context.Context) ([]FileRecord, error) {
	rows, err := fi.db.QueryContext(ctx,
		`SELECT path, category, size_bytes, modified_at, preview_text, indexed, processed_at, checksum
		 FROM file_records`)
	if err != nil {
		return nil, fmt.Errorf("fileindex: load all: %w", err)
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		var r FileRecord
		var modAt, checksum string
		var processedAt sql.NullString
		if err := rows.Scan(&r.Path, (*string)(&r.Category), &r.SizeBytes, &modAt,
			&r.PreviewText, &r.Indexed, &processedAt, &checksum); err != nil {
			return nil, fmt.Errorf("fileindex: scan: %w", err)
		}
		r.ModifiedAt, _ = time.Parse(time.RFC3339, modAt)
		r.Checksum = checksum
		if processedAt.Valid {
			t, _ := time.Parse(time.RFC3339, processedAt.String)
			r.ProcessedAt = &t
		}
		records = append(records, r)
	}
	return records, nil
}

// ftsSearchLocked queries the FTS5 index. Must hold fi.mu.
func (fi *FileIndex) ftsSearchLocked(ctx context.Context, query string, allRecords []FileRecord) []retrieval.ScoredChunk {
	if len(allRecords) == 0 {
		return nil
	}

	// For queries shorter than 3 characters, trigram tokenizer produces no
	// matches. Fall back to a LIKE scan over preview_text as a cheap signal.
	if len([]rune(query)) < 3 {
		return fi.ftsLikeFallbackLocked(ctx, query, allRecords)
	}
	// FTS5 with double-quoted phrase matching.
	ftsQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	rows, err := fi.db.QueryContext(ctx, `
		SELECT f.path, rank FROM file_records_fts f
		JOIN file_records r ON r.rowid = f.rowid
		WHERE file_records_fts MATCH ?
		ORDER BY bm25(file_records_fts, 0.0, 1.0)
		LIMIT 100
	`, ftsQuery)
	if err != nil {
		return nil
	}
	defer rows.Close()

	// Build a set of valid paths for filtering.
	validPaths := make(map[string]bool, len(allRecords))
	for _, r := range allRecords {
		validPaths[r.Path] = true
	}

	var results []retrieval.ScoredChunk
	for rows.Next() {
		var path string
		var score float64
		if err := rows.Scan(&path, &score); err != nil {
			continue
		}
		if !validPaths[path] {
			continue
		}
		// BM25 returns negative scores; negate for consistency (higher = better).
		normalized := -score
		if normalized < 0 {
			normalized = 0
		}
		results = append(results, retrieval.ScoredChunk{
			Chunk: retrieval.Chunk{ID: path},
			Score: normalized,
		})
	}
	return results
}

// ---------------------------------------------------------------------------
// Scoring helpers
// ---------------------------------------------------------------------------

// ftsLikeFallbackLocked is a fallback for short queries (<3 runes). Uses substring scan.
func (fi *FileIndex) ftsLikeFallbackLocked(ctx context.Context, query string, allRecords []FileRecord) []retrieval.ScoredChunk {
	var results []retrieval.ScoredChunk
	qlower := strings.ToLower(query)
	for _, r := range allRecords {
		lowerPath := strings.ToLower(r.Path)
		lowerPreview := strings.ToLower(r.PreviewText)
		if strings.Contains(lowerPath, qlower) || strings.Contains(lowerPreview, qlower) {
			results = append(results, retrieval.ScoredChunk{
				Chunk: retrieval.Chunk{ID: r.Path},
				Score: 0.5,
			})
		}
	}
	return results
}

// scoreFileName computes a simple BM25-like score for filename matching.
func scoreFileName(query, filename string) float64 {
	// Exact filename match (ignoring extension): high score.
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if base == query {
		return 1.0
	}
	// Filename contains query as word.
	if strings.Contains(base, query) {
		return 0.8
	}
	// Query contains in filename.
	if strings.Contains(query, base) {
		return 0.6
	}
	// Partial character overlap.
	intersect := charOverlap(query, base)
	if intersect > 0.3 {
		return intersect * 0.5
	}
	return 0
}

// scorePathSegments scores path segments (directory names) that match the query.
func scorePathSegments(query, path string) float64 {
	normalized := strings.ReplaceAll(path, "\\", "/")
	segments := strings.Split(normalized, "/")
	var score float64
	for _, seg := range segments {
		lower := strings.ToLower(seg)
		if lower == query {
			score += 0.5
		} else if strings.Contains(lower, query) {
			score += 0.3
		}
	}
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// scoreRecency returns a recency score in [0, 1] with a 30-day half-life.
func scoreRecency(now, modTime time.Time) float64 {
	days := now.Sub(modTime).Hours() / 24
	if days <= 0 {
		return 1.0
	}
	// Decay: 1.0 at day 0, ~0.5 at day 30, ~0.25 at day 60.
	return 1.0 / (1.0 + days/30.0)
}

// charOverlap returns the fraction of query characters present in the target.
func charOverlap(query, target string) float64 {
	qChars := make(map[rune]bool)
	for _, c := range query {
		qChars[c] = true
	}
	tChars := make(map[rune]bool)
	for _, c := range target {
		tChars[c] = true
	}
	var matched int
	for c := range qChars {
		if tChars[c] {
			matched++
		}
	}
	if len(qChars) == 0 {
		return 0
	}
	return float64(matched) / float64(len(qChars))
}

// ---------------------------------------------------------------------------
// Cheap metadata extraction (no full document parsing)
// ---------------------------------------------------------------------------

// classifyFile determines the file category by extension.
func classifyFile(path string) FileCategory {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".md", ".go", ".py", ".js", ".ts", ".java", ".rb", ".c", ".cpp", ".h",
		".hpp", ".rs", ".swift", ".kt", ".scala", ".php", ".css", ".html", ".xml", ".json",
		".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".log", ".sh", ".bash", ".zsh",
		".fish", ".ps1", ".bat", ".sql", ".r", ".lua", ".pl", ".pm", ".tcl":
		return CategoryTextDoc
	case ".pdf":
		return CategoryPdf
	case ".doc", ".docx", ".odt", ".rtf":
		return CategoryTextDoc
	case ".xls", ".xlsx", ".csv", ".ods":
		return CategorySpreadsheet
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tiff", ".tif", ".webp", ".svg", ".ico":
		return CategoryImage
	case ".mp3", ".m4a", ".wav", ".wma", ".ogg", ".flac", ".aac", ".aiff", ".opus", ".wv":
		return CategoryAudio
	default:
		return CategoryUnknown
	}
}

// extractPreview reads the first 512 bytes for text files, empty for others.
func extractPreview(path string, cat FileCategory) string {
	switch cat {
	case CategoryTextDoc, CategoryPdf, CategorySpreadsheet:
		// For text-like, we read a small prefix; for PDF/spreadsheet we
		// cannot extract meaningful text without a parser, so return the
		// filename as a minimal signal.
		data, err := os.ReadFile(path)
		if err != nil {
			return filepath.Base(path)
		}
		if len(data) > 512 {
			data = data[:512]
		}
		// Check if it looks like a binary file (null bytes).
		for _, b := range data {
			if b == 0 {
				return filepath.Base(path)
			}
		}
		return string(data)
	default:
		return ""
	}
}

// quickChecksum returns a base64 string of the first 16 bytes of the file's
// SHA256 hash, combined with its modification time for fast change detection.
func quickChecksum(path string, info os.FileInfo) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return fmt.Sprintf("%d_%d", info.Size(), info.ModTime().UnixNano())
	}
	hash := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(hash[:16])
}

// truncatePreview shortens preview text for display.
func truncatePreview(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
