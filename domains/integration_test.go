package domains

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/session"
)

// ──────────────────────────────────────────────
// 测试辅助 Provider
// ──────────────────────────────────────────────

// handoffProvider 模拟 LLM：第一次返回 transfer_to_<name> 工具调用，之后返回 content。
type handoffProvider struct {
	called  atomic.Int64
	tool    string
	content string
}

func (p *handoffProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	call := p.called.Add(1) - 1
	if call == 0 {
		return &agentcore.ProviderResponse{
			ToolCalls: []agentcore.ToolCall{
				{ID: "call_handoff", Name: "transfer_to_" + p.tool, Arguments: `{"input":"test"}`},
			},
		}, nil
	}
	return &agentcore.ProviderResponse{Content: p.content}, nil
}

func (p *handoffProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return nil, fmt.Errorf("streaming not implemented")
}

// riskKeywordProvider 返回包含风险关键词的固定内容。
type riskKeywordProvider struct {
	content string
}

func (p *riskKeywordProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	return &agentcore.ProviderResponse{Content: p.content}, nil
}

func (p *riskKeywordProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return nil, fmt.Errorf("streaming not implemented")
}

// ──────────────────────────────────────────────
// 1. HandoffDelegate 闭环测试
// ──────────────────────────────────────────────

func TestRouterHandoffDelegateToAssistant(t *testing.T) {
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "router-test",
			Model:    "stub",
			Provider: &handoffProvider{tool: DomainAssistant, content: "done"},
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := RouterConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	// 执行 Agent Run — 模拟用户发出任务请求
	out, err := agent.Run(context.Background(), "帮我查一下相关专利")
	if err != nil {
		t.Fatalf("Router run error: %v", err)
	}
	if out != "done" {
		t.Fatalf("final output = %q, want %q", out, "done")
	}

	// 验证状态
	state := agent.State()
	if state == nil {
		t.Fatal("state is nil")
	}

	// 验证消息历史中包含 Handoff 工具调用
	msgs := state.Messages()
	foundHandoff := false
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			if tc.Name == "transfer_to_"+DomainAssistant {
				foundHandoff = true
				break
			}
		}
	}
	if !foundHandoff {
		t.Error("expected handoff tool call in conversation history")
	}
}

func TestRouterHandoffDelegateToChat(t *testing.T) {
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "router-test",
			Model:    "stub",
			Provider: &handoffProvider{tool: DomainChat, content: "hello back"},
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := RouterConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	out, err := agent.Run(context.Background(), "你好")
	if err != nil {
		t.Fatalf("Router run error: %v", err)
	}
	if out != "hello back" {
		t.Fatalf("final output = %q, want %q", out, "hello back")
	}
}

func TestRouterHandoffDelegateToPatent(t *testing.T) {
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "router-test",
			Model:    "stub",
			Provider: &handoffProvider{tool: DomainPatent, content: "patent analysis done"},
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := RouterConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	out, err := agent.Run(context.Background(), "帮我分析这个专利的新颖性")
	if err != nil {
		t.Fatalf("Router run error: %v", err)
	}
	if out != "patent analysis done" {
		t.Fatalf("final output = %q, want %q", out, "patent analysis done")
	}
}

func TestRouterHandoffDelegateToLegal(t *testing.T) {
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "router-test",
			Model:    "stub",
			Provider: &handoffProvider{tool: DomainLegal, content: "legal analysis done"},
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := RouterConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	out, err := agent.Run(context.Background(), "请问这个合同条款是否有效")
	if err != nil {
		t.Fatalf("Router run error: %v", err)
	}
	if out != "legal analysis done" {
		t.Fatalf("final output = %q, want %q", out, "legal analysis done")
	}
}

// ──────────────────────────────────────────────
// 2. Session 连续性测试
// ──────────────────────────────────────────────

func TestSessionContinuityWithAgentStore(t *testing.T) {
	dir := t.TempDir()
	fileStore, err := session.NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	agentStore := session.NewAgentStore(fileStore, dir)

	provider := &handoffProvider{tool: DomainChat, content: "second response"}

	// 第一轮：创建带 Store 的 agent 并执行
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "session-test",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
		Store: agentStore,
	}

	agent := agentcore.New(cfg)
	_, err = agent.Run(context.Background(), "第一轮消息")
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// SaveState 通过 Store 持久化状态快照
	threadKey := "test-thread"
	if err := agent.SaveState(context.Background(), threadKey); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	agent.Close()

	// 第二轮：新 agent 从 Store 加载状态
	agent2 := agentcore.New(cfg)
	defer agent2.Close()
	if err := agent2.LoadState(context.Background(), threadKey); err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	// 验证之前的历史已恢复
	msgs := agent2.State().Messages()
	foundFirst := false
	for _, m := range msgs {
		if m.Role == agentcore.RoleUser && strings.Contains(m.Content, "第一轮消息") {
			foundFirst = true
			break
		}
	}
	if !foundFirst {
		t.Error("loaded agent should have previous conversation history")
	}

	// 继续对话
	provider.called.Store(0) // 重置 provider
	out, err := agent2.Run(context.Background(), "第二轮消息")
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if out != "second response" {
		t.Fatalf("second run output = %q, want %q", out, "second response")
	}

	// 验证两轮消息都在历史中
	msgs = agent2.State().Messages()
	foundBoth := 0
	for _, m := range msgs {
		if m.Role == agentcore.RoleUser {
			if strings.Contains(m.Content, "第一轮消息") || strings.Contains(m.Content, "第二轮消息") {
				foundBoth++
			}
		}
	}
	if foundBoth < 2 {
		t.Errorf("expected 2+ user messages, got %d", foundBoth)
	}
}

func TestSessionContinuityViaCheckpoint(t *testing.T) {
	saver := agentcore.NewMemoryCheckpointSaver()

	provider := &handoffProvider{tool: DomainChat, content: "continued"}

	// 创建带 Checkpoint 的 agent
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "cp-test",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
		Checkpoint: &agentcore.CheckpointSettings{
			Saver:    saver,
			ThreadID: "continuity-test",
		},
	}

	agent := agentcore.New(cfg)
	_, err := agent.Run(context.Background(), "第一轮消息")
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// 从 checkpoint 恢复
	agent2 := agentcore.New(cfg)
	if err := agent2.RestoreLatestCheckpoint(context.Background(), "continuity-test"); err != nil {
		t.Fatalf("RestoreLatestCheckpoint: %v", err)
	}

	// 验证历史恢复
	msgs := agent2.State().Messages()
	found := false
	for _, m := range msgs {
		if m.Role == agentcore.RoleUser && strings.Contains(m.Content, "第一轮消息") {
			found = true
			break
		}
	}
	if !found {
		t.Error("checkpoint should preserve user message")
	}
}

// ──────────────────────────────────────────────
// 3. 护栏运行时注入测试
// ──────────────────────────────────────────────

func TestGuardrailDisclaimerInjection(t *testing.T) {
	// 创建一个带 LevelStandard 护栏的 assistant-agent（包含 "生成法律文书" 风险关键词）
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "guardrail-test",
			Model:    "stub",
			Provider: &riskKeywordProvider{content: "根据您的需求，为您生成法律文书草稿如下："},
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
	}

	cfg := AssistantAgentConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	out, err := agent.Run(context.Background(), "帮我起草一份合同")
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// 验证护栏注入了免责声明（内容包含风险关键词 → 追加 DisclaimerAssistant）
	if !strings.Contains(out, guardrails.DisclaimerAssistant) {
		t.Errorf("output should contain assistant disclaimer, got:\n%s", out)
	}
}

func TestGuardrailBlockedPhrase(t *testing.T) {
	// 直接测试护栏钩子：带违禁词的内容被拦截
	hook := guardrails.New(
		guardrails.WithLevel(guardrails.LevelStandard),
		guardrails.WithBlockedPhrases([]string{"恶意代码"}),
	)

	mcc := &agentcore.ModelCallContext{
		Response: &agentcore.ProviderResponse{Content: "以下是恶意代码的实现方式"},
	}
	hook.AfterModelCall(context.Background(), &agentcore.AgentRunContext{}, mcc)

	if mcc.Err == nil {
		t.Fatal("expected guardrail error for blocked phrase")
	}
	if mcc.Response.Content == "以下是恶意代码的实现方式" {
		t.Error("blocked content should be replaced")
	}
}

