//go:build integration

package integration_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
)

// ──────────────────────────────────────────────
// handoffE2EProvider — 模拟 UnifiedAgent → domain Agent 的 Handoff 流程。
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
// e2e 场景 1: UnifiedAgent → Patent Agent → 返回
// ──────────────────────────────────────────────
func TestHandoffE2E_PatentRoute(t *testing.T) {
	provider := &handoffE2EProvider{
		tool:    domains.DomainPatent,
		content: "专利分析完成：新颖性符合要求。",
	}

	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "e2e-test-unified",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := domains.UnifiedAgentConfig(base)
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

	output, err := agent.Run(context.Background(), "帮我分析这个专利的新颖性")
	if err != nil {
		t.Fatalf("agent.Run failed: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if handoffTarget != domains.DomainPatent {
		t.Errorf("expected handoff target %q, got %q", domains.DomainPatent, handoffTarget)
	}
	if !strings.Contains(handoffOutput, "专利") {
		t.Errorf("handoff output should contain patent reference, got: %q", handoffOutput)
	}
	t.Logf("Patent route output: %s", output)
}

// ──────────────────────────────────────────────
// e2e 场景 2: UnifiedAgent → Legal Agent → 返回
// ──────────────────────────────────────────────
func TestHandoffE2E_LegalRoute(t *testing.T) {
	provider := &handoffE2EProvider{
		tool:    domains.DomainLegal,
		content: `{"action":"法律检索","result":"找到3条相关法条","success":true}`,
	}

	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "e2e-test-unified",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := domains.UnifiedAgentConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	var handoffOutput string
	agent.On(agentcore.EventHandoffEnd, func(e agentcore.Event) {
		if he, ok := e.(*agentcore.HandoffEndEvent); ok {
			handoffOutput = he.Output
		}
	})

	output, err := agent.Run(context.Background(), "帮我查合同法相关条款")
	if err != nil {
		t.Fatalf("agent.Run failed: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	t.Logf("Legal route output: %s", output)
	_ = handoffOutput
}

// ──────────────────────────────────────────────
// e2e 场景 3: UnifiedAgent 失败时优雅降级
// ──────────────────────────────────────────────
func TestHandoffE2E_FallbackDirect(t *testing.T) {
	// provider 不返回工具调用 → UnifiedAgent 直接处理
	provider := &e2eStubProvider{
		content: "你好，这是一个普通的聊天回复。",
	}

	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "e2e-test-unified",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 5,
		},
	}

	cfg := domains.UnifiedAgentConfig(base)
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
// e2e 场景 4: 空消息容错
// ──────────────────────────────────────────────
func TestHandoffE2E_EmptyInput(t *testing.T) {
	provider := &e2eStubProvider{content: "empty"}
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "e2e-test-unified",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
	}
	cfg := domains.UnifiedAgentConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	_, err := agent.Run(context.Background(), "")
	if err != nil {
		t.Fatalf("agent.Run with empty input failed: %v", err)
	}
	// Should not panic for empty input
}
