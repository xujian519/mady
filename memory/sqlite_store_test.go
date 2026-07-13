package memory

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func sqliteTestStore(t *testing.T, opts ...SQLiteOption) *SQLiteMemoryStore {
	t.Helper()
	dir := t.TempDir()
	defaultOpts := []SQLiteOption{WithSQLiteClock(func() time.Time {
		return time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	})}
	store, err := NewSQLiteMemoryStore(filepath.Join(dir, "test_memory.db"), append(defaultOpts, opts...)...)
	if err != nil {
		t.Fatalf("NewSQLiteMemoryStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSQLiteStore_RememberAndGet(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	id, err := s.Remember(ctx, "用户喜欢中文回答", testScope(), LayerUser, nil)
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	entry, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.Content != "用户喜欢中文回答" {
		t.Fatalf("content mismatch: got %q", entry.Content)
	}
	if entry.Scope.UserID != "test_user" {
		t.Fatalf("scope.UserID: got %q", entry.Scope.UserID)
	}
	if entry.Layer != LayerUser {
		t.Fatalf("layer: got %q", entry.Layer)
	}
	if entry.Importance <= 0 {
		t.Fatalf("importance should be > 0, got %f", entry.Importance)
	}
	if entry.DecayFactor != 0.95 {
		t.Fatalf("decay_factor: got %f", entry.DecayFactor)
	}
}

func TestSQLiteStore_RememberWithMetadata(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	meta := map[string]any{"source": "test", "confidence": 0.9}
	id, err := s.Remember(ctx, "重要决策", testScope(), LayerLongTerm, meta)
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	entry, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.Metadata == nil {
		t.Fatal("expected non-nil metadata")
	}
	if entry.Metadata["source"] != "test" {
		t.Fatalf("metadata source: got %v", entry.Metadata["source"])
	}
}

func TestSQLiteStore_EmptyContent(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	_, err := s.Remember(ctx, "", testScope(), LayerUser, nil)
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestSQLiteStore_RememberBatch(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	entries := []MemoryEntry{
		{Scope: testScope(), Layer: LayerUser, Content: "偏好A"},
		{Scope: testScope(), Layer: LayerSession, Content: "上下文B"},
		{Scope: testScope(), Layer: LayerLongTerm, Content: "长期事实C"},
	}

	if err := s.RememberBatch(ctx, entries); err != nil {
		t.Fatalf("RememberBatch: %v", err)
	}

	for i := range entries {
		if entries[i].ID == "" {
			t.Fatalf("entry %d: expected non-empty id", i)
		}
	}

	stats := s.Stats()
	if stats.TotalEntries != 3 {
		t.Fatalf("expected 3 entries, got %d", stats.TotalEntries)
	}
	if stats.UserCount != 1 || stats.SessionCount != 1 || stats.LongTermCnt != 1 {
		t.Fatalf("counts mismatch: %+v", stats)
	}
}

func TestSQLiteStore_Update(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	id, _ := s.Remember(ctx, "原始内容", testScope(), LayerUser, nil)

	if err := s.Update(ctx, id, "更新后的内容"); err != nil {
		t.Fatalf("Update: %v", err)
	}

	entry, _ := s.Get(ctx, id)
	if entry.Content != "更新后的内容" {
		t.Fatalf("content: got %q", entry.Content)
	}
}

func TestSQLiteStore_Forget(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	id, _ := s.Remember(ctx, "待删除", testScope(), LayerUser, nil)

	if err := s.Forget(ctx, id); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	_, err := s.Get(ctx, id)
	if err == nil {
		t.Fatal("expected error after Forget")
	}

	if s.Stats().TotalEntries != 0 {
		t.Fatal("expected 0 entries after forget")
	}
}

func TestSQLiteStore_ForgetAll(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	scope1 := MemoryScope{UserID: "user1"}
	scope2 := MemoryScope{UserID: "user2"}

	s.Remember(ctx, "user1 记忆", scope1, LayerUser, nil)
	s.Remember(ctx, "user1 session", scope1, LayerSession, nil)
	s.Remember(ctx, "user2 记忆", scope2, LayerUser, nil)

	err := s.ForgetAll(ctx, MemoryFilter{UserID: "user1"})
	if err != nil {
		t.Fatalf("ForgetAll: %v", err)
	}

	stats := s.Stats()
	if stats.TotalEntries != 1 {
		t.Fatalf("expected 1 entry, got %d", stats.TotalEntries)
	}
	if stats.UserCount != 1 {
		t.Fatalf("expected 1 user entry, got %d", stats.UserCount)
	}
}

func TestSQLiteStore_Recall(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	scope := MemoryScope{UserID: "u1", SessionID: "s1"}
	s.Remember(ctx, "用户喜欢简洁的中文回答风格", scope, LayerUser, nil)
	s.Remember(ctx, "代理人资格考试相关法规条文", scope, LayerLongTerm, nil)
	s.Remember(ctx, "当前会话讨论了专利侵权分析", scope, LayerSession, nil)

	results, err := s.Recall(ctx, "中文回答风格", MemoryFilter{UserID: "u1"})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	top := results[0]
	if !strings.Contains(top.Entry.Content, "中文回答") {
		t.Fatalf("top result should mention 中文回答, got %q", top.Entry.Content)
	}
	if top.Composite <= 0 {
		t.Fatalf("composite score should be > 0, got %f", top.Composite)
	}
	if top.Rank != 0 {
		t.Fatalf("top result rank should be 0, got %d", top.Rank)
	}
}

func TestSQLiteStore_RecallWithBudget(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	scope := MemoryScope{UserID: "u1"}
	s.Remember(ctx, "短记忆", scope, LayerUser, nil)
	s.Remember(ctx, "这是一段比较长的记忆内容用于测试token预算截断功能是否正常工作", scope, LayerUser, nil)

	results, err := s.RecallWithBudget(ctx, "记忆", MemoryFilter{UserID: "u1"}, 5)
	if err != nil {
		t.Fatalf("RecallWithBudget: %v", err)
	}

	totalTokens := int64(0)
	for _, r := range results {
		totalTokens += estimateTokens(r.Entry.Content)
	}
	if totalTokens > 5 {
		t.Fatalf("total tokens %d exceeds budget 5", totalTokens)
	}
}

func TestSQLiteStore_List(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.Remember(ctx, "记忆内容"+string(rune('A'+i)), testScope(), LayerUser, nil)
	}

	entries, err := s.List(ctx, LayerUser, ListOptions{Limit: 3, Offset: 0, Asc: false})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	entries2, _ := s.List(ctx, LayerUser, ListOptions{Limit: 3, Offset: 3, Asc: false})
	if len(entries2) != 2 {
		t.Fatalf("expected 2 entries on page 2, got %d", len(entries2))
	}
}

func TestSQLiteStore_Prune(t *testing.T) {
	oldClock := func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) }
	dir := t.TempDir()
	store, err := NewSQLiteMemoryStore(filepath.Join(dir, "prune_test.db"), WithSQLiteClock(oldClock))
	if err != nil {
		t.Fatalf("NewSQLiteMemoryStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	store.Remember(ctx, "低重要性内容", testScope(), LayerSession, nil)

	newClock := func() time.Time { return time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC) }
	store.now = newClock

	removed, err := store.Prune(ctx, LayerSession, 0.5)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if removed == 0 {
		t.Fatal("expected at least 1 pruned entry")
	}
	if store.Stats().TotalEntries != 0 {
		t.Fatalf("expected 0 entries after prune, got %d", store.Stats().TotalEntries)
	}
}

func TestSQLiteStore_Stats(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	s.Remember(ctx, "user1 偏好", MemoryScope{UserID: "u1"}, LayerUser, nil)
	s.Remember(ctx, "session1 上下文", MemoryScope{UserID: "u1", SessionID: "s1"}, LayerSession, nil)
	s.Remember(ctx, "长期事实", MemoryScope{UserID: "u1"}, LayerLongTerm, nil)

	stats := s.Stats()
	if stats.TotalEntries != 3 {
		t.Fatalf("TotalEntries: got %d", stats.TotalEntries)
	}
	if stats.UserCount != 1 {
		t.Fatalf("UserCount: got %d", stats.UserCount)
	}
	if stats.SessionCount != 1 {
		t.Fatalf("SessionCount: got %d", stats.SessionCount)
	}
	if stats.LongTermCnt != 1 {
		t.Fatalf("LongTermCnt: got %d", stats.LongTermCnt)
	}
}

func TestSQLiteStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist_test.db")
	ctx := context.Background()

	store1, err := NewSQLiteMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteMemoryStore: %v", err)
	}

	id, _ := store1.Remember(ctx, "跨重启保留的记忆", testScope(), LayerUser, nil)
	store1.Close()

	store2, err := NewSQLiteMemoryStore(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { store2.Close() })

	entry, err := store2.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if entry.Content != "跨重启保留的记忆" {
		t.Fatalf("content after reopen: got %q", entry.Content)
	}

	if store2.Stats().TotalEntries != 1 {
		t.Fatalf("expected 1 entry after reopen, got %d", store2.Stats().TotalEntries)
	}
}

func TestSQLiteStore_Concurrency(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Remember(ctx, "并发记忆"+string(rune('A'+n)), testScope(), LayerUser, nil)
		}(i)
	}
	wg.Wait()

	if s.Stats().TotalEntries != 20 {
		t.Fatalf("expected 20 entries, got %d", s.Stats().TotalEntries)
	}

	var wg2 sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			s.Recall(ctx, "并发", MemoryFilter{UserID: "test_user"})
		}()
	}
	wg2.Wait()
}

func TestSQLiteStore_EmbeddingRoundTrip(t *testing.T) {
	s := sqliteTestStore(t)
	ctx := context.Background()

	entries := []MemoryEntry{{
		Scope:     testScope(),
		Layer:     LayerLongTerm,
		Content:   "带向量的记忆",
		Embedding: []float32{0.1, 0.2, 0.3, 0.4, 0.5},
	}}
	if err := s.RememberBatch(ctx, entries); err != nil {
		t.Fatalf("RememberBatch: %v", err)
	}

	entry, err := s.Get(ctx, entries[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(entry.Embedding) != 5 {
		t.Fatalf("embedding length: got %d, want 5", len(entry.Embedding))
	}
	for i, v := range entry.Embedding {
		expected := []float32{0.1, 0.2, 0.3, 0.4, 0.5}[i]
		if v != expected {
			t.Fatalf("embedding[%d]: got %f, want %f", i, v, expected)
		}
	}
}
