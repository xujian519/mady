package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
)

// PatentWikiCard is a single wiki-card result returned by SearchPatentWikiCards.
type PatentWikiCard struct {
	ID     string
	Title  string
	Domain string
	Body   string
	Score  float64
}

// SearchPatentWikiCards searches the XiaoNuo wiki cards stored in knowledge.db.
// The dir filter accepts Sati-style keys: specification, claims, drafting, figures.
// When query is empty, it lists the most recent cards matching the dir filter.
func (s *SQLiteStore) SearchPatentWikiCards(query, dir string, limit int, includeBody bool) ([]PatentWikiCard, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	pathFilter := ""
	if dir != "" {
		switch dir {
		case "specification":
			pathFilter = "说明书"
		case "claims":
			pathFilter = "权利要求"
		case "drafting":
			pathFilter = "撰写"
		case "figures":
			pathFilter = "附图"
		}
	}

	var rows *sql.Rows
	var err error
	if query != "" {
		ftsQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
		sqlQuery := `
			SELECT c.id, c.document_id, c.content, d.title, d.domain,
			       bm25(docs_fts) AS score
			FROM docs_fts
			JOIN chunks c ON c.id = docs_fts.rowid
			JOIN documents d ON d.id = c.document_id
			WHERE docs_fts MATCH ? AND d.source = 'wiki'
		`
		args := []any{ftsQuery}
		if pathFilter != "" {
			sqlQuery += ` AND (d.title LIKE ? OR d.id LIKE ?)`
			args = append(args, "%"+pathFilter+"%", "%"+pathFilter+"%")
		}
		sqlQuery += ` ORDER BY score LIMIT ?`
		args = append(args, limit*3)
		rows, err = s.db.QueryContext(s.baseCtx, sqlQuery, args...)
	} else {
		sqlQuery := `
			SELECT c.id, c.document_id, c.content, d.title, d.domain, 0.0 AS score
			FROM chunks c
			JOIN documents d ON d.id = c.document_id
			WHERE d.source = 'wiki' AND c.chunk_index = 0
		`
		args := []any{}
		if pathFilter != "" {
			sqlQuery += ` AND (d.title LIKE ? OR d.id LIKE ?)`
			args = append(args, "%"+pathFilter+"%", "%"+pathFilter+"%")
		}
		sqlQuery += ` ORDER BY d.indexed_at DESC LIMIT ?`
		args = append(args, limit*3)
		rows, err = s.db.QueryContext(s.baseCtx, sqlQuery, args...)
	}
	if err != nil {
		return nil, fmt.Errorf("wiki search query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	seen := make(map[string]bool)
	var results []PatentWikiCard
	for rows.Next() && len(results) < limit {
		var chunkID int
		var docID, content, title, domain string
		var score float64
		if err := rows.Scan(&chunkID, &docID, &content, &title, &domain, &score); err != nil {
			return nil, fmt.Errorf("wiki search scan: %w", err)
		}
		if seen[docID] {
			continue
		}
		seen[docID] = true
		r := PatentWikiCard{
			ID:     docID,
			Title:  title,
			Domain: domain,
			Score:  -score,
		}
		if includeBody {
			r.Body = content
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// PatentCaseDoc is a single case/judgment result returned by SearchPatentCases.
type PatentCaseDoc struct {
	DocumentID     string
	DocType        string
	Title          string
	DecisionNumber string
	CaseNumber     string
	Court          string
	Source         string
	CharCount      int
	Snippet        string
	Score          float64
}

// SearchPatentCases searches the XiaoNuo case/judgment documents in knowledge.db.
// It excludes wiki-derived cards so that patent_wiki_search owns the审查标准 path.
// docType may be "case" (invalidation/reexamination decisions) or "judgment".
func (s *SQLiteStore) SearchPatentCases(query, docType, court string, limit int, includeContent bool) ([]PatentCaseDoc, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	ftsQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	sqlQuery := `
		SELECT c.id, c.document_id, c.content, d.title, d.doc_type,
		       d.decision_number, d.case_number, d.court, d.source, d.char_count,
		       bm25(docs_fts) AS score
		FROM docs_fts
		JOIN chunks c ON c.id = docs_fts.rowid
		JOIN documents d ON d.id = c.document_id
		WHERE docs_fts MATCH ? AND d.source != 'wiki'
	`
	args := []any{ftsQuery}

	if docType != "" {
		sqlQuery += ` AND d.doc_type = ?`
		args = append(args, docType)
	} else {
		sqlQuery += ` AND d.doc_type IN ('case', 'judgment')`
	}
	if court != "" {
		sqlQuery += ` AND d.court LIKE ?`
		args = append(args, "%"+court+"%")
	}
	sqlQuery += ` ORDER BY score LIMIT ?`
	args = append(args, limit*3)

	rows, err := s.db.QueryContext(s.baseCtx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("case search query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	seen := make(map[string]bool)
	var results []PatentCaseDoc
	for rows.Next() && len(results) < limit {
		var chunkID int
		var docID, content, title, dt, decisionNumber, caseNumber, courtStr, source string
		var charCount int
		var score float64
		if err := rows.Scan(&chunkID, &docID, &content, &title, &dt, &decisionNumber, &caseNumber, &courtStr, &source, &charCount, &score); err != nil {
			return nil, fmt.Errorf("case search scan: %w", err)
		}
		if seen[docID] {
			continue
		}
		seen[docID] = true
		r := PatentCaseDoc{
			DocumentID:     docID,
			DocType:        dt,
			Title:          title,
			DecisionNumber: decisionNumber,
			CaseNumber:     caseNumber,
			Court:          courtStr,
			Source:         source,
			CharCount:      charCount,
			Score:          -score,
		}
		if includeContent {
			r.Snippet = content
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
