package memory_test

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/memory"
)

// TestMemoryAgentIntegration 验证记忆在 Agent 管线中的端到端流程：
// 用户对话 → 提取记忆 → 检索注入。
func TestMemoryAgentIntegration(t *testing.T) {
	ctx := context.Background()

	// 1. 创建记忆存储和管理器
	store := memory.NewInMemoryStore()
	mgr := memory.NewManager(store, nil, nil, memory.DefaultManagerConfig())
	scope := memory.MemoryScope{UserID: "test_user"}

	// 2. 创建 MemoryExtension
	extCfg := memory.DefaultExtensionConfig()
	extCfg.AutoExtract = false // Phase 1 使用显式 Remember
	ext := memory.NewExtension(mgr, scope, extCfg)

	// 3. 模拟：Agent 记住用户信息（TransformContext 返回修改后的消息列表）
	_ = ext.TransformContext(ctx, nil) // nil 输入，不应 panic

	// 4. 模拟用户输入，通过 Manager 直接存入记忆
	mgr.Remember(ctx, "用户偏好使用 Go 语言", scope, memory.LayerUser, nil)

	// 5. 模拟：Agent 收到新问题，通过 TransformContext 注入记忆
	msgs := []agentcore.Message{
		{Role: agentcore.RoleSystem, Content: "你是助手"},
		{Role: agentcore.RoleUser, Content: "我喜欢什么编程语言？"},
	}
	result := ext.TransformContext(ctx, msgs)

	// 6. 验证记忆上下文被注入
	if len(result) <= 2 {
		t.Fatal("expected memory context to be injected")
	}
	foundMemory := false
	for _, m := range result {
		if m.Role == agentcore.RoleSystem && searchStr(m.Content, "Go") {
			foundMemory = true
			break
		}
	}
	if !foundMemory {
		t.Fatal("expected memory about Go to be in context")
	}
}

// TestMemorySessionScope 验证会话级作用域隔离：
// 不同会话的记忆不会互相干扰。
func TestMemorySessionScope(t *testing.T) {
	ctx := context.Background()
	store := memory.NewInMemoryStore()
	mgr := memory.NewManager(store, nil, nil, memory.DefaultManagerConfig())

	// 两个不同会话
	scope1 := memory.MemoryScope{UserID: "user1", SessionID: "session_a"}
	scope2 := memory.MemoryScope{UserID: "user1", SessionID: "session_b"}

	mgr.Remember(ctx, "会话A的关键信息", scope1, memory.LayerSession, nil)
	mgr.Remember(ctx, "会话B的不同信息", scope2, memory.LayerSession, nil)

	// 验证：在会话A中只看到会话A的记忆
	filterA := memory.MemoryFilter{UserID: "user1", SessionID: "session_a", TopK: 10}
	results, err := mgr.Search(ctx, "会话", filterA)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	for _, r := range results {
		if searchStr(r.Entry.Content, "B") {
			t.Fatal("session_a should not see session_b memories")
		}
	}

	// 验证 User 层记忆跨会话可见（搜索时仅按 UserID + Layer 搜索）
	mgr.Remember(ctx, "用户偏好中文", scope1, memory.LayerUser, nil)

	filterUser := memory.MemoryFilter{UserID: "user1", Layer: memory.LayerUser, TopK: 10}
	results2, _ := mgr.Search(ctx, "中文", filterUser)
	if len(results2) == 0 {
		t.Fatal("user memories should be visible across sessions")
	}

	// User 层记忆不限定会话，但带 SessionID filter 时会按 scope 过滤
	// 这是正确的隔离行为：明确指定 SessionID 时只返回该会话的记忆
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestMemoryProjectScopeIsolation 验证项目级作用域隔离：
// 同一用户在不同 project/session 之间不能通过 TransformContext / Provide / recall 工具看到对方记忆。
func TestMemoryProjectScopeIsolation(t *testing.T) {
	ctx := context.Background()
	store := memory.NewInMemoryStore()
	mgr := memory.NewManager(store, nil, nil, memory.DefaultManagerConfig())

	scopeA := memory.MemoryScope{UserID: "user1", AgentID: "mady-agent", SessionID: "session_a", ProjectID: "project_a"}
	scopeB := memory.MemoryScope{UserID: "user1", AgentID: "mady-agent", SessionID: "session_b", ProjectID: "project_b"}

	// 在项目 A 存入一条长期记忆
	_, err := mgr.Remember(ctx, "项目A的保密技术方案", scopeA, memory.LayerLongTerm, nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// 使用项目 B 的扩展调用 TransformContext
	extCfg := memory.DefaultExtensionConfig()
	extCfg.AutoExtract = false
	extB := memory.NewExtension(mgr, scopeB, extCfg)

	msgs := []agentcore.Message{
		{Role: agentcore.RoleSystem, Content: "你是助手"},
		{Role: agentcore.RoleUser, Content: "我们的技术方案是什么？"},
	}
	result := extB.TransformContext(ctx, msgs)

	for _, m := range result {
		if m.Role == agentcore.RoleSystem && searchStr(m.Content, "项目A") {
			t.Fatal("project_b extension should not see project_a memories via TransformContext")
		}
	}

	// 使用项目 B 的扩展调用 Provide（ContextBuilder 路径）
	input := agentcore.BuildInput{
		Messages:      msgs,
		ContextWindow: 128000,
	}
	provided, err := extB.Provide(ctx, input, agentcore.DefaultLayerConfig(agentcore.LayerMemory))
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}
	for _, m := range provided {
		if m.Role == agentcore.RoleSystem && searchStr(m.Content, "项目A") {
			t.Fatal("project_b extension should not see project_a memories via Provide")
		}
	}

	// 使用项目 B 的 recall 工具
	tools := extB.Tools()
	var recall *agentcore.Tool
	for _, tool := range tools {
		if tool.Name == "recall" {
			recall = tool
			break
		}
	}
	if recall == nil {
		t.Fatal("recall tool not found")
	}
	resp, err := recall.Func(ctx, []byte(`{"query":"技术方案"}`))
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}
	if s, ok := resp.(string); ok && searchStr(s, "项目A") {
		t.Fatal("project_b recall tool should not see project_a memories")
	}
}
