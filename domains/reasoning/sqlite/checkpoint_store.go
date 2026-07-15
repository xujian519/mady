// Package sqlite provides SQLite-backed persistence for reasoning workflow
// checkpoints. It implements reasoning.CheckpointStore without polluting the
// domain package with infrastructure imports.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver

	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/store"
)

var (
	_ reasoning.CheckpointStore = (*SQLiteCheckpointStore)(nil)
	_ store.CaseStore           = (*SQLiteCheckpointStore)(nil)
	_ store.Closer              = (*SQLiteCheckpointStore)(nil)
)

// Each checkpoint is stored as a JSON blob with indexed metadata columns
// for efficient case-level queries.
type SQLiteCheckpointStore struct {
	db *sql.DB
}

// NewCheckpointStore opens or creates a SQLite checkpoint database at the
// given path. The database is opened in WAL mode for safe concurrent reads
// during writes. If the file does not exist it is created with the full
// schema.
func NewCheckpointStore(dbPath string) (*SQLiteCheckpointStore, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("reasoning/sqlite: open %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(4)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("reasoning/sqlite: ping %s: %w", dbPath, err)
	}

	s := &SQLiteCheckpointStore{db: db}
	if err := s.initSchema(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("reasoning/sqlite: init schema: %w", err)
	}
	return s, nil
}

func (s *SQLiteCheckpointStore) initSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS stage_checkpoints (
			checkpoint_id  TEXT PRIMARY KEY,
			case_id        TEXT NOT NULL DEFAULT '',
			case_type      TEXT NOT NULL DEFAULT '',
			current_stage  INTEGER NOT NULL DEFAULT 0,
			data           TEXT NOT NULL,
			created_at     TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_checkpoints_case ON stage_checkpoints(case_id);
	`)
	return err
}

// Save persists a StageCheckpoint. If a checkpoint with the same ID exists,
// it is replaced.
func (s *SQLiteCheckpointStore) Save(ctx context.Context, cp *reasoning.StageCheckpoint) error {
	data, err := reasoning.MarshalCheckpoint(cp)
	if err != nil {
		return fmt.Errorf("reasoning/sqlite: marshal: %w", err)
	}

	caseType := string(cp.CaseType)

	_, err = s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO stage_checkpoints (checkpoint_id, case_id, case_type, current_stage, data)
		VALUES (?, ?, ?, ?, ?)
	`,
		cp.CheckpointID, cp.CaseID, caseType, cp.CurrentStage, string(data),
	)
	if err != nil {
		return fmt.Errorf("reasoning/sqlite: save: %w", err)
	}
	return nil
}

// Load retrieves a StageCheckpoint by its ID.
func (s *SQLiteCheckpointStore) Load(ctx context.Context, checkpointID string) (*reasoning.StageCheckpoint, error) {
	var data string
	err := s.db.QueryRowContext(ctx,
		`SELECT data FROM stage_checkpoints WHERE checkpoint_id = ?`,
		checkpointID,
	).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("checkpoint %q not found", checkpointID)
		}
		return nil, fmt.Errorf("reasoning/sqlite: load: %w", err)
	}

	cp, err := reasoning.UnmarshalCheckpoint([]byte(data))
	if err != nil {
		return nil, fmt.Errorf("reasoning/sqlite: unmarshal: %w", err)
	}
	return cp, nil
}

// Delete removes a StageCheckpoint by its ID.
func (s *SQLiteCheckpointStore) Delete(ctx context.Context, checkpointID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM stage_checkpoints WHERE checkpoint_id = ?`,
		checkpointID,
	)
	if err != nil {
		return fmt.Errorf("reasoning/sqlite: delete: %w", err)
	}
	return nil
}

// ListByCase returns all checkpoint IDs for a given case, ordered by
// creation time descending (newest first).
func (s *SQLiteCheckpointStore) ListByCase(ctx context.Context, caseID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT checkpoint_id FROM stage_checkpoints WHERE case_id = ? ORDER BY created_at DESC`,
		caseID,
	)
	if err != nil {
		return nil, fmt.Errorf("reasoning/sqlite: list by case: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("reasoning/sqlite: scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Close releases the database connection.
func (s *SQLiteCheckpointStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// --- store.CaseStore 接口实现 ---

// CaseID 返回 ""，该存储用于所有案件。
func (s *SQLiteCheckpointStore) CaseID() string { return "" }

// RunID 返回 ""，该存储不限定于单次运行。
func (s *SQLiteCheckpointStore) RunID() string { return "" }

// Version 返回当前 schema 版本（1）。
func (s *SQLiteCheckpointStore) Version() int { return 1 }

// Migrate 执行 schema 迁移。当前为版本 1（初始 schema）。
func (s *SQLiteCheckpointStore) Migrate(ctx context.Context) (int, error) {
	if err := s.initSchema(ctx); err != nil {
		return 0, fmt.Errorf("checkpoint migrate: %w", err)
	}
	return s.Version(), nil
}

// 编译时检查
var _ reasoning.CheckpointStore = (*SQLiteCheckpointStore)(nil)
