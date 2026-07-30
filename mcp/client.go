package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
)

const protocolVersion = "2025-11-25"
const stderrContextMaxBytes = 4 * 1024

const (
	// defaultRequestTimeout is used when ctx is nil or RequestTimeout is unset.
	defaultRequestTimeout = 30 * time.Second
	// defaultRetryCount is the maximum number of RPC retry attempts.
	defaultRetryCount = 3
	// closeWaitTimeout is the grace period to wait for a child process to exit
	// during Close(), before sending SIGKILL.
	closeWaitTimeout = 2 * time.Second
	// reconnectInitWaitTimeout is the wait for cmd.Wait after a failed initialize.
	reconnectInitWaitTimeout = 5 * time.Second
	// reconnectKillWaitTimeout is the wait after Process.Kill during reconnect cleanup.
	reconnectKillWaitTimeout = time.Second

	// Scanner buffer sizes for read loops.
	scannerInitialBuf = 1024 * 1024      // 1 MiB
	scannerMaxBuf     = 10 * 1024 * 1024 // 10 MiB
	stderrInitialBuf  = 16 * 1024        // 16 KiB
	stderrMaxBuf      = 1024 * 1024      // 1 MiB

	// jsonRPCMethodNotFound is the JSON-RPC standard error code for unknown methods.
	jsonRPCMethodNotFound = -32601
)

// hookSet manages a thread-safe list of notification/event hook callbacks,
// shared by both stdio Client and HTTPClient to avoid code duplication.
type hookSet struct {
	mu    sync.RWMutex
	hooks []func(context.Context, string, json.RawMessage) error
}

// add registers a notification hook if non-nil.
func (s *hookSet) add(h func(context.Context, string, json.RawMessage) error) {
	if h == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks = append(s.hooks, h)
}

// snapshot returns a copy of the current hook list for iteration.
func (s *hookSet) snapshot() []func(context.Context, string, json.RawMessage) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]func(context.Context, string, json.RawMessage) error(nil), s.hooks...)
}

// StdioConfig 配置 MCP stdio 传输方式。
type StdioConfig struct {
	Name                string
	Command             string
	Args                []string
	Env                 []string
	Dir                 string
	ToolPrefix          string
	ClientName          string
	ClientVersion       string
	RequestTimeout      time.Duration
	NotificationHandler func(context.Context, string, json.RawMessage) error
	RequestHandler      func(context.Context, string, json.RawMessage) (any, error)
	ErrorHandler        func(context.Context, error)
	Discovery           DiscoveryConfig
}

// Client 是 MCP 协议的客户端实现，支持 stdio 传输方式。
type Client struct {
	cfg    StdioConfig
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[string]chan rpcResponse
	closed  bool
	closeCh chan struct{}
	readErr error

	nextID    atomic.Int64
	errBuf    bytes.Buffer
	discovery *discoveryState
	capState  *capabilityState
	eventSink runtimeEventSink
	hooks     hookSet

	bgCtx             context.Context
	bgCancel          context.CancelFunc
	reconnectMu       sync.Mutex
	reconnectBackoff  time.Duration
	reconnectAttempts int // 连续重连失败累计次数
}

const (
	maxReconnectBackoff   = 30 * time.Second
	initialReconnectDelay = 500 * time.Millisecond
	maxReconnectAttempts  = 10 // 连续重连失败超过此次数后重置退避
)

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Tool 表示 MCP 服务器提供的一个工具定义。
type Tool struct {
	Name         string         `json:"name"`
	Title        string         `json:"title,omitempty"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
}

type toolListResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// ToolResult 是工具调用的结果。
type ToolResult struct {
	Content           []ToolResultContent `json:"content,omitempty"`
	StructuredContent any                 `json:"structuredContent,omitempty"`
	IsError           bool                `json:"isError,omitempty"`
}

// ToolResultContent 是工具调用结果中的一段内容。
type ToolResultContent struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	Data     string            `json:"data,omitempty"`
	MIMEType string            `json:"mimeType,omitempty"`
	URI      string            `json:"uri,omitempty"`
	Name     string            `json:"name,omitempty"`
	Resource *EmbeddedResource `json:"resource,omitempty"`
}

// EmbeddedResource 表示嵌入在工具结果中的资源。
type EmbeddedResource struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

type toolBridge interface {
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (*ToolResult, error)
}

// rpcCaller abstracts the JSON-RPC call mechanism shared by Client and HTTPClient.
type rpcCaller interface {
	call(ctx context.Context, method string, params any, out any) error
}

// paginatedListTools implements the paginated tools/list logic shared by
// both stdio Client and HTTPClient via a common rpcCaller interface.
func paginatedListTools(ctx context.Context, caller rpcCaller) ([]Tool, error) {
	var out []Tool
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var result toolListResult
		if err := caller.call(ctx, "tools/list", params, &result); err != nil {
			return nil, err
		}
		out = append(out, result.Tools...)
		if result.NextCursor == "" {
			return out, nil
		}
		cursor = result.NextCursor
	}
}

// callToolCommon implements the tools/call logic shared by both stdio Client
// and HTTPClient via a common rpcCaller interface.
func callToolCommon(ctx context.Context, caller rpcCaller, name string, arguments map[string]any) (*ToolResult, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	var result ToolResult
	if err := caller.call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// NewStdioClient creates a new MCP client over stdio transport and
// performs the MCP initialize handshake.
func NewStdioClient(ctx context.Context, cfg StdioConfig) (*Client, error) {
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, fmt.Errorf("mcp: command is required")
	}
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...) //nolint:gosec // MCP runs user-configured commands by design
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}
	if len(cfg.Env) > 0 {
		cmd.Env = append(cmd.Environ(), cfg.Env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stderr pipe: %w", err)
	}
	// Run the server in its own process group so that Close() can clean up
	// grand-children spawned by wrapper commands like npx/npm exec.
	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start server: %w", err)
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())
	c := &Client{
		cfg:       cfg,
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		pending:   make(map[string]chan rpcResponse),
		closeCh:   make(chan struct{}),
		bgCtx:     bgCtx,
		bgCancel:  bgCancel,
		discovery: newDiscoveryState(cfg.Discovery),
		capState:  newCapabilityState(),
	}
	go c.readLoop()
	go c.captureStderr(stderr)
	if err := c.initialize(ctx); err != nil {
		if err := c.Close(); err != nil {
			slog.Warn("MCP: close client on init failure", "err", err)
		}
		return nil, err
	}
	return c, nil
}

func (c *Client) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    util.DefaultString(c.cfg.ClientName, "mady"),
			"version": util.DefaultString(c.cfg.ClientVersion, "0.1.0"),
		},
	}
	var result struct {
		ProtocolVersion string          `json:"protocolVersion"`
		Capabilities    json.RawMessage `json:"capabilities"`
	}
	if err := c.call(ctx, "initialize", params, &result); err != nil {
		return fmt.Errorf("mcp: initialize: %w", err)
	}
	// 核验服务端协议版本兼容性。MCP 版本号为 "YYYY-MM-DD" 格式，字符串字典
	// 序即日期序。客户端按向下兼容策略适配服务端：服务端版本不高于客户端时
	// 视为兼容、静默；仅当服务端版本比客户端更新（可能引入客户端不认的新
	// 特性）时告警，提示升级客户端。服务端未返回 protocolVersion（旧版或非
	// 标准实现）同样不阻塞、不告警。
	if result.ProtocolVersion != "" && result.ProtocolVersion > protocolVersion {
		slog.Warn("mcp: server protocol version newer than client; some features may be unsupported", "server", result.ProtocolVersion, "client", protocolVersion)
	}
	caps, err := decodeCapabilities(result.Capabilities)
	if err != nil {
		return fmt.Errorf("mcp: decode capabilities: %w", err)
	}
	c.capState.set(ctx, caps)
	return c.notify("notifications/initialized", nil)
}

// ListTools retrieves the full list of tools from the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	return paginatedListTools(ctx, c)
}

// CallTool invokes a tool on the MCP server with the given arguments.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (*ToolResult, error) {
	return callToolCommon(ctx, c, name, arguments)
}

// AgentTools converts MCP tools to agentcore Tool definitions.
func (c *Client) AgentTools(ctx context.Context) ([]*agentcore.Tool, error) {
	return agentToolsFor(ctx, c.cfg.ToolPrefix, c)
}

// Close shuts down the MCP client, closing pipes and waiting for the process to exit.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.closeCh)
	c.bgCancel()
	for id, ch := range c.pending {
		delete(c.pending, id)
		close(ch)
	}
	c.mu.Unlock()

	// Close all stdio pipes first. This unblocks readLoop/captureStderr even
	// when the child process spawned grand-children that keep the pipes open
	// after the direct child exits, preventing cmd.Wait() from hanging forever.
	if c.stdin != nil {
		if err := c.stdin.Close(); err != nil {
			slog.Warn("MCP: close stdin pipe", "err", err)
		}
	}
	if c.stdout != nil {
		if err := c.stdout.Close(); err != nil {
			slog.Warn("MCP: close stdout pipe", "err", err)
		}
	}
	if c.stderr != nil {
		if err := c.stderr.Close(); err != nil {
			slog.Warn("MCP: close stderr pipe", "err", err)
		}
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- c.cmd.Wait()
	}()
	timer := time.NewTimer(closeWaitTimeout)
	defer timer.Stop()
	select {
	case err := <-waitDone:
		if err != nil && !strings.Contains(err.Error(), "signal: killed") {
			return err
		}
		return nil
	case <-timer.C:
	}

	// Graceful wait timed out. Force-kill the whole process tree and wait
	// briefly; if it still does not reap (e.g. grandchildren hold pipes),
	// log and abandon rather than block callers indefinitely.
	if c.cmd.Process != nil {
		if err := killProcessTree(c.cmd.Process.Pid); err != nil {
			slog.Warn("mcp: client: kill process tree failed", "err", err)
		}
	}
	killTimer := time.NewTimer(closeWaitTimeout)
	defer killTimer.Stop()
	select {
	case err := <-waitDone:
		if err != nil && !strings.Contains(err.Error(), "signal: killed") {
			return err
		}
	case <-killTimer.C:
		slog.Warn("mcp: client: cmd.Wait did not return after kill, goroutine may leak")
	}
	return nil
}
