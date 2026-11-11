package a2a

import (
	"log/slog"
	"time"

	"github.com/xujian519/mady/a2a/pool"
	"github.com/xujian519/mady/a2a/registry"
)

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithLogger sets the logger for the A2A server.
func WithLogger(logger *slog.Logger) ServerOption {
	return func(s *Server) { s.logger = logger }
}

// WithTaskTTL sets the time-to-live duration for completed tasks before eviction.
func WithTaskTTL(ttl time.Duration) ServerOption {
	return func(s *Server) { s.taskTTL = ttl }
}

// WithRateLimiter sets the rate limiter for incoming requests.
func WithRateLimiter(limiter *RateLimiter) ServerOption {
	return func(s *Server) { s.rateLimiter = limiter }
}

// WithAllowedOrigins 追加 WebSocket Origin 白名单（完整 Origin 字符串，
// 如 "https://app.example.com"）。同源与本地回环来源默认放行，无需配置。
func WithAllowedOrigins(origins ...string) ServerOption {
	return func(s *Server) { s.allowedOrigins = append(s.allowedOrigins, origins...) }
}

// WithSessionManager enables session management with the given task TTL.
func WithSessionManager(ttl time.Duration) ServerOption {
	return func(s *Server) {
		s.sessionMgr = NewSessionManager()
		s.sessionTTL = ttl
	}
}

// WithCORS sets the CORS configuration for the A2A server.
func WithCORS(cfg CORSConfig) ServerOption {
	return func(s *Server) { s.cors = cfg }
}

// WithAuth sets the authentication configuration for the A2A server.
func WithAuth(cfg AuthConfig) ServerOption {
	return func(s *Server) { s.auth = cfg }
}

// WithMaxRequestBody sets the maximum request body size in bytes.
func WithMaxRequestBody(n int64) ServerOption {
	return func(s *Server) { s.maxRequestBody = n }
}

// WithTaskTimeout sets the default timeout for task execution.
func WithTaskTimeout(d time.Duration) ServerOption {
	return func(s *Server) { s.taskTimeout = d }
}

// WithRequestTimeout sets the HTTP request timeout.
func WithRequestTimeout(d time.Duration) ServerOption {
	return func(s *Server) { s.requestTimeout = d }
}

// WithMaxEventHistory sets the maximum number of events retained per task and total.
func WithMaxEventHistory(perTask, total int) ServerOption {
	return func(s *Server) {
		s.maxHistoryLen = perTask
		s.maxTotalHist = total
	}
}

// WithFederation 启用 A2A 联邦网络支持：注入 Agent 注册表和心跳健康池。
// pool 会在 NewServer 时自动启动后台健康检查，在 Shutdown 时停止。
// 传 nil 则不启用联邦功能（默认行为，向后兼容）。
func WithFederation(reg registry.Registry, p *pool.Pool) ServerOption {
	return func(s *Server) {
		s.federationRegistry = reg
		s.federationPool = p
	}
}
