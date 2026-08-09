package acp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/xujian519/mady/acp"
	"github.com/xujian519/mady/agentcore"
)

// safeBuffer 是并发安全的 bytes.Buffer：ACP prompt 的响应由异步 goroutine
// 写入，测试主 goroutine 轮询读取，需避免数据竞争。
type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.b.Bytes()...)
}

// truncationAgentFactory 返回包装真实 agentcore.Agent 的实例，Core() 暴露
// LastFinishReason() 供 handlePrompt 读取。
type truncationAgentFactory struct {
	agent *agentcore.Agent
}

func (f *truncationAgentFactory) CreateAgent(context.Context, string, string, string, string) (acp.AgentInstance, error) {
	return &coreWrappedInstance{core: f.agent}, nil
}

func (f *truncationAgentFactory) DefaultModel() string { return "test-model" }
func (f *truncationAgentFactory) DefaultMode() string  { return "test-mode" }
func (f *truncationAgentFactory) AvailableModes() []acp.SessionMode {
	return []acp.SessionMode{{ID: "test-mode", Name: "Test Mode", Description: "Default test mode"}}
}

type coreWrappedInstance struct {
	core *agentcore.Agent
}

func (a *coreWrappedInstance) Run(ctx context.Context, input string) (string, error) {
	return a.core.Run(ctx, input)
}
func (a *coreWrappedInstance) Core() *agentcore.Agent { return a.core }
func (a *coreWrappedInstance) Model() string          { return "test-model" }
func (a *coreWrappedInstance) Mode() string           { return "test-mode" }

// lengthProvider 每次返回被 max_tokens 截断的响应。
type lengthProvider struct{}

func (lengthProvider) Complete(_ context.Context, _ *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	return &agentcore.ProviderResponse{Content: "partial answer", FinishReason: "length"}, nil
}

func (lengthProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	resp, _ := lengthProvider{}.Complete(ctx, req)
	ch := make(chan agentcore.StreamDelta, 2)
	ch <- agentcore.StreamDelta{Content: resp.Content}
	ch <- agentcore.StreamDelta{FinishReason: resp.FinishReason}
	ch <- agentcore.StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

// waitForJSONRPCResponse 轮询输出流直到出现指定 id 的 JSON-RPC 响应。
func waitForJSONRPCResponse(t *testing.T, buf *safeBuffer, wantID string) *acp.JSONRPCResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
		for _, line := range lines {
			var r acp.JSONRPCResponse
			if err := json.Unmarshal(line, &r); err != nil {
				continue // 行不完整（异步写入中），继续等待
			}
			if fmt.Sprintf("%v", r.ID) == wantID {
				return &r
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// TestPromptResponseCarriesFinishReason 验证 ACP prompt 的 PromptResponse
// 透传模型结束原因：真实 agent 输出被 max_tokens 截断（续写一次后仍截断）
// 时，客户端应通过 finishReason="length" 得知输出可能不完整。
func TestPromptResponseCarriesFinishReason(t *testing.T) {
	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:     "acp-trunc-test",
			Model:    "stub",
			Provider: lengthProvider{},
		},
		ExecutionConfig: agentcore.ExecutionConfig{MaxTurns: 10},
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	sm := acp.NewSessionManager(acp.SessionManagerConfig{
		AgentFactory: &truncationAgentFactory{agent: agent},
		Logger:       logger,
	})
	if _, err := sm.CreateSession(context.Background(), "/tmp", "s1"); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	output := &safeBuffer{}
	input := `{"jsonrpc":"2.0","id":1,"method":"session/prompt","params":{"sessionId":"s1","prompt":[{"type":"text","text":"hi"}]}}` + "\n"
	srv := acp.NewServer(acp.ServerConfig{
		SessionManager: sm,
		AgentInfo:      acp.AgentInfo{Name: "test", Version: "1.0"},
		Reader:         bytes.NewReader([]byte(input)),
		Writer:         output,
		Logger:         logger,
	})

	go func() { _ = srv.Run(context.Background()) }()

	resp := waitForJSONRPCResponse(t, output, "1")
	if resp == nil {
		t.Fatal("timed out waiting for prompt response")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	var pr acp.PromptResponse
	if err := json.Unmarshal(resp.Result, &pr); err != nil {
		t.Fatalf("unmarshal PromptResponse: %v", err)
	}
	if pr.StopReason != "end_turn" {
		t.Fatalf("StopReason = %q, want %q", pr.StopReason, "end_turn")
	}
	if pr.FinishReason != "length" {
		t.Fatalf("FinishReason = %q, want %q（截断输出须透传 length 供客户端提示）", pr.FinishReason, "length")
	}
}
