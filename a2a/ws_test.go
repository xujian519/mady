package a2a

// C4 安全修复回归测试：WebSocket Origin 校验。
// 默认放行同源与本地回环来源，其余来源仅在显式白名单内放行。

import (
	"net/http/httptest"
	"testing"
)

func TestCheckWSOrigin(t *testing.T) {
	srv := &Server{allowedOrigins: []string{"https://app.example.com"}}

	cases := []struct {
		name   string
		origin string
		host   string
		want   bool
	}{
		{"无 Origin 头（非浏览器客户端）", "", "example.com", true},
		{"同源 http", "http://example.com", "example.com", true},
		{"同源 https", "https://example.com", "example.com", true},
		{"本地回环来源跨端口（本地开发前端）", "http://localhost:3000", "localhost:8080", true},
		{"回环 IP 来源", "http://127.0.0.1:5173", "127.0.0.1:8080", true},
		{"白名单内的跨域来源", "https://app.example.com", "api.example.com", true},
		{"白名单外的跨域来源", "https://evil.example.com", "api.example.com", false},
		{"伪造的子域名前缀", "https://api.example.com.evil.com", "api.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/ws", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if got := srv.checkWSOrigin(req); got != tc.want {
				t.Fatalf("checkWSOrigin(origin=%q, host=%q) = %v, want %v",
					tc.origin, tc.host, got, tc.want)
			}
		})
	}
}
