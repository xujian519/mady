package tui

import (
	"context"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/agentconfig"
)

func TestAgentRunInTUISession(t *testing.T) {
	provider := agentconfig.BuildProvider()
	model := agentconfig.DefaultModel()

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

	timeout := time.After(10 * time.Second)
	var events []string
	for i := 0; i < 6; i++ {
		select {
		case e := <-eventCh:
			events = append(events, e)
		case <-timeout:
			t.Fatalf("timeout waiting for events %d, got %v", i, events)
		}
	}

	t.Logf("events received: %v", events)

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
		t.Error("no 'end' event received")
	}
}
