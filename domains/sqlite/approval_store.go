// Package sqlite provides SQLite-backed persistence for approval records.
// It implements domains.ApprovalStore without polluting the domain package
// with infrastructure imports.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver

	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/store"
)

// SQLiteApprovalStore persists ApprovalRecords to a SQLite database.
// Each record is stored as a JSON blob with indexed metadata columns
// for efficient case-level queries.
type SQLiteApprovalStore struct {
	db *sql.DB
}

// NewApprovalStore opens or creates a SQLite approval database at the
// given path. The database is opened in WAL mode for safe concurrent reads
// during writes. If the file does not exist it is created with the full
// schema.
func NewApprovalStore(dbPath string) (*SQLiteApprovalStore, error) {
	db, err := openDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("approval/sqlite: %w", err)
	}

	s := &SQLiteApprovalStore{db: db}
	if err := s.initSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("approval/sqlite: init schema: %w", err)
	}
	if err := s.probeWritable(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("approval/sqlite: write probe: %w", err)
	}
	return s, nil
}

func (s *SQLiteApprovalStore) initSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS approval_records (
			id              TEXT PRIMARY KEY,
			session_id      TEXT NOT NULL DEFAULT '',
			case_id         TEXT NOT NULL DEFAULT '',
			trigger_keyword TEXT NOT NULL DEFAULT '',
			decision        TEXT NOT NULL DEFAULT '',
			data            TEXT NOT NULL,
			created_at      TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_approval_session ON approval_records(session_id);
		CREATE INDEX IF NOT EXISTS idx_approval_case   ON approval_records(case_id);

		CREATE TABLE IF NOT EXISTS pending_approvals (
			id              TEXT PRIMARY KEY,
			session_id      TEXT NOT NULL DEFAULT '',
			case_id         TEXT NOT NULL DEFAULT '',
			trigger_keyword TEXT NOT NULL DEFAULT '',
			original_output TEXT NOT NULL DEFAULT '',
			tool_calls_json TEXT NOT NULL DEFAULT '[]',
			status          TEXT NOT NULL DEFAULT 'pending',
			created_at      TEXT NOT NULL DEFAULT (datetime('now')),
			expires_at      TEXT,
			responded_at    TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_pending_session ON pending_approvals(session_id);
		CREATE INDEX IF NOT EXISTS idx_pending_status  ON pending_approvals(status);
	`)
	return err
}

func (s *SQLiteApprovalStore) probeWritable(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	probeID := fmt.Sprintf("__approval_probe_%d", time.Now().UnixNano())
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO approval_records (id, data) VALUES (?, ?)
	`, probeID, `{"probe":true}`); err != nil {
		return err
	}
	return nil
}

// Save persists an ApprovalRecord. If a record with the same ID exists,
// it is replaced.
func (s *SQLiteApprovalStore) Save(ctx context.Context, record domains.ApprovalRecord) error {
	data, err := marshalRecord(record)
	if err != nil {
		return fmt.Errorf("approval/sqlite: marshal: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO approval_records (id, session_id, case_id, trigger_keyword, decision, data)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		record.ID, record.SessionID, record.CaseID,
		record.TriggerKeyword, string(record.Decision), string(data),
	)
	if err != nil {
		return fmt.Errorf("approval/sqlite: save: %w", err)
	}
	return nil
}

// List returns all records for the given session, oldest first.
func (s *SQLiteApprovalStore) List(ctx context.Context, sessionID string) ([]domains.ApprovalRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM approval_records WHERE session_id = ? ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("approval/sqlite: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []domains.ApprovalRecord
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("approval/sqlite: scan: %w", err)
		}
		rec, err := unmarshalRecord([]byte(data))
		if err != nil {
			return nil, fmt.Errorf("approval/sqlite: unmarshal: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// ListByCase returns all records for the given case ID, oldest first.
func (s *SQLiteApprovalStore) ListByCase(ctx context.Context, caseID string) ([]domains.ApprovalRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM approval_records WHERE case_id = ? ORDER BY created_at ASC`,
		caseID,
	)
	if err != nil {
		return nil, fmt.Errorf("approval/sqlite: list by case: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []domains.ApprovalRecord
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("approval/sqlite: scan: %w", err)
		}
		rec, err := unmarshalRecord([]byte(data))
		if err != nil {
			return nil, fmt.Errorf("approval/sqlite: unmarshal: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// Delete removes an approval record by its ID.
func (s *SQLiteApprovalStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM approval_records WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("approval/sqlite: delete: %w", err)
	}
	return nil
}

// Close releases the database connection.
func (s *SQLiteApprovalStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// ---------------------------------------------------------------------------
// PendingStore implementation
// ---------------------------------------------------------------------------

// pendingRow 映射 pending_approvals 表的行。
type pendingRow struct {
	ID             string  `json:"id"`
	SessionID      string  `json:"session_id"`
	CaseID         string  `json:"case_id"`
	TriggerKeyword string  `json:"trigger_keyword"`
	OriginalOutput string  `json:"original_output"`
	ToolCallsJSON  string  `json:"tool_calls_json"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
	ExpiresAt      *string `json:"expires_at"`
	RespondedAt    *string `json:"responded_at"`
}

// SavePending persists a pending approval request to the database.
func (s *SQLiteApprovalStore) SavePending(ctx context.Context, p domains.PendingApproval) error {
	var expiresAt *string
	if p.ExpiresAt != nil {
		v := p.ExpiresAt.Format(time.RFC3339)
		expiresAt = &v
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO pending_approvals
			(id, session_id, case_id, trigger_keyword, original_output, tool_calls_json, status, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.SessionID, p.CaseID, p.TriggerKeyword, p.OriginalOutput,
		p.ToolCallsJSON, string(p.Status), p.CreatedAt.Format(time.RFC3339), expiresAt)
	if err != nil {
		return fmt.Errorf("pending/sqlite: save: %w", err)
	}
	return nil
}

// LoadPending retrieves a pending approval request by its ID.
func (s *SQLiteApprovalStore) LoadPending(ctx context.Context, id string) (*domains.PendingApproval, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, session_id, case_id, trigger_keyword, original_output,
		        tool_calls_json, status, created_at, expires_at, responded_at
		 FROM pending_approvals WHERE id = ?`, id)
	var pr pendingRow
	if err := row.Scan(&pr.ID, &pr.SessionID, &pr.CaseID, &pr.TriggerKeyword,
		&pr.OriginalOutput, &pr.ToolCallsJSON, &pr.Status, &pr.CreatedAt,
		&pr.ExpiresAt, &pr.RespondedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("pending/sqlite: load: %w", err)
	}
	return rowToPending(pr), nil
}

// ListPending returns all pending approval requests ordered by creation time.
func (s *SQLiteApprovalStore) ListPending(ctx context.Context) ([]domains.PendingApproval, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, case_id, trigger_keyword, original_output,
		        tool_calls_json, status, created_at, expires_at, responded_at
		 FROM pending_approvals WHERE status = 'pending' ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("pending/sqlite: list pending: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domains.PendingApproval
	for rows.Next() {
		var pr pendingRow
		if err := rows.Scan(&pr.ID, &pr.SessionID, &pr.CaseID, &pr.TriggerKeyword,
			&pr.OriginalOutput, &pr.ToolCallsJSON, &pr.Status, &pr.CreatedAt,
			&pr.ExpiresAt, &pr.RespondedAt); err != nil {
			return nil, fmt.Errorf("pending/sqlite: scan: %w", err)
		}
		out = append(out, *rowToPending(pr))
	}
	return out, rows.Err()
}

// ListPendingBySession returns pending approval requests for a specific session.
func (s *SQLiteApprovalStore) ListPendingBySession(ctx context.Context, sessionID string) ([]domains.PendingApproval, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, case_id, trigger_keyword, original_output,
		        tool_calls_json, status, created_at, expires_at, responded_at
		 FROM pending_approvals WHERE session_id = ? AND status = 'pending' ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("pending/sqlite: list by session: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domains.PendingApproval
	for rows.Next() {
		var pr pendingRow
		if err := rows.Scan(&pr.ID, &pr.SessionID, &pr.CaseID, &pr.TriggerKeyword,
			&pr.OriginalOutput, &pr.ToolCallsJSON, &pr.Status, &pr.CreatedAt,
			&pr.ExpiresAt, &pr.RespondedAt); err != nil {
			return nil, fmt.Errorf("pending/sqlite: scan: %w", err)
		}
		out = append(out, *rowToPending(pr))
	}
	return out, rows.Err()
}

// DeletePending removes a pending approval request by its ID.
func (s *SQLiteApprovalStore) DeletePending(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pending_approvals WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("pending/sqlite: delete: %w", err)
	}
	return nil
}

// Respond atomically marks a pending request as responded and inserts the
// approval record in a single transaction, preventing state splitting.
func (s *SQLiteApprovalStore) Respond(ctx context.Context, id string, record domains.ApprovalRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("pending/sqlite: respond begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1) Mark pending as responded.
	now := time.Now().Format(time.RFC3339)
	res, err := tx.ExecContext(ctx,
		`UPDATE pending_approvals SET status = 'responded', responded_at = ? WHERE id = ? AND status = 'pending'`,
		now, id)
	if err != nil {
		return fmt.Errorf("pending/sqlite: respond update: %w", err)
	}
	n, _ := res.RowsAffected()
	// n == 0 is not an error here: the pending may have been already responded
	// (duplicate /approve call). The approval record write below still proceeds.
	_ = n

	// 2) Insert approval record.
	data, err := marshalRecord(record)
	if err != nil {
		return fmt.Errorf("pending/sqlite: marshal: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO approval_records (id, session_id, case_id, trigger_keyword, decision, data)
		VALUES (?, ?, ?, ?, ?, ?)
	`, record.ID, record.SessionID, record.CaseID,
		record.TriggerKeyword, string(record.Decision), string(data))
	if err != nil {
		return fmt.Errorf("pending/sqlite: insert record: %w", err)
	}

	return tx.Commit()
}

// ExpirePending marks all expired pending requests as expired based on their expiry time.
func (s *SQLiteApprovalStore) ExpirePending(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE pending_approvals
		SET status = 'expired', responded_at = datetime('now')
		WHERE status = 'pending' AND expires_at IS NOT NULL AND expires_at < datetime('now')
	`)
	if err != nil {
		return 0, fmt.Errorf("pending/sqlite: expire: %w", err)
	}
	return res.RowsAffected()
}

// rowToPending converts a database row to a domain PendingApproval.
func rowToPending(pr pendingRow) *domains.PendingApproval {
	createdAt, _ := time.Parse(time.RFC3339, pr.CreatedAt)
	var expiresAt *time.Time
	if pr.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *pr.ExpiresAt)
		if err == nil {
			expiresAt = &t
		}
	}
	var respondedAt *time.Time
	if pr.RespondedAt != nil {
		t, err := time.Parse(time.RFC3339, *pr.RespondedAt)
		if err == nil {
			respondedAt = &t
		}
	}
	return &domains.PendingApproval{
		ID:             pr.ID,
		SessionID:      pr.SessionID,
		CaseID:         pr.CaseID,
		TriggerKeyword: pr.TriggerKeyword,
		OriginalOutput: pr.OriginalOutput,
		ToolCallsJSON:  pr.ToolCallsJSON,
		Status:         domains.PendingStatus(pr.Status),
		CreatedAt:      createdAt,
		ExpiresAt:      expiresAt,
		RespondedAt:    respondedAt,
	}
}

// --- JSON serialization ---

type approvalRecordJSON struct {
	ID             string    `json:"id"`
	SessionID      string    `json:"session_id"`
	CaseID         string    `json:"case_id"`
	Timestamp      time.Time `json:"timestamp"`
	TriggerKeyword string    `json:"trigger_keyword"`
	OriginalOutput string    `json:"original_output"`
	Decision       string    `json:"decision"`
	ModifiedOutput string    `json:"modified_output,omitempty"`
	Feedback       string    `json:"feedback,omitempty"`
	State          string    `json:"state,omitempty"`
}

func marshalRecord(r domains.ApprovalRecord) ([]byte, error) {
	return json.Marshal(approvalRecordJSON{
		ID:             r.ID,
		SessionID:      r.SessionID,
		CaseID:         r.CaseID,
		Timestamp:      r.Timestamp,
		TriggerKeyword: r.TriggerKeyword,
		OriginalOutput: r.OriginalOutput,
		Decision:       string(r.Decision),
		ModifiedOutput: r.ModifiedOutput,
		Feedback:       r.Feedback,
		State:          string(r.State),
	})
}

func unmarshalRecord(data []byte) (domains.ApprovalRecord, error) {
	var j approvalRecordJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return domains.ApprovalRecord{}, err
	}
	rec := domains.ApprovalRecord{
		ID:             j.ID,
		SessionID:      j.SessionID,
		CaseID:         j.CaseID,
		Timestamp:      j.Timestamp,
		TriggerKeyword: j.TriggerKeyword,
		OriginalOutput: j.OriginalOutput,
		Decision:       domains.ApprovalDecision(j.Decision),
		ModifiedOutput: j.ModifiedOutput,
		Feedback:       j.Feedback,
		State:          domains.ApprovalState(j.State),
	}
	// 兼容早期未持久化 State 的记录：由 Decision 推导重建，
	// 保证状态机在读取侧始终可正确还原。
	if rec.State == "" {
		rec.State = domains.DecisionToState(rec.Decision)
	}
	return rec, nil
}

// --- store.CaseStore ---

var (
	_ store.CaseStore      = (*SQLiteApprovalStore)(nil)
	_ store.Closer         = (*SQLiteApprovalStore)(nil)
	_ domains.PendingStore = (*SQLiteApprovalStore)(nil)
)

// CaseID returns "" since this store serves all cases.
func (s *SQLiteApprovalStore) CaseID() string { return "" }

// RunID returns "" since this store is not scoped to a single run.
func (s *SQLiteApprovalStore) RunID() string { return "" }

// Version returns the current schema version (1).
func (s *SQLiteApprovalStore) Version() int { return 1 }

// Migrate runs schema migrations. Currently at version 1 (initial schema).
func (s *SQLiteApprovalStore) Migrate(ctx context.Context) (int, error) {
	if err := s.initSchema(ctx); err != nil {
		return 0, fmt.Errorf("approval migrate: %w", err)
	}
	return s.Version(), nil
}
