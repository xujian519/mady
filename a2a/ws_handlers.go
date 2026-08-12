package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

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

//nolint:dupl // intentional pattern: each WS handler follows the same boilerplate
func (s *Server) handleWSGetTask(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
	s.wsHandlerFor(func(ctx context.Context, wc *wsConn, req JSONRPCRequest) (any, error) {
		var params GetTaskRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}
		}
		task, err := s.handler.GetTask(ctx, params)
		if err != nil {
			return nil, &JSONRPCError{Code: A2AErrorTaskNotFound, Message: err.Error()}
		}
		return task, nil
	})(ctx, wc, req)
}

func (s *Server) handleWSCancelTask(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
	s.wsHandlerFor(func(ctx context.Context, wc *wsConn, req JSONRPCRequest) (any, error) {
		var params CancelTaskRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}
		}
		task, err := s.handler.CancelTask(ctx, params)
		if err != nil {
			return nil, &JSONRPCError{Code: A2AErrorTaskNotCancelable, Message: err.Error()}
		}
		s.recordTask(task)
		return task, nil
	})(ctx, wc, req)
}

//nolint:dupl // intentional pattern: each WS handler follows the same boilerplate
func (s *Server) handleWSQueryTasks(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
	s.wsHandlerFor(func(ctx context.Context, wc *wsConn, req JSONRPCRequest) (any, error) {
		var params QueryTasksRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}
		}
		result, err := s.handler.QueryTasks(ctx, params)
		if err != nil {
			return nil, &JSONRPCError{Code: JSONRPCInternalError, Message: err.Error()}
		}
		return result, nil
	})(ctx, wc, req)
}

func (s *Server) handleWSSetPushNotification(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
	s.wsHandlerFor(func(ctx context.Context, wc *wsConn, req JSONRPCRequest) (any, error) {
		var params SetPushNotificationRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}
		}
		if err := s.handler.SetPushNotification(ctx, params); err != nil {
			return nil, &JSONRPCError{Code: JSONRPCInternalError, Message: err.Error()}
		}
		return nil, nil
	})(ctx, wc, req)
}

func (s *Server) handleWSGetPushNotification(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
	s.wsHandlerFor(func(ctx context.Context, wc *wsConn, req JSONRPCRequest) (any, error) {
		var params GetTaskRequest
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &JSONRPCError{Code: JSONRPCInvalidParams, Message: err.Error()}
		}
		cfg, err := s.handler.GetPushNotification(ctx, params.ID)
		if err != nil {
			return nil, &JSONRPCError{Code: JSONRPCInternalError, Message: err.Error()}
		}
		return cfg, nil
		//nolint:gocognit // 原因：内联匿名函数，包含多个推送配置分支
	})(ctx, wc, req)
}

//nolint:gocognit // 原因：WebSocket 订阅处理，多参数校验和推送
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
