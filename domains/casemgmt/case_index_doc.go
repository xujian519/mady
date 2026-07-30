package casemgmt

import (
	"context"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver
)

// GetDocuments 返回案件的所有已记录文档。
func (ci *CaseIndex) GetDocuments(ctx context.Context, caseID string) ([]CaseDocument, error) {
	rows, err := ci.db.QueryContext(ctx, `
		SELECT case_id, doc_type, doc_path, doc_hash, parsed_at
		FROM case_documents WHERE case_id = ?
	`, caseID)
	if err != nil {
		return nil, fmt.Errorf("case_index: get docs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var docs []CaseDocument
	for rows.Next() {
		var d CaseDocument
		var parsedStr string
		if err := rows.Scan(&d.CaseID, &d.DocType, &d.DocPath, &d.DocHash, &parsedStr); err != nil {
			return nil, err
		}
		d.ParsedAt, _ = time.Parse(time.RFC3339, parsedStr)
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// --- 文档管理 ---

// RecordDocument 记录或更新已解析的权威文档。
func (ci *CaseIndex) RecordDocument(ctx context.Context, doc CaseDocument) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	if doc.ParsedAt.IsZero() {
		doc.ParsedAt = time.Now()
	}

	_, err := ci.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO case_documents (case_id, doc_type, doc_path, doc_hash, parsed_at)
		VALUES (?, ?, ?, ?, ?)
	`, doc.CaseID, doc.DocType, doc.DocPath, doc.DocHash, doc.ParsedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("case_index: record doc: %w", err)
	}
	return nil
}

// GetDocument 获取特定类型的文档。返回 sql.ErrNoRows 如果不存在。
func (ci *CaseIndex) GetDocument(ctx context.Context, caseID, docType string) (*CaseDocument, error) {
	var d CaseDocument
	var parsedStr string
	err := ci.db.QueryRowContext(ctx, `
		SELECT case_id, doc_type, doc_path, doc_hash, parsed_at
		FROM case_documents WHERE case_id = ? AND doc_type = ?
	`, caseID, docType).Scan(&d.CaseID, &d.DocType, &d.DocPath, &d.DocHash, &parsedStr)
	if err != nil {
		return nil, err
	}
	d.ParsedAt, _ = time.Parse(time.RFC3339, parsedStr)
	return &d, nil
}
