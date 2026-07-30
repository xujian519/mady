package a2a

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// 默认仅允许同源 WebSocket 握手（严格 host 相等，防子域名前缀绕过）；
	// handleWebSocket 会替换为 Server.checkWSOrigin，
	// 即同源 + 本地回环来源 + 配置白名单。
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || r.Host == "" {
			return false
		}
		return strings.EqualFold(u.Host, r.Host)
	},
}

// checkWSOrigin 校验 WebSocket 握手的 Origin（C4 修复）：
//   - 无 Origin 头（非浏览器客户端）放行；
//   - 同源放行（Origin 的 host 与请求 Host 严格相等，防子域名前缀绕过）；
//   - 本地回环来源（localhost/127.0.0.1/::1，任意端口）放行，保持本地开发体验；
//   - 其余来源仅在 WithAllowedOrigins 配置的显式白名单内才放行。
func (s *Server) checkWSOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if r.Host != "" && strings.EqualFold(u.Host, r.Host) {
		return true
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return slices.Contains(s.allowedOrigins, origin)
}

type wsConn struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	closed bool
}

func (c *wsConn) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("websocket closed")
	}
	return c.conn.WriteJSON(v)
}

func (c *wsConn) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.auth.APIKey != "" || s.auth.BearerToken != "" {
		// Note: token/apiKey via query params is a pragmatic trade-off —
		// browser WebSocket API cannot set custom headers on upgrade
		// requests. The downside is potential leakage in upstream proxy
		// logs. Use RedactURL() for logging when URL containing these
		// params needs to be recorded.
		key := r.URL.Query().Get("apiKey")
		token := r.URL.Query().Get("token")
		if !s.checkWSAuth(key, token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// 按服务器配置替换 Origin 校验（同源 + 回环来源 + 白名单）。
	up := upgrader
	up.CheckOrigin = s.checkWSOrigin
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("websocket upgrade failed", "error", err)
		return
	}

	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	wc := &wsConn{conn: conn}
	go s.wsPingLoop(wc)
	go s.wsReadLoop(wc, r)
}

func (s *Server) wsPingLoop(wc *wsConn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		wc.mu.Lock()
		if wc.closed {
			wc.mu.Unlock()
			return
		}
		err := wc.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second))
		wc.mu.Unlock()
		if err != nil {
			return
		}
	}
}

func (s *Server) checkWSAuth(key, token string) bool {
	if s.auth.APIKey != "" && subtle.ConstantTimeCompare([]byte(key), []byte(s.auth.APIKey)) == 1 {
		return true
	}
	if s.auth.BearerToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.auth.BearerToken)) == 1 {
		return true
	}
	return false
}

func (s *Server) wsReadLoop(wc *wsConn, r *http.Request) {
	defer func() { _ = wc.close() }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	for {
		_ = wc.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, message, err := wc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.logger.Debug("websocket read error", "error", err)
			}
			return
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(message, &req); err != nil {
			if werr := wc.writeJSON(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &JSONRPCError{Code: JSONRPCParseError, Message: err.Error()},
			}); werr != nil {
				_ = wc.close()
				return
			}
			continue
		}

		switch req.Method {
		case "tasks/send":
			s.handleWSSendTask(ctx, wc, req)
		case "tasks/get":
			s.handleWSGetTask(ctx, wc, req)
		case "tasks/cancel":
			s.handleWSCancelTask(ctx, wc, req)
		case "tasks/query":
			s.handleWSQueryTasks(ctx, wc, req)
		case "tasks/subscribe":
			if !s.handler.Card().Capabilities.Streaming {
				if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: A2AErrorUnsupportedOperation, Message: "streaming not supported"}}); err != nil {
					_ = wc.close()
					return
				}
				continue
			}
			s.handleWSSubscribe(ctx, wc, req, cancel)
		case "tasks/resubscribe":
			if !s.handler.Card().Capabilities.Streaming {
				if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: A2AErrorUnsupportedOperation, Message: "streaming not supported"}}); err != nil {
					_ = wc.close()
					return
				}
				continue
			}
			s.handleWSResubscribe(ctx, wc, req, cancel)
		case "tasks/pushNotification/set":
			if !s.handler.Card().Capabilities.PushNotifications {
				if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: A2AErrorPushNotSupported, Message: "push notifications not supported"}}); err != nil {
					_ = wc.close()
					return
				}
				continue
			}
			s.handleWSSetPushNotification(ctx, wc, req)
		case "tasks/pushNotification/get":
			if !s.handler.Card().Capabilities.PushNotifications {
				if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: A2AErrorPushNotSupported, Message: "push notifications not supported"}}); err != nil {
					_ = wc.close()
					return
				}
				continue
			}
			s.handleWSGetPushNotification(ctx, wc, req)
		default:
			if err := wc.writeJSON(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &JSONRPCError{Code: JSONRPCMethodNotFound, Message: "method not found: " + req.Method},
			}); err != nil {
				_ = wc.close()
				return
			}
		}
	}
}

func (s *Server) handleWSSendTask(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
	var params SendTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	card := s.handler.Card()
	if len(card.DefaultInputModes) > 0 {
		requestedModes := ExtractInputModes(params.Message)
		if err := ValidateInputModes(requestedModes, card.DefaultInputModes); err != nil {
			if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: A2AErrorContentTypeNotSupported, Message: err.Error()}}); err != nil {
				_ = wc.close()
				return
			}
			return
		}
	}

	task, err := s.handler.SendTask(ctx, params)
	if err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInternalError, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	s.recordTask(task)
	if s.sessionMgr != nil && task.SessionID != "" {
		s.sessionMgr.AddTask(task.SessionID, task.ID)
	}
	if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: task}); err != nil {
		_ = wc.close()
		return
	}
}

func (s *Server) handleWSGetTask(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
	var params GetTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	task, err := s.handler.GetTask(ctx, params)
	if err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: A2AErrorTaskNotFound, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: task}); err != nil {
		_ = wc.close()
		return
	}
}

func (s *Server) handleWSCancelTask(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
	var params CancelTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	task, err := s.handler.CancelTask(ctx, params)
	if err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: A2AErrorTaskNotCancelable, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	s.recordTask(task)
	if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: task}); err != nil {
		_ = wc.close()
		return
	}
}

func (s *Server) handleWSQueryTasks(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
	var params QueryTasksRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	result, err := s.handler.QueryTasks(ctx, params)
	if err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInternalError, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}); err != nil {
		_ = wc.close()
		return
	}
}

func (s *Server) handleWSSetPushNotification(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
	var params SetPushNotificationRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	if err := s.handler.SetPushNotification(ctx, params); err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInternalError, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: nil}); err != nil {
		_ = wc.close()
		return
	}
}

func (s *Server) handleWSGetPushNotification(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
	var params GetTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	cfg, err := s.handler.GetPushNotification(ctx, params.ID)
	if err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInternalError, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: cfg}); err != nil {
		_ = wc.close()
		return
	}
}

func (s *Server) handleWSSubscribe(ctx context.Context, wc *wsConn, req JSONRPCRequest, _ context.CancelFunc) {
	var params SendTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	card := s.handler.Card()
	if len(card.DefaultInputModes) > 0 {
		requestedModes := ExtractInputModes(params.Message)
		if err := ValidateInputModes(requestedModes, card.DefaultInputModes); err != nil {
			if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: A2AErrorContentTypeNotSupported, Message: err.Error()}}); err != nil {
				_ = wc.close()
				return
			}
			return
		}
	}

	taskID := params.ID
	if taskID == "" {
		taskID = fmt.Sprintf("task-%d", time.Now().UnixNano())
		params.ID = taskID
	}

	ch := s.subscribeToTask(taskID)
	defer s.unsubscribeFromTask(taskID, ch)

	type taskResult struct {
		task *Task
		err  error
	}
	resultCh := make(chan taskResult, 1)
	go func() {
		task, err := s.handler.SendTask(ctx, params)
		resultCh <- taskResult{task, err}
	}()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case r := <-resultCh:
			if r.err != nil {
				if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInternalError, Message: r.err.Error()}}); err != nil {
					_ = wc.close()
					return
				}
				return
			}
			s.recordTask(r.task)
			if s.sessionMgr != nil && r.task.SessionID != "" {
				s.sessionMgr.AddTask(r.task.SessionID, r.task.ID)
			}
			final := isTerminalState(r.task.State) || r.task.State == TaskStateInputRequired
			if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: r.task}); err != nil {
				_ = wc.close()
				return
			}
			if final {
				return
			}
		case <-heartbeat.C:
			wc.mu.Lock()
			if wc.closed {
				wc.mu.Unlock()
				return
			}
			err := wc.conn.WriteMessage(websocket.TextMessage, []byte(": heartbeat\n\n"))
			wc.mu.Unlock()
			if err != nil {
				return
			}
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := wc.writeJSON(ev); err != nil {
				_ = wc.close()
				return
			}
			if ev.Final {
				return
			}
		}
	}
}

func (s *Server) handleWSResubscribe(ctx context.Context, wc *wsConn, req JSONRPCRequest, cancel context.CancelFunc) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	if _, err := s.handler.GetTask(ctx, GetTaskRequest{ID: params.ID}); err != nil {
		if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: JSONRPCInvalidParams, Message: fmt.Sprintf("task %q not found", params.ID)}}); err != nil {
			_ = wc.close()
			return
		}
		return
	}

	ch := s.subscribeToTask(params.ID)
	go s.wsForwardEvents(ctx, wc, ch, params.ID, cancel)
}

func (s *Server) subscribeToTask(taskID string) chan *TaskUpdateEvent {
	ts := s.getTaskState(taskID)
	ts.mu.Lock()
	ch := make(chan *TaskUpdateEvent, 16)
	ts.subs = append(ts.subs, ch)
	ts.mu.Unlock()
	return ch
}

func (s *Server) unsubscribeFromTask(taskID string, ch chan *TaskUpdateEvent) {
	ts := s.getTaskState(taskID)
	ts.mu.Lock()
	for i, c := range ts.subs {
		if c == ch {
			ts.subs = append(ts.subs[:i], ts.subs[i+1:]...)
			break
		}
	}
	close(ch)
	ts.mu.Unlock()
}

func (s *Server) wsForwardEvents(ctx context.Context, wc *wsConn, ch chan *TaskUpdateEvent, taskID string, cancel context.CancelFunc) {
	defer cancel()
	defer s.unsubscribeFromTask(taskID, ch)

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			wc.mu.Lock()
			if wc.closed {
				wc.mu.Unlock()
				return
			}
			err := wc.conn.WriteMessage(websocket.TextMessage, []byte(": heartbeat\n\n"))
			wc.mu.Unlock()
			if err != nil {
				return
			}
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := wc.writeJSON(ev); err != nil {
				_ = wc.close()
				return
			}
			if ev.Final {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// WebSocket Client
// ---------------------------------------------------------------------------
