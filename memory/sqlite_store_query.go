package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xujian519/mady/pkg/vecbytes"
)

// --- 内部辅助 ---

const selectColumns = `SELECT id, user_id, agent_id, session_id, project_id, layer, content,
	embedding, importance, access_count, created_at, updated_at, last_access, decay_factor, metadata`

type scanner interface {
	Scan(dest ...any) error
}

func scanEntry(sc scanner) (MemoryEntry, error) {
	var (
		entry      MemoryEntry
		layerStr   string
		embBlob    []byte
		createdAt  string
		updatedAt  string
		lastAccess string
		metaJSON   string
	)
	err := sc.Scan(
		&entry.ID, &entry.Scope.UserID, &entry.Scope.AgentID, &entry.Scope.SessionID,
		&entry.Scope.ProjectID, &layerStr, &entry.Content, &embBlob,
		&entry.Importance, &entry.AccessCount, &createdAt, &updatedAt,
		&lastAccess, &entry.DecayFactor, &metaJSON,
	)
	if err != nil {
		return MemoryEntry{}, err
	}
	entry.Layer = MemoryLayer(layerStr)
	entry.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	entry.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	entry.LastAccess, _ = time.Parse(time.RFC3339Nano, lastAccess)
	if len(embBlob) > 0 {
		entry.Embedding = vecbytes.BytesToFloats(embBlob)
	}
	// Metadata 列在 schema 中未强制 JSON 约束，历史数据可能含 null/坏值。
	// 此处与上方 time.Parse 的处理保持一致：best-effort 解析，损坏时降级为
	// 零值（nil map），避免单条记录的 metadata 损坏导致整条 MemoryEntry 读不出。
	if metaJSON != "" && metaJSON != "{}" {
		_ = json.Unmarshal([]byte(metaJSON), &entry.Metadata)
	}
	return entry, nil
}

func (s *SQLiteMemoryStore) queryCandidates(ctx context.Context, filter MemoryFilter, limit int) ([]MemoryEntry, error) {
	where, args := buildWhereClause(filter)
	query := selectColumns + ` FROM memories ` + where + ` LIMIT ?` //nolint:gosec // safe: where clause uses ? placeholders
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory/sqlite: query candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []MemoryEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("memory/sqlite: scan: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *SQLiteMemoryStore) updateAccessStats(ctx context.Context, ids []string, now time.Time) {
	if len(ids) == 0 {
		return
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, now.Format(time.RFC3339Nano))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	_, _ = s.db.ExecContext(ctx,
		`UPDATE memories SET last_access = ?, access_count = access_count + 1 WHERE id IN (`+ //nolint:gosec // safe: IN clause uses ? placeholders
			strings.Join(placeholders, ",")+`)`,
		args...,
	)
}

func buildWhereClause(filter MemoryFilter) (string, []any) {
	var clauses []string
	var args []any

	if filter.UserID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, filter.UserID)
	}
	if filter.AgentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, filter.AgentID)
	}
	if filter.SessionID != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, filter.SessionID)
	}
	if filter.ProjectID != "" {
		clauses = append(clauses, "project_id = ?")
		args = append(args, filter.ProjectID)
	}
	if filter.Layer != "" {
		clauses = append(clauses, "layer = ?")
		args = append(args, string(filter.Layer))
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}
