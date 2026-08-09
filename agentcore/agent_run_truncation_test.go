package agentcore

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// sequenceProvider 按调用次数返回预设的 ProviderResponse 序列。
// 用于模拟"第一次输出被 max_tokens 截断、第二次输出完整"等场景。
type sequenceProvider struct {
	mu        sync.Mutex
	callCount int
	responses []*ProviderResponse
}

func newSequenceProvider(responses ...*ProviderResponse) *sequenceProvider {
	return &sequenceProvider{responses: responses}
}

func (p *sequenceProvider) call() *ProviderResponse {
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := p.callCount
	p.callCount++
	if idx >= len(p.responses) {
		idx = len(p.responses) - 1
	}
	return p.responses[idx]
}

func (p *sequenceProvider) callCountGet() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callCount
}

func (p *sequenceProvider) Complete(_ context.Context, _ *ProviderRequest) (*ProviderResponse, error) {
	return p.call(), nil
}

func (p *sequenceProvider) Stream(ctx context.Context, req *ProviderRequest) (<-chan StreamDelta, error) {
	resp, _ := p.Complete(ctx, req)
	ch := make(chan StreamDelta, 2)
	if resp.Content != "" {
		ch <- StreamDelta{Content: resp.Content}
	}
	if resp.FinishReason != "" {
		ch <- StreamDelta{FinishReason: resp.FinishReason}
	}
	ch <- StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

// TestRun_TextTruncationAutoContinuation 验证：纯文本输出被 max_tokens 截断
// （finish_reason="length"）时，Agent 自动追加一轮续写，模型第二轮给出完整
// 输出后，最终输出为完整内容而非半截文本。
func TestRun_TextTruncationAutoContinuation(t *testing.T) {
	provider := newSequenceProvider(
		&ProviderResponse{Content: "回答前半部分，", FinishReason: "length"},
		&ProviderResponse{Content: "回答前半部分，后半部分完整。", FinishReason: "stop"},
	)
	a := New(Config{
		ModelConfig: ModelConfig{
			Name:     "trunc_continuation",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 10,
		},
	})

	output, err := a.Run(context.Background(), "请回答")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if want := "回答前半部分，后半部分完整。"; output != want {
		t.Fatalf("output = %q, want %q（截断后应自动续写为完整输出）", output, want)
	}
	if got := a.LastFinishReason(); got != "stop" {
		t.Fatalf("LastFinishReason = %q, want %q", got, "stop")
	}
	if got := provider.callCountGet(); got != 2 {
		t.Fatalf("provider called %d times, want 2（一次截断 + 一次续写）", got)
	}
	if a.state.Status() != StatusFinished {
		t.Fatalf("status = %q, want %q", a.state.Status(), StatusFinished)
	}
}

// TestRun_TextTruncationContinuationExhausted 验证：续写后仍被截断时不再
// 无限续写，按截断结果正常结束，FinishReason 保留 "length" 供下游提示。
func TestRun_TextTruncationContinuationExhausted(t *testing.T) {
	provider := newSequenceProvider(
		&ProviderResponse{Content: "第一部分，", FinishReason: "length"},
		&ProviderResponse{Content: "第二部分，", FinishReason: "length"},
	)
	a := New(Config{
		ModelConfig: ModelConfig{
			Name:     "trunc_exhausted",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 10,
		},
	})

	output, err := a.Run(context.Background(), "请回答")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if want := "第二部分，"; output != want {
		t.Fatalf("output = %q, want %q（续写耗尽后返回最后一轮截断内容）", output, want)
	}
	if got := a.LastFinishReason(); got != "length" {
		t.Fatalf("LastFinishReason = %q, want %q（应保留 length 供下游提示）", got, "length")
	}
	if got := provider.callCountGet(); got != 2 {
		t.Fatalf("provider called %d times, want 2（只续写一次，不无限循环）", got)
	}
}

// TestRun_TextTruncationSteeringApplied 验证：自动续写前注入的 steering 消息
// 会被持久化进对话历史，模型在续写轮中能看到"继续完成"指令。
func TestRun_TextTruncationSteeringApplied(t *testing.T) {
	provider := newSequenceProvider(
		&ProviderResponse{Content: "截断内容", FinishReason: "length"},
		&ProviderResponse{Content: "完整内容", FinishReason: "stop"},
	)
	a := New(Config{
		ModelConfig: ModelConfig{
			Name:     "trunc_steering",
			Model:    "stub",
			Provider: provider,
		},
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 10,
		},
	})

	if _, err := a.Run(context.Background(), "请回答"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	found := false
	for _, m := range a.state.Messages() {
		if m.Role == RoleSystem && strings.Contains(m.Content, "输出长度上限") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected truncation continuation steering message in conversation history")
	}
}
