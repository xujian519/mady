package pool

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/xujian519/mady/a2a/registry"
)

// CheckFunc 健康检查函数类型。返回 true 表示 Agent 存活。
type CheckFunc func(ctx context.Context, url string) bool

// Pool 管理一组 Agent 的健康状态，定期执行心跳检查。
// 不健康的 Agent 自动从活跃列表摘除。
// 配置方法（WithInterval/WithTimeout/WithTTL）应在 Start 之前调用。
type Pool struct {
	mu       sync.RWMutex
	entries  map[string]*poolEntry
	interval time.Duration // 心跳周期，默认 30s
	timeout  time.Duration // 检查超时，默认 5s
	ttl      int           // 连续失败摘除阈值，默认 3 次
	checkFn  CheckFunc
	stopCh   chan struct{}
	started  bool
	closed   bool // 防止 stopCh 被重复关闭
}

type poolEntry struct {
	reg      *registry.Registration
	failures int
}

// New 创建心跳池，使用指定的健康检查函数。
func New(checkFn CheckFunc) *Pool {
	if checkFn == nil {
		checkFn = DefaultCheckFunc
	}
	return &Pool{
		entries:  make(map[string]*poolEntry),
		interval: 30 * time.Second,
		timeout:  5 * time.Second,
		ttl:      3,
		checkFn:  checkFn,
	}
}

// WithInterval 设置心跳间隔（默认 30s）。
func (p *Pool) WithInterval(d time.Duration) *Pool {
	if d > 0 {
		p.interval = d
	}
	return p
}

// WithTimeout 设置健康检查超时（默认 5s）。
func (p *Pool) WithTimeout(d time.Duration) *Pool {
	if d > 0 {
		p.timeout = d
	}
	return p
}

// WithTTL 设置连续失败摘除阈值（默认 3 次）。
func (p *Pool) WithTTL(n int) *Pool {
	if n > 0 {
		p.ttl = n
	}
	return p
}

// Join 将 Agent 加入心跳池。
func (p *Pool) Join(reg *registry.Registration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries[reg.Name] = &poolEntry{
		reg:      reg,
		failures: 0,
	}
}

// Leave 从心跳池移除 Agent。
func (p *Pool) Leave(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, name)
}

// Start 启动心跳检查循环。在 ctx 取消时自动停止。
func (p *Pool) Start(ctx context.Context) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.closed = false
	stopCh := make(chan struct{})
	p.stopCh = stopCh
	p.mu.Unlock()

	go func() {
		p.loop(ctx, stopCh)
	}()
}

func (p *Pool) loop(ctx context.Context, stopCh <-chan struct{}) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.stop()
			return
		case <-stopCh:
			return
		case <-ticker.C:
			p.checkAll(ctx)
		}
	}
}

func (p *Pool) checkAll(ctx context.Context) {
	p.mu.Lock()
	entries := make([]*poolEntry, 0, len(p.entries))
	for _, entry := range p.entries {
		entries = append(entries, entry)
	}
	p.mu.Unlock()

	for _, entry := range entries {
		alive := p.checkFn(ctx, entry.reg.URL)

		p.mu.Lock()
		if alive {
			entry.failures = 0
			entry.reg.HeartbeatAt = time.Now()
		} else {
			entry.failures++
			if entry.failures >= p.ttl {
				delete(p.entries, entry.reg.Name)
			}
		}
		p.mu.Unlock()
	}
}

// Stop 停止心跳检查。
func (p *Pool) Stop() {
	p.stop()
}

func (p *Pool) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.started = false
	if p.stopCh != nil {
		close(p.stopCh)
		p.stopCh = nil
	}
}

// Alive 返回当前存活的 Agent 列表。
func (p *Pool) Alive() []*registry.Registration {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*registry.Registration, 0, len(p.entries))
	for _, entry := range p.entries {
		cp := *entry.reg
		result = append(result, &cp)
	}
	return result
}

// DefaultCheckFunc 默认健康检查：向 Agent 的 /health 端点发送 GET 请求，
// 检查是否返回 200 状态码。
func DefaultCheckFunc(ctx context.Context, url string) bool {
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url+"/health", nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
