package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Client calls remote A2A agents.
// ---------------------------------------------------------------------------

// Client is an A2A client for interacting with remote agents.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	apiKey      string
	bearerToken string

	mu           sync.RWMutex
	idCounter    int64
	maxRetries   int
	retryBackoff time.Duration
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// WithAPIKey sets an API key for authentication (sent as X-API-Key header).
func WithAPIKey(key string) ClientOption {
	return func(c *Client) {
		c.apiKey = key
	}
}

// WithBearerToken sets a Bearer token for authentication (sent as Authorization header).
func WithBearerToken(token string) ClientOption {
	return func(c *Client) {
		c.bearerToken = token
	}
}

// WithRetry sets retry policy for the client.
func WithRetry(maxRetries int, backoff time.Duration) ClientOption {
	return func(c *Client) {
		c.maxRetries = maxRetries
		c.retryBackoff = backoff
	}
}

// NewClient creates an A2A client targeting the given agent URL.
func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// setAuthHeaders applies authentication headers to the request.
func (c *Client) setAuthHeaders(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
}

// ---------------------------------------------------------------------------
// Discovery
// ---------------------------------------------------------------------------

// GetAgentCard fetches the agent card from /.well-known/agent.json.
func (c *Client) GetAgentCard(ctx context.Context) (*AgentCard, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/.well-known/agent.json", nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("agent card: %d %s", resp.StatusCode, string(body))
	}

	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("decode agent card: %w", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return &card, nil
}

// ---------------------------------------------------------------------------
// Task operations
// ---------------------------------------------------------------------------

// SendTask sends a task to the remote agent (synchronous).
func (c *Client) SendTask(ctx context.Context, req SendTaskRequest) (*Task, error) {
	var task Task
	if err := c.callAndDecode(ctx, "tasks/send", req, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// SendTaskSubscribe sends a task and subscribes to streaming updates via SSE.
func (c *Client) SendTaskSubscribe(ctx context.Context, req SendTaskRequest) (*TaskStream, error) {
	rpcReq := JSONRPCRequest{JSONRPC: "2.0", ID: c.nextID(), Method: "tasks/sendSubscribe"}
	params, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	rpcReq.Params = params

	body, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	c.setAuthHeaders(httpReq)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if httpResp.StatusCode != http.StatusOK {
		_ = httpResp.Body.Close()
		return nil, fmt.Errorf("sse: %d", httpResp.StatusCode)
	}

	stream := &TaskStream{
		ch:       make(chan *TaskUpdateEvent, 8),
		body:     httpResp.Body,
		cancel:   ctx,
		client:   c,
		taskID:   req.ID,
		maxRetry: c.maxRetries,
	}

	go stream.readLoop()
	return stream, nil
}

// GetTask retrieves the current state of a task.
func (c *Client) GetTask(ctx context.Context, req GetTaskRequest) (*Task, error) {
	var task Task
	if err := c.callAndDecode(ctx, "tasks/get", req, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// CancelTask cancels a running task.
func (c *Client) CancelTask(ctx context.Context, req CancelTaskRequest) (*Task, error) {
	var task Task
	if err := c.callAndDecode(ctx, "tasks/cancel", req, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// QueryTasks queries tasks by session ID or state.
func (c *Client) QueryTasks(ctx context.Context, req QueryTasksRequest) (*QueryTasksResult, error) {
	var result QueryTasksResult
	if err := c.callAndDecode(ctx, "tasks/query", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetPushNotification configures push notifications for a task.
func (c *Client) SetPushNotification(ctx context.Context, req SetPushNotificationRequest) error {
	resp, err := c.call(ctx, "tasks/pushNotification/set", req)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

// GetPushNotification retrieves the push notification config for a task.
func (c *Client) GetPushNotification(ctx context.Context, taskID string) (*PushNotificationConfig, error) {
	resp, err := c.call(ctx, "tasks/pushNotification/get", map[string]string{"id": taskID})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, resp.Error
	}

	data, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}

	var cfg PushNotificationConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode push config: %w", err)
	}
	return &cfg, nil
}

// ResubscribeTask reconnects to an existing task's SSE stream, replaying
// historical events followed by live updates.
func (c *Client) ResubscribeTask(ctx context.Context, taskID string) (*TaskStream, error) {
	return c.resubscribe(ctx, taskID, "")
}

func (c *Client) resubscribe(ctx context.Context, taskID, lastEventID string) (*TaskStream, error) {
	rpcReq := JSONRPCRequest{JSONRPC: "2.0", ID: c.nextID(), Method: "tasks/resubscribe"}
	params, err := json.Marshal(map[string]string{"id": taskID})
	if err != nil {
		return nil, err
	}
	rpcReq.Params = params

	body, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if lastEventID != "" {
		httpReq.Header.Set("Last-Event-ID", lastEventID)
	}
	c.setAuthHeaders(httpReq)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if httpResp.StatusCode != http.StatusOK {
		_ = httpResp.Body.Close()
		return nil, fmt.Errorf("sse: %d", httpResp.StatusCode)
	}

	stream := &TaskStream{
		ch:       make(chan *TaskUpdateEvent, 8),
		body:     httpResp.Body,
		cancel:   ctx,
		client:   c,
		taskID:   taskID,
		maxRetry: c.maxRetries,
	}

	go stream.readLoop()
	return stream, nil
}
