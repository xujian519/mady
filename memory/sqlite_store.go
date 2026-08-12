package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver

	"github.com/xujian519/mady/pkg/vecbytes"
	"github.com/xujian519/mady/retrieval"
	"github.com/xujian519/mady/store"
)

// SQLiteMemoryStore 是 MemoryStore 的 SQLite 持久化实现。
// 数据存储在本地 SQLite 文件中，重启后不丢失。
// Phase 2 实现：适合生产部署，替代 InMemoryStore 的非持久化场景。
//
// 检索策略与 InMemoryStore 保持一致：关键词匹配 + 复合评分（语义+新鲜度+重要性）。
// Embedding 字段以 BLOB 形式存储，供未来向量检索升级使用。
type SQLiteMemoryStore struct {
	db       *sql.DB
	scoring  ScoringConfig
	now      func() time.Time
	embedder retrieval.Embedder
}

// SQLiteOption 是 SQLiteMemoryStore 的函数式配置选项。
type SQLiteOption func(*SQLiteMemoryStore)

// WithSQLiteClock 注入时间函数（测试用）。
func WithSQLiteClock(clock func() time.Time) SQLiteOption {
	return func(s *SQLiteMemoryStore) { s.now = clock }
}

// WithSQLiteEmbedder 注入向量编码器，启用语义检索。
// 当 embedder 非 nil 时，Remember/RememberBatch 自动生成 embedding，
// Recall 使用向量相似度替代关键词匹配。
// 当 embedder 为 nil 时（默认），退化为纯关键词检索。
func WithSQLiteEmbedder(emb retrieval.Embedder) SQLiteOption {
	return func(s *SQLiteMemoryStore) { s.embedder = emb }
}

// NewSQLiteMemoryStore 打开或创建指定路径的 SQLite 记忆数据库。
// 如果文件不存在则自动创建并初始化 schema。
func NewSQLiteMemoryStore(dbPath string, opts ...SQLiteOption) (*SQLiteMemoryStore, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("memory/sqlite: open %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(4)

	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory/sqlite: ping %s: %w", dbPath, err)
	}

	s := &SQLiteMemoryStore{
		db:      db,
		scoring: DefaultScoringConfig(),
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}

	if err := s.initSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory/sqlite: init schema: %w", err)
	}

	return s, nil
}

// initSchema 创建表和索引（幂等）。
func (s *SQLiteMemoryStore) initSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS memories (
			id            TEXT PRIMARY KEY,
			user_id       TEXT NOT NULL DEFAULT '',
			agent_id      TEXT NOT NULL DEFAULT '',
			session_id    TEXT NOT NULL DEFAULT '',
			project_id    TEXT NOT NULL DEFAULT '',
			layer         TEXT NOT NULL,
			content       TEXT NOT NULL,
			embedding     BLOB,
			importance    REAL NOT NULL DEFAULT 0,
			access_count  INTEGER NOT NULL DEFAULT 0,
			created_at    TEXT NOT NULL,
			updated_at    TEXT NOT NULL,
			last_access   TEXT NOT NULL,
			decay_factor  REAL NOT NULL DEFAULT 0.95,
			metadata      TEXT NOT NULL DEFAULT '{}'
		);
		CREATE INDEX IF NOT EXISTS idx_memories_layer ON memories(layer);
		CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(user_id, agent_id, session_id, project_id);
	`)
	return err
}

// --- MemoryStore 接口实现 ---

// Remember 存入一条记忆。
func (s *SQLiteMemoryStore) Remember(ctx context.Context, content string, scope MemoryScope, layer MemoryLayer, metadata map[string]any) (string, error) {
	if content == "" {
		return "", fmt.Errorf("memory: content is empty")
	}
	if !layer.IsValid() {
		return "", fmt.Errorf("memory: invalid layer %q", layer)
	}

	id := nextMemoryID()
	now := s.now()

	metaJSON := "{}"
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			metaJSON = string(b)
		}
	}

	var embVal any
	if s.embedder != nil {
		if vecs, err := s.embedder.Embed(ctx, []string{content}); err == nil && len(vecs) > 0 {
			embVal = vecbytes.FloatsToBytes(vecs[0])
		}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO memories (id, user_id, agent_id, session_id, project_id, layer, content,
		                      embedding, importance, access_count, created_at, updated_at,
		                      last_access, decay_factor, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, 0.95, ?)
	`,
		id, scope.UserID, scope.AgentID, scope.SessionID, scope.ProjectID,
		string(layer), content, embVal, estimateImportance(content),
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano), metaJSON,
	)
	if err != nil {
		return "", fmt.Errorf("memory/sqlite: insert: %w", err)
	}

	return id, nil
}

// RememberBatch 批量存入。
//
//nolint:gocognit // 原因：批量记忆写入，含事务和多行预处理
func (s *SQLiteMemoryStore) RememberBatch(ctx context.Context, entries []MemoryEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("memory/sqlite: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := s.now()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO memories (id, user_id, agent_id, session_id, project_id, layer, content,
		                                  embedding, importance, access_count, created_at, updated_at,
		                                  last_access, decay_factor, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("memory/sqlite: prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for i := range entries {
		e := &entries[i]
		if e.ID == "" {
			e.ID = nextMemoryID()
		}
		if e.CreatedAt.IsZero() {
			e.CreatedAt = now
		}
		if e.UpdatedAt.IsZero() {
			e.UpdatedAt = now
		}
		if e.LastAccess.IsZero() {
			e.LastAccess = now
		}
		if e.DecayFactor == 0 {
			e.DecayFactor = 0.95
		}
		if e.Importance == 0 {
			e.Importance = estimateImportance(e.Content)
		}
		if len(e.Embedding) == 0 && s.embedder != nil {
			if vecs, err := s.embedder.Embed(ctx, []string{e.Content}); err == nil && len(vecs) > 0 {
				e.Embedding = vecs[0]
			}
		}

		metaJSON := "{}"
		if e.Metadata != nil {
			if b, err := json.Marshal(e.Metadata); err == nil {
				metaJSON = string(b)
			}
		}

		var embVal any
		if len(e.Embedding) > 0 {
			embVal = vecbytes.FloatsToBytes(e.Embedding)
		}

		_, err := stmt.ExecContext(ctx,
			e.ID, e.Scope.UserID, e.Scope.AgentID, e.Scope.SessionID, e.Scope.ProjectID,
			string(e.Layer), e.Content, embVal, e.Importance, e.AccessCount,
			e.CreatedAt.Format(time.RFC3339Nano), e.UpdatedAt.Format(time.RFC3339Nano),
			e.LastAccess.Format(time.RFC3339Nano), e.DecayFactor, metaJSON,
		)
		if err != nil {
			return fmt.Errorf("memory/sqlite: batch insert %s: %w", e.ID, err)
		}
	}

	return tx.Commit()
}

// Recall 按语义检索记忆，返回按复合评分降序排列的结果。
// 当配置了 embedder 时使用向量相似度，否则退化为关键词匹配。
func (s *SQLiteMemoryStore) Recall(ctx context.Context, query string, filter MemoryFilter) ([]ScoredMemory, error) {
	limit := filter.EffectiveTopK() * 10
	if limit < 100 {
		limit = 100
	}
	candidates, err := s.queryCandidates(ctx, filter, limit)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	now := s.now()

	var queryVec []float32
	if s.embedder != nil {
		if vecs, embErr := s.embedder.Embed(ctx, []string{query}); embErr == nil && len(vecs) > 0 {
			queryVec = vecs[0]
		}
	}

	scored := make([]ScoredMemory, 0, len(candidates))
	for _, entry := range candidates {
		var semantic float64
		if queryVec != nil && len(entry.Embedding) > 0 {
			semantic = retrieval.CosineSimilarity(queryVec, entry.Embedding)
			if semantic < 0 {
				semantic = 0
			}
		} else {
			semantic = keywordScore(query, entry.Content)
		}
		if semantic < 0.25 {
			continue
		}
		recency := recencyScore(entry.LastAccess, now, s.scoring.RecencyHalfLife)
		composite := s.scoring.SemanticWeight*semantic +
			s.scoring.RecencyWeight*recency +
			s.scoring.ImportanceWeight*entry.Importance

		scored = append(scored, ScoredMemory{
			Entry:      entry,
			Semantic:   semantic,
			Recency:    recency,
			Importance: entry.Importance,
			Composite:  composite,
		})
	}

	sortScoredByComposite(scored)

	topK := filter.EffectiveTopK()
	if len(scored) > topK {
		scored = scored[:topK]
	}
	for i := range scored {
		scored[i].Rank = i
	}

	if len(scored) > 0 {
		ids := make([]string, len(scored))
		for i := range scored {
			ids[i] = scored[i].Entry.ID
		}
		s.updateAccessStats(ctx, ids, now)
	}

	return scored, nil
}

// RecallWithBudget 在 token 预算约束下检索。
func (s *SQLiteMemoryStore) RecallWithBudget(ctx context.Context, query string, filter MemoryFilter, maxTokens int64) ([]ScoredMemory, error) {
	results, err := s.Recall(ctx, query, filter)
	if err != nil {
		return nil, err
	}

	var filtered []ScoredMemory
	tokensUsed := int64(0)
	for _, r := range results {
		t := estimateTokens(r.Entry.Content)
		if tokensUsed+t > maxTokens {
			continue
		}
		tokensUsed += t
		filtered = append(filtered, r)
	}
	return filtered, nil
}

// Get 按 ID 获取单条记忆。
func (s *SQLiteMemoryStore) Get(ctx context.Context, id string) (*MemoryEntry, error) {
	row := s.db.QueryRowContext(ctx, selectColumns+` FROM memories WHERE id = ?`, id)
	entry, err := scanEntry(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("memory: entry %q not found", id)
		}
		return nil, fmt.Errorf("memory/sqlite: get: %w", err)
	}
	return &entry, nil
}

// Update 更新记忆内容。
func (s *SQLiteMemoryStore) Update(ctx context.Context, id string, content string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE memories SET content = ?, updated_at = ? WHERE id = ?`,
		content, s.now().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return fmt.Errorf("memory/sqlite: update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory: entry %q not found", id)
	}
	return nil
}

// Forget 按 ID 删除。
func (s *SQLiteMemoryStore) Forget(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("memory/sqlite: forget: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory: entry %q not found", id)
	}
	return nil
}

// ForgetAll 按过滤条件批量删除。
func (s *SQLiteMemoryStore) ForgetAll(ctx context.Context, filter MemoryFilter) error {
	where, args := buildWhereClause(filter)
	_, err := s.db.ExecContext(ctx, `DELETE FROM memories `+where, args...) //nolint:gosec // safe: where clause uses ? placeholders
	if err != nil {
		return fmt.Errorf("memory/sqlite: forget_all: %w", err)
	}
	return nil
}

// List 按层分页列出记忆。
func (s *SQLiteMemoryStore) List(ctx context.Context, layer MemoryLayer, opts ListOptions) ([]MemoryEntry, error) {
	if !layer.IsValid() {
		return nil, fmt.Errorf("memory: invalid layer %q", layer)
	}

	order := "DESC"
	if opts.Asc {
		order = "ASC"
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	// UserID 非空时下推到 SQL WHERE 子句，在存储层完成 scope 过滤。
	// 既是性能优化（减少传输和客户端过滤），也是安全边界（避免跨用户泄漏）。
	var (
		query string
		args  []any
	)
	if opts.UserID != "" {
		query = selectColumns + ` FROM memories WHERE layer = ? AND user_id = ? ORDER BY created_at ` + order + ` LIMIT ? OFFSET ?`
		args = []any{string(layer), opts.UserID, limit, opts.Offset}
	} else {
		query = selectColumns + ` FROM memories WHERE layer = ? ORDER BY created_at ` + order + ` LIMIT ? OFFSET ?`
		args = []any{string(layer), limit, opts.Offset}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory/sqlite: list: %w", err)
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

// Prune 清理低衰减/低重要性记忆。
func (s *SQLiteMemoryStore) Prune(ctx context.Context, layer MemoryLayer, threshold float64) (int64, error) {
	if !layer.IsValid() {
		return 0, fmt.Errorf("memory: invalid layer %q", layer)
	}

	rows, err := s.db.QueryContext(ctx,
		selectColumns+` FROM memories WHERE layer = ?`, string(layer),
	)
	if err != nil {
		return 0, fmt.Errorf("memory/sqlite: prune query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	now := s.now()
	var toDelete []string
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return 0, fmt.Errorf("memory/sqlite: prune scan: %w", err)
		}
		recency := recencyScore(entry.LastAccess, now, s.scoring.RecencyHalfLife)
		score := s.scoring.RecencyWeight*recency + s.scoring.ImportanceWeight*entry.Importance
		if score < threshold {
			toDelete = append(toDelete, entry.ID)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(toDelete) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(toDelete))
	args := make([]any, len(toDelete))
	for i, id := range toDelete {
		placeholders[i] = "?"
		args[i] = id
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM memories WHERE id IN (`+strings.Join(placeholders, ",")+`)`, //nolint:gosec // safe: IN clause uses ? placeholders
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("memory/sqlite: prune delete: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Stats 返回统计信息。
func (s *SQLiteMemoryStore) Stats(ctx context.Context) MemoryStats {
	var stats MemoryStats

	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories`)
	_ = row.Scan(&stats.TotalEntries)

	row = s.db.QueryRowContext(ctx,
		`SELECT
			COUNT(CASE WHEN layer = 'user' THEN 1 END),
			COUNT(CASE WHEN layer = 'session' THEN 1 END),
			COUNT(CASE WHEN layer = 'long_term' THEN 1 END)
		 FROM memories`)
	_ = row.Scan(&stats.UserCount, &stats.SessionCount, &stats.LongTermCnt)

	return stats
}

// Close 释放所有资源。
func (s *SQLiteMemoryStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// BuildBM25Index 从数据库中全量加载记忆并构建 BM25 索引。
// 返回的索引可用于混合检索（稠密向量 + BM25 稀疏 + RRF 融合）。
func (s *SQLiteMemoryStore) BuildBM25Index(ctx context.Context) (*BM25Index, error) {
	rows, err := s.db.QueryContext(ctx, selectColumns+` FROM memories`)
	if err != nil {
		return nil, fmt.Errorf("memory/sqlite: build bm25: %w", err)
	}
	defer func() { _ = rows.Close() }()

	index := NewBM25Index(DefaultBM25Config())
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("memory/sqlite: build bm25 scan: %w", err)
		}
		index.Add(entry.ID, entry.Content)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory/sqlite: build bm25 rows: %w", err)
	}

	return index, nil
}

// 编译时检查
var (
	_ MemoryStore     = (*SQLiteMemoryStore)(nil)
	_ store.CaseStore = (*SQLiteMemoryStore)(nil)
	_ store.Closer    = (*SQLiteMemoryStore)(nil)
)

// --- store.CaseStore 接口实现 ---

// CaseID 返回 ""，该存储用于所有作用域。
func (s *SQLiteMemoryStore) CaseID() string { return "" }

// RunID 返回 ""，该存储不限定于单次运行。
func (s *SQLiteMemoryStore) RunID() string { return "" }

// Version 返回当前 schema 版本（1）。
func (s *SQLiteMemoryStore) Version() int { return 1 }

// Migrate 执行 schema 迁移。当前为版本 1（初始 schema）。
func (s *SQLiteMemoryStore) Migrate(ctx context.Context) (int, error) {
	if err := s.initSchema(ctx); err != nil {
		return 0, fmt.Errorf("memory migrate: %w", err)
	}
	return s.Version(), nil
}
