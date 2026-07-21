package graph

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecuteWithPolicy_Nil(t *testing.T) {
	// nil policy 应该直接执行，行为不变。
	called := false
	node := func(ctx context.Context, state PregelState) (PregelState, error) {
		called = true
		return PregelState{"result": "ok"}, nil
	}

	out, err := executeWithPolicy(context.Background(), "test", node, PregelState{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("node was not called")
	}
	if out["result"] != "ok" {
		t.Errorf("expected 'ok', got %v", out["result"])
	}
}

func TestExecuteWithPolicy_RetrySuccess(t *testing.T) {
	// 前两次失败，第三次成功。
	attempts := 0
	node := func(ctx context.Context, state PregelState) (PregelState, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("transient error")
		}
		return PregelState{"done": true}, nil
	}

	policy := &NodePolicy{
		MaxRetries: 3,
		RetryDelay: 1 * time.Millisecond,
	}

	out, err := executeWithPolicy(context.Background(), "test", node, PregelState{}, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if out["done"] != true {
		t.Errorf("expected done=true, got %v", out["done"])
	}
}

func TestExecuteWithPolicy_RetryExhausted(t *testing.T) {
	// 所有重试都失败。
	node := func(ctx context.Context, state PregelState) (PregelState, error) {
		return nil, errors.New("always fail")
	}

	policy := &NodePolicy{
		MaxRetries: 2,
		RetryDelay: 1 * time.Millisecond,
	}

	_, err := executeWithPolicy(context.Background(), "test", node, PregelState{}, policy)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if !strings.Contains(err.Error(), "重试 2 次") {
		t.Errorf("error message should mention retries: %v", err)
	}
}

func TestExecuteWithPolicy_Timeout(t *testing.T) {
	// 节点执行时间超过 timeout。
	node := func(ctx context.Context, state PregelState) (PregelState, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return PregelState{}, nil
		}
	}

	policy := &NodePolicy{
		Timeout: 50 * time.Millisecond,
	}

	_, err := executeWithPolicy(context.Background(), "test", node, PregelState{}, policy)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestExecuteWithPolicy_TimeoutDuringRetry(t *testing.T) {
	// 超时跨越重试：第一次失败，重试期间超时。
	attempts := 0
	node := func(ctx context.Context, state PregelState) (PregelState, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("fail first")
		}
		// 第二次等待很长时间（超时应该截断）
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
			return PregelState{}, nil
		}
	}

	policy := &NodePolicy{
		MaxRetries: 3,
		RetryDelay: 5 * time.Millisecond,
		Timeout:    100 * time.Millisecond,
	}

	_, err := executeWithPolicy(context.Background(), "test", node, PregelState{}, policy)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestExecuteWithPolicy_SideEffect(t *testing.T) {
	// 副作用节点返回空 state，merge 为 no-op。
	node := func(ctx context.Context, state PregelState) (PregelState, error) {
		// 模拟 I/O 操作。
		return PregelState{"should_be_ignored": "value"}, nil
	}

	policy := &NodePolicy{
		SideEffect: true,
	}

	out, err := executeWithPolicy(context.Background(), "test", node, PregelState{"input": "data"}, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("side effect node should return empty state, got %v", out)
	}
}

func TestExecuteWithPolicy_PanicRecovery(t *testing.T) {
	// panic 应该被捕获并作为可重试的错误返回。
	node := func(ctx context.Context, state PregelState) (PregelState, error) {
		panic("unexpected nil pointer")
	}

	policy := &NodePolicy{
		MaxRetries: 0, // 不重试，但 panic 应该被恢复。
	}

	_, err := executeWithPolicy(context.Background(), "test", node, PregelState{}, policy)
	if err == nil {
		t.Fatal("expected error from panic recovery")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("error should mention panic: %v", err)
	}
}

func TestExecuteWithPolicy_PanicThenRetry(t *testing.T) {
	// panic 后重试成功。
	attempts := 0
	node := func(ctx context.Context, state PregelState) (PregelState, error) {
		attempts++
		if attempts < 2 {
			panic("crash")
		}
		return PregelState{"recovered": true}, nil
	}

	policy := &NodePolicy{
		MaxRetries: 2,
		RetryDelay: 1 * time.Millisecond,
	}

	out, err := executeWithPolicy(context.Background(), "test", node, PregelState{}, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["recovered"] != true {
		t.Errorf("expected recovered=true, got %v", out["recovered"])
	}
}

func TestExecuteWithPolicy_ContextCancelled(t *testing.T) {
	// context 取消后不应重试。
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	node := func(ctx context.Context, state PregelState) (PregelState, error) {
		return nil, errors.New("some error")
	}

	policy := &NodePolicy{
		MaxRetries: 5,
		RetryDelay: 1 * time.Millisecond,
	}

	_, err := executeWithPolicy(ctx, "test", node, PregelState{}, policy)
	if err == nil {
		t.Fatal("expected context canceled error")
	}
}

func TestPregelGraph_SetNodePolicy(t *testing.T) {
	pg := NewPregelGraph()
	pg.AddNode("node_a", func(ctx context.Context, state PregelState) (PregelState, error) {
		return state, nil
	})

	// 设置策略。
	err := pg.SetNodePolicy("node_a", NodePolicy{MaxRetries: 3})
	if err != nil {
		t.Fatalf("SetNodePolicy failed: %v", err)
	}

	// 编译后策略应该被复制。
	cpg, err := pg.Compile("node_a")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if cpg.nodePolicies["node_a"].MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", cpg.nodePolicies["node_a"].MaxRetries)
	}
}

func TestPregelGraph_SetNodePolicy_UnknownNode(t *testing.T) {
	pg := NewPregelGraph()
	err := pg.SetNodePolicy("nonexistent", NodePolicy{MaxRetries: 1})
	if err == nil {
		t.Fatal("expected error for unknown node")
	}
}

func TestCompiledPregelRun_WithRetry(t *testing.T) {
	// 集成测试：带重试策略的 Pregel 图执行。
	pg := NewPregelGraph()

	attempts := 0
	pg.AddNode("flaky", func(ctx context.Context, state PregelState) (PregelState, error) {
		attempts++
		if attempts < 2 {
			return nil, errors.New("transient")
		}
		return PregelState{"result": "success"}, nil
	})
	pg.AddEdge("flaky", PregelEnd)

	pg.SetNodePolicy("flaky", NodePolicy{
		MaxRetries: 2,
		RetryDelay: 1 * time.Millisecond,
	})

	cpg, err := pg.Compile("flaky")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	state, err := cpg.Run(context.Background(), PregelState{"input": "test"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if state["result"] != "success" {
		t.Errorf("expected 'success', got %v", state["result"])
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestCompiledPregelRun_WithTimeout(t *testing.T) {
	// 集成测试：超时的 Pregel 图执行。
	pg := NewPregelGraph()

	pg.AddNode("slow", func(ctx context.Context, state PregelState) (PregelState, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return PregelState{"slow": "done"}, nil
		}
	})
	pg.AddEdge("slow", PregelEnd)

	pg.SetNodePolicy("slow", NodePolicy{
		Timeout: 50 * time.Millisecond,
	})

	cpg, err := pg.Compile("slow")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	_, err = cpg.Run(context.Background(), PregelState{})
	if err == nil {
		t.Fatal("expected timeout error from Run")
	}
}
