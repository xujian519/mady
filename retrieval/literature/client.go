package literature

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/xujian519/mady/pkg/cache"
	"github.com/xujian519/mady/pkg/hostlimit"
)

// 默认预算（对齐上游 dsh tool-literature）。
const (
	DefaultTimeout   = 30 * time.Second
	DefaultCacheTTL  = 5 * time.Minute
	DefaultCacheSize = 500
	userAgent        = "mady-tool-literature/0.1 (+https://github.com/xujian519/mady)"
	maxResponseBytes = 8 << 20
)

// HTTPError 是非 2xx 响应。
type HTTPError struct {
	Status int
	URL    string
	Body   string
}

func (e *HTTPError) Error() string {
	body := e.Body
	if len([]rune(body)) > 500 {
		body = string([]rune(body)[:500])
	}
	return fmt.Sprintf("HTTP %d for %s: %s", e.Status, e.URL, body)
}

// Doer 是 HTTP 客户端抽象（测试注入）。
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client 是文献源共享的 HTTP 客户端：按主机限速 + GET 短 TTL 缓存。
type Client struct {
	doer    Doer
	limiter *hostlimit.Limiter
	cache   *cache.Async[[]byte]
	timeout time.Duration
}

// ClientOptions 配置客户端。
type ClientOptions struct {
	Doer      Doer
	Timeout   time.Duration
	CacheTTL  time.Duration
	CacheSize int
}

// NewClient 创建客户端。
func NewClient(opts ClientOptions) *Client {
	doer := opts.Doer
	if doer == nil {
		doer = &http.Client{Timeout: DefaultTimeout}
	}
	ttl := opts.CacheTTL
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	size := opts.CacheSize
	if size == 0 {
		size = DefaultCacheSize
	}
	return &Client{
		doer:    doer,
		limiter: hostlimit.New(),
		cache:   cache.NewAsync[[]byte](ttl, size),
		timeout: opts.Timeout,
	}
}

// GetJSON 取 JSON 并解码到 target。响应体为空或不是 JSON 时不入缓存。
func (c *Client) GetJSON(ctx context.Context, url string, limit hostlimit.Limit, target any) error {
	body, err := c.get(ctx, url, "application/json", limit)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("非 JSON 响应（%s）：%.300s", url, body)
	}
	return nil
}

// GetText 取文本（arXiv Atom、Crossref 等）。
func (c *Client) GetText(ctx context.Context, url string, limit hostlimit.Limit) ([]byte, error) {
	return c.get(ctx, url, "*/*", limit)
}

func (c *Client) get(ctx context.Context, url, accept string, limit hostlimit.Limit) ([]byte, error) {
	key := "GET\x00" + accept + "\x00" + url
	return c.cache.GetOrLoadContext(ctx, key,
		func() ([]byte, error) { return c.fetch(ctx, url, accept, limit) },
		func(body []byte) bool { return looksValid(body, accept) },
	)
}

func (c *Client) fetch(ctx context.Context, url, accept string, limit hostlimit.Limit) ([]byte, error) {
	release, err := c.limiter.Acquire(ctx, url, limit)
	if err != nil {
		return nil, err
	}
	defer release()

	requestCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.doer.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{Status: resp.StatusCode, URL: url, Body: string(body)}
	}
	return body, nil
}

// looksValid 过滤不应进缓存的响应：空 body，或声明要 JSON 却拿到 HTML 错误页。
func looksValid(body []byte, accept string) bool {
	if len(body) == 0 {
		return false
	}
	if accept == "application/json" {
		var probe any
		return json.Unmarshal(body, &probe) == nil
	}
	return true
}
