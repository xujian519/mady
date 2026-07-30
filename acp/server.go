package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/domains"
	"github.com/xujian519/mady/pkg/csync"
)

// ---------------------------------------------------------------------------
// ACP Server — types and lifecycle.
//
// Method implementations are split across sibling files:
//   server_handlers.go    — request dispatch + all handle* methods
//   server_permissions.go — tool-call permission request/record
//   server_filesystem.go  — client-mediated file reads/writes
//   server_response.go    — JSON-RPC response/notification/error helpers
//   server_auth.go        — authentication helpers + noopAuthProvider
// ---------------------------------------------------------------------------

// Server implements the ACP protocol over stdio transport.
type Server struct {
	sessionMgr *SessionManager
	agentInfo  AgentInfo
	authProv   AuthProvider
	logger     *slog.Logger
	reader     *bufio.Reader
	rawReader  io.Reader // underlying reader for read deadline support
	writer     io.Writer
	writerMu   sync.Mutex

	// approvalStore 持久化编辑器端的人工工具授权决策（allow/reject），
	// 供 P3 专家盲测的 HITL 触点数据收集；为 nil 时不留痕。
	approvalStore domains.ApprovalStore

	// Outbound client requests (e.g. session/request_permission) keyed by id.
	nextOutID atomic.Int64
	pending   *csync.Map[string, chan acpResponse]

	// Capabilities advertised by the client in initialize (fs, terminal).
	clientCaps atomic.Pointer[ClientCapabilities]

	// authenticated 标记当前连接是否已通过 authenticate 认证。
	// 仅在 AuthProvider 声明了认证方式（authRequired）时作为门禁使用。
	authenticated atomic.Bool

	// allowedFSCaps is the set of FS capabilities the server accepts from clients.
	allowedFSCaps map[string]bool
}

// acpResponse carries a routed client response to a waiting outbound request.
type acpResponse struct {
	result json.RawMessage
	err    error
}

// ServerConfig configures an ACP Server instance.
type ServerConfig struct {
	SessionManager *SessionManager
	AgentInfo      AgentInfo
	AuthProvider   AuthProvider
	Logger         *slog.Logger
	Reader         io.Reader
	Writer         io.Writer
	// AllowedFSCapabilities is the set of filesystem capabilities the server
	// will accept from clients. An empty map means no FS capabilities are allowed.
	// Keys are capability names like "FS.ReadTextFile", "FS.WriteTextFile".
	AllowedFSCapabilities map[string]bool
	// ApprovalStore 可选：配置后 session/request_permission 的人工授权结论
	// 会按 domains.RecordApprovalDecision 模式留痕（与 TUI/Server 触点一致）。
	ApprovalStore domains.ApprovalStore
}

// ---------------------------------------------------------------------------
// Server construction

// NewServer creates a new ACP Server with the given configuration.
func NewServer(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}
	if cfg.Reader == nil {
		cfg.Reader = os.Stdin
	}
	if cfg.Writer == nil {
		cfg.Writer = os.Stdout
	}
	if cfg.AuthProvider == nil {
		cfg.AuthProvider = &noopAuthProvider{}
		cfg.Logger.Warn("acp: 未配置认证提供者，允许未认证访问（仅限本地开发）；" +
			"对外暴露时请配置认证（如 MADY_ACP_TOKEN）")
	}

	return &Server{
		sessionMgr:    cfg.SessionManager,
		agentInfo:     cfg.AgentInfo,
		authProv:      cfg.AuthProvider,
		approvalStore: cfg.ApprovalStore,
		allowedFSCaps: cfg.AllowedFSCapabilities,
		logger:        cfg.Logger,
		reader:        bufio.NewReader(cfg.Reader),
		rawReader:     cfg.Reader,
		writer:        cfg.Writer,
		pending:       csync.NewMap[string, chan acpResponse](),
	}
}

// isTimeoutError returns true when an I/O error is due to a read deadline expiry.
func isTimeoutError(err error) bool {
	if e, ok := err.(interface{ Timeout() bool }); ok {
		return e.Timeout()
	}
	return false
}

// ---------------------------------------------------------------------------
// Server lifecycle: Run

// Run starts the ACP server loop, reading JSON-RPC requests from stdin.
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("ACP server starting on stdio")

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Set a read deadline on the raw reader (e.g. stdin) so that
		// ReadBytes doesn't block forever on a partial line. If the
		// underlying reader doesn't support deadlines, this is a no-op.
		if f, ok := s.rawReader.(interface{ SetReadDeadline(t time.Time) error }); ok {
			if err := f.SetReadDeadline(time.Now().Add(5 * time.Minute)); err != nil {
				slog.Warn("ACP: set read deadline on stdin", "err", err)
			}
		}

		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			// Timeout is not fatal — loop back to check ctx.Done().
			if isTimeoutError(err) {
				continue
			}
			return fmt.Errorf("read stdin: %w", err)
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(nil, -32700, "Parse error", err.Error())
			continue
		}

		if req.JSONRPC != "2.0" {
			s.writeError(req.ID, -32600, "Invalid Request", "jsonrpc must be 2.0")
			continue
		}

		// A message with an id but no method is a response to one of our
		// outbound client requests (e.g. session/request_permission). Route it
		// to the waiting caller instead of treating it as a request.
		if req.Method == "" && req.ID != nil {
			s.deliverClientResponse(req.ID, line)
			continue
		}

		s.handleRequest(ctx, &req)
	}
}

// ---------------------------------------------------------------------------
// Outbound requests: sendRequest, deliverClientResponse

// sendRequest issues an outbound JSON-RPC request to the client and waits for
// the response. Used for client-side methods like session/request_permission.
func (s *Server) sendRequest(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	idStr := fmt.Sprintf("acp-out-%d", s.nextOutID.Add(1))
	paramsBytes, marshalErr := json.Marshal(params)
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal params: %w", marshalErr)
	}
	reqBytes, err := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: idStr, Method: method, Params: paramsBytes})
	if err != nil {
		return nil, err
	}

	ch := make(chan acpResponse, 1)
	s.pending.Set(idStr, ch)
	defer func() {
		s.pending.Del(idStr)
	}()

	s.writerMu.Lock()
	_, werr := fmt.Fprintf(s.writer, "%s\n", reqBytes)
	s.writerMu.Unlock()
	if werr != nil {
		return nil, werr
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil, fmt.Errorf("client request %s timed out", method)
	case r := <-ch:
		return r.result, r.err
	}
}

// deliverClientResponse routes a client response to the waiting sendRequest.
func (s *Server) deliverClientResponse(id any, line []byte) {
	idStr, ok := id.(string)
	if !ok {
		return
	}
	ch, ok := s.pending.Get(idStr)
	if !ok {
		return
	}
	var resp JSONRPCResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		ch <- acpResponse{err: err}
		return
	}
	if resp.Error != nil {
		ch <- acpResponse{err: resp.Error}
		return
	}
	ch <- acpResponse{result: resp.Result}
}
