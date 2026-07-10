package agentcore

import (
	"context"
	"testing"
)

// BenchmarkAgentNew measures Agent creation overhead.
func BenchmarkAgentNew(b *testing.B) {
	for b.Loop() {
		agent := New(Config{
			ModelConfig: ModelConfig{
				Name:     "bench",
				Model:    "bench",
				Provider: &benchProvider{},
			},
			ExecutionConfig: ExecutionConfig{
				MaxTurns: 1,
			},
		})
		agent.Close()
	}
}

// BenchmarkAgentRun measures the overhead of a single agent run with no tools.
func BenchmarkAgentRun(b *testing.B) {
	agent := New(Config{
		ModelConfig: ModelConfig{
			Name:     "bench",
			Model:    "benchmark-model",
			Provider: &benchProvider{},
		},
		ExecutionConfig: ExecutionConfig{
			MaxTurns: 1,
		},
	})
	defer agent.Close()

	ctx := context.Background()
	b.ResetTimer()

	for b.Loop() {
		_, _ = agent.Run(ctx, "hello")
	}
}

// BenchmarkToolRegistry measures tool lookup performance at scale.
func BenchmarkToolRegistry(b *testing.B) {
	reg := NewRegistry()
	for i := range 1000 {
		reg.Register(&Tool{
			Name:        "tool_" + string(rune('a'+i%26)),
			Description: "benchmark tool",
			Parameters:  map[string]any{"type": "object"},
		})
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = reg.Get("tool_a")
	}
}

// BenchmarkCheckpointWriteRead measures checkpoint I/O throughput.
func BenchmarkCheckpointWriteRead(b *testing.B) {
	saver := NewMemoryCheckpointSaver()
	ctx := context.Background()

	b.Run("Write", func(b *testing.B) {
		snap := StateSnapshot{
			Messages: make([]Message, 100),
		}
		for b.Loop() {
			_, _ = saver.Append(ctx, "bench-key", snap)
		}
	})

	b.Run("Read", func(b *testing.B) {
		for b.Loop() {
			_, _, _ = saver.Latest(ctx, "bench-key")
		}
	})
}

// BenchmarkEventBusThroughput measures event publish throughput.
func BenchmarkEventBusThroughput(b *testing.B) {
	bus := NewEventBus()
	bus.On(EventMessageDelta, func(e Event) {})

	event := &MessageDeltaEvent{Delta: "hello"}
	b.ResetTimer()

	for b.Loop() {
		bus.Emit(event)
	}
	bus.Close()
}

// benchProvider is a no-op provider for benchmarks.
type benchProvider struct{}

func (p *benchProvider) Complete(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	return &ProviderResponse{
		Content: "benchmark response",
	}, nil
}

func (p *benchProvider) Stream(ctx context.Context, req *ProviderRequest) (<-chan StreamDelta, error) {
	ch := make(chan StreamDelta, 1)
	ch <- StreamDelta{
		Content: "benchmark response",
	}
	close(ch)
	return ch, nil
}
