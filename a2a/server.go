package a2a


import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
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

// ---------------------------------------------------------------------------
type taskSubState struct {
	mu      sync.RWMutex
	subs    []chan *TaskUpdateEvent
	history []historyEntry
	nextSeq int
}

type historyEntry struct {
	event *TaskUpdateEvent
	seq   int
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

func (s *Server) getTaskState(taskID string) *taskSubState {
	s.taskStatesMu.RLock()
	ts, ok := s.taskStates[taskID]
	s.taskStatesMu.RUnlock()
	if ok {
		return ts
	}

	s.taskStatesMu.Lock()
	defer s.taskStatesMu.Unlock()

	if ts, ok = s.taskStates[taskID]; ok {
		return ts
	}

	ts = &taskSubState{}
	s.taskStates[taskID] = ts
	return ts
}

func (s *Server) startCleanup() {
	s.cleanupTicker = time.NewTicker(s.taskTTL / 2)
	s.cleanupStop = make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Default().Error("[a2a] cleanup goroutine panicked", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		for {
			select {
			case <-s.cleanupTicker.C:
				s.purgeOldTasks()
			case <-s.cleanupStop:
				return
			}
		}
	}()
}

func (s *Server) purgeOldTasks() {
	cutoff := time.Now().Add(-s.taskTTL)

	s.taskStatesMu.Lock()
	var toDelete []string
	for id, ts := range s.taskStates {
		ts.mu.RLock()
		hist := ts.history
		if len(hist) == 0 {
			ts.mu.RUnlock()
			continue
		}
		lastEv := hist[len(hist)-1]
		ts.mu.RUnlock()

		var lastUpdate time.Time
		if lastEv.event.Result != nil && len(lastEv.event.Result.History) > 0 {
			lastUpdate = lastEv.event.Result.History[len(lastEv.event.Result.History)-1].Timestamp
		}
		if lastUpdate.IsZero() {
			continue
		}
		if lastUpdate.Before(cutoff) {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		ts := s.taskStates[id]
		ts.mu.Lock()
		for _, ch := range ts.subs {
			close(ch)
		}
		ts.subs = nil
		removed := len(ts.history)
		ts.history = nil
		ts.mu.Unlock()
		s.totalHistSize.Add(-int64(removed))
		delete(s.taskStates, id)
		s.logger.Debug("purged old task history", "task_id", id)
	}
	s.taskStatesMu.Unlock()

	if s.sessionMgr != nil && s.sessionTTL > 0 {
		sessionCutoff := time.Now().Add(-s.sessionTTL)
		count := s.sessionMgr.PurgeStale(sessionCutoff)
		if count > 0 {
			s.logger.Debug("purged stale sessions", "count", count)
		}
	}
}

func isTerminalState(state TaskState) bool {
	switch state {
	case TaskStateCompleted, TaskStateFailed, TaskStateCanceled:
		return true
	default:
		return false
	}
}

// ValidateCard checks that an AgentCard has required fields.
func ValidateCard(card AgentCard) error {
	if card.Name == "" {
		return fmt.Errorf("agent card: name is required")
	}
	if card.URL == "" {
		return fmt.Errorf("agent card: url is required")
	}
	for i, skill := range card.Skills {
		if skill.ID == "" {
			return fmt.Errorf("agent card: skill[%d]: id is required", i)
		}
		if skill.Name == "" {
			return fmt.Errorf("agent card: skill[%d]: name is required", i)
		}
		if err := validateSkillParameters(skill.Parameters, i); err != nil {
			return err
		}
	}
	return nil
}

func validateSkillParameters(params map[string]any, skillIndex int) error {
	if params == nil {
		return nil
	}
	schemaType, ok := params["type"]
	if !ok {
		return fmt.Errorf("agent card: skill[%d]: parameters must have a 'type' field (JSON Schema)", skillIndex)
	}
	typeStr, ok := schemaType.(string)
	if !ok {
		return fmt.Errorf("agent card: skill[%d]: parameters 'type' must be a string", skillIndex)
	}
	validTypes := map[string]bool{
		"object":  true,
		"array":   true,
		"string":  true,
		"number":  true,
		"integer": true,
		"boolean": true,
		"null":    true,
	}
	if !validTypes[typeStr] {
		return fmt.Errorf("agent card: skill[%d]: parameters has invalid JSON Schema type %q", skillIndex, typeStr)
	}
	if typeStr == "object" {
		if props, ok := params["properties"]; ok {
			if _, ok := props.(map[string]any); !ok {
				return fmt.Errorf("agent card: skill[%d]: parameters 'properties' must be an object", skillIndex)
			}
		}
	}
	return nil
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

func (s *Server) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONRPCError(w, nil, JSONRPCInvalidRequest, "only POST allowed")
		return
	}
	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		writeJSONRPCError(w, nil, JSONRPCInvalidRequest, "Content-Type must be application/json")
		return
	}

	lr := io.LimitReader(r.Body, s.maxRequestBody)
	body, err := io.ReadAll(lr)
	if err != nil {
		writeJSONRPCError(w, nil, JSONRPCParseError, err.Error())
		return
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		writeJSONRPCError(w, nil, JSONRPCParseError, "empty request body")
		return
	}

	if trimmed[0] == '[' {
		var reqs []JSONRPCRequest
		if err := json.Unmarshal(body, &reqs); err != nil {
			writeJSONRPCError(w, nil, JSONRPCParseError, err.Error())
			return
		}
		s.handleBatchJSONRPC(w, r, reqs)
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONRPCError(w, nil, JSONRPCParseError, err.Error())
		return
	}

	s.dispatchJSONRPC(withLastEventID(r.Context(), r.Header.Get("Last-Event-ID")), w, req)
}

func (s *Server) handleBatchJSONRPC(w http.ResponseWriter, r *http.Request, reqs []JSONRPCRequest) {
	if len(reqs) == 0 {
		writeJSONRPCError(w, nil, JSONRPCInvalidRequest, "empty batch")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var results []JSONRPCResponse

	for _, req := range reqs {
		if req.JSONRPC != "2.0" {
			results = append(results, JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &JSONRPCError{Code: JSONRPCInvalidRequest, Message: "jsonrpc must be 2.0"},
			})
			continue
		}

		if req.Method == "tasks/sendSubscribe" || req.Method == "tasks/resubscribe" {
			results = append(results, JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &JSONRPCError{Code: JSONRPCInvalidRequest, Message: "streaming methods not allowed in batch requests"},
			})
			continue
		}

		rec := httptestNewRecorder()
		s.dispatchJSONRPC(withLastEventID(r.Context(), r.Header.Get("Last-Event-ID")), rec, req)

		var resp JSONRPCResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err == nil {
			results = append(results, resp)
		}
	}

	_ = json.NewEncoder(w).Encode(results)
}

func (s *Server) dispatchJSONRPC(ctx context.Context, w http.ResponseWriter, req JSONRPCRequest) {
	start := time.Now()
	s.logger.Debug("jsonrpc request", "method", req.Method, "id", req.ID)

	if req.JSONRPC != "2.0" {
		writeJSONRPCError(w, req.ID, JSONRPCInvalidRequest, "jsonrpc must be 2.0")
		return
	}

	card := s.handler.Card()

	isStreaming := req.Method == "tasks/sendSubscribe" || req.Method == "tasks/resubscribe"

	if !isStreaming && s.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.requestTimeout)
		defer cancel()
	}

	switch req.Method {
	case "tasks/send":
		s.handleSendTask(ctx, w, req)
	case "tasks/sendSubscribe":
		if !card.Capabilities.Streaming {
			writeJSONRPCError(w, req.ID, A2AErrorUnsupportedOperation, "streaming not supported")
			return
		}
		s.handleSendTaskSubscribe(ctx, w, req)
	case "tasks/get":
		s.handleGetTask(ctx, w, req)
	case "tasks/cancel":
		s.handleCancelTask(ctx, w, req)
	case "tasks/query":
		s.handleQueryTasks(ctx, w, req)
	case "tasks/pushNotification/set":
		if !card.Capabilities.PushNotifications {
			writeJSONRPCError(w, req.ID, A2AErrorPushNotSupported, "push notifications not supported")
			return
		}
		s.handleSetPushNotification(ctx, w, req)
	case "tasks/pushNotification/get":
		if !card.Capabilities.PushNotifications {
			writeJSONRPCError(w, req.ID, A2AErrorPushNotSupported, "push notifications not supported")
			return
		}
		s.handleGetPushNotification(ctx, w, req)
	case "tasks/resubscribe":
		if !card.Capabilities.Streaming {
			writeJSONRPCError(w, req.ID, A2AErrorUnsupportedOperation, "streaming not supported")
			return
		}
		s.handleResubscribe(ctx, w, req)
	default:
		writeJSONRPCError(w, req.ID, JSONRPCMethodNotFound, fmt.Sprintf("method %q not found", req.Method))
	}

	s.logger.Debug("jsonrpc complete", "method", req.Method, "id", req.ID, "duration", time.Since(start))
}

func (s *Server) handleSendTask(ctx context.Context, w http.ResponseWriter, req JSONRPCRequest) {
	var params SendTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInvalidParams, err.Error())
		return
	}

	card := s.handler.Card()
	if len(card.DefaultInputModes) > 0 {
		requestedModes := ExtractInputModes(params.Message)
		if err := ValidateInputModes(requestedModes, card.DefaultInputModes); err != nil {
			writeJSONRPCError(w, req.ID, A2AErrorContentTypeNotSupported, err.Error())
			return
		}
	}

	task, err := s.handler.SendTask(ctx, params)
	if err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInternalError, err.Error())
		return
	}

	s.recordTask(task)

	if s.sessionMgr != nil && task.SessionID != "" {
		s.sessionMgr.AddTask(task.SessionID, task.ID)
	}

	writeJSONRPCResult(w, req.ID, task)
}

func (s *Server) handleSendTaskSubscribe(ctx context.Context, w http.ResponseWriter, req JSONRPCRequest) {
	var params SendTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInvalidParams, err.Error())
		return
	}

	card := s.handler.Card()
	if len(card.DefaultInputModes) > 0 {
		requestedModes := ExtractInputModes(params.Message)
		if err := ValidateInputModes(requestedModes, card.DefaultInputModes); err != nil {
			writeJSONRPCError(w, req.ID, A2AErrorContentTypeNotSupported, err.Error())
			return
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONRPCError(w, req.ID, JSONRPCInternalError, "streaming not supported")
		return
	}

	taskID := params.ID
	if taskID == "" {
		taskID = fmt.Sprintf("task-%d", time.Now().UnixNano())
		params.ID = taskID
	}

	ch := make(chan *TaskUpdateEvent, 16)
	ts := s.getTaskState(taskID)
	ts.mu.Lock()
	ts.subs = append(ts.subs, ch)
	ts.mu.Unlock()

	defer func() {
		ts.mu.Lock()
		for i, c := range ts.subs {
			if c == ch {
				ts.subs = append(ts.subs[:i], ts.subs[i+1:]...)
				break
			}
		}
		close(ch)
		ts.mu.Unlock()
	}()

	type taskResult struct {
		task *Task
		err  error
	}
	resultCh := make(chan taskResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Default().Error("[a2a] send task goroutine panicked", "panic", r, "stack", string(debug.Stack()))
				resultCh <- taskResult{err: fmt.Errorf("panic: %v", r)}
			}
		}()
		task, err := s.handler.SendTask(ctx, params)
		resultCh <- taskResult{task, err}
	}()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	seq := 0

	for {
		select {
		case <-ctx.Done():
			return
		case r := <-resultCh:
			if r.err != nil {
				seq++
				s.writeSSEEvent(w, flusher, &TaskUpdateEvent{ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInternalError, Message: r.err.Error()}, Final: true}, seq)
				return
			}
			s.recordTask(r.task)
			if s.sessionMgr != nil && r.task.SessionID != "" {
				s.sessionMgr.AddTask(r.task.SessionID, r.task.ID)
			}
			final := isTerminalState(r.task.State) || r.task.State == TaskStateInputRequired
			seq++
			if !s.writeSSEEvent(w, flusher, &TaskUpdateEvent{ID: req.ID, Result: r.task, Final: final}, seq) {
				return
			}
			if final {
				return
			}
		case <-heartbeat.C:
			if !s.writeSSEComment(w, flusher, "heartbeat") {
				return
			}
		case ev, ok := <-ch:
			if !ok {
				return
			}
			ev.ID = req.ID
			seq++
			if !s.writeSSEEvent(w, flusher, ev, seq) {
				return
			}
			if ev.Final {
				return
			}
		}
	}
}

func (s *Server) handleGetTask(ctx context.Context, w http.ResponseWriter, req JSONRPCRequest) {
	var params GetTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInvalidParams, err.Error())
		return
	}

	task, err := s.handler.GetTask(ctx, params)
	if err != nil {
		writeJSONRPCError(w, req.ID, A2AErrorTaskNotFound, err.Error())
		return
	}

	s.recordTask(task)
	writeJSONRPCResult(w, req.ID, task)
}

func (s *Server) handleCancelTask(ctx context.Context, w http.ResponseWriter, req JSONRPCRequest) {
	var params CancelTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInvalidParams, err.Error())
		return
	}

	task, err := s.handler.CancelTask(ctx, params)
	if err != nil {
		writeJSONRPCError(w, req.ID, A2AErrorTaskNotCancelable, err.Error())
		return
	}

	s.recordTask(task)
	writeJSONRPCResult(w, req.ID, task)
}

func (s *Server) handleQueryTasks(ctx context.Context, w http.ResponseWriter, req JSONRPCRequest) {
	var params QueryTasksRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInvalidParams, err.Error())
		return
	}

	result, err := s.handler.QueryTasks(ctx, params)
	if err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInternalError, err.Error())
		return
	}

	writeJSONRPCResult(w, req.ID, result)
}

func (s *Server) handleSetPushNotification(ctx context.Context, w http.ResponseWriter, req JSONRPCRequest) {
	var params SetPushNotificationRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInvalidParams, err.Error())
		return
	}

	if err := s.handler.SetPushNotification(ctx, params); err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInternalError, err.Error())
		return
	}

	writeJSONRPCResult(w, req.ID, map[string]string{"status": "ok"})
}

func (s *Server) handleGetPushNotification(ctx context.Context, w http.ResponseWriter, req JSONRPCRequest) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInvalidParams, err.Error())
		return
	}

	cfg, err := s.handler.GetPushNotification(ctx, params.ID)
	if err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInternalError, err.Error())
		return
	}

	writeJSONRPCResult(w, req.ID, cfg)
}

func (s *Server) handleResubscribe(ctx context.Context, w http.ResponseWriter, req JSONRPCRequest) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInvalidParams, err.Error())
		return
	}

	if _, err := s.handler.GetTask(ctx, GetTaskRequest{ID: params.ID}); err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInvalidParams, fmt.Sprintf("task %q not found", params.ID))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONRPCError(w, req.ID, JSONRPCInternalError, "streaming not supported")
		return
	}

	ts := s.getTaskState(params.ID)

	afterSeq := lastEventIDFromCtx(ctx)

	ts.mu.RLock()
	var replay []historyEntry
	if afterSeq > 0 {
		for _, entry := range ts.history {
			if entry.seq > afterSeq {
				replay = append(replay, entry)
			}
		}
	} else {
		replay = make([]historyEntry, len(ts.history))
		copy(replay, ts.history)
	}
	ts.mu.RUnlock()

	for _, entry := range replay {
		evCopy := *entry.event
		evCopy.ID = req.ID
		if !s.writeSSEEvent(w, flusher, &evCopy, entry.seq) {
			return
		}
		if evCopy.Final {
			return
		}
	}

	ch := make(chan *TaskUpdateEvent, 16)
	ts.mu.Lock()
	ts.subs = append(ts.subs, ch)
	ts.mu.Unlock()

	defer func() {
		ts.mu.Lock()
		for i, c := range ts.subs {
			if c == ch {
				ts.subs = append(ts.subs[:i], ts.subs[i+1:]...)
				break
			}
		}
		close(ch)
		ts.mu.Unlock()
	}()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if !s.writeSSEComment(w, flusher, "heartbeat") {
				return
			}
		case ev, ok := <-ch:
			if !ok {
				return
			}
			ev.ID = req.ID
			ts.mu.Lock()
			ts.nextSeq++
			seq := ts.nextSeq
			trimmed := 0
			if len(ts.history) >= s.maxHistoryLen {
				trimmed = len(ts.history) - s.maxHistoryLen + 1
				ts.history = ts.history[trimmed:]
			}
			ts.history = append(ts.history, historyEntry{event: ev, seq: seq})
			chans := make([]chan *TaskUpdateEvent, len(ts.subs))
			copy(chans, ts.subs)
			ts.mu.Unlock()

			s.totalHistSize.Add(1 - int64(trimmed))

			if !s.writeSSEEvent(w, flusher, ev, seq) {
				return
			}
			if ev.Final {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// SSE helpers
// ---------------------------------------------------------------------------

func (s *Server) writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, ev *TaskUpdateEvent, eventID int) bool {
	data, err := json.Marshal(ev)
	if err != nil {
		return false
	}
	if eventID > 0 {
		_, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", eventID, data)
	} else {
		_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	}
	if err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func (s *Server) writeSSEComment(w http.ResponseWriter, flusher http.Flusher, comment string) bool {
	_, err := fmt.Fprintf(w, ": %s\n\n", comment)
	if err != nil {
		return false
	}
	flusher.Flush()
	return true
}

// recordTask stores a task snapshot in event history for resubscription.
func (s *Server) recordTask(task *Task) {
	ts := s.getTaskState(task.ID)
	ts.mu.Lock()
	ev := &TaskUpdateEvent{Result: task, Final: isTerminalState(task.State)}
	ts.nextSeq++
	trimmed := 0
	if len(ts.history) >= s.maxHistoryLen {
		trimmed = len(ts.history) - s.maxHistoryLen + 1
		ts.history = ts.history[trimmed:]
	}
	ts.history = append(ts.history, historyEntry{event: ev, seq: ts.nextSeq})
	ts.mu.Unlock()

	newSize := s.totalHistSize.Add(1 - int64(trimmed))

	if newSize > int64(s.maxTotalHist) {
		s.taskStatesMu.Lock()
		for s.totalHistSize.Load() > int64(s.maxTotalHist) {
			oldest := ""
			oldestTime := time.Now()
			for id, t := range s.taskStates {
				t.mu.RLock()
				if len(t.history) > 0 {
					first := t.history[0].event
					if first.Result != nil && len(first.Result.History) > 0 && first.Result.History[0].Timestamp.Before(oldestTime) {
						oldestTime = first.Result.History[0].Timestamp
						oldest = id
					}
				}
				t.mu.RUnlock()
			}
			if oldest == "" {
				break
			}
			ots := s.taskStates[oldest]
			ots.mu.Lock()
			removed := len(ots.history)
			ots.mu.Unlock()
			s.totalHistSize.Add(-int64(removed))
			delete(s.taskStates, oldest)
		}
		s.taskStatesMu.Unlock()
	}
}

func (s *Server) PublishTaskUpdate(taskID string, ev *TaskUpdateEvent) {
	ts := s.getTaskState(taskID)
	ts.mu.Lock()
	ts.nextSeq++
	trimmed := 0
	if len(ts.history) >= s.maxHistoryLen {
		trimmed = len(ts.history) - s.maxHistoryLen + 1
		ts.history = ts.history[trimmed:]
	}
	ts.history = append(ts.history, historyEntry{event: ev, seq: ts.nextSeq})
	chans := make([]chan *TaskUpdateEvent, len(ts.subs))
	copy(chans, ts.subs)
	ts.mu.Unlock()

	s.totalHistSize.Add(1 - int64(trimmed))

	for _, ch := range chans {
		select {
		case ch <- ev:
		default:
			slog.Default().Warn("a2a: subscriber channel full, event dropped", "task_id", taskID)
		}
	}
}


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


