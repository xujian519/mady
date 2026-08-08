package agentcore

import (
	"context"
	"sync"
	"testing"
)

// finishReasonProvider 在非流式路径返回固定 FinishReason 的 stub Provider。
type finishReasonProvider struct {
	finishReason string
}

func (p *finishReasonProvider) Complete(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	return &ProviderResponse{Content: "answer", FinishReason: p.finishReason}, nil
}

func (p *finishReasonProvider) Stream(ctx context.Context, req *ProviderRequest) (<-chan StreamDelta, error) {
	ch := make(chan StreamDelta, 2)
	ch <- StreamDelta{Content: "answer"}
	ch <- StreamDelta{Done: true, FinishReason: p.finishReason}
	close(ch)
	return ch, nil
}

// collectAgentEnd 订阅 EventAgentEnd 并记录事件。
func collectAgentEnd(agent *Agent) *[]*AgentEndEvent {
	var mu sync.Mutex
	events := &[]*AgentEndEvent{}
	agent.On(EventAgentEnd, func(e Event) {
		if ev, ok := e.(*AgentEndEvent); ok {
			mu.Lock()
			*events = append(*events, ev)
			mu.Unlock()
		}
	})
	return events
}

// TestAgentEndEvent_CarriesFinishReasonLength 是 S1 回归防护：模型收尾轮次
// （无工具调用）上报 FinishReason="length"（max_tokens 截断）时，
// AgentEndEvent 必须带上该标记，供 UI 提示用户输出可能不完整。
func TestAgentEndEvent_CarriesFinishReasonLength(t *testing.T) {
	agent := New(Config{
		ModelConfig: ModelConfig{
			Name:     "truncated",
			Model:    "stub",
			Provider: &finishReasonProvider{finishReason: "length"},
		},
	})
	events := collectAgentEnd(agent)

	out, err := agent.Run(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if out != "answer" {
		t.Fatalf("output = %q", out)
	}
	if len(*events) != 1 {
		t.Fatalf("expected 1 AgentEndEvent, got %d", len(*events))
	}
	if got := (*events)[0].FinishReason; got != "length" {
		t.Fatalf("AgentEndEvent.FinishReason = %q, want %q", got, "length")
	}
}

// TestAgentEndEvent_CarriesFinishReasonError 覆盖流式路径：provider 异常
// 终止（chatcompat 合成 FinishReason="error"）时，标记须透传到 AgentEndEvent。
func TestAgentEndEvent_CarriesFinishReasonError(t *testing.T) {
	agent := New(Config{
		ModelConfig: ModelConfig{
			Name:      "aborted",
			Model:     "stub",
			Provider:  &finishReasonProvider{finishReason: "error"},
			Streaming: true,
		},
	})
	events := collectAgentEnd(agent)

	if _, err := agent.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if len(*events) != 1 {
		t.Fatalf("expected 1 AgentEndEvent, got %d", len(*events))
	}
	if got := (*events)[0].FinishReason; got != "error" {
		t.Fatalf("AgentEndEvent.FinishReason = %q, want %q", got, "error")
	}
}

// TestAgentEndEvent_NormalFinishReason 确认正常结束时事件携带 "stop"，
// 不会被误判为截断/异常。
func TestAgentEndEvent_NormalFinishReason(t *testing.T) {
	agent := New(Config{
		ModelConfig: ModelConfig{
			Name:     "normal",
			Model:    "stub",
			Provider: &finishReasonProvider{finishReason: "stop"},
		},
	})
	events := collectAgentEnd(agent)

	if _, err := agent.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if len(*events) != 1 {
		t.Fatalf("expected 1 AgentEndEvent, got %d", len(*events))
	}
	if got := (*events)[0].FinishReason; got != "stop" {
		t.Fatalf("AgentEndEvent.FinishReason = %q, want %q", got, "stop")
	}
}
