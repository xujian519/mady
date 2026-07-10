package agentcore

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

func TestTerminateResult(t *testing.T) {
	r := TerminateResult("done")
	if !r.Terminate || r.ForLLM != "done" || r.ForUser != "done" {
		t.Fatalf("unexpected DualToolOutput: %+v", r)
	}
}

func TestExecutorEarlyExit(t *testing.T) {
	tool := &Tool{
		Name:       "fin",
		Parameters: simpleStringParams(),
		Func: func(_ context.Context, _ json.RawMessage) (any, error) {
			return TerminateResult("final"), nil
		},
	}
	reg := NewRegistry()
	reg.Register(tool)
	ex := NewExecutor(reg)
	res := ex.Execute(context.Background(), ToolCall{ID: "1", Name: "fin", Arguments: `{"input":"x"}`}, NewState())
	if !res.Terminate {
		t.Fatalf("expected Terminate=true, got %+v", res)
	}
	if res.Result != "final" {
		t.Fatalf("result=%q want final", res.Result)
	}
}

type earlyExitProvider struct {
	mu       sync.Mutex
	calls    int
	toolName string
}

func (p *earlyExitProvider) Complete(_ context.Context, _ *ProviderRequest) (*ProviderResponse, error) {
	p.mu.Lock()
	call := p.calls
	p.calls++
	p.mu.Unlock()
	if call == 0 {
		return &ProviderResponse{
			ToolCalls: []ToolCall{{ID: "c1", Name: p.toolName, Arguments: `{"input":"x"}`}},
		}, nil
	}
	return &ProviderResponse{Content: "SHOULD_NOT_BE_USED"}, nil
}

func (p *earlyExitProvider) Stream(_ context.Context, _ *ProviderRequest) (<-chan StreamDelta, error) {
	ch := make(chan StreamDelta, 1)
	ch <- StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

func (p *earlyExitProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestAgentEarlyExit(t *testing.T) {
	tool := &Tool{
		Name:       "finish",
		Parameters: simpleStringParams(),
		Func: func(_ context.Context, _ json.RawMessage) (any, error) {
			return TerminateResult("tool-final-answer"), nil
		},
	}
	p := &earlyExitProvider{toolName: "finish"}
	agent := New(Config{
		ModelConfig:     ModelConfig{Name: "ee", Model: "stub", Provider: p},
		ExecutionConfig: ExecutionConfig{MaxTurns: 10},
		Tools:           []*Tool{tool},
	})
	out, err := agent.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if out != "tool-final-answer" {
		t.Fatalf("output=%q want tool-final-answer", out)
	}
	if cc := p.callCount(); cc != 1 {
		t.Fatalf("provider called %d times, want 1 (early-exit should skip the second LLM turn)", cc)
	}
}

func simpleStringParams() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{"type": "string"},
		},
		"required": []any{"input"},
	}
}
