package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
)

const maxResponseBytes = 10 << 20 // 10MB — max MCP HTTP response body size

const (
	headerSessionID       = "Mcp-Session-Id"
	headerProtocolVersion = "Mcp-Protocol-Version"
	headerLastEventID     = "Last-Event-ID"
)

var errSessionExpired = errors.New("mcp session expired")
var errServerStreamUnsupported = errors.New("mcp server stream unsupported")

type sessionExpiredError struct {
	sessionID string
}

func (e sessionExpiredError) Error() string { return errSessionExpired.Error() }
func (e sessionExpiredError) Unwrap() error { return errSessionExpired }

type HTTPConfig struct {
	Name                string
	Endpoint            string
	ToolPrefix          string
	ClientName          string
	ClientVersion       string
	RequestTimeout      time.Duration
	Headers             map[string]string
	Client              *http.Client
	EnableServerStream  bool
	NotificationHandler func(context.Context, string, json.RawMessage) error
	RequestHandler      func(context.Context, string, json.RawMessage) (any, error)
	ErrorHandler        func(context.Context, error)
	Discovery           DiscoveryConfig
}

type HTTPClient struct {
	cfg               HTTPConfig
	httpClient        *http.Client
	sessionID         string
	negotiatedProto   string
	nextID            atomic.Int64
	closed            atomic.Bool
	initMu            sync.Mutex
	stateMu           sync.RWMutex
	bgCtx             context.Context
	bgCancel          context.CancelFunc
	streamDone        chan struct{}
	streamStarted     bool
	discovery         *discoveryState
	capState          *capabilityState
	eventSink         runtimeEventSink
	hooksMu           sync.RWMutex
	notificationHooks []func(context.Context, string, json.RawMessage) error
}

type HTTPExtension struct {
	name              string
	cfg               HTTPConfig
	client            *HTTPClient
	tools             []*agentcore.Tool
	agent             *agentcore.Agent
	toolNames         []string
	refreshMu         sync.Mutex
	refreshScheduleMu sync.Mutex
	refreshInFlight   bool
	refreshPending    bool
}

func NewHTTPClient(ctx context.Context, cfg HTTPConfig) (*HTTPClient, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("mcp: endpoint is required")
	}
	httpClient := cfg.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	bgCtx, bgCancel := context.WithCancel(context.Background())
	c := &HTTPClient{
		cfg:             cfg,
		httpClient:      httpClient,
		negotiatedProto: protocolVersion,
		bgCtx:           bgCtx,
		bgCancel:        bgCancel,
		streamDone:      make(chan struct{}),
		discovery:       newDiscoveryState(cfg.Discovery),
		capState:        newCapabilityState(),
	}
	if err := c.initializeSession(ctx); err != nil {
		return nil, err
	}
	if cfg.EnableServerStream {
		c.ensureServerStream()
	} else {
		close(c.streamDone)
	}
	return c, nil
}

func NewHTTPExtension(ctx context.Context, cfg HTTPConfig) (*HTTPExtension, error) {
	client, err := NewHTTPClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	tools, err := client.AgentTools(ctx)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	name := cfg.Name
	if name == "" {
		name = "mcp-http"
	}
	return &HTTPExtension{
		name:   name,
		cfg:    cfg,
		client: client,
		tools:  tools,
	}, nil
}

func (e *HTTPExtension) Name() string { return e.name }
func (e *HTTPExtension) Init(ctx context.Context, agent *agentcore.Agent) error {
	e.agent = agent
	e.toolNames = toolNames(e.tools)
	if e.client != nil {
		e.client.SetEventSink(agent.EmitEvent)
		e.client.AddNotificationHook(func(ctx context.Context, method string, params json.RawMessage) error {
			if method != "notifications/tools/list_changed" || !e.client.SupportsToolListChanged() {
				return nil
			}
			e.scheduleRefresh()
			return nil
		})
		e.client.AddCapabilityHook(func(ctx context.Context, caps ServerCapabilities) {
			e.emitCapabilitiesEvent(caps)
		})
		e.emitCapabilitiesEvent(e.client.Capabilities())
	}
	return nil
}
func (e *HTTPExtension) Client() *HTTPClient { return e.client }
func (e *HTTPExtension) Dispose() error {
	if e.client == nil {
		return nil
	}
	return e.client.Close()
}
func (e *HTTPExtension) Tools() []*agentcore.Tool {
	e.refreshMu.Lock()
	defer e.refreshMu.Unlock()
	return append([]*agentcore.Tool(nil), e.tools...)
}
func (e *HTTPExtension) SnapshotEvents() []agentcore.Event {
	if e.client == nil {
		return nil
	}
	return []agentcore.Event{CapabilitiesUpdatedEvent{
		At:           time.Now(),
		Extension:    e.name,
		Transport:    "http",
		Capabilities: e.client.Capabilities(),
	}}
}

func (e *HTTPExtension) emitCapabilitiesEvent(caps ServerCapabilities) {
	if e.agent == nil {
		return
	}
	e.agent.EmitEvent(CapabilitiesUpdatedEvent{
		At:           time.Now(),
		Extension:    e.name,
		Transport:    "http",
		Capabilities: caps,
	})
}

func (c *HTTPClient) initializeSession(ctx context.Context) error {
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
	headers, err := c.callInitialize(ctx, params, &result)
	if err != nil {
		return err
	}
	proto := protocolVersion
	if result.ProtocolVersion != "" {
		proto = result.ProtocolVersion
	}
	c.setSessionState(headers.Get(headerSessionID), proto)
	caps, err := decodeCapabilities(result.Capabilities)
	if err != nil {
		return err
	}
	c.capState.set(ctx, caps)
	c.ensureServerStream()
	return c.notifyInitialize(ctx)
}

func (c *HTTPClient) ListTools(ctx context.Context) ([]Tool, error) {
	var out []Tool
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var result toolListResult
		if _, err := c.call(ctx, "tools/list", params, &result); err != nil {
			return nil, err
		}
		out = append(out, result.Tools...)
		if result.NextCursor == "" {
			return out, nil
		}
		cursor = result.NextCursor
	}
}

func (c *HTTPClient) CallTool(ctx context.Context, name string, arguments map[string]any) (*ToolResult, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	var result ToolResult
	if _, err := c.call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *HTTPClient) AgentTools(ctx context.Context) ([]*agentcore.Tool, error) {
	return agentToolsFor(ctx, c.cfg.ToolPrefix, c)
}

func (c *HTTPClient) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.bgCancel()
	c.initMu.Lock()
	defer c.initMu.Unlock()
	c.stateMu.Lock()
	if !c.streamStarted {
		select {
		case <-c.streamDone:
		default:
			close(c.streamDone)
		}
	}
	c.stateMu.Unlock()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-c.streamDone:
	case <-timer.C:
	}
	sessionID, _ := c.sessionState()
	if sessionID == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, c.cfg.Endpoint, nil)
	if err != nil {
		return nil
	}
	c.applyHeaders(req, true, true)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	return nil
}

func (c *HTTPClient) shouldRunServerStream() bool {
	if c.closed.Load() {
		return false
	}
	if !c.cfg.EnableServerStream {
		return false
	}
	caps := c.Capabilities()
	if c.cfg.RequestHandler != nil || c.cfg.NotificationHandler != nil {
		return true
	}
	if caps.Tools.ListChanged || caps.Resources.Subscribe || caps.Resources.ListChanged || caps.Prompts.ListChanged {
		return true
	}
	return false
}

func (c *HTTPClient) ensureServerStream() {
	if !c.shouldRunServerStream() {
		return
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.streamStarted {
		return
	}
	c.streamStarted = true
	go c.runServerStream()
}

func (c *HTTPClient) extensionName() string {
	return util.DefaultString(c.cfg.Name, "mcp-http")
}

var _ agentcore.Extension = (*HTTPExtension)(nil)
var _ agentcore.ToolProvider = (*HTTPExtension)(nil)
