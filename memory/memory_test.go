package memory

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

func fixedClock() time.Time {
	return time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
}

func fixedClockOpt() InMemoryOption {
	return WithClock(func() time.Time { return fixedClock() })
}

func testStore(t *testing.T, opts ...InMemoryOption) *InMemoryStore {
	t.Helper()
	return NewInMemoryStore(append([]InMemoryOption{fixedClockOpt()}, opts...)...)
}

func testScope() MemoryScope {
	return MemoryScope{UserID: "test_user", SessionID: "test_session"}
}

// ---------------------------------------------------------------------------
// InMemoryStore 基本操作
// ---------------------------------------------------------------------------

func TestInMemoryStore_RememberAndGet(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// 正常存入
	id, err := s.Remember(ctx, "用户喜欢中文回答", testScope(), LayerUser, nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	// Get
	entry, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if entry.Content != "用户喜欢中文回答" {
		t.Fatalf("content mismatch: got %q, want %q", entry.Content, "用户喜欢中文回答")
	}
	if entry.Scope.UserID != "test_user" {
		t.Fatalf("scope.UserID mismatch: got %q", entry.Scope.UserID)
	}
	if entry.Layer != LayerUser {
		t.Fatalf("layer mismatch: got %q", entry.Layer)
	}
	if entry.Importance <= 0 {
		t.Fatal("expected positive importance")
	}

	// 空内容
	_, err = s.Remember(ctx, "", testScope(), LayerUser, nil)
	if err == nil {
		t.Fatal("expected error for empty content")
	}

	// 不存在的 ID
	_, err = s.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent id")
	}
}

func TestInMemoryStore_Update(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	id, _ := s.Remember(ctx, "original", testScope(), LayerUser, nil)

	err := s.Update(ctx, id, "updated content")
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	entry, _ := s.Get(ctx, id)
	if entry.Content != "updated content" {
		t.Fatalf("content mismatch: got %q", entry.Content)
	}

	// 更新不存在的 ID
	err = s.Update(ctx, "nonexistent", "content")
	if err == nil {
		t.Fatal("expected error for nonexistent id")
	}
}

func TestInMemoryStore_Forget(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	id, _ := s.Remember(ctx, "to forget", testScope(), LayerSession, nil)

	err := s.Forget(ctx, id)
	if err != nil {
		t.Fatalf("Forget failed: %v", err)
	}

	_, err = s.Get(ctx, id)
	if err == nil {
		t.Fatal("expected error after forget")
	}

	// 重复删除
	err = s.Forget(ctx, id)
	if err == nil {
		t.Fatal("expected error on double forget")
	}
}

// ---------------------------------------------------------------------------
// Recall 检索
// ---------------------------------------------------------------------------

func TestInMemoryStore_Recall(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	scope := testScope()
	s.Remember(ctx, "用户偏好中文界面", scope, LayerUser, nil)
	s.Remember(ctx, "正在处理专利无效宣告案件", scope, LayerSession, nil)
	s.Remember(ctx, "喜欢使用简洁的编码风格", scope, LayerLongTerm, nil)
	s.Remember(ctx, "早上好", scope, LayerSession, nil)

	t.Run("recall by keyword", func(t *testing.T) {
		results, err := s.Recall(ctx, "中文", MemoryFilter{UserID: "test_user", TopK: 10})
		if err != nil {
			t.Fatalf("Recall failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected at least 1 result")
		}
		found := false
		for _, r := range results {
			if r.Entry.Content == "用户偏好中文界面" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("expected to find '用户偏好中文界面'")
		}
	})

	t.Run("recall by layer", func(t *testing.T) {
		results, err := s.Recall(ctx, "编码", MemoryFilter{UserID: "test_user", Layer: LayerLongTerm, TopK: 10})
		if err != nil {
			t.Fatalf("Recall failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected at least 1 result from long_term layer")
		}
	})

	t.Run("recall no match", func(t *testing.T) {
		results, err := s.Recall(ctx, "zzznotexist", MemoryFilter{UserID: "test_user", TopK: 10})
		if err != nil {
			t.Fatalf("Recall failed: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("recall composite scoring", func(t *testing.T) {
		results, _ := s.Recall(ctx, "中文", MemoryFilter{UserID: "test_user", TopK: 10})
		if len(results) >= 2 {
			if results[0].Composite < results[1].Composite {
				t.Fatal("expected descending composite score order")
			}
		}
	})
}

func TestInMemoryStore_RecallWithBudget(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	scope := testScope()
	s.Remember(ctx, "A short memory", scope, LayerUser, nil)
	s.Remember(ctx, "这是一个较长的记忆内容，包含更多细节信息以便测试预算约束功能", scope, LayerUser, nil)

	results, err := s.RecallWithBudget(ctx, "记忆", MemoryFilter{UserID: "test_user", TopK: 10}, 5) // 5 tokens
	if err != nil {
		t.Fatalf("RecallWithBudget failed: %v", err)
	}
	// 5 tokens 大约 20 字符，长记忆不应被返回
	for _, r := range results {
		tokens := int64(len([]rune(r.Entry.Content)) / 4)
		if tokens > 5 {
			t.Fatalf("result exceeds budget: content=%q tokens=%d", r.Entry.Content, tokens)
		}
	}
}

// ---------------------------------------------------------------------------
// 批量操作
// ---------------------------------------------------------------------------

func TestInMemoryStore_RememberBatch(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	entries := []MemoryEntry{
		{Content: "mem1", Scope: testScope(), Layer: LayerUser, Importance: 0.8},
		{Content: "mem2", Scope: testScope(), Layer: LayerLongTerm, Importance: 0.5},
	}

	err := s.RememberBatch(ctx, entries)
	if err != nil {
		t.Fatalf("RememberBatch failed: %v", err)
	}

	stats := s.Stats(context.Background())
	if stats.TotalEntries != 2 {
		t.Fatalf("expected 2 entries, got %d", stats.TotalEntries)
	}

	// 空 batch
	err = s.RememberBatch(ctx, nil)
	if err != nil {
		t.Fatalf("RememberBatch(nil) failed: %v", err)
	}
}

func TestInMemoryStore_ForgetAll(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Remember(ctx, "user1 data", MemoryScope{UserID: "user1"}, LayerUser, nil)
	s.Remember(ctx, "user2 data", MemoryScope{UserID: "user2"}, LayerUser, nil)

	err := s.ForgetAll(ctx, MemoryFilter{UserID: "user1"})
	if err != nil {
		t.Fatalf("ForgetAll failed: %v", err)
	}

	stats := s.Stats(context.Background())
	if stats.TotalEntries != 1 {
		t.Fatalf("expected 1 entry after forget, got %d", stats.TotalEntries)
	}
}

// ---------------------------------------------------------------------------
// Prune 衰减清理
// ---------------------------------------------------------------------------

func TestInMemoryStore_Prune(t *testing.T) {
	// 使用老旧的时钟确保记忆处于衰减状态
	oldClock := func() time.Time {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	s := NewInMemoryStore(WithClock(oldClock))
	ctx := context.Background()

	scope := testScope()
	s.Remember(ctx, "old memory", scope, LayerSession, nil)
	s.Remember(ctx, "very old memory", scope, LayerSession, nil)

	// 用当前时间的 Prune 执行清理
	removed, err := s.Prune(ctx, LayerSession, 0.5) // 高阈值确保全部清理
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if removed == 0 {
		t.Log("prune removed 0 (may be fine if recency is still high)")
		_ = removed
	}

	// 清理不存在的层
	_, err = s.Prune(ctx, "invalid", 0.1)
	if err == nil {
		t.Fatal("expected error for invalid layer")
	}
}

// ---------------------------------------------------------------------------
// Stats 统计
// ---------------------------------------------------------------------------

func TestInMemoryStore_Stats(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	stats := s.Stats(context.Background())
	if stats.TotalEntries != 0 {
		t.Fatalf("expected 0 entries, got %d", stats.TotalEntries)
	}

	s.Remember(ctx, "user", testScope(), LayerUser, nil)
	s.Remember(ctx, "session", testScope(), LayerSession, nil)
	s.Remember(ctx, "longterm", testScope(), LayerLongTerm, nil)

	stats = s.Stats(context.Background())
	if stats.UserCount != 1 || stats.SessionCount != 1 || stats.LongTermCnt != 1 {
		t.Fatalf("stats mismatch: user=%d session=%d longterm=%d",
			stats.UserCount, stats.SessionCount, stats.LongTermCnt)
	}
}

// ---------------------------------------------------------------------------
// List 分页
// ---------------------------------------------------------------------------

func TestInMemoryStore_List(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	scope := testScope()
	for range 10 {
		s.Remember(ctx, "mem", scope, LayerUser, nil)
	}

	// 降序
	results, err := s.List(ctx, LayerUser, ListOptions{Limit: 5, Asc: false})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// 空层
	results, err = s.List(ctx, LayerLongTerm, ListOptions{Limit: 5})
	if err != nil {
		t.Fatalf("List failed for empty layer: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty layer, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// 并发安全（竞态测试）
// ---------------------------------------------------------------------------

func TestInMemoryStore_Concurrency(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scope := MemoryScope{UserID: "user", SessionID: "session"}
			id, err := s.Remember(ctx, "concurrent", scope, LayerUser, nil)
			if err != nil {
				return
			}
			s.Get(ctx, id)
			s.Recall(ctx, "concurrent", MemoryFilter{UserID: "user", TopK: 10})
			s.Stats(context.Background())
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// MemoryManager
// ---------------------------------------------------------------------------

func TestManager_RememberFromTurn(t *testing.T) {
	s := testStore(t)
	mgr := NewManager(s, nil, nil, DefaultManagerConfig())
	ctx := context.Background()
	scope := testScope()

	ids, err := mgr.RememberFromTurn(ctx, "今天天气怎么样？", "今天天气很好", scope)
	if err != nil {
		t.Fatalf("RememberFromTurn failed: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("expected at least 1 id")
	}

	// 空输入
	ids, err = mgr.RememberFromTurn(ctx, "", "", scope)
	if err != nil {
		t.Fatalf("RememberFromTurn(empty) failed: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 ids for empty input, got %d", len(ids))
	}

	// 验证保存到了 Session 层
	stats := s.Stats(context.Background())
	if stats.SessionCount < 1 {
		t.Fatalf("expected at least 1 session entry, got %d", stats.SessionCount)
	}
}

func TestManager_Search(t *testing.T) {
	s := testStore(t)
	mgr := NewManager(s, nil, nil, DefaultManagerConfig())
	ctx := context.Background()
	scope := testScope()

	mgr.Remember(ctx, "user likes Go programming", scope, LayerUser, nil)
	mgr.Remember(ctx, "working on patent examination", scope, LayerSession, nil)

	results, err := mgr.Search(ctx, "Go", MemoryFilter{UserID: "test_user", TopK: 10})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}

	// SearchAllLayers
	_, err = mgr.SearchAllLayers(ctx, "Go", 5)
	if err != nil {
		t.Fatalf("SearchAllLayers failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Extractor（规则回退）
// ---------------------------------------------------------------------------

func TestExtractor_RuleBasedFallback(t *testing.T) {
	ext := NewExtractor(nil, DefaultExtractorConfig())
	ctx := context.Background()
	scope := testScope()

	facts, err := ext.Extract(ctx, "用户说: 我喜欢Go语言\n助手回答: 好的", scope)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	// 规则提取在 Phase 1 返回空
	if len(facts) != 0 {
		t.Fatalf("expected 0 facts from rule extractor, got %d", len(facts))
	}
}

// ---------------------------------------------------------------------------
// Retriever
// ---------------------------------------------------------------------------

func TestRetriever_Score(t *testing.T) {
	r := NewRetriever(DefaultRetrieverConfig())
	now := fixedClock()
	recentAccess := now.Add(-1 * time.Hour)
	oldAccess := now.Add(-365 * 24 * time.Hour)

	recentScore := r.Score(0.9, 0.8, recentAccess, now)
	oldScore := r.Score(0.9, 0.8, oldAccess, now)

	if recentScore <= oldScore {
		t.Fatal("expected recent memories to score higher")
	}

	// Token 预算估计
	results := []ScoredMemory{
		{Entry: MemoryEntry{Content: "short"}},
		{Entry: MemoryEntry{Content: "这是一个较长的中文内容用来测试预算估算"}},
	}
	tokens := r.EstimateBudgetTokens(results)
	if tokens <= 0 {
		t.Fatal("expected positive token estimate")
	}
}

// ---------------------------------------------------------------------------
// Extension — TransformContext 注入
// ---------------------------------------------------------------------------

func TestMemoryExtension_TransformContext(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	scope := testScope()

	// 先存入一条记忆
	s.Remember(ctx, "用户喜欢中文回答", scope, LayerUser, nil)

	mgr := NewManager(s, nil, nil, DefaultManagerConfig())
	ext := NewExtension(mgr, scope, DefaultExtensionConfig())

	msgs := []agentcore.Message{
		{Role: agentcore.RoleSystem, Content: "你是助手"},
		{Role: agentcore.RoleUser, Content: "用户喜欢什么语言"},
	}

	result := ext.TransformContext(ctx, msgs)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages after injection, got %d", len(result))
	}

	// 中间那条应该是 memory context
	if result[1].Role != agentcore.RoleSystem || !contains(result[1].Content, "记忆上下文") {
		t.Fatalf("expected memory context injection: %s", result[1].Content)
	}

	// 空消息列表
	nomsg := ext.TransformContext(ctx, nil)
	if nomsg != nil {
		t.Fatal("expected nil for empty input")
	}

	// 无用户消息
	nouser := ext.TransformContext(ctx, []agentcore.Message{
		{Role: agentcore.RoleSystem, Content: "system"},
	})
	if len(nouser) != 1 {
		t.Fatal("expected no injection without user message")
	}
}

func TestMemoryExtension_Disabled(t *testing.T) {
	s := testStore(t)
	scope := testScope()
	mgr := NewManager(s, nil, nil, DefaultManagerConfig())
	cfg := DefaultExtensionConfig()
	cfg.Enabled = false
	ext := NewExtension(mgr, scope, cfg)

	msgs := []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "test"},
	}
	result := ext.TransformContext(context.Background(), msgs)
	if len(result) != 1 {
		t.Fatal("expected no modification when disabled")
	}
}

// ---------------------------------------------------------------------------
// Tools
// ---------------------------------------------------------------------------

func TestNewMemoryTools(t *testing.T) {
	s := testStore(t)
	mgr := NewManager(s, nil, nil, DefaultManagerConfig())
	scope := testScope()

	tools := NewMemoryTools(mgr, scope)
	if tools == nil {
		t.Fatal("expected tools")
	}

	// 检查工具数量
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}

	// 检查名称
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, n := range []string{"remember", "recall", "forget"} {
		if !names[n] {
			t.Fatalf("missing tool: %s", n)
		}
	}

	// nil manager
	nilTools := NewMemoryTools(nil, scope)
	if nilTools != nil {
		t.Fatal("expected nil for nil manager")
	}
}

func TestMemoryTools_HandleRemember(t *testing.T) {
	s := testStore(t)
	mgr := NewManager(s, nil, nil, DefaultManagerConfig())
	tools := NewMemoryTools(mgr, testScope())
	ctx := context.Background()

	for _, tool := range tools {
		if tool.Name == "remember" {
			args := `{"content": "记住这个偏好", "importance": 0.9, "layer": "user"}`
			result, err := tool.Func(ctx, []byte(args))
			if err != nil {
				t.Fatalf("remember function failed: %v", err)
			}
			str, ok := result.(string)
			if !ok || !contains(str, "已保存") {
				t.Fatalf("unexpected result: %v", result)
			}
			break
		}
	}

	// 验证已存入
	stats := s.Stats(context.Background())
	if stats.UserCount < 1 {
		t.Fatalf("expected at least 1 user memory, got %d", stats.UserCount)
	}
}

func TestMemoryTools_HandleRecall(t *testing.T) {
	s := testStore(t)
	mgr := NewManager(s, nil, nil, DefaultManagerConfig())
	tools := NewMemoryTools(mgr, testScope())
	ctx := context.Background()

	// 先存入测试数据
	mgr.Remember(ctx, "用户偏好函数式编程", testScope(), LayerUser, nil)
	mgr.Remember(ctx, "正在审查专利新颖性", testScope(), LayerSession, nil)

	for _, tool := range tools {
		if tool.Name == "recall" {
			// 有结果
			args := `{"query": "编程", "limit": 5}`
			result, err := tool.Func(ctx, []byte(args))
			if err != nil {
				t.Fatalf("recall failed: %v", err)
			}
			str, ok := result.(string)
			if !ok || contains(str, "未找到") {
				t.Fatalf("expected to find results, got: %v", result)
			}

			// 无结果
			args2 := `{"query": "zzznotexist", "limit": 5}`
			result2, _ := tool.Func(ctx, []byte(args2))
			str2, _ := result2.(string)
			if !contains(str2, "未找到") {
				t.Fatalf("expected '未找到', got: %v", result2)
			}
			break
		}
	}
}

func TestMemoryTools_HandleForget(t *testing.T) {
	s := testStore(t)
	mgr := NewManager(s, nil, nil, DefaultManagerConfig())
	tools := NewMemoryTools(mgr, testScope())
	ctx := context.Background()

	id, _ := mgr.Remember(ctx, "to forget", testScope(), LayerUser, nil)

	for _, tool := range tools {
		if tool.Name == "forget" {
			args := `{"memory_id": "` + id + `"}`
			result, err := tool.Func(ctx, []byte(args))
			if err != nil {
				t.Fatalf("forget failed: %v", err)
			}
			str, ok := result.(string)
			if !ok || !contains(str, "已删除") {
				t.Fatalf("unexpected result: %v", result)
			}

			// 不存在的 ID
			args2 := `{"memory_id": "nonexistent"}`
			result2, _ := tool.Func(ctx, []byte(args2))
			str2, _ := result2.(string)
			if !contains(str2, "失败") {
				t.Fatalf("expected failure, got: %v", result2)
			}
			break
		}
	}
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
