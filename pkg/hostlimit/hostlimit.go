package hostlimit

import (
	"context"
	"net/url"
	"sync"
	"time"
)

// Limit 是一个主机的限速参数。零值字段表示不限制该维度。
type Limit struct {
	// MinInterval 是同一主机两次请求开始之间的最小间隔。
	MinInterval time.Duration
	// MaxConcurrent 是同一主机最大在途请求数。
	MaxConcurrent int
}

// Limiter 按主机维护限速状态；并发安全，可在多连接器间共享。
type Limiter struct {
	mu      sync.Mutex
	next    map[string]time.Time
	sems    map[string]chan struct{}
	now     func() time.Time
	sleepFn func(context.Context, time.Duration) error
}

// New 创建限速器。
func New() *Limiter {
	return &Limiter{
		next:    make(map[string]time.Time),
		sems:    make(map[string]chan struct{}),
		now:     time.Now,
		sleepFn: SleepContext,
	}
}

// Acquire 为 rawURL 申请一个请求槽位；返回的 release 必须在请求结束后调用。
// 不同主机的 Acquire 互不阻塞：限速状态按主机分别维护。
func (l *Limiter) Acquire(ctx context.Context, rawURL string, limit Limit) (func(), error) {
	host := hostOf(rawURL)
	if host == "" {
		return func() {}, nil
	}
	if limit.MinInterval > 0 {
		if err := l.pace(ctx, host, limit.MinInterval); err != nil {
			return nil, err
		}
	}
	if limit.MaxConcurrent > 0 {
		sem := l.semaphore(host, limit.MaxConcurrent)
		select {
		case sem <- struct{}{}:
			return func() { <-sem }, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return func() {}, nil
}

// semaphore 返回主机的并发槽位；同一主机再次以不同上限调用时取较大值。
func (l *Limiter) semaphore(host string, maxConcurrent int) chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	sem, ok := l.sems[host]
	if ok && cap(sem) >= maxConcurrent {
		return sem
	}
	sem = make(chan struct{}, maxConcurrent)
	l.sems[host] = sem
	return sem
}

// pace 预约该主机下一次允许开始的时间并等待到点。
func (l *Limiter) pace(ctx context.Context, host string, interval time.Duration) error {
	l.mu.Lock()
	now := l.now()
	start := now
	if next, ok := l.next[host]; ok && next.After(now) {
		start = next
	}
	l.next[host] = start.Add(interval)
	l.mu.Unlock()
	return l.sleepFn(ctx, start.Sub(now))
}

// hostOf 提取 URL 的主机名；解析失败返回空（不限速）。
func hostOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// SleepContext 是可取消的睡眠。
func SleepContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
