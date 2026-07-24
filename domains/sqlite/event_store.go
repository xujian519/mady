// Package sqlite provides SQLite-backed persistence for agent lifecycle
// events (workflow_events table). This enables audit trails and startup
// recovery without full Event History Replay.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver

	"github.com/xujian519/mady/store"
)

// EventRecord represents a persisted agent lifecycle event.
type EventRecord struct {
	ID        int64  // auto-increment
	EventType string // agent_start, agent_end, approval_prompt, tool_call_start, etc.
	SessionID string
	AgentName string
	Payload   json.RawMessage // serialized event body
	CreatedAt time.Time
}

// SQLEventStore persists agent lifecycle events to SQLite.
// It is a write-mostly store; reads are typically for audit or recovery.
// Old events can be pruned by time or count to control storage growth.
type SQLEventStore struct {
	db *sql.DB
}

// NewEventStore opens or creates a SQLite event database at the given path.
func NewEventStore(dbPath string) (*SQLEventStore, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("event/sqlite: open %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(4)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("event/sqlite: ping %s: %w", dbPath, err)
	}

	s := &SQLEventStore{db: db}
	if err := s.initSchema(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("event/sqlite: init schema: %w", err)
	}
	return s, nil
}

func (s *SQLEventStore) initSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS workflow_events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type  TEXT NOT NULL,
			session_id  TEXT NOT NULL DEFAULT '',
			agent_name  TEXT NOT NULL DEFAULT '',
			payload     TEXT NOT NULL,
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_we_session ON workflow_events(session_id, created_at);
		CREATE INDEX IF NOT EXISTS idx_we_type     ON workflow_events(event_type);
	`)
	return err
}

// Append inserts a new event record. Returns the auto-increment ID.
func (s *SQLEventStore) Append(ctx context.Context, eventType, sessionID, agentName string, payload json.RawMessage) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO workflow_events (event_type, session_id, agent_name, payload) VALUES (?, ?, ?, ?)`,
		eventType, sessionID, agentName, string(payload),
	)
	if err != nil {
		return 0, fmt.Errorf("event/sqlite: append: %w", err)
	}
	return res.LastInsertId()
}

// ListBySession returns events for a session, newest first, capped at limit.
func (s *SQLEventStore) ListBySession(ctx context.Context, sessionID string, limit int) ([]EventRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, event_type, session_id, agent_name, payload, created_at
		 FROM workflow_events WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("event/sqlite: list by session: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

// ListByType returns events of a specific type, newest first, capped at limit.
func (s *SQLEventStore) ListByType(ctx context.Context, eventType string, limit int) ([]EventRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, event_type, session_id, agent_name, payload, created_at
		 FROM workflow_events WHERE event_type = ? ORDER BY created_at DESC LIMIT ?`,
		eventType, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("event/sqlite: list by type: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

// Prune deletes events older than the given cutoff. Returns count deleted.
func (s *SQLEventStore) Prune(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM workflow_events WHERE created_at < ?`,
		olderThan.Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("event/sqlite: prune: %w", err)
	}
	return res.RowsAffected()
}

// Close releases the database connection.
func (s *SQLEventStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func scanEvents(rows *sql.Rows) ([]EventRecord, error) {
	var out []EventRecord
	for rows.Next() {
		var r EventRecord
		var payloadStr, createdAtStr string
		if err := rows.Scan(&r.ID, &r.EventType, &r.SessionID, &r.AgentName, &payloadStr, &createdAtStr); err != nil {
			return nil, fmt.Errorf("event/sqlite: scan: %w", err)
		}
		r.Payload = json.RawMessage(payloadStr)
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- store.CaseStore 接口实现 ---

var (
	_ store.Closer = (*SQLEventStore)(nil)
)

func (s *SQLEventStore) CaseID() string { return "" }
func (s *SQLEventStore) RunID() string  { return "" }
func (s *SQLEventStore) Version() int   { return 1 }

func (s *SQLEventStore) Migrate(ctx context.Context) (int, error) {
	if err := s.initSchema(ctx); err != nil {
		return 0, fmt.Errorf("event migrate: %w", err)
	}
	return s.Version(), nil
}
