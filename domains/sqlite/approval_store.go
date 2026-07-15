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
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("approval/sqlite: open %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(4)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("approval/sqlite: ping %s: %w", dbPath, err)
	}

	s := &SQLiteApprovalStore{db: db}
	if err := s.initSchema(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("approval/sqlite: init schema: %w", err)
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
	`)
	return err
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
	defer rows.Close()

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
	defer rows.Close()

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
	})
}

func unmarshalRecord(data []byte) (domains.ApprovalRecord, error) {
	var j approvalRecordJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return domains.ApprovalRecord{}, err
	}
	return domains.ApprovalRecord{
		ID:             j.ID,
		SessionID:      j.SessionID,
		CaseID:         j.CaseID,
		Timestamp:      j.Timestamp,
		TriggerKeyword: j.TriggerKeyword,
		OriginalOutput: j.OriginalOutput,
		Decision:       domains.ApprovalDecision(j.Decision),
		ModifiedOutput: j.ModifiedOutput,
		Feedback:       j.Feedback,
	}, nil
}

// --- store.CaseStore ---

var (
	_ store.CaseStore = (*SQLiteApprovalStore)(nil)
	_ store.Closer    = (*SQLiteApprovalStore)(nil)
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
