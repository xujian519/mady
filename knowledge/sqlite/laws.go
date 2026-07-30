package sqlite

import (
	"log/slog"

	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver
)

// searchLawsLike falls back to LIKE pattern matching across name and
// content columns. Results are ordered by the law.order field.
func (s *SQLiteStore) searchLawsLike(keyword string, topK int) ([]LawRecord, error) {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(keyword)
	pattern := "%" + escaped + "%"
	rows, err := s.lawsDB.QueryContext(s.baseCtx, `
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
	defer func() { _ = rows.Close() }()

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

// SearchLaws searches for laws matching the given keyword. It uses
// BM25-ranked FTS5 search for better relevance. Otherwise it falls
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

// OpenLawsDB opens laws-full.db for law full-text search.
func (s *SQLiteStore) OpenLawsDB(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", path)
	lawsDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open laws-full.db: %w", err)
	}
	lawsDB.SetMaxOpenConns(2)
	// 关闭旧连接（防止重复调用 OpenLawsDB 泄漏句柄）。
	if s.lawsDB != nil {
		if cerr := s.lawsDB.Close(); cerr != nil {
			slog.Warn("knowledge/sqlite: close previous lawsDB", "err", cerr)
		}
	}
	s.lawsDB = lawsDB

	// Detect whether the law_fts (FTS5) table exists.
	row := lawsDB.QueryRowContext(s.baseCtx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='law_fts'")
	var ftsCount int
	if err := row.Scan(&ftsCount); err == nil && ftsCount > 0 {
		s.hasLawFTS = true
	}
	return nil
}

// HasLawFTS returns true when the open laws database includes a law_fts
// FTS5 virtual table for fast BM25-ranked search.
func (s *SQLiteStore) HasLawFTS() bool { return s.hasLawFTS }

// searchLawsFTS uses the law_fts FTS5 virtual table with BM25 ranking.
func (s *SQLiteStore) searchLawsFTS(keyword string, topK int) ([]LawRecord, error) {
	// Wrap query in double quotes for FTS5 phrase matching. The trigram
	// tokenizer handles CJK text by splitting into 3-character n-grams.
	ftsQuery := `"` + strings.ReplaceAll(keyword, `"`, `""`) + `"`
	rows, err := s.lawsDB.QueryContext(s.baseCtx, `
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
	defer func() { _ = rows.Close() }()

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
