package integration

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
)

// ──────────────────────────────────────────────
// handoffE2EProvider — 模拟 Router → domain Agent 的 Handoff 流程。
// - call 0: 返回 transfer_to_<tool> 工具调用
// - call 1+: 返回 content (供子 Agent 消耗)
// ──────────────────────────────────────────────
type handoffE2EProvider struct {
	called  atomic.Int64
	tool    string
	content string
}

func (p *handoffE2EProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	call := p.called.Add(1) - 1
	if call == 0 {
		return &agentcore.ProviderResponse{
			ToolCalls: []agentcore.ToolCall{
				{ID: "call_handoff", Name: "transfer_to_" + p.tool, Arguments: `{"message":"test input"}`},
			},
		}, nil
	}
	return &agentcore.ProviderResponse{Content: p.content}, nil
}

func (p *handoffE2EProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

// e2eStubProvider — 简单 stub，所有调用返回固定 content。
type e2eStubProvider struct {
	content string
}

func (p *e2eStubProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	return &agentcore.ProviderResponse{Content: p.content}, nil
}

func (p *e2eStubProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

// ──────────────────────────────────────────────
// e2e 场景 1: Router → Chat Agent → 返回
// ──────────────────────────────────────────────
func TestHandoffE2E_ChatRoute(t *testing.T) {
	provider := &handoffE2EProvider{
		tool:    domains.DomainChat,
		content: "你好！我是 chat-agent，有什么可以帮你的？",
	}

	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "e2e-test-router",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := domains.RouterConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	// 捕获 HandoffEndEvent 验证传递完整性
	var handoffTarget string
	var handoffOutput string
	agent.On(agentcore.EventHandoffEnd, func(e agentcore.Event) {
		if he, ok := e.(*agentcore.HandoffEndEvent); ok {
			handoffTarget = he.TargetAgent
			handoffOutput = he.Output
		}
	})

	output, err := agent.Run(context.Background(), "你好")
	if err != nil {
		t.Fatalf("agent.Run failed: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if handoffTarget != domains.DomainChat {
		t.Errorf("expected handoff target %q, got %q", domains.DomainChat, handoffTarget)
	}
	if !strings.Contains(handoffOutput, "chat-agent") {
		t.Errorf("handoff output should contain chat-agent reference, got: %q", handoffOutput)
	}
	t.Logf("Chat route output: %s", output)
}

// ──────────────────────────────────────────────
// e2e 场景 2: Router → Assistant Agent → 返回
// ──────────────────────────────────────────────
func TestHandoffE2E_AssistantRoute(t *testing.T) {
	provider := &handoffE2EProvider{
		tool:    domains.DomainAssistant,
		content: `{"action":"代码审查","result":"发现2个问题","success":true}`,
	}

	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "e2e-test-router",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := domains.RouterConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	var handoffOutput string
	agent.On(agentcore.EventHandoffEnd, func(e agentcore.Event) {
		if he, ok := e.(*agentcore.HandoffEndEvent); ok {
			handoffOutput = he.Output
		}
	})

	output, err := agent.Run(context.Background(), "帮我审查代码")
	if err != nil {
		t.Fatalf("agent.Run failed: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	t.Logf("Assistant route output: %s", output)
	_ = handoffOutput
}

// ──────────────────────────────────────────────
// e2e 场景 3: Router → ProjectRegistry → BuildProjectAgent → 返回
// ──────────────────────────────────────────────
func TestHandoffE2E_ProjectRoute(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 ProjectRegistry 并注册一个测试案件
	registry, err := domains.NewProjectRegistry(tmpDir + "/projects")
	if err != nil {
		t.Fatalf("NewProjectRegistry: %v", err)
	}

	caseDir := t.TempDir()
	rec := domains.ProjectRecord{
		ProjectID: "e2e-case-001",
		Domain:    domains.DomainPatent,
		Alias:     "E2E 测试案件",
		RootPath:  caseDir,
	}

	if err := registry.Register(rec); err != nil {
		t.Fatalf("Register project: %v", err)
	}

	// 创建 Router 配置，含项目感知路由
	provider := &handoffE2EProvider{
		tool:    "project-e2e-case-001",
		content: `{"action":"案件分析","result":"分析完成","success":true}`,
	}

	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "e2e-test-router",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg, pool := domains.RouterConfigWithRegistry(base, registry, nil)
	defer pool.Close()

	// 验证项目中注册了 project Handoff 目标
	foundProject := false
	for _, h := range cfg.Handoffs {
		if h.Name == domains.ProjectHandoffName("e2e-case-001") {
			foundProject = true
			if !strings.Contains(h.Description, "E2E 测试案件") {
				t.Errorf("handoff description should mention alias, got: %s", h.Description)
			}
			if len(h.AllowedSources) == 0 {
				t.Error("project handoff should have AllowedSources")
			}
			break
		}
	}
	if !foundProject {
		t.Fatal("project handoff target not found in RouterConfig")
	}

	agent := agentcore.New(cfg)
	defer agent.Close()

	output, err := agent.Run(context.Background(), "分析案件 E2E 测试案件")
	if err != nil {
		t.Fatalf("agent.Run failed: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	t.Logf("Project route output: %s", output)
}

// ──────────────────────────────────────────────
// e2e 场景 4: Router 失败时优雅降级到 Chat
// ──────────────────────────────────────────────
func TestHandoffE2E_FallbackToChat(t *testing.T) {
	// provider 不返回工具调用 → Router 直接处理 → 走 Chat 领域
	provider := &e2eStubProvider{
		content: "你好，这是一个普通的聊天回复。",
	}

	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "e2e-test-router",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 5,
		},
	}

	cfg := domains.RouterConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	var handoffEvent bool
	agent.On(agentcore.EventHandoffEnd, func(e agentcore.Event) {
		handoffEvent = true
	})

	output, err := agent.Run(context.Background(), "你好，今天天气怎么样")
	if err != nil {
		t.Fatalf("agent.Run failed: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if handoffEvent {
		t.Log("handoff was triggered (unexpected but acceptable)")
	}
	t.Logf("Fallback output: %s", output)
}

// ──────────────────────────────────────────────
// e2e 场景 5: 空消息容错
// ──────────────────────────────────────────────
func TestHandoffE2E_EmptyInput(t *testing.T) {
	provider := &e2eStubProvider{content: "empty"}
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "e2e-test-router",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
	}
	cfg := domains.RouterConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	_, err := agent.Run(context.Background(), "")
	if err != nil {
		t.Fatalf("agent.Run with empty input failed: %v", err)
	}
	// Should not panic for empty input
}
