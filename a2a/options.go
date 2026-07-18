package a2a

import (
	"log/slog"
	"time"
)

// ServerOption configures a Server.
type ServerOption func(*Server)

func WithLogger(logger *slog.Logger) ServerOption {
	return func(s *Server) { s.logger = logger }
}

func WithTaskTTL(ttl time.Duration) ServerOption {
	return func(s *Server) { s.taskTTL = ttl }
}

func WithRateLimiter(limiter *RateLimiter) ServerOption {
	return func(s *Server) { s.rateLimiter = limiter }
}

// WithAllowedOrigins 追加 WebSocket Origin 白名单（完整 Origin 字符串，
// 如 "https://app.example.com"）。同源与本地回环来源默认放行，无需配置。
func WithAllowedOrigins(origins ...string) ServerOption {
	return func(s *Server) { s.allowedOrigins = append(s.allowedOrigins, origins...) }
}

func WithSessionManager(ttl time.Duration) ServerOption {
	return func(s *Server) {
		s.sessionMgr = NewSessionManager()
		s.sessionTTL = ttl
	}
}

func WithCORS(cfg CORSConfig) ServerOption {
	return func(s *Server) { s.cors = cfg }
}

func WithAuth(cfg AuthConfig) ServerOption {
	return func(s *Server) { s.auth = cfg }
}

func WithMaxRequestBody(n int64) ServerOption {
	return func(s *Server) { s.maxRequestBody = n }
}

func WithTaskTimeout(d time.Duration) ServerOption {
	return func(s *Server) { s.taskTimeout = d }
}

func WithRequestTimeout(d time.Duration) ServerOption {
	return func(s *Server) { s.requestTimeout = d }
}

func WithMaxEventHistory(perTask, total int) ServerOption {
	return func(s *Server) {
		s.maxHistoryLen = perTask
		s.maxTotalHist = total
	}
}
