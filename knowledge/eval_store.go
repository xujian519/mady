package knowledge

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // SQLite driver; blank import to register database/sql driver
)

// EvalStore 持久化 EvalResult 到 SQLite。
// 使用独立的 eval.db 文件，不依赖 knowledge.db 的 schema。
type EvalStore struct {
	db *sql.DB
}

// EvalStoreConfig 配置 EvalStore。
type EvalStoreConfig struct {
	// DSN 是 SQLite 文件路径（如 "~/.mady/eval.db"）。
	DSN string
}

// EvalStats 是时间范围内的评估统计。
type EvalStats struct {
	TotalEvaluations    int
	AvgFaithfulness     float64
	AvgAnswerRelevancy  float64
	AvgContextPrecision float64
	LowFaithfulness     int     // Faithfulness < 0.7 的数量
	LowFaithfulnessRate float64 // 低忠实度占比
	TimeRange           string  // 时间范围描述
}

// NewEvalStore 打开或创建 eval.db 并自动执行迁移。
func NewEvalStore(cfg EvalStoreConfig) (*EvalStore, error) {
	db, err := sql.Open("sqlite", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("eval store: open: %w", err)
	}
	// SQLite 写并发设置。
	db.SetMaxOpenConns(1)

	store := &EvalStore{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("eval store: migrate: %w", err)
	}
	return store, nil
}

// migrate 自动创建表结构。
func (s *EvalStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS eval_results (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			turn              INTEGER NOT NULL DEFAULT 0,
			question          TEXT NOT NULL DEFAULT '',
			answer            TEXT NOT NULL DEFAULT '',
			context_snippets  INTEGER NOT NULL DEFAULT 0,
			faithfulness      REAL NOT NULL DEFAULT 0,
			answer_relevancy  REAL NOT NULL DEFAULT 0,
			context_precision REAL NOT NULL DEFAULT 0,
			duration_ms       INTEGER NOT NULL DEFAULT 0,
			warnings          TEXT NOT NULL DEFAULT '',
			agent_name        TEXT NOT NULL DEFAULT '',
			created_at        TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_eval_created_at ON eval_results(created_at);
		CREATE INDEX IF NOT EXISTS idx_eval_faithfulness ON eval_results(faithfulness);
	`)
	return err
}

// Save 持久化单次评估结果。
func (s *EvalStore) Save(ctx context.Context, result EvalResult) error {
	// 解析 duration string → int ms
	durationMs := parseDurationMs(result.Duration)

	warnings := ""
	if len(result.Warnings) > 0 {
		warnings = result.Warnings[0]
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO eval_results (turn, question, answer, context_snippets,
			faithfulness, answer_relevancy, context_precision, duration_ms, warnings)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		result.Turn, result.Question, result.Answer, result.ContextSnippets,
		result.Faithfulness, result.AnswerRelevancy, result.ContextPrecision,
		durationMs, warnings,
	)
	if err != nil {
		return fmt.Errorf("eval store: save: %w", err)
	}
	return nil
}

// QueryByThreshold 查询低于指定忠实度阈值的评估。
func (s *EvalStore) QueryByThreshold(ctx context.Context, minFaithfulness float64, limit int) ([]EvalResult, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT turn, question, answer, context_snippets,
			faithfulness, answer_relevancy, context_precision, duration_ms, warnings
		FROM eval_results
		WHERE faithfulness < ?
		ORDER BY faithfulness ASC
		LIMIT ?`, minFaithfulness, limit)
	if err != nil {
		return nil, fmt.Errorf("eval store: query threshold: %w", err)
	}
	defer rows.Close()

	var results []EvalResult
	for rows.Next() {
		var r EvalResult
		var durationMs int
		var warnings string
		if err := rows.Scan(&r.Turn, &r.Question, &r.Answer, &r.ContextSnippets,
			&r.Faithfulness, &r.AnswerRelevancy, &r.ContextPrecision, &durationMs, &warnings); err != nil {
			return nil, fmt.Errorf("eval store: scan: %w", err)
		}
		r.Duration = fmt.Sprintf("%dms", durationMs)
		if warnings != "" {
			r.Warnings = []string{warnings}
		}
		results = append(results, r)
	}
	return results, nil
}

// QueryStats 计算时间范围内的评估统计。
func (s *EvalStore) QueryStats(ctx context.Context, since, until time.Time) (*EvalStats, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS total,
			COALESCE(AVG(faithfulness), 0) AS avg_faith,
			COALESCE(AVG(answer_relevancy), 0) AS avg_rel,
			COALESCE(AVG(context_precision), 0) AS avg_prec,
			COALESCE(SUM(CASE WHEN faithfulness < 0.7 THEN 1 ELSE 0 END), 0) AS low_faith
		FROM eval_results
		WHERE created_at >= ? AND created_at <= ?`,
		since.Format("2006-01-02 15:04:05"),
		until.Format("2006-01-02 15:04:05"),
	)

	stats := &EvalStats{}
	if err := row.Scan(&stats.TotalEvaluations, &stats.AvgFaithfulness,
		&stats.AvgAnswerRelevancy, &stats.AvgContextPrecision, &stats.LowFaithfulness); err != nil {
		return nil, fmt.Errorf("eval store: query stats: %w", err)
	}

	if stats.TotalEvaluations > 0 {
		stats.LowFaithfulnessRate = float64(stats.LowFaithfulness) / float64(stats.TotalEvaluations)
	}
	stats.TimeRange = fmt.Sprintf("%s ~ %s", since.Format("2006-01-02"), until.Format("2006-01-02"))
	return stats, nil
}

// Close 关闭数据库连接。
func (s *EvalStore) Close() error {
	return s.db.Close()
}

// parseDurationMs 将 "1.234ms" / "12.345µs" 等格式解析为毫秒整数。
func parseDurationMs(d string) int {
	if d == "" {
		return 0
	}
	dur, err := time.ParseDuration(d)
	if err != nil {
		return 0
	}
	return int(dur.Milliseconds())
}
