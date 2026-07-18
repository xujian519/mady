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
	"strings"
	"time"
)

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
