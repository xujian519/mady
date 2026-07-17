package server

// C2 安全修复回归测试：Agent 池引用计数。
// 修复前：loadAgent 从池中取 agent 不持锁，releaseAgent 的淘汰/替换逻辑
// 可能关闭仍被其他请求使用的 agent（use-after-free）。
// 修复后：池化 agent 由 poolEntry 引用计数跟踪，只有 refs 归零且已淘汰
// 时才真正 Close。

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// closeProbeExtension 通过 Dispose 调用次数观测 agent 被 Close 的次数。
type closeProbeExtension struct {
	disposeCount *atomic.Int32
}

func (e *closeProbeExtension) Name() string { return "close-probe" }
func (e *closeProbeExtension) Init(_ context.Context, _ *agentcore.Agent) error {
	return nil
}
func (e *closeProbeExtension) Dispose() error {
	e.disposeCount.Add(1)
	return nil
}

// newPoolTestServer 构造带 Close 探针与内存存储的测试服务器。
func newPoolTestServer(t *testing.T, poolLimit int) (*Server, *atomic.Int32) {
	t.Helper()
	var disposeCount atomic.Int32
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		Store:      newMemoryStore(),
		Extensions: []agentcore.Extension{&closeProbeExtension{disposeCount: &disposeCount}},
	})
	srv.poolLimit = poolLimit
	return srv, &disposeCount
}

// borrowAndSave 借用 agent 并保存状态（保证下次池命中时 LoadState 成功）。
func borrowAndSave(t *testing.T, srv *Server, threadID string) *poolEntry {
	t.Helper()
	entry, err := srv.loadAgent(context.Background(), threadID, nil)
	if err != nil {
		t.Fatalf("loadAgent(%s) failed: %v", threadID, err)
	}
	if err := srv.saveAgentState(context.Background(), entry.agent, threadID); err != nil {
		t.Fatalf("saveAgentState(%s) failed: %v", threadID, err)
	}
	return entry
}

func TestAgentPoolSharedBorrowNotClosedEarly(t *testing.T) {
	srv, disposeCount := newPoolTestServer(t, 64)
	ctx := context.Background()

	// 首次借用（全新 agent）并归还入池。
	first := borrowAndSave(t, srv, "thread-1")
	srv.releaseAgent(first, "thread-1")

	// 两次并发借用同一 threadID：必须拿到同一个池化 agent。
	e2, err := srv.loadAgent(ctx, "thread-1", nil)
	if err != nil {
		t.Fatalf("second borrow failed: %v", err)
	}
	e3, err := srv.loadAgent(ctx, "thread-1", nil)
	if err != nil {
		t.Fatalf("third borrow failed: %v", err)
	}
	if e2.agent != e3.agent || e2.agent != first.agent {
		t.Fatal("expected pooled agent to be shared across borrows")
	}

	// 归还第一次借用：agent 仍被 e3 使用，绝不能关闭。
	srv.releaseAgent(e2, "thread-1")
	if got := disposeCount.Load(); got != 0 {
		t.Fatalf("agent closed while still in use (dispose count = %d)", got)
	}

	// 归还第二次借用：refcount 归零但 entry 仍在池中复用，也不应关闭。
	srv.releaseAgent(e3, "thread-1")
	if got := disposeCount.Load(); got != 0 {
		t.Fatalf("pooled idle agent should stay alive (dispose count = %d)", got)
	}

	// 服务器关闭：池内空闲 agent 被关闭（恰好一次）。
	srv.Close()
	if got := disposeCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 close after server Close, got %d", got)
	}
}

func TestAgentPoolEvictionSkipsInUseAgent(t *testing.T) {
	srv, disposeCount := newPoolTestServer(t, 1)
	ctx := context.Background()

	// thread-1 入池后再借出（处于"使用中"状态）。
	first := borrowAndSave(t, srv, "thread-1")
	srv.releaseAgent(first, "thread-1")
	inUse, err := srv.loadAgent(ctx, "thread-1", nil)
	if err != nil {
		t.Fatalf("borrow in-use agent failed: %v", err)
	}

	// thread-2 的全新 agent 归还时池已满：淘汰必须跳过使用中的 thread-1，
	// 直接关闭 thread-2 的 agent。
	other := borrowAndSave(t, srv, "thread-2")
	srv.releaseAgent(other, "thread-2")
	if got := disposeCount.Load(); got != 1 {
		t.Fatalf("expected only thread-2 agent closed, dispose count = %d", got)
	}

	// 使用中的 agent 仍然可用。
	if _, err := inUse.agent.Run(ctx, "ping"); err != nil {
		t.Fatalf("in-use agent broken after eviction pressure: %v", err)
	}

	// 归还在用 agent：refcount 归零，entry 仍池化，不关闭。
	srv.releaseAgent(inUse, "thread-1")
	if got := disposeCount.Load(); got != 1 {
		t.Fatalf("in-use agent must survive eviction, dispose count = %d", got)
	}

	srv.Close()
	if got := disposeCount.Load(); got != 2 {
		t.Fatalf("expected 2 closes after server Close, got %d", got)
	}
}

func TestAgentPoolCloseDefersInUseAgentUntilRelease(t *testing.T) {
	srv, disposeCount := newPoolTestServer(t, 64)

	first := borrowAndSave(t, srv, "thread-1")
	srv.releaseAgent(first, "thread-1")
	inUse, err := srv.loadAgent(context.Background(), "thread-1", nil)
	if err != nil {
		t.Fatalf("borrow failed: %v", err)
	}

	// 服务器关闭时 agent 仍在使用：只标记淘汰，不立即关闭。
	srv.Close()
	if got := disposeCount.Load(); got != 0 {
		t.Fatalf("Close() must not close in-use agent, dispose count = %d", got)
	}

	// 最后一次归还时才真正关闭。
	srv.releaseAgent(inUse, "thread-1")
	if got := disposeCount.Load(); got != 1 {
		t.Fatalf("expected agent closed on final release, dispose count = %d", got)
	}
}

// TestAgentPoolConcurrentStress 在竞态检测器（-race）下压测池的并发
// 借用/归还/淘汰路径：小池限 + 多 threadID 制造持续淘汰压力。
func TestAgentPoolConcurrentStress(t *testing.T) {
	srv, _ := newPoolTestServer(t, 2)

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				threadID := fmt.Sprintf("thread-%d", (worker+i)%5)
				entry, err := srv.loadAgent(context.Background(), threadID, nil)
				if err != nil {
					continue
				}
				_ = srv.saveAgentState(context.Background(), entry.agent, threadID)
				srv.releaseAgent(entry, threadID)
			}
		}(w)
	}
	wg.Wait()
	srv.Close()
}
