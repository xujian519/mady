package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// --- thread handlers ---

func (s *Server) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	thread, err := ts.CreateThread(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

func (s *Server) handleListThreads(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	threads, err := ts.ListThreads(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"threads": threads})
}

func (s *Server) handleGetThread(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	thread, err := ts.GetThread(r.Context(), r.PathValue("key"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

func (s *Server) handleDeleteThread(w http.ResponseWriter, r *http.Request) {
	// Delete is a destructive operation; require authorization.
	if !s.authorizeThreadAccess(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	store := s.snapshotConfig().Store
	if store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store configured"})
		return
	}
	if _, ok := s.threadStore(); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	if err := store.Delete(r.Context(), r.PathValue("key")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetThreadConfig(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	key := r.PathValue("key")
	cfg, _, err := ts.GetThreadConfig(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ThreadConfigResponse{
		ThreadID: key,
		Config:   agentcore.CloneCallConfig(cfg),
	})
}

func (s *Server) handlePutThreadConfig(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	var req ThreadConfigRequest
	if err := json.NewDecoder(s.limitedBody(w, r)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	thread, err := ts.SetThreadConfig(r.Context(), r.PathValue("key"), req.Config)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ThreadConfigResponse{
		ThreadID: thread.Info.ID,
		Config:   agentcore.CloneCallConfig(thread.Config),
	})
}

func (s *Server) handleGetThreadThinking(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	key := r.PathValue("key")
	thinking, _, err := ts.GetThreadThinking(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ThreadThinkingResponse{
		ThreadID: key,
		Thinking: agentcore.CloneThinkingConfig(thinking),
	})
}

func (s *Server) handlePutThreadThinking(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	var req ThreadThinkingRequest
	if err := json.NewDecoder(s.limitedBody(w, r)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	thread, err := ts.SetThreadThinking(r.Context(), r.PathValue("key"), req.Thinking)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ThreadThinkingResponse{
		ThreadID: thread.Info.ID,
		Thinking: agentcore.CloneThinkingConfig(thread.Thinking),
	})
}

func (s *Server) handleBranchThread(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.threadStore()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread store not configured"})
		return
	}
	var req BranchThreadRequest
	if r.Body != nil {
		if err := json.NewDecoder(s.limitedBody(w, r)).Decode(&req); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}
	thread, err := ts.BranchThread(r.Context(), r.PathValue("key"), req.EntryID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

// --- state handlers ---

func (s *Server) handleListStates(w http.ResponseWriter, r *http.Request) {
	store := s.snapshotConfig().Store
	if store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store configured"})
		return
	}
	keys, err := store.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (s *Server) handleGetState(w http.ResponseWriter, r *http.Request) {
	store := s.snapshotConfig().Store
	if store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store configured"})
		return
	}
	key := r.PathValue("key")
	snap, err := store.Load(r.Context(), key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleDeleteState(w http.ResponseWriter, r *http.Request) {
	store := s.snapshotConfig().Store
	if store == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no store configured"})
		return
	}
	key := r.PathValue("key")
	if err := store.Delete(r.Context(), key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- thread / config helpers ---

func (s *Server) threadStore() (threadStore, bool) {
	store := s.snapshotConfig().Store
	if store == nil {
		return nil, false
	}
	ts, ok := store.(threadStore)
	return ts, ok
}

func (s *Server) snapshotConfig() agentcore.Config {
	cfg := s.config.Get()
	cfg.SelectedSkills = agentcore.CloneStringSlice(cfg.SelectedSkills)
	cfg.SkillPaths = agentcore.CloneStringSlice(cfg.SkillPaths)
	cfg.AvailableSkills = cloneSkills(cfg.AvailableSkills)
	cfg.SkillDiagnostics = cloneSkillDiagnostics(cfg.SkillDiagnostics)
	return cfg
}

func (s *Server) threadCallConfig(ctx context.Context, threadID string) (*agentcore.CallConfig, bool, error) {
	ts, ok := s.threadStore()
	if !ok {
		return nil, false, nil
	}
	cfg, hasCfg, err := ts.GetThreadConfig(ctx, threadID)
	if err != nil {
		return nil, false, err
	}
	return cfg, hasCfg, nil
}

// authorizeThreadAccess checks authorization for destructive thread operations.
// Delegates to the skill API auth token if configured, otherwise allows access
// (local/single-user deployment mode).
func (s *Server) authorizeThreadAccess(r *http.Request) bool {
	cfg := s.snapshotConfig()
	token := strings.TrimSpace(cfg.SkillAPIAuthToken)
	if token == "" {
		return true // no auth configured — local deployment
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	expected := "Bearer " + token
	return subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) == 1
}
