package server

// TODO(refactor): 此文件超过 1228 行，建议按职责拆分为多个文件以提升可维护性。
// 参考 docs/GO-DEVELOPMENT-STANDARDS.md 2.4 节。

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/pkg/csync"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/skill"
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

type threadStore interface {
	CreateThread(ctx context.Context) (*session.ThreadSnapshot, error)
	ListThreads(ctx context.Context) ([]session.Info, error)
	GetThread(ctx context.Context, key string) (*session.ThreadSnapshot, error)
	BranchThread(ctx context.Context, key, entryID string) (*session.ThreadSnapshot, error)
	GetThreadConfig(ctx context.Context, key string) (*agentcore.CallConfig, bool, error)
	SetThreadConfig(ctx context.Context, key string, cfg *agentcore.CallConfig) (*session.ThreadSnapshot, error)
	GetThreadThinking(ctx context.Context, key string) (*agentcore.ThinkingConfig, bool, error)
	SetThreadThinking(ctx context.Context, key string, cfg *agentcore.ThinkingConfig) (*session.ThreadSnapshot, error)
}

// BranchThreadRequest 是创建分支会话的请求体。
type BranchThreadRequest struct {
	EntryID string `json:"entry_id,omitempty"`
}

// ThreadThinkingRequest 是查询思考链的请求体。
type ThreadThinkingRequest struct {
	Thinking *agentcore.ThinkingConfig `json:"thinking,omitempty"`
}

// ThreadThinkingResponse 是思考链的响应体。
type ThreadThinkingResponse struct {
	ThreadID string                    `json:"thread_id"`
	Thinking *agentcore.ThinkingConfig `json:"thinking,omitempty"`
}

// ThreadConfigRequest 是更新会话配置的请求体。
type ThreadConfigRequest struct {
	Config *agentcore.CallConfig `json:"config,omitempty"`
}

// ThreadConfigResponse 是会话配置的响应体。
type ThreadConfigResponse struct {
	ThreadID string                `json:"thread_id"`
	Config   *agentcore.CallConfig `json:"config,omitempty"`
}

// SkillSummary 是技能的概要信息。
type SkillSummary struct {
	Name                   string            `json:"name"`
	Description            string            `json:"description"`
	FilePath               string            `json:"file_path"`
	BaseDir                string            `json:"base_dir"`
	DisableModelInvocation bool              `json:"disable_model_invocation,omitempty"`
	Metadata               map[string]string `json:"metadata,omitempty"`
	SelectedByDefault      bool              `json:"selected_by_default,omitempty"`
}

// SkillsResponse 是技能列表的响应体。
type SkillsResponse struct {
	Skills []SkillSummary `json:"skills"`
}

// SkillDiagnosticsResponse 是技能诊断信息的响应体。
type SkillDiagnosticsResponse struct {
	Diagnostics []skill.Diagnostic `json:"diagnostics"`
}

// SkillRegistryStatusResponse 是技能注册表状态的响应体。
type SkillRegistryStatusResponse struct {
	Skills                  []SkillSummary     `json:"skills"`
	ThreadID                string             `json:"thread_id,omitempty"`
	HasThreadConfig         bool               `json:"has_thread_config,omitempty"`
	SelectedSkills          []string           `json:"selected_skills,omitempty"`
	EffectiveSelectedSkills []string           `json:"effective_selected_skills,omitempty"`
	MissingSelectedSkills   []string           `json:"missing_selected_skills,omitempty"`
	AddedSkills             []string           `json:"added_skills,omitempty"`
	RemovedSkills           []string           `json:"removed_skills,omitempty"`
	UpdatedSkills           []string           `json:"updated_skills,omitempty"`
	AddedDiagnostics        []skill.Diagnostic `json:"added_diagnostics,omitempty"`
	RemovedDiagnostics      []skill.Diagnostic `json:"removed_diagnostics,omitempty"`
	SkillPaths              []string           `json:"skill_paths,omitempty"`
	Reloadable              bool               `json:"reloadable"`
	Diagnostics             []skill.Diagnostic `json:"diagnostics"`
	TotalSkills             int                `json:"total_skills"`
	VisibleSkills           int                `json:"visible_skills"`
	HiddenSkills            int                `json:"hidden_skills"`
	DiagnosticsCount        int                `json:"diagnostics_count"`
}

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

// --- request / response types ---

// ChatRequest 是聊天 API 的请求体。
type ChatRequest struct {
	Message        string                    `json:"message"`
	Stream         bool                      `json:"stream"`
	ThreadID       string                    `json:"thread_id,omitempty"`
	Model          string                    `json:"model,omitempty"`
	ResponseFormat *agentcore.ResponseFormat `json:"response_format,omitempty"`
	Thinking       *agentcore.ThinkingConfig `json:"thinking,omitempty"`
	Skills         []string                  `json:"skills,omitempty"`
}

// ChatResponse 是聊天 API 的响应体。
type ChatResponse struct {
	Output   string `json:"output"`
	ThreadID string `json:"thread_id,omitempty"`
	Error    string `json:"error,omitempty"`
}

// --- chat handler ---

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(s.limitedBody(w, r)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ChatResponse{Error: "invalid request body"})
		return
	}
	threadID, err := s.ensureThreadID(r.Context(), req.ThreadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{Error: err.Error()})
		return
	}
	req.ThreadID = threadID
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, ChatResponse{ThreadID: req.ThreadID, Error: "message is required"})
		return
	}

	if req.Stream {
		s.handleStreamChat(w, r, req)
	} else {
		s.handleSyncChat(w, r, req)
	}
}

func (s *Server) handleSyncChat(w http.ResponseWriter, r *http.Request, req ChatRequest) {
	entry, err := s.loadAgent(r.Context(), req.ThreadID, requestCallConfig(req))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{
			ThreadID: req.ThreadID,
			Error:    err.Error(),
		})
		return
	}
	agent := entry.agent

	output, err := agent.Run(r.Context(), req.Message)
	if saveErr := s.saveAgentState(r.Context(), agent, req.ThreadID); saveErr != nil && err == nil {
		err = saveErr
	}
	s.releaseAgent(entry, req.ThreadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{
			ThreadID: req.ThreadID,
			Error:    err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, ChatResponse{Output: output, ThreadID: req.ThreadID})
}

func (s *Server) handleStreamChat(w http.ResponseWriter, r *http.Request, req ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{Error: "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	entry, err := s.loadAgent(r.Context(), req.ThreadID, requestCallConfig(req))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{
			ThreadID: req.ThreadID,
			Error:    err.Error(),
		})
		return
	}
	agent := entry.agent

	// dead marks the connection as unwritable (client disconnect, marshal
	// failure, underlying write error). Once set, subsequent writeSSE calls
	// short-circuit — no point doing further work on a dead connection, and
	// continuing to write would just spam the log. Guarded by the writeSSE
	// mutex + atomic for lock-free reads from the handler path.
	var dead atomic.Bool
	var mu sync.Mutex
	writeSSE := func(eventType string, data any) {
		if dead.Load() {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if dead.Load() {
			return
		}
		payload, err := json.Marshal(data)
		if err != nil {
			log.Printf("server: SSE marshal error (event=%s): %v", eventType, err)
			dead.Store(true)
			return
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, payload); err != nil {
			log.Printf("server: SSE write error (event=%s): %v", eventType, err)
			dead.Store(true)
			return
		}
		flusher.Flush()
	}

	// Register the SSE writer as a scoped handler on the agent's event bus.
	// The handler is unregistered BEFORE the agent is released back to the
	// pool — not just via defer. If we relied on defer alone, there would be
	// a window between releaseAgent (agent back in pool) and the deferred
	// unregister (runs at function return): a concurrent request on the same
	// thread could reuse the agent and its events would flow into this dead
	// ResponseWriter. Unregistering before release closes that window.
	// The defer is kept as an idempotent safety net.
	unregister := agent.OnAll(func(e agentcore.Event) {
		writeSSE(string(e.EventKind()), streamEventPayload(req.ThreadID, e))
	})
	defer unregister()
	agent.EmitExtensionSnapshots()

	output, runErr := agent.Run(r.Context(), req.Message)
	saveErr := s.saveAgentState(r.Context(), agent, req.ThreadID)
	if saveErr != nil && runErr == nil {
		runErr = saveErr
	}
	unregister() // detach BEFORE releasing — see comment above
	s.releaseAgent(entry, req.ThreadID)

	done := StreamDoneEvent{
		Schema:   streamSchemaChatDone,
		Type:     "done",
		ThreadID: req.ThreadID,
		Output:   output,
	}
	if runErr != nil {
		done.Error = runErr.Error()
	}
	writeSSE("done", done)
}

// --- state handlers ---

func (s *Server) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	thread, err := ts.CreateThread(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

func (s *Server) handleListThreads(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	threads, err := ts.ListThreads(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"threads": threads})
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSkillAPI(w, r, false) {
		return
	}
	writeJSON(w, http.StatusOK, SkillsResponse{
		Skills: s.skillSummaries(),
	})
}

func (s *Server) handleSkillDiagnostics(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSkillAPI(w, r, false) {
		return
	}
	cfg := s.snapshotConfig()
	writeJSON(w, http.StatusOK, SkillDiagnosticsResponse{
		Diagnostics: cloneSkillDiagnostics(cfg.SkillDiagnostics),
	})
}

func (s *Server) handleSkillStatus(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSkillAPI(w, r, false) {
		return
	}
	cfg := s.snapshotConfig()
	threadID := strings.TrimSpace(r.URL.Query().Get("thread_id"))
	selectedSkills := agentcore.CloneStringSlice(cfg.SelectedSkills)
	effectiveSkills := agentcore.CloneStringSlice(cfg.SelectedSkills)
	hasThreadConfig := false
	if threadID != "" {
		threadCfg, ok, err := s.threadCallConfig(r.Context(), threadID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		hasThreadConfig = ok
		effectiveSkills = effectiveSkillSelection(cfg.SelectedSkills, threadCfg)
	}
	skills := skillSummariesFor(cfg.AvailableSkills, selectedSkills)
	diagnostics := cloneSkillDiagnostics(cfg.SkillDiagnostics)
	_, missing := skill.ResolveSelection(cfg.AvailableSkills, effectiveSkills)
	var visible, hidden int
	for _, item := range skills {
		if item.DisableModelInvocation {
			hidden++
		} else {
			visible++
		}
	}
	writeJSON(w, http.StatusOK, SkillRegistryStatusResponse{
		Skills:                  skills,
		ThreadID:                threadID,
		HasThreadConfig:         hasThreadConfig,
		SelectedSkills:          selectedSkills,
		EffectiveSelectedSkills: effectiveSkills,
		MissingSelectedSkills:   missing,
		SkillPaths:              agentcore.CloneStringSlice(cfg.SkillPaths),
		Reloadable:              len(cfg.SkillPaths) > 0,
		Diagnostics:             diagnostics,
		TotalSkills:             len(skills),
		VisibleSkills:           visible,
		HiddenSkills:            hidden,
		DiagnosticsCount:        len(diagnostics),
	})
}

func (s *Server) handleSkillEvents(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSkillAPI(w, r, false) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	var dead atomic.Bool
	var mu sync.Mutex
	writeSSE := func(eventType string, data any) {
		if dead.Load() {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if dead.Load() {
			return
		}
		payload, err := json.Marshal(data)
		if err != nil {
			log.Printf("server: SSE marshal error (event=%s): %v", eventType, err)
			dead.Store(true)
			return
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, payload); err != nil {
			log.Printf("server: SSE write error (event=%s): %v", eventType, err)
			dead.Store(true)
			return
		}
		flusher.Flush()
	}

	writeSSE("skills_snapshot", skillSnapshotEventPayload(s.snapshotConfig()))

	ch := make(chan agentcore.Event, 8)
	unregister := s.On(agentcore.EventSkillsReloaded, func(e agentcore.Event) {
		select {
		case ch <- e:
		default:
		}
	})
	defer unregister()

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			writeSSE(string(e.EventKind()), streamEventPayload("", e))
		}
	}
}

func (s *Server) handleReloadSkills(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSkillAPI(w, r, true) {
		return
	}
	cfg := s.snapshotConfig()
	if len(cfg.SkillPaths) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "skill reload not configured"})
		return
	}
	skills, diagnostics, err := skill.Load(cfg.SkillPaths...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	oldCfg := s.config.Get()
	oldSkills := cloneSkills(oldCfg.AvailableSkills)
	oldDiagnostics := cloneSkillDiagnostics(oldCfg.SkillDiagnostics)
	newCfg := oldCfg
	newCfg.AvailableSkills = cloneSkills(skills)
	newCfg.SkillDiagnostics = cloneSkillDiagnostics(diagnostics)
	s.config.Set(newCfg)
	cfg = s.snapshotConfig()
	skillsSummary := skillSummariesFor(cfg.AvailableSkills, cfg.SelectedSkills)
	oldSkillSummaries := skillSummariesFor(oldSkills, cfg.SelectedSkills)
	addedSkills, removedSkills, updatedSkills := diffSkillSummaries(oldSkillSummaries, skillsSummary)
	addedDiagnostics, removedDiagnostics := diffSkillDiagnostics(oldDiagnostics, cfg.SkillDiagnostics)
	var visible, hidden int
	for _, item := range skillsSummary {
		if item.DisableModelInvocation {
			hidden++
		} else {
			visible++
		}
	}
	_, missing := skill.ResolveSelection(cfg.AvailableSkills, cfg.SelectedSkills)
	s.EmitEvent(agentcore.NewSkillsReloadedEvent(
		cfg.SkillPaths,
		len(skillsSummary),
		visible,
		hidden,
		len(cfg.SkillDiagnostics),
		addedSkills,
		removedSkills,
		updatedSkills,
		addedDiagnostics,
		removedDiagnostics,
	))
	writeJSON(w, http.StatusOK, SkillRegistryStatusResponse{
		Skills:                  skillsSummary,
		SelectedSkills:          agentcore.CloneStringSlice(cfg.SelectedSkills),
		EffectiveSelectedSkills: agentcore.CloneStringSlice(cfg.SelectedSkills),
		MissingSelectedSkills:   missing,
		AddedSkills:             addedSkills,
		RemovedSkills:           removedSkills,
		UpdatedSkills:           updatedSkills,
		AddedDiagnostics:        addedDiagnostics,
		RemovedDiagnostics:      removedDiagnostics,
		SkillPaths:              agentcore.CloneStringSlice(cfg.SkillPaths),
		Reloadable:              true,
		Diagnostics:             cloneSkillDiagnostics(cfg.SkillDiagnostics),
		TotalSkills:             len(skillsSummary),
		VisibleSkills:           visible,
		HiddenSkills:            hidden,
		DiagnosticsCount:        len(cfg.SkillDiagnostics),
	})
}

func (s *Server) handleGetThread(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	thread, err := ts.GetThread(r.Context(), r.PathValue("key"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

func (s *Server) handleDeleteThread(w http.ResponseWriter, r *http.Request) {
	// Delete is a destructive operation; require authorization.
	if !s.authorizeThreadAccess(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := s.snapshotConfig().Store
	if store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store configured"})
		return
	}
	if _, ok := s.threadStore(); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	if err := store.Delete(r.Context(), r.PathValue("key")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetThreadConfig(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	key := r.PathValue("key")
	cfg, _, err := ts.GetThreadConfig(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ThreadConfigResponse{
		ThreadID: key,
		Config:   agentcore.CloneCallConfig(cfg),
	})
}

func (s *Server) handlePutThreadConfig(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	var req ThreadConfigRequest
	if err := json.NewDecoder(s.limitedBody(w, r)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	thread, err := ts.SetThreadConfig(r.Context(), r.PathValue("key"), req.Config)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ThreadConfigResponse{
		ThreadID: thread.Info.ID,
		Config:   agentcore.CloneCallConfig(thread.Config),
	})
}

func (s *Server) handleGetThreadThinking(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	key := r.PathValue("key")
	thinking, _, err := ts.GetThreadThinking(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ThreadThinkingResponse{
		ThreadID: key,
		Thinking: agentcore.CloneThinkingConfig(thinking),
	})
}

func (s *Server) handlePutThreadThinking(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	var req ThreadThinkingRequest
	if err := json.NewDecoder(s.limitedBody(w, r)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	thread, err := ts.SetThreadThinking(r.Context(), r.PathValue("key"), req.Thinking)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ThreadThinkingResponse{
		ThreadID: thread.Info.ID,
		Thinking: agentcore.CloneThinkingConfig(thread.Thinking),
	})
}

func (s *Server) handleBranchThread(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	var req BranchThreadRequest
	if r.Body != nil {
		if err := json.NewDecoder(s.limitedBody(w, r)).Decode(&req); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}
	thread, err := ts.BranchThread(r.Context(), r.PathValue("key"), req.EntryID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

func (s *Server) handleListStates(w http.ResponseWriter, r *http.Request) {
	store := s.snapshotConfig().Store
	if store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store configured"})
		return
	}
	keys, err := store.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (s *Server) handleGetState(w http.ResponseWriter, r *http.Request) {
	store := s.snapshotConfig().Store
	if store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store configured"})
		return
	}
	key := r.PathValue("key")
	snap, err := store.Load(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleDeleteState(w http.ResponseWriter, r *http.Request) {
	store := s.snapshotConfig().Store
	if store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store configured"})
		return
	}
	key := r.PathValue("key")
	if err := store.Delete(r.Context(), key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[server] writeJSON failed: %v", err)
	}
}

// poolEntry 包装池化 agent，用引用计数跟踪并发使用（C2 use-after-free 修复）。
// 所有字段仅由 poolMu 保护下访问：
//   - loadAgent 借用时 refs+1，releaseAgent 归还时 refs-1；
//   - 淘汰/关闭仅标记 evicted 并摘除出池；
//   - 只有 refs 归零且 evicted 时才真正 Close agent，
//     保证使用中的 agent 绝不会被提前关闭。
type poolEntry struct {
	agent   *agentcore.Agent
	refs    int  // 正在使用该 agent 的请求数
	evicted bool // 已从池中摘除；refs 归零后需要 Close
	pooled  bool // 是否已存入 agentPool
}

// loadAgent 借用 threadID 对应的 agent（优先复用池化实例）。
// 调用方必须在使用完成后调用 releaseAgent 归还恰好一次。
func (s *Server) loadAgent(ctx context.Context, threadID string, callCfg *agentcore.CallConfig) (*poolEntry, error) {
	if threadID != "" && callCfg == nil {
		// 池命中路径：在 poolMu 内原子完成查找与引用计数递增，
		// 防止与淘汰/关闭竞争（此前 Load 不持锁，release 的淘汰逻辑
		// 可能关闭正在使用的 agent）。
		s.poolMu.Lock()
		cached, ok := s.agentPool.Load(threadID)
		if ok {
			cached.(*poolEntry).refs++
		}
		s.poolMu.Unlock()
		if ok {
			entry := cached.(*poolEntry)
			if ts, has := s.threadStore(); has {
				if threadCfg, hasCfg, err := ts.GetThreadConfig(ctx, threadID); err == nil && hasCfg {
					entry.agent.ApplyCallConfig(threadCfg)
				}
			}
			if err := entry.agent.LoadState(ctx, threadID); err == nil {
				return entry, nil
			}
			// LoadState 失败：摘除并按引用计数释放（仍有其他请求使用时延迟关闭）。
			s.discardPoolEntry(threadID, entry)
		}
	}

	cfg := s.snapshotConfig()
	var provider agentcore.ThreadConfigProvider
	if ts, ok := s.threadStore(); ok {
		provider = ts
	}
	agent, err := agentcore.LoadAgent(ctx, cfg, agentcore.LoadAgentOptions{
		ThreadID:          threadID,
		CallCfg:           callCfg,
		ThreadCfgProvider: provider,
	})
	if err != nil {
		return nil, err
	}
	return &poolEntry{agent: agent, refs: 1}, nil
}

// discardPoolEntry 将 entry 从池中摘除并归还一次引用；若已无其他请求使用，
// 立即 Close，否则由最后一次 releaseAgent 负责 Close。
func (s *Server) discardPoolEntry(threadID string, entry *poolEntry) {
	s.poolMu.Lock()
	defer s.poolMu.Unlock()
	// 仅当池中仍是同一个 entry 时摘除，避免误删并发新建的同 key entry。
	if cur, ok := s.agentPool.Load(threadID); ok && cur == entry {
		s.agentPool.Delete(threadID)
		entry.evicted = true
	}
	entry.refs--
	if entry.refs == 0 && entry.evicted {
		entry.agent.Close()
	}
}

// releaseAgent 归还 loadAgent 借用的 entry。池化 entry 保留复用；
// 非池化 entry 尝试入池（池满或已有存活 entry 时直接关闭）。
func (s *Server) releaseAgent(entry *poolEntry, threadID string) {
	if threadID == "" {
		entry.agent.Close()
		return
	}
	s.poolMu.Lock()
	defer s.poolMu.Unlock()
	entry.refs--
	if entry.evicted {
		// 已被淘汰：最后一次归还时关闭。
		if entry.refs == 0 {
			entry.agent.Close()
		}
		return
	}
	if entry.pooled {
		// 正常归还池化 entry，保留在池中复用。
		return
	}
	// 非池化来源的 agent：尝试入池。
	if _, ok := s.agentPool.Load(threadID); ok {
		// 池中已有同 threadID 的存活 entry（并发请求已建立），关闭多余的 agent。
		entry.agent.Close()
		return
	}
	if s.poolCountLocked() >= s.poolLimit {
		s.evictIdleLocked()
	}
	if s.poolCountLocked() >= s.poolLimit {
		// 没有空闲 entry 可淘汰：不入池，直接关闭。
		entry.agent.Close()
		return
	}
	entry.pooled = true
	s.agentPool.Store(threadID, entry)
}

// poolCountLocked 返回当前池内 entry 数。调用方须持有 poolMu。
func (s *Server) poolCountLocked() int {
	count := 0
	s.agentPool.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// evictIdleLocked 淘汰一个空闲（refs==0）entry 并立即关闭其 agent。
// 使用中的 entry 不会被淘汰。调用方须持有 poolMu。
func (s *Server) evictIdleLocked() {
	s.agentPool.Range(func(key, value any) bool {
		entry := value.(*poolEntry)
		if entry.refs != 0 {
			return true // 继续使用中的 entry，寻找下一个
		}
		s.agentPool.Delete(key)
		entry.evicted = true
		entry.agent.Close()
		return false
	})
}

func (s *Server) saveAgentState(ctx context.Context, agent *agentcore.Agent, threadID string) error {
	store := s.snapshotConfig().Store
	if threadID == "" || store == nil {
		return nil
	}
	return agent.SaveState(ctx, threadID)
}

func requestCallConfig(req ChatRequest) *agentcore.CallConfig {
	if req.Model == "" && req.ResponseFormat == nil && req.Thinking == nil && len(req.Skills) == 0 {
		return nil
	}
	return &agentcore.CallConfig{
		Model:          req.Model,
		ResponseFormat: agentcore.CloneResponseFormat(req.ResponseFormat),
		Thinking:       agentcore.CloneThinkingConfig(req.Thinking),
		Skills:         agentcore.CloneStringSlice(req.Skills),
	}
}

func (s *Server) skillSummaries() []SkillSummary {
	cfg := s.snapshotConfig()
	return skillSummariesFor(cfg.AvailableSkills, cfg.SelectedSkills)
}

func cloneSkillMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneSkillDiagnostics(in []skill.Diagnostic) []skill.Diagnostic {
	if len(in) == 0 {
		return nil
	}
	out := make([]skill.Diagnostic, len(in))
	copy(out, in)
	return out
}

func cloneSkills(in []skill.Skill) []skill.Skill {
	if len(in) == 0 {
		return nil
	}
	out := make([]skill.Skill, 0, len(in))
	for _, item := range in {
		cp := item
		cp.AllowedTools = agentcore.CloneStringSlice(item.AllowedTools)
		cp.Metadata = cloneSkillMetadata(item.Metadata)
		out = append(out, cp)
	}
	return out
}

func (s *Server) ensureThreadID(ctx context.Context, threadID string) (string, error) {
	if threadID != "" {
		return threadID, nil
	}
	ts, ok := s.threadStore()
	if !ok {
		return "", nil
	}
	thread, err := ts.CreateThread(ctx)
	if err != nil {
		return "", err
	}
	return thread.Info.ID, nil
}

func (s *Server) threadStore() (threadStore, bool) {
	store := s.snapshotConfig().Store
	if store == nil {
		return nil, false
	}
	ts, ok := store.(threadStore)
	return ts, ok
}

func (s *Server) snapshotConfig() agentcore.Config {
	cfg := s.config.Get()
	cfg.SelectedSkills = agentcore.CloneStringSlice(cfg.SelectedSkills)
	cfg.SkillPaths = agentcore.CloneStringSlice(cfg.SkillPaths)
	cfg.AvailableSkills = cloneSkills(cfg.AvailableSkills)
	cfg.SkillDiagnostics = cloneSkillDiagnostics(cfg.SkillDiagnostics)
	return cfg
}

// authorizeThreadAccess checks authorization for destructive thread operations.
// Delegates to the skill API auth token if configured, otherwise allows access
// (local/single-user deployment mode).
func (s *Server) authorizeThreadAccess(r *http.Request) bool {
	cfg := s.snapshotConfig()
	token := strings.TrimSpace(cfg.SkillAPIAuthToken)
	if token == "" {
		return true // no auth configured — local deployment
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	expected := "Bearer " + token
	return subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) == 1
}

func (s *Server) authorizeSkillAPI(w http.ResponseWriter, r *http.Request, reload bool) bool {
	cfg := s.snapshotConfig()
	if (!reload && cfg.DisableSkillRegistryAPI) || (reload && cfg.DisableSkillReloadAPI) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill API disabled"})
		return false
	}
	token := strings.TrimSpace(cfg.SkillAPIAuthToken)
	if token == "" {
		return true
	}
	if r == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return false
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	expected := "Bearer " + token
	if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) == 1 {
		return true
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="skills"`)
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid authorization"})
	return false
}

func skillSummariesFor(skills []skill.Skill, selected []string) []SkillSummary {
	if len(skills) == 0 {
		return nil
	}
	selectedSet := make(map[string]bool, len(selected))
	for _, name := range selected {
		selectedSet[name] = true
	}
	out := make([]SkillSummary, 0, len(skills))
	for _, item := range skills {
		out = append(out, SkillSummary{
			Name:                   item.Name,
			Description:            item.Description,
			FilePath:               item.FilePath,
			BaseDir:                item.BaseDir,
			DisableModelInvocation: item.DisableModelInvocation,
			Metadata:               cloneSkillMetadata(item.Metadata),
			SelectedByDefault:      selectedSet[item.Name],
		})
	}
	return out
}

func (s *Server) threadCallConfig(ctx context.Context, threadID string) (*agentcore.CallConfig, bool, error) {
	ts, ok := s.threadStore()
	if !ok {
		return nil, false, nil
	}
	cfg, hasCfg, err := ts.GetThreadConfig(ctx, threadID)
	if err != nil {
		return nil, false, err
	}
	return cfg, hasCfg, nil
}

func effectiveSkillSelection(defaultSkills []string, threadCfg *agentcore.CallConfig) []string {
	base := &agentcore.CallConfig{Skills: agentcore.CloneStringSlice(defaultSkills)}
	merged := agentcore.MergeCallConfig(base, threadCfg)
	if merged == nil {
		return nil
	}
	return agentcore.CloneStringSlice(merged.Skills)
}

func skillSnapshotEventPayload(cfg agentcore.Config) SkillsSnapshotStreamEvent {
	skills := skillSummariesFor(cfg.AvailableSkills, cfg.SelectedSkills)
	var visible, hidden int
	for _, item := range skills {
		if item.DisableModelInvocation {
			hidden++
		} else {
			visible++
		}
	}
	return SkillsSnapshotStreamEvent{
		Schema:    streamSchemaSkillsSnapshot,
		Type:      "skills_snapshot",
		Timestamp: time.Now(),
		Payload: SkillRegistryStatusResponse{
			Skills:                  skills,
			SelectedSkills:          agentcore.CloneStringSlice(cfg.SelectedSkills),
			EffectiveSelectedSkills: agentcore.CloneStringSlice(cfg.SelectedSkills),
			SkillPaths:              agentcore.CloneStringSlice(cfg.SkillPaths),
			Reloadable:              len(cfg.SkillPaths) > 0,
			Diagnostics:             cloneSkillDiagnostics(cfg.SkillDiagnostics),
			TotalSkills:             len(skills),
			VisibleSkills:           visible,
			HiddenSkills:            hidden,
			DiagnosticsCount:        len(cfg.SkillDiagnostics),
		},
	}
}

func diffSkillSummaries(oldSkills, newSkills []SkillSummary) (added, removed, updated []string) {
	oldByName := make(map[string]SkillSummary, len(oldSkills))
	newByName := make(map[string]SkillSummary, len(newSkills))
	for _, item := range oldSkills {
		oldByName[item.Name] = item
	}
	for _, item := range newSkills {
		newByName[item.Name] = item
	}
	for name, newItem := range newByName {
		oldItem, ok := oldByName[name]
		if !ok {
			added = append(added, name)
			continue
		}
		if !skillSummaryEqual(oldItem, newItem) {
			updated = append(updated, name)
		}
	}
	for name := range oldByName {
		if _, ok := newByName[name]; !ok {
			removed = append(removed, name)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(updated)
	return added, removed, updated
}

func diffSkillDiagnostics(oldDiagnostics, newDiagnostics []skill.Diagnostic) (added, removed []skill.Diagnostic) {
	oldByKey := make(map[string]skill.Diagnostic, len(oldDiagnostics))
	newByKey := make(map[string]skill.Diagnostic, len(newDiagnostics))
	for _, item := range oldDiagnostics {
		oldByKey[item.Path+"\x00"+item.Message] = item
	}
	for _, item := range newDiagnostics {
		newByKey[item.Path+"\x00"+item.Message] = item
	}
	for key, item := range newByKey {
		if _, ok := oldByKey[key]; !ok {
			added = append(added, item)
		}
	}
	for key, item := range oldByKey {
		if _, ok := newByKey[key]; !ok {
			removed = append(removed, item)
		}
	}
	sort.Slice(added, func(i, j int) bool {
		if added[i].Path == added[j].Path {
			return added[i].Message < added[j].Message
		}
		return added[i].Path < added[j].Path
	})
	sort.Slice(removed, func(i, j int) bool {
		if removed[i].Path == removed[j].Path {
			return removed[i].Message < removed[j].Message
		}
		return removed[i].Path < removed[j].Path
	})
	return added, removed
}

func skillSummaryEqual(a, b SkillSummary) bool {
	if a.Name != b.Name ||
		a.Description != b.Description ||
		a.FilePath != b.FilePath ||
		a.BaseDir != b.BaseDir ||
		a.DisableModelInvocation != b.DisableModelInvocation {
		return false
	}
	if len(a.Metadata) != len(b.Metadata) {
		return false
	}
	for key, value := range a.Metadata {
		if b.Metadata[key] != value {
			return false
		}
	}
	return true
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
