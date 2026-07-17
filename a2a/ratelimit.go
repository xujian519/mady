package a2a

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Token Bucket Rate Limiter
// ---------------------------------------------------------------------------

// RateLimiter implements a per-IP token bucket rate limiter.
type RateLimiter struct {
	mu              sync.RWMutex
	buckets         map[string]*bucket
	rate            float64 // tokens per second
	capacity        int     // bucket capacity
	cleanupInterval time.Duration
	stop            chan struct{}

	// trustedProxies 可信反向代理的 CIDR 列表（trustedMu 保护）。
	// 仅当请求的 RemoteAddr 命中可信代理时，才采纳 X-Forwarded-For /
	// X-Real-IP，防止外部客户端伪造代理头绕过限流（C5 修复）。
	trustedMu      sync.RWMutex
	trustedProxies []*net.IPNet
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

// NewRateLimiter creates a token bucket rate limiter.
// rate: tokens added per second.
// capacity: maximum tokens per bucket.
// 默认仅信任本地回环代理（127.0.0.0/8、::1/128）：本机反代场景下
// X-Forwarded-For / X-Real-IP 会被采纳，外部客户端无法伪造。
func NewRateLimiter(rate float64, capacity int) *RateLimiter {
	rl := &RateLimiter{
		buckets:         make(map[string]*bucket),
		rate:            rate,
		capacity:        capacity,
		cleanupInterval: 5 * time.Minute,
		stop:            make(chan struct{}),
	}
	rl.trustedProxies = parseCIDRs([]string{"127.0.0.0/8", "::1/128"})
	go rl.cleanupLoop()
	return rl
}

// SetTrustedProxies 替换可信反向代理 CIDR 列表（如 "10.0.0.0/8"）。
// 传入空列表表示不信任任何代理（代理头一律忽略）。
func (rl *RateLimiter) SetTrustedProxies(cidrs ...string) error {
	parsed := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, ipNet, err := net.ParseCIDR(c)
		if err != nil {
			return fmt.Errorf("a2a: invalid trusted proxy CIDR %q: %w", c, err)
		}
		parsed = append(parsed, ipNet)
	}
	rl.trustedMu.Lock()
	rl.trustedProxies = parsed
	rl.trustedMu.Unlock()
	return nil
}

// parseCIDRs 解析 CIDR 列表，跳过非法项（仅用于可信默认值）。
func parseCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if _, ipNet, err := net.ParseCIDR(c); err == nil {
			out = append(out, ipNet)
		}
	}
	return out
}

// isTrustedProxy 报告 IP 是否属于可信代理网段。
func (rl *RateLimiter) isTrustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	rl.trustedMu.RLock()
	defer rl.trustedMu.RUnlock()
	for _, ipNet := range rl.trustedProxies {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// Stop stops the rate limiter's background cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stop)
}

// cleanupLoop periodically removes stale buckets.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.purgeStale()
		case <-rl.stop:
			return
		}
	}
}

// purgeStale removes buckets that haven't been used in the last cleanup interval.
func (rl *RateLimiter) purgeStale() {
	cutoff := time.Now().Add(-rl.cleanupInterval)
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for ip, b := range rl.buckets {
		if b.lastCheck.Before(cutoff) {
			delete(rl.buckets, ip)
		}
	}
}

// Allow checks if the given IP has enough tokens for a single request.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[ip]
	if !ok {
		b = &bucket{tokens: float64(rl.capacity) - 1, lastCheck: time.Now()}
		rl.buckets[ip] = b
		return true
	}

	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens = minFloat(float64(rl.capacity), b.tokens+elapsed*rl.rate)
	b.lastCheck = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// Middleware returns an HTTP middleware that applies rate limiting.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		// 仅当 RemoteAddr 是可信代理时才采纳 X-Forwarded-For / X-Real-IP，
		// 否则外部客户端可伪造代理头无限更换限流身份（C5 修复）。
		if rl.isTrustedProxy(net.ParseIP(ip)) {
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				ip = strings.Split(fwd, ",")[0]
				ip = strings.TrimSpace(ip)
			} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
				ip = strings.TrimSpace(realIP)
			}
		}
		if !rl.Allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &JSONRPCError{Code: -32006, Message: "rate limit exceeded"},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
