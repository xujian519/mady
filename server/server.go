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
	"github.com/xujian519/mady/agentcore/iface"
	"github.com/xujian519/mady/agui"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/domains/inventiveness"
	"github.com/xujian519/mady/pkg/csync"
	"github.com/xujian519/mady/retrieval/domain"
)

// registerAPIRoute 同时注册新版（/api/v1/*）和旧版（/api/*）路由。
// 新版附加 X-API-Version 头，旧版附加废弃提示头。
func registerAPIRoute(mux *http.ServeMux, pattern string, handler http.Handler) {
	parts := strings.SplitN(pattern, " ", 2)
	var method, path string
	if len(parts) == 2 {
		method = parts[0]
		path = parts[1]
	} else {
		path = parts[0]
	}

	v1Path := strings.Replace(path, "/api/", "/api/v1/", 1)
	if v1Path == path {
		// 没有 /api/ 前缀的路径（如 /v1/disclosure/*），保持原样
		mux.Handle(pattern, handler)
		return
	}

	var v1Pattern string
	if method != "" {
		v1Pattern = method + " " + v1Path
	} else {
		v1Pattern = v1Path
	}

	mux.Handle(v1Pattern, withVersionHeader(handler, "v1"))
	mux.Handle(pattern, withDeprecationNotice(handler))
}

// Server exposes an Agent as an HTTP/SSE API.
type Server struct {
	config   *csync.Value[agentcore.Config]
	eventBus iface.EventBus
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

	// disclosureRetriever 是技术交底书分析时检索现有技术的专利领域检索器。
	// 由外部调用方（如 cmd/mady/server.go）通过 SetDisclosureRetriever 注入；
	// 未配置时 retrieve_prior_art 节点降级为 evidence_coverage=none（纯 LLM 知识）。
	disclosureRetriever atomic.Pointer[domain.DomainRetriever]

	// inventivenessResults 存储创造性三步法评估的异步结果。
	// 由 InventivenessTrigger 在 disclosure 管线完成后自动填充，
	// 在 disclosure 状态查询端点一并返回。key = disclosure task ID。
	inventivenessResults *csync.Map[string, *inventiveness.InventivenessResult]

	// metrics 记录 HTTP 请求指标。默认使用 NopMetricsRecorder（静默），
	// 可通过 SetMetricsRecorder 注入 Prometheus 等生产实现。
	metrics MetricsRecorder

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
		config:               csync.NewValue(cfg),
		eventBus:             agentcore.NewIFaceEventBus(agentcore.NewEventBus()),
		poolLimit:            64,
		inventivenessResults: csync.NewMap[string, *inventiveness.InventivenessResult](),
		metrics:              NopMetricsRecorder{},
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

// SetMetricsRecorder 注入自定义指标记录器。传入 nil 时复位为 NopMetricsRecorder。
func (s *Server) SetMetricsRecorder(m MetricsRecorder) {
	if m == nil {
		s.metrics = NopMetricsRecorder{}
		return
	}
	s.metrics = m
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

// SetDisclosureRetriever 注入技术交底书分析时使用的专利领域检索器。
// 传入 nil 可解除配置。未配置时 retrieve_prior_art 节点降级为 evidence_coverage=none。
func (s *Server) SetDisclosureRetriever(r domain.DomainRetriever) {
	var ptr *domain.DomainRetriever
	if r != nil {
		ptr = &r
	}
	s.disclosureRetriever.Store(ptr)
}

// getDisclosureRetriever 返回当前配置的专利领域检索器，未配置时为 nil。
func (s *Server) getDisclosureRetriever() domain.DomainRetriever {
	ptr := s.disclosureRetriever.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// SetInventivenessResult 存储创造性分析异步结果。
// 由 InventivenessTrigger 在 disclosure 完成后自动调用，taskID 对应 disclosure 任务。
func (s *Server) SetInventivenessResult(taskID string, result *inventiveness.InventivenessResult) {
	s.inventivenessResults.Set(taskID, result)
}

// GetInventivenessResult 查询创造性分析异步结果。未完成时为 nil。
func (s *Server) GetInventivenessResult(taskID string) *inventiveness.InventivenessResult {
	result, _ := s.inventivenessResults.Get(taskID)
	return result
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

func (s *Server) On(t iface.EventType, h iface.EventHandler) func() {
	return s.eventBus.On(t, h)
}
func (s *Server) OnAll(h iface.EventHandler) func() { return s.eventBus.OnAll(h) }
func (s *Server) EmitEvent(e iface.Event)           { s.eventBus.Emit(e) }
func (s *Server) EventBus() iface.EventBus          { return s.eventBus }
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

	// 健康检查端点
	mux.HandleFunc("GET /health", handleHealthFast)
	mux.HandleFunc("GET /ready", s.handleReady)

	// Chat API
	registerAPIRoute(mux, "POST /api/chat", http.HandlerFunc(s.handleChat))

	// Skills API
	registerAPIRoute(mux, "GET /api/skills", http.HandlerFunc(s.handleListSkills))
	registerAPIRoute(mux, "GET /api/skills/diagnostics", http.HandlerFunc(s.handleSkillDiagnostics))
	registerAPIRoute(mux, "GET /api/skills/events", http.HandlerFunc(s.handleSkillEvents))
	registerAPIRoute(mux, "GET /api/skills/status", http.HandlerFunc(s.handleSkillStatus))
	registerAPIRoute(mux, "POST /api/skills/reload", http.HandlerFunc(s.handleReloadSkills))

	// Disclosure API（已是 /v1/disclosure/* 版本化）
	mux.HandleFunc("POST /v1/disclosure/analyze", s.handleDisclosureAnalyze)
	mux.HandleFunc("GET /v1/disclosure/analyze/{task_id}", s.handleDisclosureStatus)
	mux.HandleFunc("GET /v1/disclosure/analyze/{task_id}/stream", s.handleDisclosureStream)
	mux.HandleFunc("POST /v1/disclosure/analyze/{task_id}/review", s.handleDisclosureReview)

	// Threads API
	registerAPIRoute(mux, "POST /api/threads", http.HandlerFunc(s.handleCreateThread))
	registerAPIRoute(mux, "GET /api/threads", http.HandlerFunc(s.handleListThreads))
	registerAPIRoute(mux, "GET /api/threads/{key}", http.HandlerFunc(s.handleGetThread))
	registerAPIRoute(mux, "GET /api/threads/{key}/config", http.HandlerFunc(s.handleGetThreadConfig))
	registerAPIRoute(mux, "PUT /api/threads/{key}/config", http.HandlerFunc(s.handlePutThreadConfig))
	registerAPIRoute(mux, "GET /api/threads/{key}/thinking", http.HandlerFunc(s.handleGetThreadThinking))
	registerAPIRoute(mux, "PUT /api/threads/{key}/thinking", http.HandlerFunc(s.handlePutThreadThinking))
	registerAPIRoute(mux, "POST /api/threads/{key}/branch", http.HandlerFunc(s.handleBranchThread))
	registerAPIRoute(mux, "DELETE /api/threads/{key}", http.HandlerFunc(s.handleDeleteThread))

	// States API
	registerAPIRoute(mux, "GET /api/states", http.HandlerFunc(s.handleListStates))
	registerAPIRoute(mux, "GET /api/states/{key}", http.HandlerFunc(s.handleGetState))
	registerAPIRoute(mux, "DELETE /api/states/{key}", http.HandlerFunc(s.handleDeleteState))

	// AG-UI 事件流（不添加版本化，保持独立路径）
	mux.Handle("/agui/{path}", s.aguiHandler())

	// 构建中间件链：loggingMiddleware 包裹所有非 /agui/ 处理逻辑
	// /agui/ 是 SSE 长连接，不适合请求日志。
	h := withCORS(mux, s.cors)
	h = loggingMiddleware(h)
	return h
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
