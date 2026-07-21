package cli

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// stubCompleter 实现 Completer 接口，返回固定内容。
type stubCompleter struct {
	mu      sync.Mutex
	calls   int
	content string
	err     error
	panicV  any
}

func (s *stubCompleter) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if s.panicV != nil {
		panic(s.panicV)
	}
	if s.err != nil {
		return nil, s.err
	}
	return &agentcore.ProviderResponse{Content: s.content}, nil
}

// TestCallProviderSimple_ReturnsContent 验证 callProviderSimple 正确提取
// ProviderResponse.Content（而非用 fmt.Sprintf("%v") 把结构体格式化成 &{...}）。
func TestCallProviderSimple_ReturnsContent(t *testing.T) {
	stub := &stubCompleter{content: "预测答案"}
	got, err := callProviderSimple(context.Background(), stub, "输入")
	if err != nil {
		t.Fatalf("不期望错误: %v", err)
	}
	if got != "预测答案" {
		t.Errorf("期望 Content %q, 得到 %q", "预测答案", got)
	}
}

// TestCallProviderSimple_PropagatesError 验证 provider 错误正确传播。
func TestCallProviderSimple_PropagatesError(t *testing.T) {
	stub := &stubCompleter{err: errors.New("网络错误")}
	got, err := callProviderSimple(context.Background(), stub, "输入")
	if err == nil {
		t.Fatalf("期望错误, 得到 nil")
	}
	if got != "" {
		t.Errorf("错误时期望空串, 得到 %q", got)
	}
}

// TestRunLive_GoroutinePanicIsolated 验证 live 模式 goroutine panic 被隔离：
// 不会卡死 wg.Wait()，panic 转为该用例的错误结果。
func TestRunLive_GoroutinePanicIsolated(t *testing.T) {
	stub := &stubCompleter{panicV: "模拟 provider panic"}
	cli := &EvalCLI{
		Mode:        ModeLive,
		Workers:     2,
		TimeoutSec:  5,
		LLMModel:    "stub",
		LLMProvider: func(string) (Completer, error) { return stub, nil },
		// 使用内置最小用例集
		Suite: "p1",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// 即便所有用例 panic，RunCLI 也应在超时前返回（不卡死 wg.Wait）
	result, err := RunCLI(ctx, cli)
	// 不应因 panic 返回 fatal 错误（panic 被隔离成单用例错误）
	if err != nil {
		t.Fatalf("RunCLI 不应整体失败: %v", err)
	}
	if result == nil || result.Report == nil {
		t.Fatalf("期望非空结果")
	}
}
