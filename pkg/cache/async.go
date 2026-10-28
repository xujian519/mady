package cache

import (
	"fmt"
	"sync"
	"time"
)

// 默认参数：与上游 dsh patent-cache 一致（10 分钟、100 条）。
const (
	DefaultTTL        = 10 * time.Minute
	DefaultMaxEntries = 100
)

type cachedValue[V any] struct {
	value     V
	expiresAt time.Time
}

type loadCall[V any] struct {
	done  chan struct{}
	value V
	err   error
}

// Async 是「LRU + TTL + 并发合并」的异步结果缓存：同一键的并发 GetOrLoad
// 只执行一次加载；只有成功且 cacheable 的结果才写入缓存。
type Async[V any] struct {
	mu       sync.Mutex
	entries  *LRU[string, cachedValue[V]]
	inflight map[string]*loadCall[V]
	ttl      time.Duration
	now      func() time.Time
}

// NewAsync 创建异步结果缓存；ttl/maxEntries ≤0 时取默认值。
func NewAsync[V any](ttl time.Duration, maxEntries int) *Async[V] {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	return &Async[V]{
		entries:  NewLRU[string, cachedValue[V]](maxEntries),
		inflight: make(map[string]*loadCall[V]),
		ttl:      ttl,
		now:      time.Now,
	}
}

// GetOrLoad 返回缓存值；未命中时执行 load 并缓存结果（cacheable 为 nil 时总是缓存
// 成功结果）。加载失败不写缓存，错误原样返回。
func (c *Async[V]) GetOrLoad(key string, load func() (V, error), cacheable func(V) bool) (V, error) {
	c.mu.Lock()
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.value, call.err
	}
	if cached, ok := c.entries.Get(key); ok {
		if c.now().Before(cached.expiresAt) {
			c.mu.Unlock()
			return cached.value, nil
		}
		c.entries.Delete(key)
	}
	call := &loadCall[V]{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	// 加载器 panic 也必须清理 inflight 并唤醒等待者：否则该键的后续请求会永远
	// 阻塞在一个永不关闭的 channel 上。panic 转成错误返回，不向等待者传播零值。
	value, err := func() (result V, loadErr error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				loadErr = fmt.Errorf("cache: loader panic: %v", recovered)
			}
		}()
		return load()
	}()

	c.mu.Lock()
	delete(c.inflight, key)
	if err == nil && (cacheable == nil || cacheable(value)) {
		c.entries.Set(key, cachedValue[V]{value: value, expiresAt: c.now().Add(c.ttl)})
	}
	c.mu.Unlock()

	call.value, call.err = value, err
	close(call.done)
	return value, err
}

// Len 返回当前缓存条目数（含已过期但尚未清理的条目）。
func (c *Async[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries.Len()
}

// Clear 清空缓存与进行中的合并表。
func (c *Async[V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries.Clear()
	clear(c.inflight)
}
