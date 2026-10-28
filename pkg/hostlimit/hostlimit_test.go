package hostlimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSameHostPaced(t *testing.T) {
	limiter := New()
	limit := Limit{MinInterval: 30 * time.Millisecond}
	start := time.Now()
	for range 3 {
		release, err := limiter.Acquire(context.Background(), "https://api.openalex.org/works", limit)
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Errorf("同主机三次请求应至少间隔 2×30ms，实际 %v", elapsed)
	}
}

func TestDifferentHostsNotSerialized(t *testing.T) {
	limiter := New()
	limit := Limit{MinInterval: 80 * time.Millisecond}
	start := time.Now()
	var wg sync.WaitGroup
	for _, host := range []string{"https://a.example", "https://b.example", "https://c.example"} {
		wg.Go(func() {
			release, err := limiter.Acquire(context.Background(), host, limit)
			if err != nil {
				t.Errorf("acquire %s: %v", host, err)
				return
			}
			defer release()
		})
	}
	wg.Wait()
	// 三个不同主机应并行通过：总耗时远小于串行的 2×80ms（留足调度余量）。
	if elapsed := time.Since(start); elapsed > 120*time.Millisecond {
		t.Errorf("不同主机不应互相排队，实际耗时 %v", elapsed)
	}
}

func TestMaxConcurrentPerHost(t *testing.T) {
	limiter := New()
	limit := Limit{MaxConcurrent: 2}
	var active, maxActive atomic.Int32
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			release, err := limiter.Acquire(context.Background(), "https://api.openalex.org/works", limit)
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			current := active.Add(1)
			for {
				previous := maxActive.Load()
				if current <= previous || maxActive.CompareAndSwap(previous, current) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			active.Add(-1)
			release()
		})
	}
	wg.Wait()
	if maxActive.Load() > 2 {
		t.Errorf("同主机并发 = %d, want ≤2", maxActive.Load())
	}
}

func TestAcquireCanceledWhileWaiting(t *testing.T) {
	limiter := New()
	limit := Limit{MaxConcurrent: 1}
	release, err := limiter.Acquire(context.Background(), "https://a.example", limit)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := limiter.Acquire(ctx, "https://a.example", limit); err == nil {
		t.Error("槽位占满时等待应被取消")
	}
}

func TestAcquireCanceledDuringPace(t *testing.T) {
	limiter := New()
	limit := Limit{MinInterval: time.Hour}
	release, err := limiter.Acquire(context.Background(), "https://a.example", limit)
	if err != nil {
		t.Fatal(err)
	}
	release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := limiter.Acquire(ctx, "https://a.example", limit); err == nil {
		t.Error("等待最小间隔时取消应返回错误")
	}
}

func TestUnlimitedPassesThrough(t *testing.T) {
	limiter := New()
	start := time.Now()
	release, err := limiter.Acquire(context.Background(), "https://a.example", Limit{})
	if err != nil {
		t.Fatal(err)
	}
	release()
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Errorf("零值 Limit 不应等待，实际 %v", elapsed)
	}
}

func TestInvalidURLNotLimited(t *testing.T) {
	limiter := New()
	release, err := limiter.Acquire(context.Background(), "://bad", Limit{MinInterval: time.Hour})
	if err != nil {
		t.Fatalf("无效 URL 不应报错：%v", err)
	}
	release()
}
