package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestMessageBus_PublishSubscribe(t *testing.T) {
	b := NewMessageBus()
	ch, cancel := b.Subscribe("t1", 2)
	defer cancel()
	b.Publish("t1", Message{Role: RoleUser, Content: "hi"})
	select {
	case m := <-ch:
		if m.Content != "hi" {
			t.Fatalf("got %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestMessageBus_cancelUnsubscribes(t *testing.T) {
	b := NewMessageBus()
	ch, cancel := b.Subscribe("t1", 1)
	cancel()
	b.Publish("t1", Message{Role: RoleUser, Content: "x"})
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(100 * time.Millisecond):
	}
}

type seqStubProvider struct{}

func (seqStubProvider) Complete(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	var lastUser string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == RoleUser {
			lastUser = req.Messages[i].Content
			break
		}
	}
	return &ProviderResponse{Content: "echo:" + lastUser}, nil
}

func (seqStubProvider) Stream(ctx context.Context, req *ProviderRequest) (<-chan StreamDelta, error) {
	ch := make(chan StreamDelta, 1)
	ch <- StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

func TestRunSequentialAgents(t *testing.T) {
	ctx := context.Background()
	cfg := func(name string) Config {
		return Config{
			ModelConfig: ModelConfig{
				Name:      name,
				Model:     "stub",
				Provider:  seqStubProvider{},
				Streaming: false,
			},
			ExecutionConfig: ExecutionConfig{
				MaxTurns: 5,
			},
			SystemPrompt: "",
		}
	}
	a1 := New(cfg("a1"))
	a2 := New(cfg("a2"))
	out, err := RunSequentialAgents(ctx, []*Agent{a1, a2}, "start")
	if err != nil {
		t.Fatal(err)
	}
	want := "echo:echo:start"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestDepthFromContext(t *testing.T) {
	ctx := context.Background()
	if d := DepthFromContext(ctx); d != 0 {
		t.Fatalf("default depth=%d want 0", d)
	}
	if d := DepthFromContext(WithDepth(ctx, 3)); d != 3 {
		t.Fatalf("depth=%d want 3", d)
	}
}

func TestRunSequentialAgentsWithDepth_Exceeds(t *testing.T) {
	cfg := func(name string) Config {
		return Config{
			ModelConfig:     ModelConfig{Name: name, Model: "stub", Provider: seqStubProvider{}},
			ExecutionConfig: ExecutionConfig{MaxTurns: 5},
		}
	}
	agents := []*Agent{New(cfg("a1")), New(cfg("a2")), New(cfg("a3"))}
	_, err := RunSequentialAgentsWithDepth(context.Background(), agents, "x", 2)
	if err == nil {
		t.Fatal("expected depth-exceeded error, got nil")
	}
	if !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("err=%v, want ErrDepthExceeded", err)
	}
}

func TestTaskToolWithDepth_Exceeds(t *testing.T) {
	leaf := &Tool{
		Name:       "leaf",
		Parameters: simpleStringParams(),
		Func: func(_ context.Context, _ json.RawMessage) (any, error) {
			return "leaf-ok", nil
		},
	}
	tt := TaskToolWithDepth("delegate", []TaskOption{
		{Name: "leaf", Description: "leaf", Tool: leaf},
	}, 1)

	ctx := context.Background()
	if _, err := tt.Func(ctx, json.RawMessage(`{"agent":"leaf","task":"x"}`)); err != nil {
		t.Fatalf("depth-0 delegation should succeed: %v", err)
	}
	_, err := tt.Func(WithDepth(ctx, 1), json.RawMessage(`{"agent":"leaf","task":"x"}`))
	if err == nil {
		t.Fatal("expected depth-exceeded error at depth 1, got nil")
	}
	if !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("err=%v, want ErrDepthExceeded", err)
	}
}
