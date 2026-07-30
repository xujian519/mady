// Package sqlite provides SQLite-backed persistence for graph-level
// checkpoints (graph.CheckpointStore). It is distinct from the reasoning
// layer's checkpoint store (domains/reasoning/sqlite/) which persists
// StageCheckpoint domain objects.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver

	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/store"
)

// SQLiteGraphCheckpointStore persists graph.Checkpoint to a SQLite database.
// Each checkpoint is a JSON blob with indexed metadata columns.
// This enables graph execution state to survive process restarts via
// graph.InterruptableGraph and graph.PregelCheckpointer.
type SQLiteGraphCheckpointStore struct {
	db *sql.DB
}

// NewGraphCheckpointStore opens or creates a checkpoint database at the
// given path. WAL mode for safe concurrent reads during writes.
func NewGraphCheckpointStore(dbPath string) (*SQLiteGraphCheckpointStore, error) {
	db, err := openDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("graph-checkpoint/sqlite: %w", err)
	}

	s := &SQLiteGraphCheckpointStore{db: db}
	if err := s.initSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("graph-checkpoint/sqlite: init schema: %w", err)
	}
	return s, nil
}

func (s *SQLiteGraphCheckpointStore) initSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS graph_checkpoints (
			id          TEXT PRIMARY KEY,
			graph_id    TEXT NOT NULL DEFAULT '',
			node_name   TEXT NOT NULL DEFAULT '',
			step_index  INTEGER NOT NULL DEFAULT 0,
			state_json  TEXT NOT NULL,
			metadata    TEXT NOT NULL DEFAULT '{}',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_gc_graph ON graph_checkpoints(graph_id, created_at);
	`)
	return err
}

// Save persists a graph checkpoint. If a checkpoint with the same ID exists,
// it is replaced.
func (s *SQLiteGraphCheckpointStore) Save(ctx context.Context, cp graph.Checkpoint) error {
	meta := "{}"
	if cp.Metadata != nil {
		b, err := json.Marshal(cp.Metadata)
		if err != nil {
			return fmt.Errorf("graph-checkpoint/sqlite: marshal metadata: %w", err)
		}
		meta = string(b)
	}

	stateStr := string(cp.State)
	if stateStr == "" {
		stateStr = "{}"
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO graph_checkpoints (id, graph_id, node_name, step_index, state_json, metadata)
		VALUES (?, ?, ?, ?, ?, ?)
	`, cp.ID, cp.GraphID, cp.NodeName, cp.StepIndex, stateStr, meta)
	if err != nil {
		return fmt.Errorf("graph-checkpoint/sqlite: save: %w", err)
	}
	return nil
}

// Load retrieves a graph checkpoint by its ID.
func (s *SQLiteGraphCheckpointStore) Load(ctx context.Context, id string) (*graph.Checkpoint, error) {
	var (
		nodeName, graphID, stateStr, metaStr string
		stepIndex                            int64
		createdAtStr                         string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, graph_id, node_name, step_index, state_json, metadata, created_at
		 FROM graph_checkpoints WHERE id = ?`, id,
	).Scan(&id, &graphID, &nodeName, &stepIndex, &stateStr, &metaStr, &createdAtStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("graph-checkpoint/sqlite: checkpoint %q not found", id)
		}
		return nil, fmt.Errorf("graph-checkpoint/sqlite: load: %w", err)
	}

	createdAt, _ := time.Parse(time.RFC3339, createdAtStr)

	var metadata map[string]any
	if metaStr != "" && metaStr != "{}" {
		_ = json.Unmarshal([]byte(metaStr), &metadata)
	}

	return &graph.Checkpoint{
		ID:        id,
		GraphID:   graphID,
		NodeName:  nodeName,
		StepIndex: stepIndex,
		State:     json.RawMessage(stateStr),
		Metadata:  metadata,
		CreatedAt: createdAt,
	}, nil
}

// List returns all checkpoints for the given graph ID, oldest first.
func (s *SQLiteGraphCheckpointStore) List(ctx context.Context, graphID string) ([]graph.Checkpoint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, graph_id, node_name, step_index, state_json, metadata, created_at
		 FROM graph_checkpoints WHERE graph_id = ? ORDER BY created_at ASC`, graphID,
	)
	if err != nil {
		return nil, fmt.Errorf("graph-checkpoint/sqlite: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []graph.Checkpoint
	for rows.Next() {
		var (
			id, gid, nodeName, stateStr, metaStr, createdAtStr string
			stepIndex                                          int64
		)
		if err := rows.Scan(&id, &gid, &nodeName, &stepIndex, &stateStr, &metaStr, &createdAtStr); err != nil {
			return nil, fmt.Errorf("graph-checkpoint/sqlite: scan: %w", err)
		}
		createdAt, _ := time.Parse(time.RFC3339, createdAtStr)
		var metadata map[string]any
		if metaStr != "" && metaStr != "{}" {
			_ = json.Unmarshal([]byte(metaStr), &metadata)
		}
		out = append(out, graph.Checkpoint{
			ID:        id,
			GraphID:   gid,
			NodeName:  nodeName,
			StepIndex: stepIndex,
			State:     json.RawMessage(stateStr),
			Metadata:  metadata,
			CreatedAt: createdAt,
		})
	}
	return out, rows.Err()
}

// Delete removes a graph checkpoint by its ID.
func (s *SQLiteGraphCheckpointStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM graph_checkpoints WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("graph-checkpoint/sqlite: delete: %w", err)
	}
	return nil
}

// LoadLatest returns the checkpoint with the highest step_index for graphID.
func (s *SQLiteGraphCheckpointStore) LoadLatest(ctx context.Context, graphID string) (*graph.Checkpoint, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, graph_id, node_name, step_index, state_json, metadata, created_at
		 FROM graph_checkpoints WHERE graph_id = ?
		 ORDER BY step_index DESC LIMIT 1`, graphID)
	var cp graph.Checkpoint
	var stateBytes, metadataBytes []byte
	var createdAt time.Time
	err := row.Scan(&cp.ID, &cp.GraphID, &cp.NodeName, &cp.StepIndex,
		&stateBytes, &metadataBytes, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("graph-checkpoint/sqlite: load latest: %w", err)
	}
	cp.State = stateBytes
	cp.CreatedAt = createdAt
	if len(metadataBytes) > 0 {
		_ = json.Unmarshal(metadataBytes, &cp.Metadata)
	}
	return &cp, nil
}

// Close releases the database connection.
func (s *SQLiteGraphCheckpointStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// --- store.CaseStore 接口实现 ---

var (
	_ graph.CheckpointStore = (*SQLiteGraphCheckpointStore)(nil)
	_ store.Closer          = (*SQLiteGraphCheckpointStore)(nil)
)

// CaseID returns an empty string — the graph-level store is not case-specific.
func (s *SQLiteGraphCheckpointStore) CaseID() string { return "" }

// RunID returns an empty string — the graph-level store is not run-specific.
func (s *SQLiteGraphCheckpointStore) RunID() string { return "" }

// Version returns the schema version (currently 1).
func (s *SQLiteGraphCheckpointStore) Version() int { return 1 }

// Migrate applies any pending schema migrations to the checkpoint store.
func (s *SQLiteGraphCheckpointStore) Migrate(ctx context.Context) (int, error) {
	if err := s.initSchema(ctx); err != nil {
		return 0, fmt.Errorf("graph-checkpoint migrate: %w", err)
	}
	return s.Version(), nil
}
