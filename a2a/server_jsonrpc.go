package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

	if err := json.NewEncoder(w).Encode(results); err != nil {
		slog.Default().Warn("a2a: 编码 JSONRPC 批量响应失败", "error", err)
	}
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
		handleTaskAction(s, ctx, w, req, s.handler.GetTask, A2AErrorTaskNotFound)
	case "tasks/cancel":
		handleTaskAction(s, ctx, w, req, s.handler.CancelTask, A2AErrorTaskNotCancelable)
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
