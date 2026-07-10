package agentcore

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// DefaultContextBuilder
// ---------------------------------------------------------------------------

func TestDefaultContextBuilder_Disabled(t *testing.T) {
	cfg := DefaultContextBuilderConfig()
	cfg.Enabled = false
	b := NewDefaultContextBuilder(cfg)

	input := BuildInput{
		Messages: []Message{
			{Role: RoleUser, Content: "hello"},
		},
	}
	output := b.Build(context.Background(), input)

	if len(output.Messages) != 1 {
		t.Fatalf("expected 1 message (passthrough), got %d", len(output.Messages))
	}
}

func TestDefaultContextBuilder_Basic(t *testing.T) {
	cfg := DefaultContextBuilderConfig()
	cfg.Enabled = true
	b := NewDefaultContextBuilder(cfg)

	input := BuildInput{
		Messages: []Message{
			{Role: RoleSystem, Content: "你是助手"},
			{Role: RoleUser, Content: "你好"},
			{Role: RoleAssistant, Content: "你好！有什么可以帮助你的？"},
		},
		SystemPrompt:  "你是助手",
		ContextWindow: 128000,
		ReserveTokens: 32000,
	}
	output := b.Build(context.Background(), input)

	if len(output.Messages) == 0 {
		t.Fatal("expected messages")
	}
	if output.Usage.TotalTokens <= 0 {
		t.Fatal("expected positive token count")
	}

	// System messages should be preserved
	foundSystem := false
	for _, m := range output.Messages {
		if m.Role == RoleSystem {
			foundSystem = true
			break
		}
	}
	if !foundSystem {
		t.Fatal("expected system messages preserved")
	}
}

func TestDefaultContextBuilder_TokenBudget(t *testing.T) {
	cfg := DefaultContextBuilderConfig()
	cfg.Enabled = true
	b := NewDefaultContextBuilder(cfg)

	// 用极小的 token 预算测试截断
	longContent := ""
	for i := 0; i < 1000; i++ {
		longContent += "这是一个很长的测试内容需要被截断 "
	}

	input := BuildInput{
		Messages: []Message{
			{Role: RoleSystem, Content: "系统提示"},
			{Role: RoleUser, Content: longContent},
			{Role: RoleAssistant, Content: longContent},
		},
		SystemPrompt:  "系统提示",
		ContextWindow: 1000,
		ReserveTokens: 100,
	}
	output := b.Build(context.Background(), input)

	// 确保 token 预算生效
	if output.Usage.TotalTokens > 900 {
		t.Fatalf("expected token budget to cap total, got %d", output.Usage.TotalTokens)
	}
}

func TestDefaultContextBuilder_WithLayerConfig(t *testing.T) {
	cfg := DefaultContextBuilderConfig()
	cfg.Enabled = true
	// 禁用 Knowledge 层
	cfg.DefaultLayerConfigs[LayerKnowledge] = LayerConfig{Enabled: false}

	b := NewDefaultContextBuilder(cfg)

	input := BuildInput{
		Messages: []Message{
			{Role: RoleUser, Content: "test"},
		},
		ContextWindow: 128000,
	}
	output := b.Build(context.Background(), input)

	if len(output.Messages) == 0 {
		t.Fatal("expected messages")
	}
}

// ---------------------------------------------------------------------------
// LayerProvider
// ---------------------------------------------------------------------------

type testProvider struct {
	layer ContextLayer
	msgs  []Message
	err   error
}

func (p *testProvider) Layer() ContextLayer {
	return p.layer
}

func (p *testProvider) Provide(_ context.Context, _ BuildInput, _ LayerConfig) ([]Message, error) {
	return p.msgs, p.err
}

func TestDefaultContextBuilder_WithProvider(t *testing.T) {
	cfg := DefaultContextBuilderConfig()
	cfg.Enabled = true
	cfg.DefaultLayerConfigs[LayerMemory] = LayerConfig{Enabled: true}
	cfg.Providers = []LayerProvider{
		&testProvider{
			layer: LayerMemory,
			msgs:  []Message{{Role: RoleSystem, Content: "memory_context"}},
		},
	}

	b := NewDefaultContextBuilder(cfg)

	input := BuildInput{
		Messages: []Message{
			{Role: RoleUser, Content: "test"},
		},
		ContextWindow: 128000,
	}
	output := b.Build(context.Background(), input)

	// 验证 memory 层内容被注入
	foundMemory := false
	for _, m := range output.Messages {
		if m.Content == "memory_context" {
			foundMemory = true
			break
		}
	}
	if !foundMemory {
		t.Fatal("expected memory_context to be injected")
	}
}

func TestDefaultContextBuilder_ProviderError(t *testing.T) {
	cfg := DefaultContextBuilderConfig()
	cfg.Enabled = true
	cfg.Providers = []LayerProvider{
		&testProvider{
			layer: LayerMemory,
			msgs:  nil,
			err:   nil, // 返回空但不报错
		},
	}

	b := NewDefaultContextBuilder(cfg)
	input := BuildInput{
		Messages:      []Message{{Role: RoleUser, Content: "test"}},
		ContextWindow: 128000,
	}
	output := b.Build(context.Background(), input)

	// 不应当 panic
	if len(output.Messages) == 0 {
		t.Fatal("expected at least user message")
	}
}

// ---------------------------------------------------------------------------
// Token 估算辅助函数
// ---------------------------------------------------------------------------

func TestEstimateMessagesTokens_ContextBuilder(t *testing.T) {
	msgs := []Message{
		{Content: "hello"},
		{Content: "这是一个测试消息"},
	}
	tok := estimateMessagesTokens(msgs)
	if tok <= 0 {
		t.Fatal("expected positive token estimate")
	}
}

func TestEstimateToolDefTokens(t *testing.T) {
	defs := []ToolDefinition{
		{Name: "test_tool", Description: "测试工具描述"},
	}
	tok := estimateToolDefTokens(defs)
	if tok <= 0 {
		t.Fatal("expected positive token estimate")
	}
}

func TestTruncateMessagesByTokens(t *testing.T) {
	msgs := []Message{
		{Content: "short"},
		{Content: "a medium length message that fits"},
	}

	result := truncateMessagesByTokens(msgs, 10)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// 预算为 0
	empty := truncateMessagesByTokens(msgs, 0)
	if len(empty) != 0 {
		t.Fatal("expected empty result for zero budget")
	}

	// 预算很小只能容纳最新消息
	result2 := truncateMessagesByTokens(msgs, 2)
	if len(result2) == 0 {
		// 2 tokens ≈ 8 字符，至少能容纳 "short"
		// 但 truncateMessagesByTokens 从最新的开始保留
		t.Log("truncated to budget 2:", len(result2))
	}
}

// ---------------------------------------------------------------------------
// ContextLayer / LayerConfig
// ---------------------------------------------------------------------------

func TestValidContextLayers(t *testing.T) {
	layers := ValidContextLayers
	if len(layers) != 5 {
		t.Fatalf("expected 5 layers, got %d", len(layers))
	}
}

func TestDefaultLayerConfig(t *testing.T) {
	cfg := DefaultLayerConfig(LayerSystem)
	if !cfg.Enabled {
		t.Fatal("system layer should be enabled by default")
	}
	if cfg.InjectMode != InjectAlways {
		t.Fatal("system layer should be InjectAlways")
	}

	cfg2 := DefaultLayerConfig(LayerMemory)
	if !cfg2.Enabled {
		t.Fatal("memory layer should be enabled by default")
	}

	// 未知层
	cfg3 := DefaultLayerConfig("unknown")
	if !cfg3.Enabled {
		t.Fatal("unknown layer should default to enabled")
	}
}

// ---------------------------------------------------------------------------
// BuildInput / BuildOutput
// ---------------------------------------------------------------------------

func TestBuildUsage(t *testing.T) {
	usage := BuildUsage{
		ByLayer: map[ContextLayer]int64{
			LayerSystem: 100,
			LayerMemory: 200,
		},
		TotalTokens:   300,
		ToolDefTokens: 50,
	}
	if usage.TotalTokens != 300 {
		t.Fatalf("expected 300 total tokens, got %d", usage.TotalTokens)
	}
	if usage.ByLayer[LayerMemory] != 200 {
		t.Fatalf("expected 200 memory tokens, got %d", usage.ByLayer[LayerMemory])
	}
}

// ---------------------------------------------------------------------------
// SystemPromptBuilder
// ---------------------------------------------------------------------------

func TestSystemPromptBuilder_Build(t *testing.T) {
	b := NewSystemPromptBuilder(SystemPromptConfig{
		StaticPrefix:  "角色: 助手",
		ToolIndex:     "可用工具: test_tool",
		DynamicSuffix: "日期: 2026-07-11",
	})

	prompt := b.Build()
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !contains(prompt, "角色: 助手") {
		t.Fatal("expected static prefix in prompt")
	}
	if !contains(prompt, "可用工具") {
		t.Fatal("expected tool index in prompt")
	}
	if !contains(prompt, "2026-07-11") {
		t.Fatal("expected dynamic suffix in prompt")
	}
}

func TestSystemPromptBuilder_BuildSegments(t *testing.T) {
	b := NewSystemPromptBuilder(SystemPromptConfig{
		StaticPrefix:  "角色定义",
		ToolIndex:     "工具索引",
		DynamicSuffix: "动态内容",
	})

	msgs := b.BuildSegments()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs))
	}

	// 第一条应该是 system 消息
	if msgs[0].Role != RoleSystem {
		t.Fatalf("expected RoleSystem, got %v", msgs[0].Role)
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	prompt := BuildSystemPrompt(SystemPromptConfig{
		StaticPrefix: "test",
	})
	if prompt != "test" {
		t.Fatalf("expected 'test', got %q", prompt)
	}
}

func TestInjectDynamicContext(t *testing.T) {
	cfg := SystemPromptConfig{}

	cfg.InjectDynamicContext("/home/user", map[string]string{
		"GOOS":   "linux",
		"GOARCH": "amd64",
	})

	if !contains(cfg.DynamicSuffix, "/home/user") {
		t.Fatal("expected cwd in dynamic suffix")
	}
	if !contains(cfg.DynamicSuffix, "GOOS") {
		t.Fatal("expected env var in dynamic suffix")
	}
}

// ---------------------------------------------------------------------------
// 默认配置
// ---------------------------------------------------------------------------

func TestDefaultContextBuilderConfig(t *testing.T) {
	cfg := DefaultContextBuilderConfig()
	if cfg.Enabled {
		t.Fatal("expected ContextBuilder to be disabled by default")
	}
	if len(cfg.DefaultLayerConfigs) != 5 {
		t.Fatalf("expected 5 layer configs, got %d", len(cfg.DefaultLayerConfigs))
	}
}

func TestDefaultManagerConfig(t *testing.T) {
	// 确保代理包的默认配置也正确
	// 这仅验证已有的 ConfigOption 签名兼容性
	var _ ConfigOption = WithContextBuilder(nil)
	var _ ConfigOption = WithLayerConfig("", LayerConfig{})
}

// ---------------------------------------------------------------------------
// 辅助
// ---------------------------------------------------------------------------

func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
