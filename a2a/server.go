package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type contextKey string

const lastEventIDKey contextKey = "lastEventID"

func withLastEventID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, lastEventIDKey, id)
}

func lastEventIDFromCtx(ctx context.Context) int {
	v := ctx.Value(lastEventIDKey)
	if v == nil {
		return 0
	}
	s, ok := v.(string)
	if !ok {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

const (
	defaultMaxRequestBody = 10 << 20 // 10 MB
	defaultTaskTimeout    = 5 * time.Minute
)

// ---------------------------------------------------------------------------
// AgentHandler is the interface that agents must implement to be exposed
// via the A2A protocol.
// ---------------------------------------------------------------------------

type AgentHandler interface {
	Card() AgentCard
	SendTask(ctx context.Context, req SendTaskRequest) (*Task, error)
	GetTask(ctx context.Context, req GetTaskRequest) (*Task, error)
	CancelTask(ctx context.Context, req CancelTaskRequest) (*Task, error)
	QueryTasks(ctx context.Context, req QueryTasksRequest) (*QueryTasksResult, error)
	SetPushNotification(ctx context.Context, req SetPushNotificationRequest) error
	GetPushNotification(ctx context.Context, taskID string) (*PushNotificationConfig, error)
}

type TaskUpdatePublisher interface {
	PublishTaskUpdate(taskID string, ev *TaskUpdateEvent)
}

type StreamingHandler interface {
	SetUpdatePublisher(TaskUpdatePublisher)
}

// Server exposes an AgentHandler over HTTP/SSE with A2A protocol endpoints.
type Server struct {
	handler AgentHandler
	cors    CORSConfig
	auth    AuthConfig
	srv     *http.Server
	logger  *slog.Logger

	taskTTL       time.Duration
	cleanupTicker *time.Ticker
	cleanupStop   chan struct{}

	rateLimiter *RateLimiter

	// allowedOrigins 是 WebSocket Origin 显式白名单（C4 修复），
	// 在同源与本地回环来源之外追加放行的 Origin（完整匹配，含 scheme+host[:port]）。
	allowedOrigins []string

	sessionMgr *SessionManager
	sessionTTL time.Duration

	taskStates    map[string]*taskSubState
	taskStatesMu  sync.RWMutex
	totalHistSize atomic.Int64
	maxHistoryLen int
	maxTotalHist  int

	maxRequestBody int64
	taskTimeout    time.Duration
	requestTimeout time.Duration
}

// CORSConfig configures cross-origin resource sharing.
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	AllowCredentials bool
}

// AuthConfig configures authentication for the A2A server.
type AuthConfig struct {
	APIKey      string
	BearerToken string
}

// NewServer creates an A2A server wrapping the given handler.
func NewServer(handler AgentHandler, opts ...ServerOption) *Server {
	s := &Server{
		handler:        handler,
		taskStates:     make(map[string]*taskSubState),
		maxHistoryLen:  64,
		maxTotalHist:   10000,
		logger:         slog.Default(),
		maxRequestBody: defaultMaxRequestBody,
		taskTimeout:    defaultTaskTimeout,
		requestTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	if sh, ok := s.handler.(StreamingHandler); ok {
		sh.SetUpdatePublisher(s)
	}

	if s.taskTTL > 0 {
		s.startCleanup()
	}
	return s
}

// Handler returns an http.Handler with all A2A routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent.json", s.handleAgentCard)
	mux.HandleFunc("/", s.handleJSONRPC)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ws", s.handleWebSocket)

	h := withAuth(withCORS(mux, s.cors), s.auth)
	if s.rateLimiter != nil {
		h = s.rateLimiter.Middleware(h)
	}
	return h
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(addr string) error {
	s.srv = &http.Server{Addr: addr, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	return s.srv.ListenAndServe()
}

// Shutdown gracefully shuts down the server and closes all subscriptions.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
		close(s.cleanupStop)
	}

	if s.rateLimiter != nil {
		s.rateLimiter.Stop()
	}

	s.taskStatesMu.Lock()
	for id, ts := range s.taskStates {
		ts.mu.Lock()
		for _, ch := range ts.subs {
			close(ch)
		}
		ts.subs = nil
		ts.mu.Unlock()
		delete(s.taskStates, id)
	}
	s.taskStatesMu.Unlock()

	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// ---------------------------------------------------------------------------
// HTTP Handlers
// ---------------------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	card := s.handler.Card()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(card)
}

// ---------------------------------------------------------------------------
// recordingResponseWriter captures dispatchJSONRPC output for batch handling.
// ---------------------------------------------------------------------------

type recordingResponseWriter struct {
	code int
	Body *bytes.Buffer
}

func httptestNewRecorder() *recordingResponseWriter {
	return &recordingResponseWriter{Body: new(bytes.Buffer)}
}

func (r *recordingResponseWriter) Header() http.Header         { return http.Header{} }
func (r *recordingResponseWriter) Write(b []byte) (int, error) { return r.Body.Write(b) }
func (r *recordingResponseWriter) WriteHeader(code int)        { r.code = code }
