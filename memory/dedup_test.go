package memory

import (
	"context"
	"testing"
	"time"
)

func TestDedupRuleBased(t *testing.T) {
	cfg := DefaultDedupConfig()

	tests := []struct {
		name       string
		sim        float64
		wantAdd    bool
		wantNoop   bool
		wantUpdate bool
	}{
		{
			name:     "high similarity → noop",
			sim:      0.95,
			wantNoop: true,
		},
		{
			name:       "medium similarity → update",
			sim:        0.87,
			wantUpdate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := []ScoredMemory{
				{
					Entry: MemoryEntry{
						ID:      "test-id",
						Content: "existing content",
					},
					Semantic: tt.sim,
				},
			}
			action, reason := ruleBasedDedup(results, cfg)
			if tt.wantNoop && action != DedupNoop {
				t.Errorf("expected NOOP, got %s: %s", action, reason)
			}
			if tt.wantUpdate && action != DedupUpdate {
				t.Errorf("expected UPDATE, got %s: %s", action, reason)
			}
			if reason == "" {
				t.Error("expected non-empty reason")
			}
		})
	}
}

func TestDedupBelowThreshold(t *testing.T) {
	store := NewInMemoryStore()
	cfg := DefaultManagerConfig()
	cfg.EnableDedup = true
	mgr := NewManager(store, nil, nil, cfg)

	ctx := context.Background()
	scope := MemoryScope{UserID: "test-user"}

	// 1. 首次存储 → ADD（无相似记忆）
	result, err := mgr.Deduplicate(ctx, "用户偏好使用表格", scope, LayerLongTerm)
	if err != nil {
		t.Fatalf("first dedup: %v", err)
	}
	if result.Action != DedupAdd {
		t.Errorf("first insert: expected ADD, got %s", result.Action)
	}
	if result.NewID == "" {
		t.Error("expected non-empty NewID")
	}

	// 2. 存储几乎相同的内容 → NOOP（规则 fallback，相似度 > 0.9）
	result2, err := mgr.Deduplicate(ctx, "用户偏好使用表格", scope, LayerLongTerm)
	if err != nil {
		t.Fatalf("second dedup: %v", err)
	}
	if result2.Action != DedupNoop {
		t.Errorf("duplicate: expected NOOP, got %s: %s", result2.Action, result2.Reason)
	}

	// 3. 存储完全不同的内容 → ADD
	result3, err := mgr.Deduplicate(ctx, "用户从事机械领域专利代理工作", scope, LayerLongTerm)
	if err != nil {
		t.Fatalf("third dedup: %v", err)
	}
	if result3.Action != DedupAdd {
		t.Errorf("different: expected ADD, got %s: %s", result3.Action, result3.Reason)
	}

	// 验证存储统计
	stats := mgr.Stats(ctx)
	if stats.LongTermCnt < 2 {
		t.Errorf("expected at least 2 long-term entries, got %d", stats.LongTermCnt)
	}
}

func TestDedupEmptyContent(t *testing.T) {
	store := NewInMemoryStore()
	mgr := NewManager(store, nil, nil, DefaultManagerConfig())

	_, err := mgr.Deduplicate(context.Background(), "", MemoryScope{}, LayerLongTerm)
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestDedupSetConfig(t *testing.T) {
	store := NewInMemoryStore()
	mgr := NewManager(store, nil, nil, DefaultManagerConfig())

	cfg := DedupConfig{
		SimilarityThreshold: 0.5,
		NoopThreshold:       0.95,
	}
	mgr.SetDedupConfig(cfg)

	// Should not panic
	ctx := context.Background()
	_, err := mgr.Deduplicate(ctx, "测试内容", MemoryScope{UserID: "test"}, LayerLongTerm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDedupApplyAction(t *testing.T) {
	store := NewInMemoryStore(WithClock(func() time.Time { return time.Now() }))
	mgr := NewManager(store, nil, nil, DefaultManagerConfig())
	ctx := context.Background()
	scope := MemoryScope{UserID: "test-user"}

	// ADD
	result, err := mgr.applyDedupAction(ctx, DedupAdd, "new fact", "content", scope, LayerLongTerm, "")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if result.NewID == "" {
		t.Error("ADD should produce NewID")
	}
	savedID := result.NewID

	// UPDATE
	result2, err := mgr.applyDedupAction(ctx, DedupUpdate, "updated", "updated content", scope, LayerLongTerm, savedID)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if result2.ExistingID != savedID {
		t.Errorf("UPDATE ExistingID = %q, want %q", result2.ExistingID, savedID)
	}
	entry, _ := store.Get(ctx, savedID)
	if entry.Content != "updated content" {
		t.Errorf("updated content = %q, want %q", entry.Content, "updated content")
	}

	// NOOP
	result3, err := mgr.applyDedupAction(ctx, DedupNoop, "same", "same content", scope, LayerLongTerm, savedID)
	if err != nil {
		t.Fatalf("noop: %v", err)
	}
	if result3.Action != DedupNoop {
		t.Errorf("NOOP action = %s", result3.Action)
	}

	// DELETE
	_, err = mgr.applyDedupAction(ctx, DedupDelete, "removed", "", scope, LayerLongTerm, savedID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = store.Get(ctx, savedID)
	if err == nil {
		t.Error("expected error after DELETE")
	}
}
