package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/xujian519/mady/agentcore"
)

// --- chat handler ---

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(s.limitedBody(w, r)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ChatResponse{Error: "invalid request body"})
		return
	}
	threadID, err := s.ensureThreadID(r.Context(), req.ThreadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{Error: err.Error()})
		return
	}
	req.ThreadID = threadID
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, ChatResponse{ThreadID: req.ThreadID, Error: "message is required"})
		return
	}

	if req.Stream {
		s.handleStreamChat(w, r, req)
	} else {
		s.handleSyncChat(w, r, req)
	}
}

func (s *Server) handleSyncChat(w http.ResponseWriter, r *http.Request, req ChatRequest) {
	entry, err := s.loadAgent(r.Context(), req.ThreadID, requestCallConfig(req))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{
			ThreadID: req.ThreadID,
			Error:    err.Error(),
		})
		return
	}
	agent := entry.agent

	output, err := agent.Run(r.Context(), req.Message)
	if saveErr := s.saveAgentState(r.Context(), agent, req.ThreadID); saveErr != nil && err == nil {
		err = saveErr
	}
	s.releaseAgent(entry, req.ThreadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{
			ThreadID: req.ThreadID,
			Error:    err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, ChatResponse{Output: output, ThreadID: req.ThreadID})
}

func (s *Server) handleStreamChat(w http.ResponseWriter, r *http.Request, req ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{Error: "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	entry, err := s.loadAgent(r.Context(), req.ThreadID, requestCallConfig(req))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ChatResponse{
			ThreadID: req.ThreadID,
			Error:    err.Error(),
		})
		return
	}
	agent := entry.agent

	// dead marks the connection as unwritable (client disconnect, marshal
	// failure, underlying write error). Once set, subsequent writeSSE calls
	// short-circuit — no point doing further work on a dead connection, and
	// continuing to write would just spam the log. Guarded by the writeSSE
	// mutex + atomic for lock-free reads from the handler path.
	var dead atomic.Bool
	var mu sync.Mutex
	writeSSE := func(eventType string, data any) {
		if dead.Load() {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if dead.Load() {
			return
		}
		payload, err := json.Marshal(data)
		if err != nil {
			log.Printf("server: SSE marshal error (event=%s): %v", eventType, err)
			dead.Store(true)
			return
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, payload); err != nil {
			log.Printf("server: SSE write error (event=%s): %v", eventType, err)
			dead.Store(true)
			return
		}
		flusher.Flush()
	}

	// Register the SSE writer as a scoped handler on the agent's event bus.
	// The handler is unregistered BEFORE the agent is released back to the
	// pool — not just via defer. If we relied on defer alone, there would be
	// a window between releaseAgent (agent back in pool) and the deferred
	// unregister (runs at function return): a concurrent request on the same
	// thread could reuse the agent and its events would flow into this dead
	// ResponseWriter. Unregistering before release closes that window.
	// The defer is kept as an idempotent safety net.
	unregister := agent.OnAll(func(e agentcore.Event) {
		writeSSE(string(e.EventKind()), streamEventPayload(req.ThreadID, e))
	})
	defer unregister()
	agent.EmitExtensionSnapshots()

	output, runErr := agent.Run(r.Context(), req.Message)
	saveErr := s.saveAgentState(r.Context(), agent, req.ThreadID)
	if saveErr != nil && runErr == nil {
		runErr = saveErr
	}
	unregister() // detach BEFORE releasing — see comment above
	s.releaseAgent(entry, req.ThreadID)

	done := StreamDoneEvent{
		Schema:   streamSchemaChatDone,
		Type:     "done",
		ThreadID: req.ThreadID,
		Output:   output,
	}
	if runErr != nil {
		done.Error = runErr.Error()
	}
	writeSSE("done", done)
}

func (s *Server) saveAgentState(ctx context.Context, agent *agentcore.Agent, threadID string) error {
	store := s.snapshotConfig().Store
	if threadID == "" || store == nil {
		return nil
	}
	return agent.SaveState(ctx, threadID)
}

func requestCallConfig(req ChatRequest) *agentcore.CallConfig {
	if req.Model == "" && req.ResponseFormat == nil && req.Thinking == nil && len(req.Skills) == 0 {
		return nil
	}
	return &agentcore.CallConfig{
		Model:          req.Model,
		ResponseFormat: agentcore.CloneResponseFormat(req.ResponseFormat),
		Thinking:       agentcore.CloneThinkingConfig(req.Thinking),
		Skills:         agentcore.CloneStringSlice(req.Skills),
	}
}

func (s *Server) ensureThreadID(ctx context.Context, threadID string) (string, error) {
	if threadID != "" {
		return threadID, nil
	}
	ts, ok := s.threadStore()
	if !ok {
		return "", nil
	}
	thread, err := ts.CreateThread(ctx)
	if err != nil {
		return "", err
	}
	return thread.Info.ID, nil
}
