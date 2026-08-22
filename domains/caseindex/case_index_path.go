package caseindex

import (
	"context"
	"fmt"
	"time"
)

// --- 路径管理 ---

// AddPath 为案件添加一个关联路径。
func (ci *CaseIndex) AddPath(ctx context.Context, caseID, path, label string) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	_, err := ci.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO case_paths (case_id, path, label) VALUES (?, ?, ?)
	`, caseID, path, label)
	if err != nil {
		return fmt.Errorf("case_index: add path: %w", err)
	}
	return nil
}

// GetPaths 返回案件的所有关联路径。
func (ci *CaseIndex) GetPaths(ctx context.Context, caseID string) ([]CasePath, error) {
	rows, err := ci.db.QueryContext(ctx, `
		SELECT case_id, path, label, created_at FROM case_paths WHERE case_id = ?
	`, caseID)
	if err != nil {
		return nil, fmt.Errorf("case_index: get paths: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var paths []CasePath
	for rows.Next() {
		var p CasePath
		var createdStr string
		if err := rows.Scan(&p.CaseID, &p.Path, &p.Label, &createdStr); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// FindByPath 按路径查找案件。匹配规则：case_paths.path == absPath 或 case_paths.path 是 absPath 的父目录。
func (ci *CaseIndex) FindByPath(ctx context.Context, absPath string) ([]CaseRecord, error) {
	rows, err := ci.db.QueryContext(ctx, `
		SELECT DISTINCT c.case_id, c.identity_stage, c.filing_number, c.publication_number,
			c.client_name, c.patent_title, c.patent_type, c.year, c.domain, c.status,
			c.primary_path, c.created_at, c.updated_at
		FROM cases c
		JOIN case_paths cp ON c.case_id = cp.case_id
		WHERE cp.path = ? OR ? LIKE cp.path || '/%'
	`, absPath, absPath)
	if err != nil {
		return nil, fmt.Errorf("case_index: find by path: %w", err)
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
