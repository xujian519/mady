package cache

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLRUEvictionAndRecency(t *testing.T) {
	lru := NewLRU[string, int](2)
	lru.Set("a", 1)
	lru.Set("b", 2)
	if _, ok := lru.Get("a"); !ok {
		t.Fatal("a should be present")
	}
	// a 刚被访问，b 成为最久未使用；写入 c 淘汰 b。
	lru.Set("c", 3)
	if lru.Has("b") {
		t.Error("b 应被淘汰")
	}
	if !lru.Has("a") || !lru.Has("c") {
		t.Error("a/c 应保留")
	}
	if lru.Len() != 2 {
		t.Errorf("len = %d", lru.Len())
	}
}

func TestLRUOverwriteKeepsSingleEntry(t *testing.T) {
	lru := NewLRU[string, int](2)
	lru.Set("a", 1)
	lru.Set("a", 2)
	if lru.Len() != 1 {
		t.Errorf("len = %d", lru.Len())
	}
	if value, _ := lru.Get("a"); value != 2 {
		t.Errorf("value = %d", value)
	}
}

func TestLRUDeleteAndClear(t *testing.T) {
	lru := NewLRU[string, int](3)
	lru.Set("a", 1)
	lru.Delete("a")
	if lru.Has("a") {
		t.Error("delete 后不应存在")
	}
	lru.Set("b", 2)
	lru.Clear()
	if lru.Len() != 0 {
		t.Errorf("clear 后 len = %d", lru.Len())
	}
}

func TestLRUConcurrentAccess(t *testing.T) {
	lru := NewLRU[int, int](16)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := range 100 {
				key := base*100 + j
				lru.Set(key, key)
				lru.Get(key)
			}
		}(i)
	}
	wg.Wait()
	if lru.Len() > 16 {
		t.Errorf("len = %d, want ≤16", lru.Len())
	}
}

func TestAsyncCachesSuccessfulResult(t *testing.T) {
	cl := NewAsync[int](time.Minute, 4)
	var calls atomic.Int32
	load := func() (int, error) {
		calls.Add(1)
		return 42, nil
	}
	for range 3 {
		value, err := cl.GetOrLoad("k", load, nil)
		if err != nil || value != 42 {
			t.Fatalf("value=%d err=%v", value, err)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("loader calls = %d, want 1", calls.Load())
	}
}

func TestAsyncErrorNotCached(t *testing.T) {
	cl := NewAsync[int](time.Minute, 4)
	boom := errors.New("boom")
	var calls atomic.Int32
	load := func() (int, error) {
		if calls.Add(1) == 1 {
			return 0, boom
		}
		return 7, nil
	}
	if _, err := cl.GetOrLoad("k", load, nil); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	value, err := cl.GetOrLoad("k", load, nil)
	if err != nil || value != 7 {
		t.Fatalf("失败结果不应入缓存：value=%d err=%v", value, err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestAsyncCacheablePredicate(t *testing.T) {
	cl := NewAsync[int](time.Minute, 4)
	var calls atomic.Int32
	load := func() (int, error) {
		calls.Add(1)
		return 0, nil
	}
	for range 2 {
		if _, err := cl.GetOrLoad("k", load, func(value int) bool { return value > 0 }); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("不可缓存的结果应每次重载，calls = %d", calls.Load())
	}
}

func TestAsyncTTLExpiry(t *testing.T) {
	cl := NewAsync[int](time.Minute, 4)
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	cl.now = func() time.Time { return now }
	var calls atomic.Int32
	load := func() (int, error) {
		return int(calls.Add(1)), nil
	}
	if value, _ := cl.GetOrLoad("k", load, nil); value != 1 {
		t.Fatalf("value = %d", value)
	}
	now = now.Add(30 * time.Second)
	if value, _ := cl.GetOrLoad("k", load, nil); value != 1 {
		t.Errorf("TTL 内应命中缓存，value = %d", value)
	}
	now = now.Add(31 * time.Second)
	if value, _ := cl.GetOrLoad("k", load, nil); value != 2 {
		t.Errorf("过期后应重载，value = %d", value)
	}
}

func TestAsyncInFlightMerge(t *testing.T) {
	cl := NewAsync[int](time.Minute, 4)
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	load := func() (int, error) {
		calls.Add(1)
		startedOnce.Do(func() { close(started) })
		<-release
		return 9, nil
	}
	results := make(chan int, 4)
	for range 4 {
		go func() {
			value, _ := cl.GetOrLoad("k", load, nil)
			results <- value
		}()
	}
	<-started
	close(release)
	for range 4 {
		if value := <-results; value != 9 {
			t.Errorf("value = %d", value)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("并发合并失败：loader calls = %d", calls.Load())
	}
}

func TestAsyncLoaderPanicRecovered(t *testing.T) {
	cl := NewAsync[int](time.Minute, 4)
	value, err := cl.GetOrLoad("k", func() (int, error) { panic("boom") }, nil)
	if err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("panic 应转为错误返回：value=%d err=%v", value, err)
	}
	// panic 后 inflight 必须清理：后续请求不能永久阻塞。
	done := make(chan struct{})
	go func() {
		defer close(done)
		if next, err := cl.GetOrLoad("k", func() (int, error) { return 7, nil }, nil); err != nil || next != 7 {
			t.Errorf("next = %d err=%v", next, err)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("panic 后同键请求被永久阻塞")
	}
}

func TestAsyncWaiterContextCanceled(t *testing.T) {
	cl := NewAsync[int](time.Minute, 4)
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	load := func() (int, error) {
		startedOnce.Do(func() { close(started) })
		<-release
		return 9, nil
	}
	go func() { _, _ = cl.GetOrLoad("k", load, nil) }()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cl.GetOrLoadContext(ctx, "k", func() (int, error) { return 0, nil }, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("等待者应返回 ctx 错误，得到 %v", err)
	}

	close(release)
	// 首个请求的结果仍应进入缓存，供后续读取。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if value, err := cl.GetOrLoad("k", func() (int, error) { return 0, nil }, nil); err == nil && value == 9 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("首个请求的结果未写入缓存")
}

func TestAsyncLRUCapacity(t *testing.T) {
	cl := NewAsync[int](time.Minute, 2)
	for _, key := range []string{"a", "b", "c"} {
		_, _ = cl.GetOrLoad(key, func() (int, error) { return 1, nil }, nil)
	}
	if cl.Len() != 2 {
		t.Errorf("len = %d, want 2", cl.Len())
	}
}

func TestNewAsyncDefaults(t *testing.T) {
	cl := NewAsync[int](0, 0)
	if cl.ttl != DefaultTTL {
		t.Errorf("ttl = %v", cl.ttl)
	}
	cl.Clear()
	if cl.Len() != 0 {
		t.Errorf("len = %d", cl.Len())
	}
}
