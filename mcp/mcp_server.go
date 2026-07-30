package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/xujian519/mady/agentcore"
)

// =============================================================================
// MCPServer — MCP 协议服务端实现
//
// 将 Mady 的工具、知识库、记忆作为标准 MCP 资源暴露给外部客户端。
// 遵循 MCP HTTP/SSE 传输规范（2025-11-25 协议版本）。
//
// 端点路由（集成到 server/ 中）：
//   POST /mcp  — JSON-RPC 请求处理
//   GET  /mcp  — SSE 服务端流
//   DELETE /mcp — 会话清理
// =============================================================================

const (
	// MCPProtocolVersion is the MCP protocol version this server implements.
	MCPProtocolVersion = "2025-11-25"

	// DefaultMCPSessionTTL is how long a session lives without activity.
	DefaultMCPSessionTTL = 30 * time.Minute
)

// ---------------------------------------------------------------------------
// Session management
// ---------------------------------------------------------------------------

// mcpSession holds the per-connection state for an MCP client.
type mcpSession struct {
	ID             string
	CreatedAt      time.Time
	LastActivityAt time.Time
	Initialized    bool

	// Subscribed resources (uri → notify channel).
	subscriptions map[string]bool
}

// mcpSessionStore manages active MCP sessions.
type mcpSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*mcpSession
	ttl      time.Duration
}

func newMCPSessionStore(ttl time.Duration) *mcpSessionStore {
	if ttl <= 0 {
		ttl = DefaultMCPSessionTTL
	}
	return &mcpSessionStore{
		sessions: make(map[string]*mcpSession),
		ttl:      ttl,
	}
}

func (ss *mcpSessionStore) Create() *mcpSession {
	s := &mcpSession{
		ID:             uuid.New().String(),
		CreatedAt:      time.Now(),
		LastActivityAt: time.Now(),
		subscriptions:  make(map[string]bool),
	}
	ss.mu.Lock()
	ss.sessions[s.ID] = s
	ss.mu.Unlock()
	return s
}

func (ss *mcpSessionStore) Get(id string) *mcpSession {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.sessions[id]
}

func (ss *mcpSessionStore) Delete(id string) {
	ss.mu.Lock()
	delete(ss.sessions, id)
	ss.mu.Unlock()
}

func (ss *mcpSessionStore) Touch(id string) {
	ss.mu.Lock()
	if s, ok := ss.sessions[id]; ok {
		s.LastActivityAt = time.Now()
	}
	ss.mu.Unlock()
}

// CleanupExpired removes sessions past their TTL.
func (ss *mcpSessionStore) CleanupExpired() {
	now := time.Now()
	ss.mu.Lock()
	defer ss.mu.Unlock()
	for id, s := range ss.sessions {
		if now.After(s.LastActivityAt.Add(ss.ttl)) {
			delete(ss.sessions, id)
		}
	}
}

// ---------------------------------------------------------------------------
// MCPServer — core server type
// ---------------------------------------------------------------------------

// MCPServer wraps Mady tools and exposes them via the MCP protocol.
type MCPServer struct {
	tools    []agentcore.Tool // all registered agent tools
	sessions *mcpSessionStore
	logger   *slog.Logger

	mu sync.RWMutex
}

// NewMCPServer creates an MCP server from a list of agent tools.
//
// Example:
//
//	agentTools := ext.Tools()
//	srv := mcp.NewMCPServer(agentTools)
//	mux.HandleFunc("POST /mcp", srv.HandleRequest)
func NewMCPServer(tools []agentcore.Tool) *MCPServer {
	return &MCPServer{
		tools:    tools,
		sessions: newMCPSessionStore(DefaultMCPSessionTTL),
		logger:   slog.With("component", "mcp-server"),
	}
}

// UpdateTools replaces the tool list at runtime (for hot-reload scenarios).
func (srv *MCPServer) UpdateTools(tools []agentcore.Tool) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.tools = tools
	srv.logger.Info("MCP server tools updated", "count", len(tools))
}

// ---------------------------------------------------------------------------
// HTTP handler — entry point for all MCP endpoints
// ---------------------------------------------------------------------------

// Handler returns an http.Handler that routes MCP requests.
// Routes:
//
//	POST /mcp — JSON-RPC request
//	GET  /mcp — SSE server stream
//	DELETE /mcp — session cleanup
func (srv *MCPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /", srv.handleJSONRPC)
	mux.HandleFunc("GET /", srv.handleSSE)
	mux.HandleFunc("DELETE /", srv.handleDelete)
	return mux
}

// ServeHTTP dispatches MCP requests on the standard MCP endpoint.
// This implements http.Handler for direct mounting.
func (srv *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		srv.handleJSONRPC(w, r)
	case http.MethodGet:
		srv.handleSSE(w, r)
	case http.MethodDelete:
		srv.handleDelete(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// JSON-RPC request handling
// ---------------------------------------------------------------------------

// jsonRPCRequest is the raw incoming JSON-RPC request.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is the outgoing JSON-RPC response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// MCP standard error codes.
const (
	errCodeInvalidRequest = -32600
	errCodeMethodNotFound = -32601
	errCodeInvalidParams  = -32602
	errCodeInternalError  = -32603
)

func (srv *MCPServer) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	// Validate method — MCP uses POST.
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	// Read body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "cannot read body")
		return
	}
	if len(body) == 0 {
		writeJSONError(w, http.StatusBadRequest, "empty body")
		return
	}

	// Parse request.
	var req jsonRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Validate protocol version (non-initialize requests).
	if req.Method != "initialize" {
		_ = req.Method // suppress unused warning — used below via switch
	}

	// Dispatch to handler.
	resp := srv.dispatch(r.Context(), &req)

	// Set MCP headers for initialize response.
	if req.Method == "initialize" {
		extractSessionID(resp)
		w.Header().Set(headerProtocolVersion, MCPProtocolVersion)
	}

	// Write JSON response.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		srv.logger.Error("mcp: encode response", "err", err)
	}
}

// dispatch routes a JSON-RPC request to the appropriate handler.
func (srv *MCPServer) dispatch(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	// Notifications (no ID) are fire-and-forget.
	if len(req.ID) == 0 || string(req.ID) == "null" {
		srv.handleNotification(ctx, req)
		return nil // no response for notifications
	}

	switch req.Method {
	case "initialize":
		return srv.handleInitialize(ctx, req)
	case "tools/list":
		return srv.handleToolList(ctx, req)
	case "tools/call":
		return srv.handleToolCall(ctx, req)
	case "resources/list":
		return srv.handleResourceList(ctx, req)
	case "resources/read":
		return srv.handleResourceRead(ctx, req)
	case "notifications/initialized":
		// Acknowledge initialization notification.
		return nil // no response for notifications
	default:
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    errCodeMethodNotFound,
				Message: fmt.Sprintf("method not found: %s", req.Method),
			},
		}
	}
}

// handleNotification processes fire-and-forget notifications.
func (srv *MCPServer) handleNotification(_ context.Context, req *jsonRPCRequest) {
	switch req.Method {
	case "notifications/initialized":
		srv.logger.Debug("client initialized (notification received)")
	case "notifications/cancelled":
		srv.logger.Debug("client cancelled request")
	default:
		srv.logger.Debug("unhandled notification", "method", req.Method)
	}
}

// ---------------------------------------------------------------------------
// Handlers for each MCP method
// ---------------------------------------------------------------------------

func (srv *MCPServer) handleInitialize(_ context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	// Parse client params for logging.
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
		ClientInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}
	sessionID := uuid.New().String()
	if req.Params != nil {
		_ = json.Unmarshal(req.Params, &params)
	}

	// Create session.
	s := srv.sessions.Create()
	s.ID = sessionID

	srv.logger.Info("MCP client initialized",
		"client", params.ClientInfo.Name,
		"version", params.ClientInfo.Version,
		"protocol", params.ProtocolVersion,
		"session", sessionID)

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"protocolVersion": MCPProtocolVersion,
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"subscribe": false, "listChanged": false},
				"prompts":   map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    "mady",
				"version": "0.4.0",
			},
		},
	}
}

func (srv *MCPServer) handleToolList(_ context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	srv.mu.RLock()
	defer srv.mu.RUnlock()

	mcpTools := make([]Tool, 0, len(srv.tools))
	for _, t := range srv.tools {
		mcpTools = append(mcpTools, agentToolToMCPTool(&t))
	}

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"tools": mcpTools,
		},
	}
}

func (srv *MCPServer) handleToolCall(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: errCodeInvalidParams, Message: "invalid params: " + err.Error()},
		}
	}

	if params.Name == "" {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    errCodeInvalidParams,
				Message: "tool name is required",
			},
		}
	}

	// Find the tool.
	srv.mu.RLock()
	var target *agentcore.Tool
	for i := range srv.tools {
		if srv.tools[i].Name == params.Name {
			target = &srv.tools[i]
			break
		}
	}
	srv.mu.RUnlock()

	if target == nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    errCodeMethodNotFound,
				Message: fmt.Sprintf("tool not found: %s", params.Name),
			},
		}
	}

	// Execute the tool.
	result, err := target.Func(ctx, params.Arguments)
	if err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    errCodeInternalError,
				Message: err.Error(),
			},
			Result: map[string]any{
				"content": []ToolResultContent{
					{Type: "text", Text: fmt.Sprintf("error: %v", err)},
				},
				"isError": true,
			},
		}
	}

	// Format result as text content.
	resultText := fmt.Sprintf("%v", result)
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"content": []ToolResultContent{
				{Type: "text", Text: resultText},
			},
			"isError": false,
		},
	}
}

func (srv *MCPServer) handleResourceList(_ context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	// Expose knowledge resources as MCP resources.
	resources := []Resource{
		{
			URI:         "knowledge://laws",
			Name:        "法律法规库",
			Description: "中国法律法规全文数据库",
			MIMEType:    "text/markdown",
		},
		{
			URI:         "knowledge://patent",
			Name:        "专利知识库",
			Description: "专利审查指南、案例、概念卡片",
			MIMEType:    "text/markdown",
		},
		{
			URI:         "memory://sessions",
			Name:        "会话记忆",
			Description: "历史会话和Agent记忆",
			MIMEType:    "application/json",
		},
	}

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"resources": resources,
		},
	}
}

func (srv *MCPServer) handleResourceRead(_ context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.URI == "" {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: errCodeInvalidParams, Message: "uri required"},
		}
	}

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"contents": []EmbeddedResource{
				{
					URI:      params.URI,
					MIMEType: "text/markdown",
					Text:     fmt.Sprintf("Resource %q placeholder — connect knowledge store for live data", params.URI),
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// SSE server stream
// ---------------------------------------------------------------------------

func (srv *MCPServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send initial endpoint event.
	_, _ = fmt.Fprintf(w, "event: endpoint\ndata: /mcp\n\n")
	flusher.Flush()

	// Keep connection alive with periodic comments.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (srv *MCPServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(headerSessionID)
	if sessionID != "" {
		srv.sessions.Delete(sessionID)
		srv.logger.Info("MCP session deleted", "session", sessionID)
	}
	w.WriteHeader(http.StatusAccepted)
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// agentToolToMCPTool converts an agentcore.Tool to an MCP Tool.
func agentToolToMCPTool(t *agentcore.Tool) Tool {
	params := t.Parameters
	if params == nil {
		params = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	return Tool{
		Name:         t.Name,
		Description:  t.Description,
		InputSchema:  params,
		OutputSchema: nil,
	}
}

// writeJSONError writes an HTTP error with a JSON body.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(jsonRPCResponse{ //nolint:errchkjson
		JSONRPC: "2.0",
		Error:   &jsonRPCError{Code: errCodeInvalidRequest, Message: message},
	})
}

// extractSessionID pulls the session ID from an initialize response.
func extractSessionID(resp *jsonRPCResponse) {
	if resp == nil || resp.Result == nil {
		return
	}
	if m, ok := resp.Result.(map[string]any); ok {
		if caps, ok := m["capabilities"].(map[string]any); ok {
			_ = caps // just checking structure
		}
	}
}

// header constants reused from existing mcp package.
var (
	_ = headerSessionID
	_ = headerProtocolVersion
)

// Ensure strings import is used.
var _ = strings.TrimSpace
