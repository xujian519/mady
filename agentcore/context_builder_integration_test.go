package agentcore

import (
	"context"
	"testing"
)

// TestContextBuilderFullPipeline 验证 ContextBuilder 完整管线：
// 多个 LayerProvider 同时注册时，各层内容被正确组装。
func TestContextBuilderFullPipeline(t *testing.T) {
	ctx := context.Background()

	// 创建两个模拟的 LayerProvider
	providers := []LayerProvider{
		&testIntegrationProvider{
			layer:   LayerMemory,
			content: "[Memory] 用户偏好使用中文",
			enabled: true,
		},
		&testIntegrationProvider{
			layer:   LayerKnowledge,
			content: "[Knowledge] 专利法第22条：新颖性",
			enabled: true,
		},
	}

	cfg := DefaultContextBuilderConfig()
	cfg.Enabled = true
	cfg.Providers = providers
	// 确保两层的 enabled 为 true
	cfg.DefaultLayerConfigs[LayerMemory] = LayerConfig{Enabled: true, MaxTokens: 5000}
	cfg.DefaultLayerConfigs[LayerKnowledge] = LayerConfig{Enabled: true, MaxTokens: 5000}

	b := NewDefaultContextBuilder(cfg)

	input := BuildInput{
		Messages: []Message{
			{Role: RoleSystem, Content: "你是专家助手"},
			{Role: RoleUser, Content: "请分析这个专利"},
		},
		ContextWindow: 128000,
	}
	output := b.Build(ctx, input)

	// System 消息应保留
	foundSystem := false
	for _, m := range output.Messages {
		if m.Role == RoleSystem && m.Content == "你是专家助手" {
			foundSystem = true
			break
		}
	}
	if !foundSystem {
		t.Fatal("expected original system message preserved")
	}

	// 检查 memory 内容是否注入
	foundMemory := false
	foundKnowledge := false
	for _, m := range output.Messages {
		if m.Role == RoleSystem {
			if contains(m.Content, "Memory") {
				foundMemory = true
			}
			if contains(m.Content, "Knowledge") {
				foundKnowledge = true
			}
		}
	}
	if !foundMemory {
		t.Fatal("expected memory layer content")
	}
	if !foundKnowledge {
		t.Fatal("expected knowledge layer content")
	}
}

// TestContextBuilderTokenBudget 验证多层的 token 预算分配。
func TestContextBuilderTokenBudget(t *testing.T) {
	ctx := context.Background()

	providers := []LayerProvider{
		&testIntegrationProvider{
			layer:   LayerMemory,
			content: longString(5000, "M"),
			enabled: true,
		},
	}

	cfg := DefaultContextBuilderConfig()
	cfg.Enabled = true
	cfg.DefaultLayerConfigs[LayerMemory] = LayerConfig{Enabled: true, MaxTokens: 500}
	cfg.Providers = providers

	b := NewDefaultContextBuilder(cfg)
	input := BuildInput{
		Messages:      []Message{{Role: RoleUser, Content: "test"}},
		ContextWindow: 128000,
	}
	output := b.Build(ctx, input)

	// 验证 token 预算生效：memory 内容应被截断
	if output.Usage.ByLayer[LayerMemory] > 600 {
		t.Fatalf("expected memory tokens capped at ~500, got %d", output.Usage.ByLayer[LayerMemory])
	}
}

// TestContextBuilderSystemPromptSegments 验证 SystemPromptConfig 分段。
func TestContextBuilderSystemPromptSegments(t *testing.T) {
	cfg := SystemPromptConfig{
		StaticPrefix:  "角色: 专家",
		ToolIndex:     "可用: search, calc",
		DynamicSuffix: "日期: 2026-07-11",
		Segments: []SystemPromptSegment{
			{Name: "rules", Content: "规则: 请用中文回答", Priority: 1},
		},
	}
	b := NewSystemPromptBuilder(cfg)
	prompt := b.Build()

	if !contains(prompt, "角色: 专家") {
		t.Fatal("expected static prefix")
	}
	if !contains(prompt, "规则") {
		t.Fatal("expected segment content")
	}
	if !contains(prompt, "日期") {
		t.Fatal("expected dynamic suffix")
	}

	// 验证 BuildSegments 返回的消息列表
	msgs := b.BuildSegments()
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 segment messages, got %d", len(msgs))
	}
}

// --- 辅助 ---

type testIntegrationProvider struct {
	layer   ContextLayer
	content string
	enabled bool
}

func (p *testIntegrationProvider) Layer() ContextLayer { return p.layer }

func (p *testIntegrationProvider) Provide(_ context.Context, _ BuildInput, _ LayerConfig) ([]Message, error) {
	if !p.enabled || p.content == "" {
		return nil, nil
	}
	return []Message{{Role: RoleSystem, Content: p.content}}, nil
}

func longString(length int, char string) string {
	s := ""
	for i := 0; i < length; i++ {
		s += char
	}
	return s
}
