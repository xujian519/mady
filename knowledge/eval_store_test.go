package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvalStore_SaveAndQuery(t *testing.T) {
	store := newTestEvalStore(t)
	defer store.Close()
	ctx := context.Background()

	result := EvalResult{
		Turn:             1,
		Question:         "这个技术方案是否具有新颖性？",
		Answer:           "根据检索结果，该方案具有新颖性。",
		ContextSnippets:  3,
		Faithfulness:     0.85,
		AnswerRelevancy:  0.92,
		ContextPrecision: 0.78,
		Duration:         "1.234ms",
	}

	if err := store.Save(ctx, result); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Query by threshold — should not include our high-faith result.
	low, err := store.QueryByThreshold(ctx, 0.5, 10)
	if err != nil {
		t.Fatalf("QueryByThreshold failed: %v", err)
	}
	if len(low) != 0 {
		t.Errorf("expected 0 low-faith results, got %d", len(low))
	}

	// Query by threshold — should include our result when threshold is high.
	high, err := store.QueryByThreshold(ctx, 0.9, 10)
	if err != nil {
		t.Fatalf("QueryByThreshold(0.9) failed: %v", err)
	}
	if len(high) != 1 {
		t.Errorf("expected 1 result, got %d", len(high))
	} else {
		if high[0].Faithfulness != 0.85 {
			t.Errorf("expected faithfulness 0.85, got %f", high[0].Faithfulness)
		}
	}
}

func TestEvalStore_QueryStats(t *testing.T) {
	store := newTestEvalStore(t)
	defer store.Close()
	ctx := context.Background()

	// Insert two results with different faithfulness.
	results := []EvalResult{
		{Turn: 1, Question: "q1", Answer: "a1", Faithfulness: 0.92, AnswerRelevancy: 0.8, ContextPrecision: 0.7, Duration: "1ms"},
		{Turn: 2, Question: "q2", Answer: "a2", Faithfulness: 0.45, AnswerRelevancy: 0.6, ContextPrecision: 0.5, Duration: "2ms"},
	}
	for _, r := range results {
		if err := store.Save(ctx, r); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	now := time.Now().UTC()
	stats, err := store.QueryStats(ctx, now.Add(-24*time.Hour), now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("QueryStats failed: %v", err)
	}

	if stats.TotalEvaluations != 2 {
		t.Errorf("expected 2 evaluations, got %d", stats.TotalEvaluations)
	}
	expectedAvg := (0.92 + 0.45) / 2
	if abs(stats.AvgFaithfulness-expectedAvg) > 0.01 {
		t.Errorf("expected avg faithfulness %.2f, got %.2f", expectedAvg, stats.AvgFaithfulness)
	}
	if stats.LowFaithfulness != 1 {
		t.Errorf("expected 1 low-faithfulness result, got %d", stats.LowFaithfulness)
	}
}

func TestEvalStore_EmptyStats(t *testing.T) {
	store := newTestEvalStore(t)
	defer store.Close()

	now := time.Now()
	stats, err := store.QueryStats(context.Background(), now.Add(-1*time.Hour), now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("QueryStats failed: %v", err)
	}
	if stats.TotalEvaluations != 0 {
		t.Errorf("expected 0 evaluations in empty store, got %d", stats.TotalEvaluations)
	}
	if stats.AvgFaithfulness != 0 {
		t.Errorf("expected 0 avg faithfulness, got %f", stats.AvgFaithfulness)
	}
}

func TestParseDurationMs(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"1.234ms", 1},
		{"500µs", 0},
		{"100ms", 100},
		{"", 0},
		{"invalid", 0},
	}
	for _, tc := range cases {
		got := parseDurationMs(tc.input)
		if got != tc.want {
			t.Errorf("parseDurationMs(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestDefaultEvalConfig(t *testing.T) {
	cfg := DefaultEvalConfig()
	if !cfg.Enabled {
		t.Error("expected Enabled=true")
	}
	if cfg.MinFaithfulness != 0.7 {
		t.Errorf("expected MinFaithfulness=0.7, got %f", cfg.MinFaithfulness)
	}
	if cfg.AlertThreshold != 0.6 {
		t.Errorf("expected AlertThreshold=0.6, got %f", cfg.AlertThreshold)
	}
	if cfg.AlertAction != "log" {
		t.Errorf("expected AlertAction=log, got %s", cfg.AlertAction)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// newTestEvalStore 创建临时 SQLite eval 数据库用于测试。
func newTestEvalStore(t testing.TB) *EvalStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "eval_test.db")
	store, err := NewEvalStore(EvalStoreConfig{DSN: dbPath})
	if err != nil {
		t.Fatalf("NewEvalStore failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(dbPath) })
	return store
}
