package server

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

type memoryStore struct {
	mu    sync.Mutex
	items map[string]agentcore.StateSnapshot
}

func newMemoryStore() *memoryStore {
	return &memoryStore{items: make(map[string]agentcore.StateSnapshot)}
}

func (s *memoryStore) Save(_ context.Context, key string, snap agentcore.StateSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = snap
	return nil
}

func (s *memoryStore) Load(_ context.Context, key string) (agentcore.StateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok := s.items[key]
	if !ok {
		return agentcore.StateSnapshot{}, http.ErrMissingFile
	}
	return snap, nil
}

func (s *memoryStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}

func (s *memoryStore) List(_ context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.items))
	for key := range s.items {
		keys = append(keys, key)
	}
	return keys, nil
}

func (s *memoryStore) Has(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.items[key]
	return ok, nil
}

type historyProvider struct{}

func (historyProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	var lastUser string
	var userCount int
	for _, msg := range req.Messages {
		if msg.Role == agentcore.RoleUser {
			userCount++
			lastUser = msg.Content
		}
	}
	return &agentcore.ProviderResponse{
		Content: "users:" + string(rune('0'+userCount)) + " last:" + lastUser,
	}, nil
}

func (historyProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

type captureThinkingProvider struct {
	lastModel          string
	lastThinking       *agentcore.ThinkingConfig
	lastResponseFormat *agentcore.ResponseFormat
	lastMessages       []agentcore.Message
}

type failingProvider struct{}

func (failingProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	return nil, errors.New("provider boom")
}

func (failingProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta)
	close(ch)
	return ch, nil
}

type blockingProvider struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockingProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	select {
	case <-p.started:
	default:
		close(p.started)
	}
	<-p.release
	return &agentcore.ProviderResponse{Content: "done"}, nil
}

func (p *blockingProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta)
	close(ch)
	return ch, nil
}

func (p *captureThinkingProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	p.lastModel = req.Model
	p.lastMessages = append([]agentcore.Message(nil), req.Messages...)
	if req.Thinking != nil {
		cp := *req.Thinking
		p.lastThinking = &cp
	} else {
		p.lastThinking = nil
	}
	p.lastResponseFormat = agentcore.CloneResponseFormat(req.ResponseFormat)
	return &agentcore.ProviderResponse{Content: "ok"}, nil
}

func (p *captureThinkingProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta, 1)
	ch <- agentcore.StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

type serverMCPToolProvider struct {
	turn int
}

func (p *serverMCPToolProvider) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	p.turn++
	if p.turn == 1 {
		return &agentcore.ProviderResponse{
			ToolCalls: []agentcore.ToolCall{{
				ID:        "call_1",
				Name:      "mcp.echo",
				Arguments: `{"text":"refresh-tools"}`,
			}},
		}, nil
	}
	return &agentcore.ProviderResponse{Content: "done"}, nil
}

func (p *serverMCPToolProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	ch := make(chan agentcore.StreamDelta)
	close(ch)
	return ch, nil
}
func TestServerSwitchModel(t *testing.T) {
	srv := New(agentcore.Config{
		ModelConfig:      agentcore.ModelConfig{Model: "deepseek-v4-flash"},
		CompactionConfig: agentcore.CompactionConfig{ContextWindow: 1_000_000},
	})

	// 仅切换模型 + 上下文窗口
	srv.SwitchModel(nil, "glm-5.2", 256_000)
	cfg := srv.snapshotConfig()
	if cfg.Model != "glm-5.2" {
		t.Errorf("Model = %q, want glm-5.2", cfg.Model)
	}
	if cfg.CompactionConfig.ContextWindow != 256_000 {
		t.Errorf("ContextWindow = %d, want 256000", cfg.CompactionConfig.ContextWindow)
	}

	// 空参数为 no-op（保持现有配置）
	srv.SwitchModel(nil, "", 0)
	cfg = srv.snapshotConfig()
	if cfg.Model != "glm-5.2" || cfg.CompactionConfig.ContextWindow != 256_000 {
		t.Errorf("empty args should be no-op, got model=%q ctx=%d", cfg.Model, cfg.CompactionConfig.ContextWindow)
	}
}
