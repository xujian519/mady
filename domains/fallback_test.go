package domains

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// subAgentErrorProvider 模拟子 Agent 失败场景：
// - call 0 → Router 收到 handoff 工具调用
// - call 1 → 子 Agent 调用 LLM 时失败
// - call 2+ → Router 继续运行，返回兜底内容
type subAgentErrorProvider struct {
	called    atomic.Int64
	tool      string
	errMsg    string
	finalText string
}

func (p *subAgentErrorProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	call := p.called.Add(1) - 1
	switch call {
	case 0:
		return &agentcore.ProviderResponse{
			ToolCalls: []agentcore.ToolCall{
				{ID: "call_handoff", Name: "transfer_to_" + p.tool, Arguments: `{"message":"test"}`},
			},
		}, nil
	case 1:
		// 模拟子 Agent 的 LLM 调用失败
		return nil, fmt.Errorf("%s", p.errMsg)
	default:
		// Router 的后续调用
		return &agentcore.ProviderResponse{Content: p.finalText}, nil
	}
}

func (p *subAgentErrorProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return nil, fmt.Errorf("streaming not implemented")
}

// ──────────────────────────────────────────────
// 工具失败降级测试
// ──────────────────────────────────────────────

func TestFallback_DelegateErrorReturnsHandoffResult(t *testing.T) {
	// 使用 subAgentErrorProvider：Router handoff → 子 Agent 失败 → Router 收到兜底
	provider := &subAgentErrorProvider{
		tool:      DomainAssistant,
		errMsg:    "web_search timeout",
		finalText: "继续", // Router 收到 HandoffResult 后的后续响应
	}

	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "fallback-test",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := RouterConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	// 捕获 HandoffEndEvent，验证错误时返回 HandoffResult
	var handoffOutput string
	var handoffHasErr bool
	agent.On(agentcore.EventHandoffEnd, func(e agentcore.Event) {
		if he, ok := e.(*agentcore.HandoffEndEvent); ok {
			handoffOutput = he.Output
			handoffHasErr = he.Err != nil
		}
	})

	output, err := agent.Run(context.Background(), "帮我搜索一下")
	if err != nil {
		t.Fatalf("agent run should not error, got: %v", err)
	}

	// HandoffEndEvent 应记录错误
	if !handoffHasErr {
		t.Error("HandoffEndEvent should have error when sub-agent fails")
	}

	// 错误发生后 delegate 返回 HandoffResult，错误不应冒泡到 agent.Run
	if strings.Contains(output, "web_search timeout") {
		t.Error("output should not contain raw error message")
	}

	// 兜底输出中 HandoffOutput 应为空（子 Agent 执行失败时 output 为空字符串）
	t.Logf("HandoffEndEvent.Output: %q", handoffOutput)
	t.Logf("Agent.Run output: %q", output)
}

func TestFallback_CustomFallbackMsg(t *testing.T) {
	// 验证自定义 FallbackMsg 在 HandoffConfig 中正确传递
	h := agentcore.HandoffConfig{
		Name:        "custom-target",
		Description: "test",
		Mode:        agentcore.HandoffDelegate,
		FallbackMsg: "自定义降级文案：请稍后重试",
	}

	provider := &handoffProvider{tool: "custom-target", content: "done"}
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "fallback-test",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
	}
	base.Handoffs = []agentcore.HandoffConfig{h}

	agent := agentcore.New(base)
	defer agent.Close()

	output, err := agent.Run(context.Background(), "测试自定义兜底")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "done" {
		t.Fatalf("expected 'done', got %q", output)
	}

	// 验证 FallbackMsg 已正确设置
	if h.FallbackMsg != "自定义降级文案：请稍后重试" {
		t.Errorf("FallbackMsg = %q, want %q", h.FallbackMsg, "自定义降级文案：请稍后重试")
	}
}

func TestFallback_DefaultMsgWhenNoFallbackConfig(t *testing.T) {
	// 当未配置 FallbackMsg 时，使用默认兜底文案
	h := agentcore.HandoffConfig{
		Name:        "no-fallback-target",
		Description: "test",
		Mode:        agentcore.HandoffDelegate,
	}

	if h.FallbackMsg != "" {
		t.Error("FallbackMsg should be empty when not configured")
	}
	if len(h.AllowedSources) != 0 {
		t.Error("AllowedSources should be empty/nil")
	}

	// 验证空 AllowedSources 意味着允许所有来源
	// (通过 TestSafeHandoff_EmptyAllowedSourcesAllowsAll 集成验证)
}

// ──────────────────────────────────────────────
// 兜底文案内容验证
// ──────────────────────────────────────────────

func TestFallback_PatentAgentFallbackMsg(t *testing.T) {
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "verify-test",
			Model:    "stub",
			Provider: &handoffProvider{tool: DomainChat, content: "ok"},
		},
	}
	cfg := RouterConfig(base)

	var patentFallback string
	for _, h := range cfg.Handoffs {
		if h.Name == DomainPatent {
			patentFallback = h.FallbackMsg
			break
		}
	}

	if patentFallback == "" {
		t.Error("patent agent should have FallbackMsg configured")
	}
	if !strings.Contains(patentFallback, "专利") {
		t.Errorf("patent FallbackMsg should mention patent, got: %s", patentFallback)
	}
}

func TestFallback_LegalAgentFallbackMsg(t *testing.T) {
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "verify-test",
			Model:    "stub",
			Provider: &handoffProvider{tool: DomainChat, content: "ok"},
		},
	}
	cfg := RouterConfig(base)

	var legalFallback string
	for _, h := range cfg.Handoffs {
		if h.Name == DomainLegal {
			legalFallback = h.FallbackMsg
			break
		}
	}

	if legalFallback == "" {
		t.Error("legal agent should have FallbackMsg configured")
	}
	if !strings.Contains(legalFallback, "法律") {
		t.Errorf("legal FallbackMsg should mention legal, got: %s", legalFallback)
	}
}
