package sqlite

// event_store.go 提供 Agent 生命周期事件（workflow_events 表）的 SQLite
// 持久化实现，支撑审计轨迹与启动恢复，无需完整的 Event History Replay。
// package 文档见 doc.go。

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

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
	db, err := openDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("event/sqlite: %w", err)
	}

	s := &SQLEventStore{db: db}
	if err := s.initSchema(context.Background()); err != nil {
		_ = db.Close()
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
			created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		);
		CREATE INDEX IF NOT EXISTS idx_we_session ON workflow_events(session_id, created_at);
		CREATE INDEX IF NOT EXISTS idx_we_type     ON workflow_events(event_type);
	`)
	return err
}

// Append inserts a new event record. Returns the auto-increment ID.
func (s *SQLEventStore) Append(ctx context.Context, eventType, sessionID, agentName string, payload json.RawMessage) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO workflow_events (event_type, session_id, agent_name, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		eventType, sessionID, agentName, string(payload), nowText(),
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
		 FROM workflow_events WHERE session_id = ? ORDER BY datetime(created_at) DESC, id DESC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("event/sqlite: list by session: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEvents(rows)
}

// ListByType returns events of a specific type, newest first, capped at limit.
func (s *SQLEventStore) ListByType(ctx context.Context, eventType string, limit int) ([]EventRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, event_type, session_id, agent_name, payload, created_at
		 FROM workflow_events WHERE event_type = ? ORDER BY datetime(created_at) DESC, id DESC LIMIT ?`,
		eventType, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("event/sqlite: list by type: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEvents(rows)
}

// Prune deletes events older than the given cutoff. Returns count deleted.
//
// 比较经 datetime() 归一化：历史行是 "YYYY-MM-DD HH:MM:SS"，而 cutoff 是
// RFC3339，直接字符串比较会把同一天的历史行判为"更早"而误删。
func (s *SQLEventStore) Prune(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM workflow_events WHERE datetime(created_at) < datetime(?)`,
		olderThan.UTC().Format(time.RFC3339),
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
		r.CreatedAt = parseTimeText(createdAtStr)
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- store.CaseStore 接口实现 ---

var (
	_ store.Closer = (*SQLEventStore)(nil)
)

// CaseID returns an empty string — the event store is not case-specific.
func (s *SQLEventStore) CaseID() string { return "" }

// RunID returns an empty string — the event store is not run-specific.
func (s *SQLEventStore) RunID() string { return "" }

// Version returns the schema version (currently 1).
func (s *SQLEventStore) Version() int { return 1 }

// Migrate applies any pending schema migrations to the event store.
func (s *SQLEventStore) Migrate(ctx context.Context) (int, error) {
	if err := s.initSchema(ctx); err != nil {
		return 0, fmt.Errorf("event migrate: %w", err)
	}
	return s.Version(), nil
}
