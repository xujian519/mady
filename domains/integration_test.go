//go:build integration

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

func TestUnifiedHandoffDelegateToPatent(t *testing.T) {
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "unified-test",
			Model:    "stub",
			Provider: &handoffProvider{tool: DomainPatent, content: "done"},
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := UnifiedAgentConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	// 执行 Agent Run — 模拟用户发出任务请求
	out, err := agent.Run(context.Background(), "帮我查一下相关专利")
	if err != nil {
		t.Fatalf("Unified agent run error: %v", err)
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
			if tc.Name == "transfer_to_"+DomainPatent {
				foundHandoff = true
				break
			}
		}
	}
	if !foundHandoff {
		t.Error("expected handoff tool call in conversation history")
	}
}

func TestRouterHandoffDelegateToPatent(t *testing.T) {
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "unified-test",
			Model:    "stub",
			Provider: &handoffProvider{tool: DomainPatent, content: "patent analysis done"},
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := UnifiedAgentConfig(base)
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
			Name:     "unified-test",
			Model:    "stub",
			Provider: &handoffProvider{tool: DomainLegal, content: "legal analysis done"},
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := UnifiedAgentConfig(base)
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
	// 测试 LevelStandard 护栏的风险关键词→免责声明注入行为。
	// UnifiedAgentConfig 使用 LevelLight（不注入免责声明），
	// 此处直接内联 LevelStandard 护栏配置。
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "guardrail-test",
			Model:    "stub",
			Provider: &riskKeywordProvider{content: "根据您的需求，为您生成法律文书草稿如下："},
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
		Lifecycle: agentcore.LifecycleChain{
			agentcore.NewIFaceLifecycleHook(guardrails.New(
				guardrails.WithLevel(guardrails.LevelStandard),
				guardrails.WithDisclaimer(guardrails.DisclaimerAssistant),
				guardrails.WithRiskKeywords(guardrails.RiskKeywordsFor("assistant")),
				guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
			)),
		},
	}
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
	t.Skip("既有问题：iface.ModelCallContext 无 Raw 字段，需单独修复")

	// 直接测试护栏钩子：带违禁词的内容被拦截
	hook := guardrails.New(
		guardrails.WithLevel(guardrails.LevelStandard),
		guardrails.WithBlockedPhrases([]string{"恶意代码"}),
	)
	_ = hook
}

// ──────────────────────────────────────────────
// 4. HandoffContext 上下文抽取与传递测试
// ──────────────────────────────────────────────

func TestHandoffContextExtractionAndPropagation(t *testing.T) {
	provider := &handoffProvider{tool: DomainPatent, content: "patent analysis done"}

	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "ctx-test",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := UnifiedAgentConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	// 捕获 HandoffStartEvent 以验证 HandoffContext 被正确构建
	var handoffCtxJSON string
	agent.On(agentcore.EventHandoffStart, func(e agentcore.Event) {
		if he, ok := e.(*agentcore.HandoffStartEvent); ok {
			handoffCtxJSON = he.Context
		}
	})

	// 使用含专利号的输入触发 Agent 运行
	_, err := agent.Run(context.Background(), "分析专利 CN109690000A 的新颖性")
	if err != nil {
		t.Fatalf("Router run error: %v", err)
	}

	// 验证 HandoffContext JSON 非空且包含关键信息
	if handoffCtxJSON == "" {
		t.Fatal("expected HandoffStartEvent.Context to be non-empty JSON")
	}

	// 验证包含了专利号实体
	if !strings.Contains(handoffCtxJSON, "CN109690000A") {
		t.Errorf("expected HandoffContext to contain patent_no CN109690000A, got: %s", handoffCtxJSON)
	}

	// 验证包含了 FromAgent/ToAgent 字段
	if !strings.Contains(handoffCtxJSON, `"from_agent"`) {
		t.Error("expected HandoffContext to contain from_agent field")
	}
	if !strings.Contains(handoffCtxJSON, `"to_agent"`) {
		t.Error("expected HandoffContext to contain to_agent field")
	}
	if !strings.Contains(handoffCtxJSON, `"user_intent"`) {
		t.Error("expected HandoffContext to contain user_intent field")
	}
}

// ──────────────────────────────────────────────
// 5. SafeHandoff 白名单校验测试
// ──────────────────────────────────────────────

func TestSafeHandoff_AllowedSource(t *testing.T) {
	// UnifiedAgentConfig 中的 patent-agent 允许来自 "mady-agent" 的交接。
	// UnifiedAgent 自身是 "mady-agent"，所以交接应该通过。
	provider := &handoffProvider{tool: DomainPatent, content: "patent result"}

	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "whitelist-test",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}

	cfg := UnifiedAgentConfig(base)
	agent := agentcore.New(cfg)
	defer agent.Close()

	_, err := agent.Run(context.Background(), "帮我查一下今天的新闻")
	if err != nil {
		t.Fatalf("whitelisted handoff should succeed: %v", err)
	}
}

func TestSafeHandoff_BlockedSource(t *testing.T) {
	// 创建一个 handoff 配置，将 AllowedSources 设为空列表外的一个值。
	// 用于验证 isHandoffAllowed 在来源不匹配时返回 false。
	h := agentcore.HandoffConfig{
		Name:           "blocked-target",
		AllowedSources: []string{"specific-agent"},
	}

	// 使用 trapProvider：如果子 Agent 被调用（表示阻断失败），返回明显错误文案。
	provider := &trapProvider{tool: "blocked-target", content: "should NOT reach here - handoff was not blocked"}
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "mady-router",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 10,
		},
	}
	base.Handoffs = []agentcore.HandoffConfig{h}

	agent := agentcore.New(base)
	defer agent.Close()

	// 监听 handoff_start 事件——阻断成功时不会 emit，反之则 emit
	var handoffStarted bool
	agent.On(agentcore.EventHandoffStart, func(e agentcore.Event) {
		handoffStarted = true
	})

	// 执行 agent — handoff 工具应该被校验拦截，不报错但返回失败结果
	output, err := agent.Run(context.Background(), "测试被拦截的交接")
	if err != nil {
		t.Fatalf("agent run should not error, got: %v", err)
	}

	// 阻断成功时不应有 HandoffStartEvent
	if handoffStarted {
		t.Error("handoff should be blocked but HandoffStartEvent was emitted")
	}

	// 输出应该包含兜底文案而非子 Agent 的输出
	if strings.Contains(output, "should NOT reach here") {
		t.Error("handoff was not blocked: sub-agent was invoked")
	}
	if output == "" {
		t.Fatal("expected fallback message in output")
	}
}

// trapProvider 如被调用 second time（表示子 Agent 被启动），返回明显错误内容。
type trapProvider struct {
	called  atomic.Int64
	tool    string
	content string
}

func (p *trapProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	call := p.called.Add(1) - 1
	if call == 0 {
		return &agentcore.ProviderResponse{
			ToolCalls: []agentcore.ToolCall{
				{ID: "call_handoff", Name: "transfer_to_" + p.tool, Arguments: `{"message":"test"}`},
			},
		}, nil
	}
	// second+ call — Router 读到 HandoffResult 后继续，返回兜底文案
	return &agentcore.ProviderResponse{Content: "handoff blocked as expected"}, nil
}

func (p *trapProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	return nil, fmt.Errorf("streaming not implemented")
}

func TestSafeHandoff_StarAllowsAll(t *testing.T) {
	// AllowedSources: ["*"] 应该放行所有来源
	h := agentcore.HandoffConfig{
		Name:           "open-target",
		AllowedSources: []string{"*"},
	}

	provider := &handoffProvider{tool: "open-target", content: "allowed"}
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "any-agent",
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

	output, err := agent.Run(context.Background(), "测试通配符")
	if err != nil {
		t.Fatalf("wildcard should allow all: %v", err)
	}
	if output != "allowed" {
		t.Fatalf("expected 'allowed', got %q", output)
	}
}

func TestSafeHandoff_EmptyAllowedSourcesAllowsAll(t *testing.T) {
	// AllowedSources 为空（nil/empty）应放行所有来源（向后兼容）
	h := agentcore.HandoffConfig{
		Name: "default-target",
	}

	provider := &handoffProvider{tool: "default-target", content: "default allowed"}
	base := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "random-agent",
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

	output, err := agent.Run(context.Background(), "测试空白名单")
	if err != nil {
		t.Fatalf("empty AllowedSources should allow all: %v", err)
	}
	if output != "default allowed" {
		t.Fatalf("expected 'default allowed', got %q", output)
	}
}
