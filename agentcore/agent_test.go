package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestTransfer_InheritsParentToolsAndExtensions(t *testing.T) {
	parentTool := &Tool{
		Name:        "parent_tool",
		Description: "A tool from parent",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return map[string]string{"result": "parent_tool_result"}, nil
		},
	}

	transferProvider := &transferTestProvider{
		responses: []string{
			`I'll transfer to child`,  // call 0: parent triggers handoff
			``,                        // call 1: summarizeUserIntent → 空内容触发 v1 回退
			`final answer from child`, // call 2: child's response
		},
	}

	parent := New(Config{
		ModelConfig: ModelConfig{
			Name:     "parent",
			Model:    "stub",
			Provider: transferProvider,
		},
		Tools: []*Tool{parentTool},

		Handoffs: []HandoffConfig{
			{
				Name:        "child",
				Description: "Child agent",
				Mode:        HandoffTransfer,
				AgentConfig: Config{
					ModelConfig: ModelConfig{
						Name:     "child",
						Model:    "stub",
						Provider: transferProvider,
					},
					ExecutionConfig: ExecutionConfig{
						MaxTurns: 3,
					},
				},
				AllowedSources: []string{"parent"},
			},
		},
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 5,
		},
	})

	out, err := parent.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "final answer from child" {
		t.Fatalf("output = %q", out)
	}

	// Verify the child agent received the parent's tool via inheritance.
	// Index: 0=parent, 1=summarizeUserIntent, 2=child
	childReq := transferProvider.requests[2]
	found := false
	for _, tool := range childReq.Tools {
		if tool.Name == "parent_tool" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("child request tools = %v, missing parent_tool", childReq.Tools)
	}
}

type transferTestProvider struct {
	responses []string
	callCount int
	requests  []*ProviderRequest
}

func (p *transferTestProvider) Complete(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	cp := *req
	cp.Messages = append([]Message(nil), req.Messages...)
	cp.Tools = append([]ToolDefinition(nil), req.Tools...)
	p.requests = append(p.requests, &cp)
	idx := p.callCount
	p.callCount++
	if idx < len(p.responses) {
		if idx == 0 {
			return &ProviderResponse{
				Content: p.responses[0],
				ToolCalls: []ToolCall{
					{
						ID:        "call_transfer",
						Name:      "transfer_to_child",
						Arguments: `{"message":"take over"}`,
					},
				},
			}, nil
		}
		return &ProviderResponse{Content: p.responses[idx]}, nil
	}
	return &ProviderResponse{Content: "default"}, nil
}

func (p *transferTestProvider) Stream(ctx context.Context, req *ProviderRequest) (<-chan StreamDelta, error) {
	ch := make(chan StreamDelta, 1)
	ch <- StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

func TestAgent_Close_SharedEventBus_NotClosed(t *testing.T) {
	parent := New(Config{ModelConfig: ModelConfig{Name: "parent"}})
	parentBus := parent.eventBus

	child := New(Config{ModelConfig: ModelConfig{Name: "child"}})
	child.SetEventBus(parentBus)

	// Close child — must NOT close the shared (parent) EventBus
	child.Close()

	// Parent bus should still be usable
	parent.EmitEvent(&AgentStartEvent{baseEvent: newBase(EventAgentStart)})

	// Closing parent should work fine
	parent.Close()
}

func TestAgent_Close_OwnedEventBus_Closed(t *testing.T) {
	agent := New(Config{ModelConfig: ModelConfig{Name: "test"}})
	bus := agent.eventBus

	agent.Close()

	// After close, Emit should be a no-op (not panic)
	bus.Emit(&AgentStartEvent{baseEvent: newBase(EventAgentStart)}) // bus.Emit is public, safe to test
}

func TestAgent_Close_Idempotent(t *testing.T) {
	agent := New(Config{ModelConfig: ModelConfig{Name: "test"}})
	agent.Close()
	agent.Close() // should not panic
}

type hangingStreamProvider struct {
	ch chan StreamDelta
}

func (p *hangingStreamProvider) Complete(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	return nil, errors.New("not implemented")
}

func (p *hangingStreamProvider) Stream(ctx context.Context, req *ProviderRequest) (<-chan StreamDelta, error) {
	p.ch = make(chan StreamDelta) // intentionally unbuffered and never closed
	return p.ch, nil
}

func TestAgentRun_Streaming_ContextCancellation(t *testing.T) {
	provider := &hangingStreamProvider{}
	agent := New(Config{
		ModelConfig: ModelConfig{
			Name:      "test",
			Model:     "stub",
			Provider:  provider,
			Streaming: true,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := agent.Run(ctx, "hello")
		errCh <- err
	}()

	// Give the goroutine time to enter runStreaming and block on the channel
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected nil (clean stop) from canceled context, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("agent.Run did not exit after context cancellation — streaming goroutine is stuck")
	}

	// Clean up the hanging provider channel
	close(provider.ch)
}

// reasoningSplitProvider streams one chunk containing both normal content and a
// thinking block. It is used to verify that runStreaming emits them as separate
// MessageDeltaEvents with the correct BlockKind.
type reasoningSplitProvider struct {
	ch chan StreamDelta
}

func (p *reasoningSplitProvider) Complete(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	return nil, errors.New("not implemented")
}

func (p *reasoningSplitProvider) Stream(ctx context.Context, req *ProviderRequest) (<-chan StreamDelta, error) {
	p.ch = make(chan StreamDelta, 4)
	p.ch <- StreamDelta{
		Content: "visible answer",
		Blocks: []ContentBlock{
			{Kind: BlockKindThinking, Text: "inner reasoning"},
		},
	}
	p.ch <- StreamDelta{Content: " continues"}
	close(p.ch)
	return p.ch, nil
}

func TestRunStreaming_EmitsContentAndReasoningAsSeparateKinds(t *testing.T) {
	provider := &reasoningSplitProvider{}
	agent := New(Config{
		ModelConfig: ModelConfig{
			Name:      "test",
			Model:     "stub",
			Provider:  provider,
			Streaming: true,
		},
	})

	var got []MessageDeltaEvent
	agent.On(EventMessageDelta, func(e Event) {
		if ev, ok := e.(*MessageDeltaEvent); ok {
			got = append(got, *ev)
		}
	})

	_, err := agent.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 delta events (content + thinking + content), got %d: %+v", len(got), got)
	}

	if got[0].Delta != "visible answer" || got[0].Kind != BlockKindText {
		t.Errorf("first delta: got delta=%q kind=%v, want %q/%v", got[0].Delta, got[0].Kind, "visible answer", BlockKindText)
	}
	if got[1].Delta != "inner reasoning" || got[1].Kind != BlockKindThinking {
		t.Errorf("second delta: got delta=%q kind=%v, want %q/%v", got[1].Delta, got[1].Kind, "inner reasoning", BlockKindThinking)
	}
	if got[2].Delta != " continues" || got[2].Kind != BlockKindText {
		t.Errorf("third delta: got delta=%q kind=%v, want %q/%v", got[2].Delta, got[2].Kind, " continues", BlockKindText)
	}
}
