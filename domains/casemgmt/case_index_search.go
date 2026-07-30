package casemgmt

import (
	"context"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // register sqlite driver
)

func (ci *CaseIndex) searchByFTS(ctx context.Context, text string) ([]CaseRecord, error) {
	rows, err := ci.db.QueryContext(ctx, `
		SELECT c.case_id, c.identity_stage, c.filing_number, c.publication_number,
			c.client_name, c.patent_title, c.patent_type, c.year, c.domain, c.status,
			c.primary_path, c.created_at, c.updated_at
		FROM cases_fts fts
		JOIN cases c ON c.case_id = fts.case_id
		WHERE cases_fts MATCH ?
		ORDER BY rank
	`, text)
	if err != nil {
		return nil, fmt.Errorf("case_index: fts search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var cases []CaseRecord
	for rows.Next() {
		rec, err := scanCase(rows.Scan)
		if err != nil {
			return nil, err
		}
		cases = append(cases, *rec)
	}
	return cases, rows.Err()
}

// SearchCases 按条件检索案件。
func (ci *CaseIndex) SearchCases(ctx context.Context, q CaseSearchQuery) ([]CaseRecord, error) {
	// 全文搜索走 FTS5
	if q.Text != "" {
		return ci.searchByFTS(ctx, q.Text)
	}

	// 结构化条件查询
	var conditions []string
	var args []any
	if q.FilingNumber != "" {
		conditions = append(conditions, "filing_number = ?")
		args = append(args, q.FilingNumber)
	}
	if q.ClientName != "" {
		conditions = append(conditions, "client_name LIKE ?")
		args = append(args, "%"+q.ClientName+"%")
	}
	if q.PatentTitle != "" {
		conditions = append(conditions, "patent_title LIKE ?")
		args = append(args, "%"+q.PatentTitle+"%")
	}
	if q.PatentType != "" {
		conditions = append(conditions, "patent_type = ?")
		args = append(args, q.PatentType)
	}
	if q.Year > 0 {
		conditions = append(conditions, "year = ?")
		args = append(args, q.Year)
	}
	if q.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, q.Status)
	}
	if q.IdentityStage != "" {
		conditions = append(conditions, "identity_stage = ?")
		args = append(args, q.IdentityStage)
	}

	query := `
		SELECT case_id, identity_stage, filing_number, publication_number,
			client_name, patent_title, patent_type, year, domain, status, primary_path,
			created_at, updated_at
		FROM cases`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ") //nolint:gosec // conditions are static strings, args are parameterized
	}
	query += " ORDER BY updated_at DESC"

	rows, err := ci.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("case_index: search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var cases []CaseRecord
	for rows.Next() {
		rec, err := scanCase(rows.Scan)
		if err != nil {
			return nil, err
		}
		cases = append(cases, *rec)
	}
	return cases, rows.Err()
}
