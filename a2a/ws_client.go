package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSClient is a WebSocket client for the A2A protocol, supporting task updates
// and JSON-RPC communication with the agent server.
// WSClient is an A2A WebSocket client for connecting to remote agents.
type WSClient struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
	bearer     string
	logger     *slog.Logger
	maxRetries int
}

// NewWSClient creates a new WebSocket client connected to the given base URL.
// NewWSClient creates a new A2A WebSocket client with the given base URL.
func NewWSClient(baseURL string) *WSClient {
	return &WSClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     slog.Default(),
		maxRetries: 3,
	}
}

// WithAPIKey sets the API key for authentication and returns the client for chaining.
// WithAPIKey sets the API key for authentication.
func (c *WSClient) WithAPIKey(key string) *WSClient {
	c.apiKey = key
	return c
}

// WithBearer sets the bearer token for authentication and returns the client for chaining.
// WithBearer sets the bearer token for authentication.
func (c *WSClient) WithBearer(token string) *WSClient {
	c.bearer = token
	return c
}

// WithMaxRetries sets the maximum number of reconnection retries and returns the client for chaining.
// WithMaxRetries sets the maximum number of reconnection retries.
func (c *WSClient) WithMaxRetries(n int) *WSClient {
	c.maxRetries = n
	return c
}

// WSConnection is an active WebSocket connection that can send and receive A2A messages.
// WSConnection represents an active WebSocket connection to an A2A agent.
type WSConnection struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	closed bool
	ch     chan *TaskUpdateEvent
	err    error
	ctx    context.Context
	cancel context.CancelFunc

	client   *WSClient
	maxRetry int
	retryNum int
}

// Connect establishes a WebSocket connection to the A2A server and returns the connection.
// Connect establishes a WebSocket connection to the A2A agent.
func (c *WSClient) Connect(ctx context.Context) (*WSConnection, error) {
	u := c.baseURL
	if strings.HasPrefix(u, "http") {
		u = "ws" + u[4:]
	}
	if !strings.HasSuffix(u, "/ws") {
		u += "/ws"
	}

	reqHeader := http.Header{}
	if c.apiKey != "" {
		reqHeader.Set("X-API-Key", c.apiKey)
	}
	if c.bearer != "" {
		reqHeader.Set("Authorization", "Bearer "+c.bearer)
	}

	conn, httpResp, err := websocket.DefaultDialer.DialContext(ctx, u, reqHeader)
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("websocket dial: %w", err)
	}

	ctx2, cancel := context.WithCancel(ctx)
	wsc := &WSConnection{
		conn:     conn,
		ch:       make(chan *TaskUpdateEvent, 16),
		ctx:      ctx2,
		cancel:   cancel,
		client:   c,
		maxRetry: c.maxRetries,
	}

	go wsc.readLoop()
	return wsc, nil
}

func (c *WSConnection) readLoop() {
	defer close(c.ch)
	defer c.cancel()

	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPingHandler(func(appData string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return c.conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
	})

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if c.tryReconnect() {
				continue
			}
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				c.mu.Lock()
				c.err = err
				c.mu.Unlock()
			}
			return
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(message, &raw); err != nil {
			continue
		}

		if _, hasResult := raw["result"]; hasResult {
			var resp JSONRPCResponse
			if err := json.Unmarshal(message, &resp); err != nil {
				continue
			}
			if resp.Result != nil {
				data, marshalErr := json.Marshal(resp.Result)
				if marshalErr != nil {
					slog.Default().Warn("a2a: failed to marshal ws result", "err", marshalErr)
					continue
				}
				var task Task
				if err := json.Unmarshal(data, &task); err == nil && task.ID != "" {
					ev := &TaskUpdateEvent{
						ID:     resp.ID,
						Result: &task,
						Final:  isTerminalState(task.State),
					}
					select {
					case c.ch <- ev:
					default:
						slog.Default().Warn("a2a: ws subscriber channel full, event dropped", "id", ev.ID)
					}
				}
			}
			continue
		}

		var ev TaskUpdateEvent
		if err := json.Unmarshal(message, &ev); err == nil && ev.Result != nil {
			select {
			case c.ch <- &ev:
			default:
				slog.Default().Warn("a2a: ws subscriber channel full, event dropped", "id", ev.ID)
			}
		}
	}
}

func (c *WSConnection) tryReconnect() bool {
	c.mu.Lock()
	if c.closed || c.client == nil || c.maxRetry <= 0 {
		c.mu.Unlock()
		return false
	}
	c.retryNum++
	attempt := c.retryNum
	c.maxRetry--
	c.mu.Unlock()

	backoff := time.Duration(500<<min(attempt-1, 5)) * time.Millisecond
	backoff = min(backoff, 30*time.Second)
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-c.ctx.Done():
		return false
	}

	u := c.client.baseURL
	if strings.HasPrefix(u, "http") {
		u = "ws" + u[4:]
	}
	if !strings.HasSuffix(u, "/ws") {
		u += "/ws"
	}

	reqHeader := http.Header{}
	if c.client.apiKey != "" {
		reqHeader.Set("X-API-Key", c.client.apiKey)
	}
	if c.client.bearer != "" {
		reqHeader.Set("Authorization", "Bearer "+c.client.bearer)
	}

	conn, httpResp, err := websocket.DefaultDialer.DialContext(c.ctx, u, reqHeader)
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}
	if err != nil {
		return false
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = conn.Close()
		return false
	}
	oldConn := c.conn
	c.conn = conn
	c.mu.Unlock()

	// RACE WINDOW: between c.conn assignment and oldConn.Close(),
	// SendRequest may write to the new conn via c.mu while we
	// configure deadlines/handlers below. This is safe because
	// gorilla/websocket supports concurrent read+write, but the
	// old conn's ReadMessage has already returned an error so its
	// read loop will not touch it again.
	_ = oldConn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPingHandler(func(appData string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
	})
	return true
}

// SendRequest sends a JSON-RPC request over the WebSocket connection.
// SendRequest sends a JSON-RPC request over the WebSocket connection.
func (c *WSConnection) SendRequest(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("connection closed")
	}

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
		Method:  method,
	}
	if params != nil {
		p, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = p
	}

	return c.conn.WriteJSON(req)
}

// Recv receives the next task update event from the WebSocket connection.
// Recv receives the next task update event from the WebSocket connection.
func (c *WSConnection) Recv() (*TaskUpdateEvent, bool) {
	ev, ok := <-c.ch
	return ev, ok
}

// Err returns any error that occurred on the WebSocket connection.
// Err returns any error that occurred during the connection lifecycle.
func (c *WSConnection) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close gracefully closes the WebSocket connection with a normal closure code.
// Close closes the WebSocket connection gracefully.
func (c *WSConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	// Best-effort close handshake; ignore errors because the peer may already be gone.
	_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(5*time.Second))
	return c.conn.Close()
}
