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

// TestContextBuilderWithMemoryProvider 验证 ContextBuilder + Memory LayerProvider 集成。
func TestContextBuilderWithMemoryProvider(t *testing.T) {
	ctx := context.Background()
	store := memory.NewInMemoryStore()
	mgr := memory.NewManager(store, nil, nil, memory.DefaultManagerConfig())
	scope := memory.MemoryScope{UserID: "integration_test"}
	mgr.Remember(ctx, "用户是专利代理人", scope, memory.LayerUser, nil)

	// 创建 ContextBuilder
	cfg := agentcore.DefaultContextBuilderConfig()
	cfg.Enabled = true
	cfg.DefaultLayerConfigs[agentcore.LayerMemory] = agentcore.DefaultLayerConfig(agentcore.LayerMemory)

	ext := memory.NewExtension(mgr, scope, memory.DefaultExtensionConfig())
	cfg.Providers = append(cfg.Providers, ext)

	builder := agentcore.NewDefaultContextBuilder(cfg)
	input := agentcore.BuildInput{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: "你是助手"},
			{Role: agentcore.RoleUser, Content: "我的职业是什么？"},
		},
		ContextWindow: 128000,
	}

	output := builder.Build(ctx, input)
	if len(output.Messages) <= 2 {
		t.Fatal("expected builder to produce more than input messages")
	}

	// 验证记忆层注入
	found := false
	for _, m := range output.Messages {
		if m.Role == agentcore.RoleSystem && searchStr(m.Content, "专利代理人") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected memory about patent agent to be in built context")
	}
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
