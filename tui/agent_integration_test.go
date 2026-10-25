package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tui/agentadapter"
	"github.com/xujian519/mady/tui/chat"
	"github.com/xujian519/mady/tui/terminal"
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

// TestAgentEventsReachChatHistory 是 TUI 层的端到端接线测试：agentcore 事件
// 经 agentadapter 转换后，必须真正落到 ChatApp 的聊天记录里。此前的
// TestAgentRunInTUISession 只验证了 agent 事件流本身，没有构造任何 TUI
// 组件，无法发现映射表或订阅接线的断裂。
func TestAgentEventsReachChatHistory(t *testing.T) {
	vt := terminal.NewVirtualTerminal(100, 30)
	app := NewTUI(vt, TUIOptions{})
	host := &tuiAppHost{TUI: app}
	chatApp := chat.NewChatAppWithHost(chat.ChatAppConfig{Context: context.Background()}, host)
	chatApp.SetHost(host)

	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:      "e2e-agent",
			Model:     "test-model",
			Provider:  &fakeStreamProvider{reply: "hello from agent"},
			Streaming: true,
		},
		SystemPrompt: "test",
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:      2,
			ExecutionMode: agentcore.ModeSerial,
		},
	})
	defer agent.Close()
	agentadapter.BindAgent(chatApp, agent)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := agent.Run(ctx, "hi"); err != nil {
		t.Fatalf("agent.Run: %v", err)
	}

	var got strings.Builder
	for _, m := range chatApp.History().Messages() {
		if m.Role == chat.RoleAssistant {
			got.WriteString(m.Text)
		}
	}
	if !strings.Contains(got.String(), "hello from agent") {
		t.Fatalf("assistant reply did not reach ChatHistory; messages=%+v", chatApp.History().Messages())
	}
}
