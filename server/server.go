package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/pkg/csync"
)

// Server exposes an Agent as an HTTP/SSE API.
type Server struct {
	config   *csync.Value[agentcore.Config]
	eventBus *agentcore.EventBus
	cors     CORSConfig
	srv      atomic.Pointer[http.Server]

	agentPool  sync.Map // threadID -> *poolEntry; cached agents for reuse (refcounted)
	poolMu     sync.Mutex
	poolLimit  int
	disclosure atomic.Pointer[disclosureTaskManager]
	discMu     sync.Mutex // guards lazy init of disclosure

	// approvalStore 持久化人工复核决策（disclosure 复核端点等 HITL 触点）。
	// 由 SetApprovalStore 注入；未配置时复核端点返回 503。
	approvalMu    sync.RWMutex
	approvalStore domains.ApprovalStore

	maxRequestBodyBytes atomic.Int64
}

// CORSConfig 配置 HTTP 跨域资源共享策略。
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	AllowCredentials bool
}

// New 根据传入的 agentcore.Config 创建一个 Server 实例。
//
// 初始化内部 Agent 池（上限 64）、事件总线以及请求体大小限制（默认 10 MiB）。
// 调用方随后通过 ServeHTTP / RegisterHandler 等方法挂载到 HTTP 路由。
func New(cfg agentcore.Config) *Server {
	s := &Server{
		config:    csync.NewValue(cfg),
		eventBus:  agentcore.NewEventBus(),
		poolLimit: 64,
	}
	s.maxRequestBodyBytes.Store(defaultMaxRequestBodyBytes)
	return s
}

// defaultMaxRequestBodyBytes caps the size of incoming JSON request bodies to
// guard against memory-exhaustion denial-of-service via oversized payloads.
const defaultMaxRequestBodyBytes = 10 << 20 // 10 MiB

// SetMaxRequestBodyBytes overrides the maximum accepted request body size in
// bytes. Values <= 0 reset it to the default (10 MiB).
func (s *Server) SetMaxRequestBodyBytes(n int64) {
	if n <= 0 {
		n = defaultMaxRequestBodyBytes
	}
	s.maxRequestBodyBytes.Store(n)
}

// SetApprovalStore 注入人工决策留痕存储（如 domains/sqlite.SQLiteApprovalStore）。
// 传入 nil 可解除配置。未配置时 disclosure 复核端点返回 503，其余端点不受影响。
func (s *Server) SetApprovalStore(store domains.ApprovalStore) {
	s.approvalMu.Lock()
	defer s.approvalMu.Unlock()
	s.approvalStore = store
}

// getApprovalStore 返回当前配置的留痕存储，未配置时为 nil。
func (s *Server) getApprovalStore() domains.ApprovalStore {
	s.approvalMu.RLock()
	defer s.approvalMu.RUnlock()
	return s.approvalStore
}

// limitedBody wraps the request body with http.MaxBytesReader so that JSON
// decoding fails fast instead of buffering an unbounded payload into memory.
func (s *Server) limitedBody(w http.ResponseWriter, r *http.Request) io.Reader {
	limit := s.maxRequestBodyBytes.Load()
	if limit <= 0 {
		limit = defaultMaxRequestBodyBytes
	}
	return http.MaxBytesReader(w, r.Body, limit)
}

func (s *Server) On(t agentcore.EventType, h agentcore.EventHandler) func() {
	return s.eventBus.On(t, h)
}
func (s *Server) OnAll(h agentcore.EventHandler) func() { return s.eventBus.OnAll(h) }
func (s *Server) EmitEvent(e agentcore.Event)           { s.eventBus.Emit(e) }
func (s *Server) Close() {
	s.eventBus.Close()
	// 摘除全部池化 entry：空闲的立即 Close，仍在使用中的标记 evicted，
	// 由最后一次 releaseAgent 归还时关闭（避免 shutdown 时关闭使用中的 agent）。
	s.poolMu.Lock()
	s.agentPool.Range(func(key, value any) bool {
		entry := value.(*poolEntry)
		s.agentPool.Delete(key)
		entry.evicted = true
		if entry.refs == 0 {
			entry.agent.Close()
		}
		return true
	})
	s.poolMu.Unlock()
	if dm := s.disclosure.Load(); dm != nil {
		dm.close()
	}
}

// Handler returns an http.Handler wired with all routes.
// Mount it on your own mux or pass directly to http.ListenAndServe.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", s.handleChat)
	mux.HandleFunc("GET /api/skills", s.handleListSkills)
	mux.HandleFunc("GET /api/skills/diagnostics", s.handleSkillDiagnostics)
	mux.HandleFunc("GET /api/skills/events", s.handleSkillEvents)
	mux.HandleFunc("GET /api/skills/status", s.handleSkillStatus)
	mux.HandleFunc("POST /api/skills/reload", s.handleReloadSkills)
	mux.HandleFunc("POST /v1/disclosure/analyze", s.handleDisclosureAnalyze)
	mux.HandleFunc("GET /v1/disclosure/analyze/{task_id}", s.handleDisclosureStatus)
	mux.HandleFunc("GET /v1/disclosure/analyze/{task_id}/stream", s.handleDisclosureStream)
	mux.HandleFunc("POST /v1/disclosure/analyze/{task_id}/review", s.handleDisclosureReview)
	mux.HandleFunc("POST /api/threads", s.handleCreateThread)
	mux.HandleFunc("GET /api/threads", s.handleListThreads)
	mux.HandleFunc("GET /api/threads/{key}", s.handleGetThread)
	mux.HandleFunc("GET /api/threads/{key}/config", s.handleGetThreadConfig)
	mux.HandleFunc("PUT /api/threads/{key}/config", s.handlePutThreadConfig)
	mux.HandleFunc("GET /api/threads/{key}/thinking", s.handleGetThreadThinking)
	mux.HandleFunc("PUT /api/threads/{key}/thinking", s.handlePutThreadThinking)
	mux.HandleFunc("POST /api/threads/{key}/branch", s.handleBranchThread)
	mux.HandleFunc("DELETE /api/threads/{key}", s.handleDeleteThread)
	mux.HandleFunc("GET /api/states", s.handleListStates)
	mux.HandleFunc("GET /api/states/{key}", s.handleGetState)
	mux.HandleFunc("DELETE /api/states/{key}", s.handleDeleteState)
	mux.Handle("/agui/{path}", s.aguiHandler())
	return withCORS(mux, s.cors)
}

func (s *Server) aguiHandler() http.Handler {
	return agui.NewHandler(s.snapshotConfig())
}

func (s *Server) ListenAndServe(addr string) error {
	handler := s.Handler()
	s.srv.Store(&http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second})
	return s.srv.Load().ListenAndServe()
}

// ListenAndServeTLS starts the server with TLS encryption.
// For production deployments always use TLS or a TLS-terminating reverse proxy.
func (s *Server) ListenAndServeTLS(addr, certFile, keyFile string) error {
	handler := s.Handler()
	s.srv.Store(&http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second})
	return s.srv.Load().ListenAndServeTLS(certFile, keyFile)
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.Close()
	httpSrv := s.srv.Load()
	if httpSrv == nil {
		return nil
	}
	return httpSrv.Shutdown(ctx)
}

// --- HTTP helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("server: writeJSON failed", "err", err)
	}
}

func withCORS(next http.Handler, cfg CORSConfig) http.Handler {
	origins := cfg.AllowOrigins
	// Default: no CORS headers (fail-closed). Callers must explicitly
	// configure allowed origins for cross-origin deployments.
	if len(origins) == 0 {
		return next
	}
	methods := cfg.AllowMethods
	if len(methods) == 0 {
		methods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	}
	headers := cfg.AllowHeaders
	if len(headers) == 0 {
		headers = []string{"Content-Type"}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if len(origins) == 1 && origins[0] == "*" && !cfg.AllowCredentials {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" {
			// Note: a bare "*" entry only grants a match when credentials are not
			// allowed. Reflecting an arbitrary Origin while also sending
			// Access-Control-Allow-Credentials: true would let any site make
			// credentialed requests, defeating CORS protection.
			for _, allowed := range origins {
				if allowed == origin || (allowed == "*" && !cfg.AllowCredentials) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					break
				}
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(headers, ", "))
		if cfg.AllowCredentials {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SSEKeepAlive sends periodic comment lines to prevent proxy/browser timeouts.
// Call this in a goroutine and cancel the context when the stream ends.
func SSEKeepAlive(ctx context.Context, w http.ResponseWriter, mu sync.Locker, interval time.Duration) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mu.Lock()
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
			mu.Unlock()
		}
	}
}
