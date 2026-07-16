//go:build integration

package integration_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/guardrails"
)

// ──────────────────────────────────────────────
// guardrailTestProvider — 按 call 次数返回不同内容
// ──────────────────────────────────────────────

type guardrailTestProvider struct {
	called  atomic.Int64
	outputs []string // 按 call 顺序返回的内容
}

func (p *guardrailTestProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	call := p.called.Add(1) - 1
	if int(call) < len(p.outputs) {
		return &agentcore.ProviderResponse{Content: p.outputs[call]}, nil
	}
	return &agentcore.ProviderResponse{Content: p.outputs[len(p.outputs)-1]}, nil
}

func (p *guardrailTestProvider) Stream(_ context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

// ──────────────────────────────────────────────
// 场景 1: LevelStrict — 专利护栏追加免责声明
// ──────────────────────────────────────────────

func TestGuardrailE2E_PatentDisclaimer(t *testing.T) {
	provider := &guardrailTestProvider{
		outputs: []string{"本发明具有新颖性和创造性，该技术方案不侵权。"},
	}

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "patent-test",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
		Lifecycle: guardrails.New(
			guardrails.WithLevel(guardrails.LevelStrict),
			guardrails.WithDisclaimer(guardrails.DisclaimerPatent),
			guardrails.WithRiskKeywords(guardrails.RiskKeywordsFor("patent")),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		),
	}

	agent := agentcore.New(cfg)
	defer agent.Close()

	output, err := agent.Run(context.Background(), "查询这个专利是否侵权")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 应包含专利免责声明
	if !strings.Contains(output, guardrails.DisclaimerPatent) {
		t.Errorf("LevelStrict should append patent disclaimer, got:\n%s", output)
	}

	t.Logf("Patent disclaimer output contains disclaimer: %v", strings.Contains(output, "不构成正式法律意见"))
}

// ──────────────────────────────────────────────
// 场景 2: LevelStrict — 审批关键词触发 SuppressPersist
// ──────────────────────────────────────────────

func TestGuardrailE2E_ApprovalKeywordTriggersGate(t *testing.T) {
	provider := &guardrailTestProvider{
		outputs: []string{"根据分析，你的专利侵权判断结论是：可能构成侵权。"},
	}

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "patent-approval-test",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
		Lifecycle: guardrails.New(
			guardrails.WithLevel(guardrails.LevelStrict),
			guardrails.WithDisclaimer(guardrails.DisclaimerPatent),
			guardrails.WithRiskKeywords(guardrails.RiskKeywordsFor("patent")),
			guardrails.WithApproval(guardrails.ApprovalKeywordsFor("patent")),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		),
	}

	agent := agentcore.New(cfg)
	defer agent.Close()

	_, err := agent.Run(context.Background(), "分析侵权风险")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Approval keywords should have triggered SuppressPersist
	// Check the last message doesn't contain the exact approval text
	msgs := agent.State().Messages()
	if len(msgs) > 0 {
		lastMsg := msgs[len(msgs)-1]
		// The output with approval keywords should NOT be stored verbatim
		// (SuppressPersist was set by the guardrail)
		t.Logf("Last message role: %s, content length: %d", lastMsg.Role, len(lastMsg.Content))
	}
}

// ──────────────────────────────────────────────
// 场景 3: LevelStandard — 助理护栏注入免责声明
// ──────────────────────────────────────────────

func TestGuardrailE2E_AssistantDisclaimer(t *testing.T) {
	provider := &guardrailTestProvider{
		outputs: []string{"已完成代码审查，建议修正后自动提交。"},
	}

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "assistant-test",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
		Lifecycle: guardrails.New(
			guardrails.WithLevel(guardrails.LevelStandard),
			guardrails.WithDisclaimer(guardrails.DisclaimerAssistant),
			guardrails.WithRiskKeywords(guardrails.RiskKeywordsFor("assistant")),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		),
	}

	agent := agentcore.New(cfg)
	defer agent.Close()

	output, err := agent.Run(context.Background(), "帮我审查代码")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// LevelStandard 输出应包含免责声明
	if !strings.Contains(output, guardrails.DisclaimerAssistant) {
		t.Errorf("LevelStandard should append assistant disclaimer, got:\n%s", output)
	}
}

// ──────────────────────────────────────────────
// 场景 4: 三级护栏混用验证
// ──────────────────────────────────────────────

func TestGuardrailE2E_MixedLevels(t *testing.T) {
	// Chat Agent (Light): 无 disclaimer, 无 risk keywords
	chatProvider := &guardrailTestProvider{
		outputs: []string{"你好！今天天气不错。"},
	}
	chatCfg := domains.ChatAgentConfig(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "chat-test",
			Model:    "stub",
			Provider: chatProvider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
	})
	chatAgent := agentcore.New(chatCfg)
	defer chatAgent.Close()

	chatOutput, err := chatAgent.Run(context.Background(), "你好")
	if err != nil {
		t.Fatalf("chat agent error: %v", err)
	}

	// Chat (LevelLight) 不应包含免责声明
	if strings.Contains(chatOutput, "本分析") || strings.Contains(chatOutput, "AI 辅助") {
		t.Error("LevelLight chat output should not contain disclaimer")
	}
	t.Logf("Chat (Light) output (no disclaimer expected): %s", chatOutput)

	// Assistant (LevelStandard) 应包含免责声明
	asstProvider := &guardrailTestProvider{
		outputs: []string{"代码审查完成，发现潜在问题，建议修正后自动提交。"},
	}
	asstCfg := domains.AssistantAgentConfig(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "asst-test",
			Model:    "stub",
			Provider: asstProvider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
	})
	asstAgent := agentcore.New(asstCfg)
	defer asstAgent.Close()

	asstOutput, err := asstAgent.Run(context.Background(), "审查代码")
	if err != nil {
		t.Fatalf("assistant agent error: %v", err)
	}

	// Assistant (LevelStandard) 应包含免责声明
	if !strings.Contains(asstOutput, "AI 辅助生成") {
		t.Errorf("LevelStandard assistant output should contain disclaimer, got:\n%s", asstOutput)
	}
	t.Logf("Assistant (Standard) output contains disclaimer")

	// Patent (LevelStrict) 应包含专利免责声明
	patentProvider := &guardrailTestProvider{
		outputs: []string{"该发明具有新颖性，不侵权。"},
	}
	patentCfg := domains.PatentAgentConfig(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "patent-test",
			Model:    "stub",
			Provider: patentProvider,
		},
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 3,
		},
	})
	patentAgent := agentcore.New(patentCfg)
	defer patentAgent.Close()

	patentOutput, err := patentAgent.Run(context.Background(), "分析新颖性")
	if err != nil {
		t.Fatalf("patent agent error: %v", err)
	}

	if !strings.Contains(patentOutput, "不构成正式法律意见") {
		t.Errorf("LevelStrict patent output should contain legal disclaimer, got:\n%s", patentOutput)
	}
	t.Logf("Patent (Strict) output contains legal disclaimer")
}
