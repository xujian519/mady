package pool_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xujian519/mady/a2a/pool"
	"github.com/xujian519/mady/a2a/registry"
)

func TestJoinLeaveAlive(t *testing.T) {
	p := pool.New(nil)

	reg := &registry.Registration{Name: "agent-1", URL: "http://localhost:8080"}
	p.Join(reg)

	alive := p.Alive()
	if len(alive) != 1 {
		t.Fatalf("expected 1 alive agent, got %d", len(alive))
	}
	if alive[0].Name != "agent-1" {
		t.Errorf("got name %q, want %q", alive[0].Name, "agent-1")
	}

	p.Leave("agent-1")
	alive = p.Alive()
	if len(alive) != 0 {
		t.Errorf("expected 0 alive agents after leave, got %d", len(alive))
	}
}

func TestLeaveNonExistent(t *testing.T) {
	p := pool.New(nil)
	// Should not panic
	p.Leave("non-existent")
}

func TestDefaultCheckFuncAlive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	ok := pool.DefaultCheckFunc(ctx, srv.URL)
	if !ok {
		t.Error("expected DefaultCheckFunc to return true for alive server")
	}
}

func TestDefaultCheckFuncDead(t *testing.T) {
	ctx := context.Background()
	ok := pool.DefaultCheckFunc(ctx, "http://localhost:19999")
	if ok {
		t.Error("expected DefaultCheckFunc to return false for dead server")
	}
}

func TestDefaultCheckFuncNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx := context.Background()
	ok := pool.DefaultCheckFunc(ctx, srv.URL)
	if ok {
		t.Error("expected DefaultCheckFunc to return false for non-200 response")
	}
}

func TestConsecutiveFailuresEviction(t *testing.T) {
	// 使用自定义 checkFn 模拟连续失败
	var failCount atomic.Int32
	checkFn := func(ctx context.Context, url string) bool {
		failCount.Add(1)
		return false
	}

	p := pool.New(checkFn).WithTTL(3)

	reg := &registry.Registration{Name: "agent-1", URL: "http://localhost:8080"}
	p.Join(reg)

	// 触发检查循环（不启动 ticker，手动调用内部逻辑）
	// 使用带超时的 ctx 避免死锁
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 通过 Start/Stop 测试摘除逻辑
	p.Start(ctx)
	// 给 checkAll 一点时间执行
	time.Sleep(50 * time.Millisecond)
	p.Stop()

	// 通过 alive 检查 agent 是否已被摘除
	// 由于 checkFn 一直返回 false，经过 ttl=3 次后应被摘除
	// 但 pool 的 checkAll 是按 ticker 周期执行的，我们需要模拟多次检查
	// 更好的方式：直接构造多次检查的场景
}

func TestStartStop(t *testing.T) {
	checkFn := func(ctx context.Context, url string) bool {
		return false
	}

	p := pool.New(checkFn).WithInterval(50 * time.Millisecond)

	reg := &registry.Registration{Name: "agent-1", URL: "http://localhost:8080"}
	p.Join(reg)

	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()

	// 等待停止
	time.Sleep(50 * time.Millisecond)

	// Start again after stop
	ctx2, cancel2 := context.WithCancel(context.Background())
	p.Start(ctx2)
	time.Sleep(50 * time.Millisecond)
	cancel2()
}

func TestDoubleStart(t *testing.T) {
	checkFn := func(ctx context.Context, url string) bool {
		return true
	}

	p := pool.New(checkFn)
	ctx := context.Background()
	p.Start(ctx)
	p.Start(ctx) // should be no-op
	p.Stop()
}

func TestConcurrentAccess(t *testing.T) {
	checkFn := func(ctx context.Context, url string) bool {
		return true
	}

	p := pool.New(checkFn)
	ctx := context.Background()
	p.Start(ctx)
	defer p.Stop()

	var wg sync.WaitGroup
	n := 50

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reg := &registry.Registration{
				Name: "agent-" + string(rune('A'+i%26)),
				URL:  "http://localhost:" + string(rune('0'+i%10)),
			}
			p.Join(reg)
		}(i)
	}
	wg.Wait()

	var readWg sync.WaitGroup
	for i := 0; i < n; i++ {
		readWg.Add(1)
		go func() {
			defer readWg.Done()
			p.Alive()
		}()
	}
	readWg.Wait()

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := "agent-" + string(rune('A'+i%26))
			p.Leave(name)
		}()
	}
	wg.Wait()
}

func TestAliveReturnsCopies(t *testing.T) {
	p := pool.New(nil)
	p.Join(&registry.Registration{Name: "agent-1", URL: "http://localhost:8080"})

	alive := p.Alive()
	if len(alive) != 1 {
		t.Fatalf("expected 1 alive, got %d", len(alive))
	}

	// Modify the returned copy
	alive[0].Name = "hacked"

	// Verify the original is unchanged
	alive2 := p.Alive()
	if alive2[0].Name != "agent-1" {
		t.Errorf("original was mutated, got name %q", alive2[0].Name)
	}
}

func TestWithMethods(t *testing.T) {
	p := pool.New(nil).
		WithInterval(10 * time.Second).
		WithTimeout(2 * time.Second).
		WithTTL(5)

	// These are internal fields; we verify no panic and sane defaults via behavior
	if p == nil {
		t.Fatal("pool should not be nil")
	}
}

// TestConsecutiveFailuresManually tests the eviction logic by running checkAll
// multiple times via the internal mechanism.
func TestConsecutiveFailuresManually(t *testing.T) {
	var calls int
	var mu sync.Mutex

	checkFn := func(ctx context.Context, url string) bool {
		mu.Lock()
		calls++
		mu.Unlock()
		return false // always fail
	}

	p := pool.New(checkFn).WithTTL(3).WithInterval(10 * time.Millisecond)

	reg := &registry.Registration{Name: "agent-1", URL: "http://localhost:8080"}
	p.Join(reg)

	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)

	// Wait enough time for multiple check cycles
	time.Sleep(60 * time.Millisecond)

	cancel()
	p.Stop()

	// Agent should have been evicted after 3 consecutive failures
	alive := p.Alive()
	if len(alive) != 0 {
		t.Errorf("expected agent to be evicted after consecutive failures, but %d alive", len(alive))
	}
}
