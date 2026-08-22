package caseindex

import (
	"context"
	"fmt"
	"time"
)

// --- 标识升级 ---

// UpgradeToFiled 将案件从 drafting 升级为 filed，写入申请号。
// 如果 filingNumber 与已有关联案件的申请号冲突则返回错误。
func (ci *CaseIndex) UpgradeToFiled(ctx context.Context, caseID, filingNumber string) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	// 检查申请号冲突
	var count int
	err := ci.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM cases WHERE filing_number = ? AND case_id != ?
	`, filingNumber, caseID).Scan(&count)
	if err != nil {
		return fmt.Errorf("case_index: upgrade check: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("申请号 %s 已被其他案件使用", filingNumber)
	}

	_, err = ci.db.ExecContext(ctx, `
		UPDATE cases SET identity_stage = ?, filing_number = ?, updated_at = ?
		WHERE case_id = ?
	`, StageFiled, filingNumber, time.Now().Format(time.RFC3339), caseID)
	if err != nil {
		return fmt.Errorf("case_index: upgrade to filed: %w", err)
	}

	// 同步 FTS（申请号已变更，需更新全文索引）
	var clientName, patentTitle, filingNum string
	_ = ci.db.QueryRowContext(ctx,
		`SELECT client_name, patent_title, filing_number FROM cases WHERE case_id = ?`, caseID,
	).Scan(&clientName, &patentTitle, &filingNum)
	ci.syncFTS(ctx, caseID, clientName, patentTitle, filingNum)

	// 记录事件
	_, _ = ci.db.ExecContext(ctx, `
		INSERT INTO case_events (case_id, event_type, event_data)
		VALUES (?, 'filed', ?)
	`, caseID, filingNumber)

	return nil
}

// UpgradeToPublished 将案件升级为 published，写入公开号。
func (ci *CaseIndex) UpgradeToPublished(ctx context.Context, caseID, publicationNumber string) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	_, err := ci.db.ExecContext(ctx, `
		UPDATE cases SET identity_stage = ?, publication_number = ?, updated_at = ?
		WHERE case_id = ?
	`, StagePublished, publicationNumber, time.Now().Format(time.RFC3339), caseID)
	if err != nil {
		return fmt.Errorf("case_index: upgrade to published: %w", err)
	}

	// 同步 FTS（保持一致性）
	var clientName, patentTitle, filingNum string
	_ = ci.db.QueryRowContext(ctx,
		`SELECT client_name, patent_title, filing_number FROM cases WHERE case_id = ?`, caseID,
	).Scan(&clientName, &patentTitle, &filingNum)
	ci.syncFTS(ctx, caseID, clientName, patentTitle, filingNum)

	_, _ = ci.db.ExecContext(ctx, `
		INSERT INTO case_events (case_id, event_type, event_data)
		VALUES (?, 'published', ?)
	`, caseID, publicationNumber)

	return nil
}
