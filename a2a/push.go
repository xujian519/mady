package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"
)

// ipResolver 抽象 DNS 解析，便于在单测中注入 mock 覆盖 DNS rebinding 向量。
// *net.DefaultResolver（*net.Resolver 类型）天然实现此接口。
type ipResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// ssrfSafeDialer 在 connect 时校验解析得到的 IP，关闭
// "validateWebhookURL 提前 LookupHost + http.Client.Do 内部二次解析"
// 之间的 DNS rebinding TOCTOU 窗口。
//
// 线程安全契约：
//   - allowPrivate 是 atomic.Bool，可在运行时通过 SetAllowPrivate 并发修改；
//   - resolver 字段不是并发安全的，必须在构造期确定，构造后不可改；
//   - DialContext 可被多个 goroutine 并发调用（http.Transport 默认行为）。
type ssrfSafeDialer struct {
	allowPrivate atomic.Bool
	resolver     ipResolver // 构造期注入；nil 时 DialContext 内部回退到 net.DefaultResolver
}

// newSSRFSafeDialer 构造带 SSRF 防护的 dialer。resolver 为 nil 时使用 net.DefaultResolver。
func newSSRFSafeDialer(allowPrivate bool, resolver ipResolver) *ssrfSafeDialer {
	d := &ssrfSafeDialer{resolver: resolver}
	d.allowPrivate.Store(allowPrivate)
	return d
}

// SetAllowPrivate 运行时切换是否放行私网 IP（原子操作，线程安全）。
// 主要供 WithAllowPrivate builder 调用；也可在 Notify 期间动态切换。
func (d *ssrfSafeDialer) SetAllowPrivate(allow bool) {
	d.allowPrivate.Store(allow)
}

func (d *ssrfSafeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if d.allowPrivate.Load() {
		return defaultDialer.DialContext(ctx, network, addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("ssrf dialer: %w", err)
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return nil, fmt.Errorf("ssrf protection: dial private IP %s refused", ip)
		}
		return defaultDialer.DialContext(ctx, network, addr)
	}
	// 自行解析后选首个公网 IP 直连，绕开 http.Transport 内部的二次 DNS 解析，
	// 从而关闭 DNS rebinding TOCTOU 窗口（攻击者无法在两次解析间切换 IP）。
	resolver := d.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("ssrf dialer lookup %q: %w", host, err)
	}
	var chosen net.IP
	for _, a := range addrs {
		if !isPrivateIP(a.IP) {
			chosen = a.IP
			break
		}
	}
	if chosen == nil {
		return nil, fmt.Errorf("ssrf protection: host %q resolves only to private IPs", host)
	}
	return defaultDialer.DialContext(ctx, network, net.JoinHostPort(chosen.String(), port))
}

var defaultDialer = &net.Dialer{}

// PushNotifier handles webhook-based push notification delivery with
// exponential backoff retry and private-IP protection.
//
// 线程安全：构造后并发调用 Notify 安全；WithAllowPrivate 亦可在运行时调用
// （底层通过 atomic.Bool 传播到 ssrfSafeDialer，对所有后续 Notify 立即生效）。
type PushNotifier struct {
	dialer     *ssrfSafeDialer
	client     *http.Client
	maxRetries int
	backoff    time.Duration
}

// NewPushNotifier creates a new push notifier.
func NewPushNotifier() *PushNotifier {
	dialer := newSSRFSafeDialer(false, nil)
	return &PushNotifier{
		dialer:     dialer,
		client:     newSSRFSafeClient(dialer),
		maxRetries: 3,
		backoff:    500 * time.Millisecond,
	}
}

func newSSRFSafeClient(dialer *ssrfSafeDialer) *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
		},
	}
}

// WithAllowPrivate 控制是否放行私网 IP（默认 false）。
// 通过 atomic.Bool 写入底层 dialer，对正在进行和后续的 Notify 调用即时生效。
// 可在构造期链式调用，也可在运行时并发调用。
func (n *PushNotifier) WithAllowPrivate(allow bool) *PushNotifier {
	n.dialer.SetAllowPrivate(allow)
	return n
}

// Notify sends a task update to the configured webhook with exponential backoff retry.
func (n *PushNotifier) Notify(ctx context.Context, cfg *PushNotificationConfig, task *Task) error {
	if !n.dialer.allowPrivate.Load() {
		if err := validateWebhookURL(cfg.URL); err != nil {
			return fmt.Errorf("invalid webhook URL: %w", err)
		}
	}

	payload := map[string]any{
		"taskId": task.ID,
		"state":  task.State,
		"task":   task,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal push payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= n.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := n.backoff * time.Duration(1<<uint(attempt-1))
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
			timer.Stop()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create push request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}
		for k, v := range cfg.Headers {
			req.Header.Set(k, v)
		}

		resp, err := n.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("send push notification: %w", err)
			continue
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("push webhook returned %d: %s", resp.StatusCode, string(respBody))
			if resp.StatusCode >= 500 {
				continue
			}
			return lastErr
		}

		return nil
	}

	return fmt.Errorf("push notification failed after %d retries: %w", n.maxRetries, lastErr)
}

func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q not allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty host")
	}
	if ip := net.ParseIP(host); ip != nil && isPrivateIP(ip) {
		return fmt.Errorf("private IP addresses not allowed")
	}
	return nil
}

var privateRanges = func() []*net.IPNet {
	cidrs := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"127.0.0.0/8", "169.254.0.0/16",
		"::1/128", "fc00::/7", "fe80::/10",
	}
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, s := range cidrs {
		_, n, _ := net.ParseCIDR(s)
		out = append(out, n)
	}
	return out
}()

func isPrivateIP(ip net.IP) bool {
	for _, network := range privateRanges {
		if network != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}
