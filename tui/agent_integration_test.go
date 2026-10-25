package tui

import (
	"context"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// fakeStreamProvider 是一个离线 Provider：按 rune 逐个产出固定回复，
// 让本测试不再依赖真实网络与 API Key（历史上它会读取环境里的失效 key，
// 在 CI/本地稳定地 401 失败）。
type fakeStreamProvider struct {
	reply string
}

func (p *fakeStreamProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	return &agentcore.ProviderResponse{Content: p.reply, FinishReason: "stop"}, nil
}

func (p *fakeStreamProvider) Stream(ctx context.Context, _ *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, len(p.reply)+1)
	go func() {
		defer close(ch)
		for _, r := range p.reply {
			select {
			case <-ctx.Done():
				return
			case ch <- agentcore.StreamDelta{Content: string(r)}:
			}
		}
		select {
		case <-ctx.Done():
		case ch <- agentcore.StreamDelta{FinishReason: "stop"}:
		}
	}()
	return ch, nil
}

func TestAgentRunInTUISession(t *testing.T) {
	provider := &fakeStreamProvider{reply: "hello"}
	model := "test-model"

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:      "test-agent",
			Model:     model,
			Provider:  provider,
			Streaming: true,
		},
		SystemPrompt: "You are a helpful assistant. Answer concisely.",
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:          3,
			ExecutionMode:     agentcore.ModeSerial,
			ValidateArguments: true,
		},
		CompactionConfig: agentcore.CompactionConfig{
			ContextWindow:    128000,
			ReserveTokens:    32000,
			KeepRecentTokens: 4000,
		},
	}

	agent := agentcore.New(cfg)
	defer agent.Close()

	// Subscribe a test event logger
	eventCh := make(chan string, 100)
	agent.On(agentcore.EventAgentStart, func(e agentcore.Event) {
		eventCh <- "start"
	})
	agent.On(agentcore.EventMessageDelta, func(e agentcore.Event) {
		eventCh <- "delta"
	})
	agent.On(agentcore.EventTurnEnd, func(e agentcore.Event) {
		eventCh <- "turn_end"
	})
	agent.On(agentcore.EventAgentError, func(e agentcore.Event) {
		ev := e.(*agentcore.AgentErrorEvent)
		eventCh <- "error:" + ev.Err.Error()
	})
	agent.On(agentcore.EventAgentEnd, func(e agentcore.Event) {
		eventCh <- "end"
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		output, err := agent.Run(ctx, "Say hello in one word.")
		if err != nil {
			t.Logf("agent.Run error: %v", err)
		}
		t.Logf("agent.Run output: %q", output)
		eventCh <- "run_done"
	}()

	timeout := time.After(15 * time.Second)
	var events []string
	seenRunDone := false
	for !seenRunDone {
		select {
		case e := <-eventCh:
			events = append(events, e)
			if e == "run_done" {
				seenRunDone = true
			}
		case <-timeout:
			t.Fatalf("timeout waiting for run_done, got %d events: %v", len(events), events)
		}
	}

	t.Logf("events received (%d): %v", len(events), events)

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
	if events[0] != "start" {
		t.Errorf("first event should be 'start', got %q", events[0])
	}

	hasEnd := false
	for _, e := range events {
		if e == "end" || e == "run_done" {
			hasEnd = true
		}
		if len(e) > 6 && e[:6] == "error:" {
			t.Fatalf("agent error: %s", e[6:])
		}
	}
	if !hasEnd {
		t.Error("no 'end' or 'run_done' event received")
	}
}
