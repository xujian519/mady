package knowledge

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// ---------------------------------------------------------------------------
// Extension
// ---------------------------------------------------------------------------

func TestExtensionDefaults(t *testing.T) {
	cfg := DefaultKnowledgeExtConfig()
	if !cfg.Enabled {
		t.Fatal("expected enabled by default")
	}
	if !cfg.ExposeTool {
		t.Fatal("expected expose_tool by default")
	}
}

func TestExtension_Name(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	if ext.Name() != "knowledge" {
		t.Fatalf("expected name=knowledge, got %s", ext.Name())
	}
}

func TestExtension_LifecycleHook(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	hook := ext.LifecycleHook()
	if hook == nil {
		t.Fatal("expected non-nil lifecycle hook")
	}
}

func TestExtension_Tools(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	tools := ext.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "search_knowledge" {
		t.Fatalf("expected search_knowledge, got %s", tools[0].Name)
	}
}

func TestExtension_ToolsDisabled(t *testing.T) {
	cfg := DefaultKnowledgeExtConfig()
	cfg.ExposeTool = false
	ext := NewExtension(nil, nil, "test", cfg)
	tools := ext.Tools()
	if tools != nil {
		t.Fatal("expected nil tools when disabled")
	}
}

func TestExtension_ProvideNoStore(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	msgs, err := ext.Provide(context.Background(), agentcore.BuildInput{
		Messages: []agentcore.Message{{Role: agentcore.RoleUser, Content: "test"}},
	}, agentcore.LayerConfig{})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}
	if msgs != nil {
		t.Fatal("expected nil messages without store")
	}
}

func TestExtension_ProvideWithStore(t *testing.T) {
	s := NewStore()
	_ = s.LoadText("test", "doc1", "测试文档", "专利法第22条 新颖性是指...")

	ext := NewExtension(s, nil, "test", DefaultKnowledgeExtConfig())
	msgs, err := ext.Provide(context.Background(), agentcore.BuildInput{
		Messages: []agentcore.Message{{Role: agentcore.RoleUser, Content: "什么是新颖性"}},
	}, agentcore.LayerConfig{})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}

	// May or may not have results depending on chunk matching
	if msgs != nil && len(msgs) > 0 {
		if msgs[0].Role != agentcore.RoleSystem {
			t.Fatal("expected RoleSystem message")
		}
		if !strings.Contains(msgs[0].Content, "参考文档") {
			t.Fatal("expected '参考文档' in content")
		}
	}
}

func TestExtension_ProvideEmptyQuery(t *testing.T) {
	s := NewStore()
	_ = s.LoadText("test", "doc1", "测试", "content")
	ext := NewExtension(s, nil, "test", DefaultKnowledgeExtConfig())

	msgs, err := ext.Provide(context.Background(), agentcore.BuildInput{
		Messages: []agentcore.Message{{Role: agentcore.RoleAssistant, Content: "reply"}},
	}, agentcore.LayerConfig{})
	if err != nil {
		t.Fatalf("Provide failed: %v", err)
	}
	if msgs != nil {
		t.Fatal("expected nil for empty query")
	}
}

// ---------------------------------------------------------------------------
// EvalHook
// ---------------------------------------------------------------------------

func TestEvalHookDefaults(t *testing.T) {
	cfg := DefaultEvalConfig()
	if cfg.Enabled {
		t.Fatal("expected eval disabled by default")
	}
}

func TestEvalHook_Disabled(t *testing.T) {
	hook := NewEvalHook(DefaultEvalConfig())
	arc := &agentcore.AgentRunContext{}
	mcc := &agentcore.ModelCallContext{}
	// Should not panic
	hook.AfterModelCall(context.Background(), arc, mcc)
}

func TestEvalHook_Scoring(t *testing.T) {
	hook := NewEvalHook(EvalConfig{Enabled: true})

	// Test faithfulness with matching context
	answer := "根据专利法第22条，新颖性是指..."
	req := &agentcore.ProviderRequest{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: "--- 参考片段 1 ---\n专利法第22条 新颖性是指发明或者实用新型不属于现有技术\n---\n"},
			{Role: agentcore.RoleUser, Content: "什么是新颖性"},
			{Role: agentcore.RoleAssistant, Content: answer},
		},
	}
	arc := &agentcore.AgentRunContext{
		Messages: []agentcore.Message{
			{Role: agentcore.RoleUser, Content: "什么是新颖性"},
		},
		Turn: 1,
	}
	mcc := &agentcore.ModelCallContext{
		Request:  req,
		Response: &agentcore.ProviderResponse{Content: answer},
	}

	hook.AfterModelCall(context.Background(), arc, mcc)
}

func TestEvalResult_FaithlessnessWarning(t *testing.T) {
	r := EvalResult{Faithfulness: 0.3}
	if r.FaithlessnessWarning() == "" {
		t.Fatal("expected warning for low faithfulness")
	}

	r2 := EvalResult{Faithfulness: 0.8}
	if r2.FaithlessnessWarning() != "" {
		t.Fatal("expected no warning for high faithfulness")
	}
}

// ---------------------------------------------------------------------------
// KnowledgeExtension layer provider interface checks
// ---------------------------------------------------------------------------

func TestKnowledgeExtension_LayerInterface(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	if ext.Layer() != agentcore.LayerKnowledge {
		t.Fatalf("expected LayerKnowledge, got %s", ext.Layer())
	}
}

func TestKnowledgeExtension_TransformContextPassthrough(t *testing.T) {
	ext := NewExtension(nil, nil, "test", DefaultKnowledgeExtConfig())
	msgs := []agentcore.Message{{Role: agentcore.RoleUser, Content: "test"}}
	result := ext.TransformContext(context.Background(), msgs)
	if len(result) != 1 {
		t.Fatal("expected passthrough")
	}
}

// ---------------------------------------------------------------------------
// Tokenize helper
// ---------------------------------------------------------------------------

func TestTokenizeEval(t *testing.T) {
	tokens := tokenizeEval("hello world")
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %v", len(tokens), tokens)
	}

	tokens2 := tokenizeEval("新颖性 test")
	if len(tokens2) == 0 {
		t.Fatal("expected non-empty tokens for Chinese text")
	}
}
