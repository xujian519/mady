package a2a

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(10, 5)
	defer rl.Stop()

	ip := "127.0.0.1"

	// Should allow initial burst
	for i := 0; i < 5; i++ {
		if !rl.Allow(ip) {
			t.Fatalf("expected allow on attempt %d", i+1)
		}
	}

	// Should deny after burst
	if rl.Allow(ip) {
		t.Fatal("expected deny after burst")
	}

	// Wait for token refill by polling with a ticker.
	var allowed bool
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(2 * time.Second)
loop:
	for {
		select {
		case <-ticker.C:
			if rl.Allow(ip) {
				allowed = true
				break loop
			}
		case <-deadline:
			break loop
		}
	}
	if !allowed {
		t.Fatal("expected allow after refill")
	}
}

func TestRateLimiter_Middleware(t *testing.T) {
	rl := NewRateLimiter(100, 2)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := NewRateLimiter(100, 2)
	defer rl.Stop()

	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Exhaust ip1
	for i := 0; i < 2; i++ {
		rl.Allow(ip1)
	}
	if rl.Allow(ip1) {
		t.Fatal("expected ip1 to be rate limited")
	}

	// ip2 should still be allowed
	if !rl.Allow(ip2) {
		t.Fatal("expected ip2 to be allowed")
	}
}

// C5 安全修复回归测试：仅可信代理的 X-Forwarded-For / X-Real-IP 才被采纳。

func TestRateLimiter_TrustedProxyHonorsForwardedFor(t *testing.T) {
	rl := NewRateLimiter(100, 2)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// RemoteAddr 是回环代理（默认可信）：XFF 中的真实客户端 IP 参与限流。
	// 用两个不同的 XFF 各发 2 次，再发第 3 次时按各自桶独立限流。
	for i := 0; i < 2; i++ {
		for _, fwd := range []string{"203.0.113.1", "203.0.113.2"} {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "127.0.0.1:8080"
			req.Header.Set("X-Forwarded-For", fwd)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("request %d for %s: expected 200, got %d", i+1, fwd, rr.Code)
			}
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for exhausted forwarded IP, got %d", rr.Code)
	}
}

func TestRateLimiter_UntrustedRemoteAddrIgnoresForwardedFor(t *testing.T) {
	rl := NewRateLimiter(100, 2)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// RemoteAddr 不是可信代理：伪造的 XFF 必须被忽略，按 RemoteAddr 限流。
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "198.51.100.7:1234"
		req.Header.Set("X-Forwarded-For", "203.0.113."+string(rune('1'+i)))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}
	// 第三次换一个新的伪造 XFF，仍应因 RemoteAddr 桶耗尽而被限流。
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.7:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.99")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 — spoofed XFF must not bypass rate limit, got %d", rr.Code)
	}
}

func TestRateLimiter_SetTrustedProxies(t *testing.T) {
	rl := NewRateLimiter(100, 1)
	defer rl.Stop()

	// 配置信任 10.0.0.0/8 网段的反代。
	if err := rl.SetTrustedProxies("10.0.0.0/8"); err != nil {
		t.Fatalf("SetTrustedProxies failed: %v", err)
	}
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 来自可信网段的代理：XFF 生效，同一真实 IP 第二次即被限流。
	for i, want := range []int{http.StatusOK, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.1.2.3:443"
		req.Header.Set("X-Forwarded-For", "203.0.113.50")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != want {
			t.Fatalf("request %d: expected %d, got %d", i+1, want, rr.Code)
		}
	}

	// 非法 CIDR 报错；空列表表示不信任任何代理。
	if err := rl.SetTrustedProxies("not-a-cidr"); err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
	if err := rl.SetTrustedProxies(); err != nil {
		t.Fatalf("SetTrustedProxies() failed: %v", err)
	}
	if rl.isTrustedProxy([]byte{127, 0, 0, 1}) {
		t.Fatal("expected no trusted proxies after SetTrustedProxies()")
	}
}
