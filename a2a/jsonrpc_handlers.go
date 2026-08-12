package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

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

//nolint:gocognit // 原因：A2A 任务订阅处理，含多输入模式校验和流管理
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

// handleTaskAction is a generic handler for task actions that unmarshal a
// request, call the handler, record the task, and write the result.
func handleTaskAction[Req any](s *Server, ctx context.Context, w http.ResponseWriter, req JSONRPCRequest,
	handler func(context.Context, Req) (*Task, error), errCode int,
) {
	var params Req
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, JSONRPCInvalidParams, err.Error())
		return
	}
	task, err := handler(ctx, params)
	if err != nil {
		writeJSONRPCError(w, req.ID, errCode, err.Error())
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

//nolint:gocognit // 原因：A2A 重订阅处理，含任务查询和推送配置
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
	if afterSeq > 0 {
		// Defensive deep copy: history entries carry pointer fields (Result
		// *Task, Artifact *Artifact) that should not be exposed to callers
		// without copying, even though the history is append-only and entries
		// are never mutated in-place after storage.
		replay := make([]historyEntry, 0, len(ts.history))
		for _, entry := range ts.history {
			if entry.seq > afterSeq {
				entry.event = deepCopyEvent(entry.event)
				replay = append(replay, entry)
			}
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
	} else {
		replay := make([]historyEntry, len(ts.history))
		for i, entry := range ts.history {
			entry.event = deepCopyEvent(entry.event)
			replay[i] = entry
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
