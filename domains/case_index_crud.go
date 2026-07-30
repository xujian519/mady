package domains

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver
)

// ListAll 返回所有案件，按更新时间倒序。
func (ci *CaseIndex) ListAll(ctx context.Context) ([]CaseRecord, error) {
	return ci.SearchCases(ctx, CaseSearchQuery{})
}

// GetCase 按 CaseID 查询案件。
func (ci *CaseIndex) GetCase(ctx context.Context, caseID string) (*CaseRecord, error) {
	row := ci.db.QueryRowContext(ctx, `
		SELECT case_id, identity_stage, filing_number, publication_number,
			client_name, patent_title, patent_type, year, domain, status, primary_path,
			created_at, updated_at
		FROM cases WHERE case_id = ?
	`, caseID)

	rec, err := scanCase(row.Scan)
	if err != nil {
		return nil, fmt.Errorf("case_index: get %s: %w", caseID, err)
	}
	return rec, nil
}

// --- Case CRUD ---

// CreateCase 创建一条新案件记录。
func (ci *CaseIndex) CreateCase(ctx context.Context, rec CaseRecord) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	rec.UpdatedAt = rec.CreatedAt

	_, err := ci.db.ExecContext(ctx, `
		INSERT INTO cases (case_id, identity_stage, filing_number, publication_number,
			client_name, patent_title, patent_type, year, domain, status, primary_path,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rec.CaseID, rec.IdentityStage, rec.FilingNumber, rec.PublicationNumber,
		rec.ClientName, rec.PatentTitle, rec.PatentType, rec.Year, rec.Domain,
		rec.Status, rec.PrimaryPath, rec.CreatedAt.Format(time.RFC3339), rec.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("case_index: create: %w", err)
	}

	// 写入 FTS 索引
	ci.syncFTS(ctx, rec.CaseID, rec.ClientName, rec.PatentTitle, rec.FilingNumber)

	// 写入 primary_path 到 case_paths
	if rec.PrimaryPath != "" {
		_, _ = ci.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO case_paths (case_id, path) VALUES (?, ?)
		`, rec.CaseID, rec.PrimaryPath)
	}

	// 记录事件
	_, _ = ci.db.ExecContext(ctx, `
		INSERT INTO case_events (case_id, event_type) VALUES (?, 'created')
	`, rec.CaseID)

	return nil
}

// FindByDraftingIdentity 按撰写期复合标识查找案件。
// 用于去重：同一客户+名称+类型+年份应只建一个案件。
func (ci *CaseIndex) FindByDraftingIdentity(ctx context.Context, clientName, patentTitle, patentType string, year int) ([]CaseRecord, error) {
	return ci.SearchCases(ctx, CaseSearchQuery{
		ClientName:  clientName,
		PatentTitle: patentTitle,
		PatentType:  patentType,
		Year:        year,
	})
}

// DeleteCase 删除案件及其所有关联数据。
func (ci *CaseIndex) DeleteCase(ctx context.Context, caseID string) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	tx, err := ci.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("case_index: delete begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, q := range []string{
		`DELETE FROM cases WHERE case_id = ?`,
		`DELETE FROM case_paths WHERE case_id = ?`,
		`DELETE FROM case_documents WHERE case_id = ?`,
		`DELETE FROM case_events WHERE case_id = ?`,
		`DELETE FROM cases_fts WHERE case_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, caseID); err != nil {
			return fmt.Errorf("case_index: delete: %w", err)
		}
	}
	return tx.Commit()
}

// FindByFilingNumber 按申请号精确查找。
func (ci *CaseIndex) FindByFilingNumber(ctx context.Context, filingNumber string) (*CaseRecord, error) {
	cases, err := ci.SearchCases(ctx, CaseSearchQuery{FilingNumber: filingNumber})
	if err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, sql.ErrNoRows
	}
	return &cases[0], nil
}

// UpdateCase 更新案件记录。所有字段以 rec 为准。
func (ci *CaseIndex) UpdateCase(ctx context.Context, rec CaseRecord) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	rec.UpdatedAt = time.Now()

	_, err := ci.db.ExecContext(ctx, `
		UPDATE cases SET
			identity_stage = ?, filing_number = ?, publication_number = ?,
			client_name = ?, patent_title = ?, patent_type = ?, year = ?,
			domain = ?, status = ?, primary_path = ?, updated_at = ?
		WHERE case_id = ?
	`,
		rec.IdentityStage, rec.FilingNumber, rec.PublicationNumber,
		rec.ClientName, rec.PatentTitle, rec.PatentType, rec.Year,
		rec.Domain, rec.Status, rec.PrimaryPath, rec.UpdatedAt.Format(time.RFC3339),
		rec.CaseID,
	)
	if err != nil {
		return fmt.Errorf("case_index: update %s: %w", rec.CaseID, err)
	}

	// 同步 FTS 索引
	ci.syncFTS(ctx, rec.CaseID, rec.ClientName, rec.PatentTitle, rec.FilingNumber)

	return nil
}
