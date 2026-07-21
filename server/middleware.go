package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture the status code for
// logging and metrics. A zero-value statusCode is treated as 200.
//
// It also forwards http.Flusher so SSE handlers continue to work through the
// middleware chain (the embedded http.ResponseWriter interface alone cannot
// promote Flush() from the concrete value — see Go interface promotion rules).
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader implements http.ResponseWriter, capturing the status code.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher by forwarding to the underlying ResponseWriter
// if it supports flushing (required for SSE endpoints).
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// generateRequestID 生成一个 16 字节的十六进制随机请求 ID。
func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// contextKey 是上下文键的私有类型，避免与其他包冲突。
type contextKey string

const requestIDKey contextKey = "request_id"

// newRequestIDContext 在 context 中注入 request_id。
func newRequestIDContext(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromContext 从 context 中提取 request_id。
// 若上下文中未设置，返回空字符串。
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// loggingMiddleware 记录每个 HTTP 请求的方法、路径、状态码、持续时间和 request_id。
//
// 优先级顺序提取 request_id：
//  1. X-Request-ID 请求头
//  2. 自动生成 32 字符十六进制随机串
//
// 输出使用 slog.Default().InfoContext()，附加结构化字段：
//   - slog.String("request_id", id)
//   - slog.String("method", r.Method)
//   - slog.String("path", r.URL.Path)
//   - slog.Int("status", statusCode)
//   - slog.Float64("duration_ms", duration)
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 提取或生成 request_id
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		// 包装 ResponseWriter 以捕获状态码
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// 将 request_id 注入请求上下文（供下游 handler 使用）
		r = r.WithContext(newRequestIDContext(r.Context(), requestID))

		// 执行请求
		next.ServeHTTP(rw, r)

		// 日志记录
		duration := time.Since(start)
		slog.Default().InfoContext(r.Context(), "HTTP",
			slog.String("request_id", requestID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rw.statusCode),
			slog.Float64("duration_ms", float64(duration.Microseconds())/1000.0),
		)

		// 回写 X-Request-ID 响应头
		w.Header().Set("X-Request-ID", requestID)
	})
}

// withVersionHeader 为响应添加 X-API-Version 头。
func withVersionHeader(next http.Handler, version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-API-Version", version)
		next.ServeHTTP(w, r)
	})
}

// withDeprecationNotice 为旧版 API 路径（如 /api/chat）添加废弃提示头。
func withDeprecationNotice(next http.Handler) http.Handler {
	const deprecationDate = "2026-07-21"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-API-Deprecated", "true")
		w.Header().Set("X-API-Deprecated-Date", deprecationDate)
		next.ServeHTTP(w, r)
	})
}
