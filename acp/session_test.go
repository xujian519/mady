package acp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xujian519/mady/acp"
	"github.com/xujian519/mady/agentcore"
)

// ---- Stubs ----

type stubAgentFactory struct {
	mu           sync.Mutex
	defaultModel string
	defaultMode  string
	createCount  int
	createError  error
}

func (f *stubAgentFactory) CreateAgent(_ context.Context, _, _, model, mode string) (acp.AgentInstance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createError != nil {
		return nil, f.createError
	}
	f.createCount++
	m := model
	if m == "" {
		m = f.DefaultModel()
	}
	md := mode
	if md == "" {
		md = f.DefaultMode()
	}
	return &stubAgentInstance{model: m, mode: md}, nil
}

func (f *stubAgentFactory) DefaultModel() string {
	if f.defaultModel != "" {
		return f.defaultModel
	}
	return "test-model"
}

func (f *stubAgentFactory) DefaultMode() string {
	if f.defaultMode != "" {
		return f.defaultMode
	}
	return "test-mode"
}

func (f *stubAgentFactory) AvailableModes() []acp.SessionMode {
	return []acp.SessionMode{{ID: "test-mode", Name: "Test Mode", Description: "Default test mode"}}
}

type stubAgentInstance struct {
	model string
	mode  string
}

func (a *stubAgentInstance) Run(_ context.Context, input string) (string, error) {
	return "response to: " + input, nil
}

func (a *stubAgentInstance) Core() *agentcore.Agent { return nil }
func (a *stubAgentInstance) Model() string           { return a.model }
func (a *stubAgentInstance) Mode() string            { return a.mode }

type stubSessionStore struct {
	mu    sync.Mutex
	metas map[string]acp.SessionMeta
}

func (s *stubSessionStore) LoadSessionMeta(sessionID string) (acp.SessionMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, ok := s.metas[sessionID]
	if !ok {
		return acp.SessionMeta{}, fmt.Errorf("session %s not found", sessionID)
	}
	return meta, nil
}

func (s *stubSessionStore) SaveSessionMeta(meta acp.SessionMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metas == nil {
		s.metas = make(map[string]acp.SessionMeta)
	}
	s.metas[meta.SessionID] = meta
	return nil
}

func (s *stubSessionStore) ListSessions(cwd string) []acp.SessionMeta {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []acp.SessionMeta
	for _, meta := range s.metas {
		if cwd != "" && meta.CWD != cwd {
			continue
		}
		result = append(result, meta)
	}
	return result
}

// ---- Test Helpers ----

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestManager(t *testing.T, factory acp.AgentFactory, store acp.SessionStore) *acp.SessionManager {
	t.Helper()
	if factory == nil {
		factory = &stubAgentFactory{}
	}
	cfg := acp.SessionManagerConfig{
		AgentFactory: factory,
		Logger:       testLogger(t),
	}
	if store != nil {
		cfg.SessionStore = store
	}
	return acp.NewSessionManager(cfg)
}

func newTestServer(t *testing.T, input string, caps map[string]bool) (*acp.Server, *bytes.Buffer) {
	t.Helper()
	factory := &stubAgentFactory{}
	sm := newTestManager(t, factory, nil)
	output := &bytes.Buffer{}
	srv := acp.NewServer(acp.ServerConfig{
		SessionManager:        sm,
		AgentInfo:             acp.AgentInfo{Name: "test", Version: "1.0"},
		Reader:                bytes.NewReader([]byte(input)),
		Writer:                output,
		AllowedFSCapabilities: caps,
	})
	return srv, output
}

func parseServerResponses(t *testing.T, raw []byte) []acp.JSONRPCResponse {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	resps := make([]acp.JSONRPCResponse, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var r acp.JSONRPCResponse
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatalf("parse response: %v", err)
		}
		resps = append(resps, r)
	}
	return resps
}

// ==============================
// Session Manager Tests
// ==============================

func TestCreateSession(t *testing.T) {
	t.Run("creates session with specified ID", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, "/tmp", "test-session-1")
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		sessions := sm.ListSessions("")
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		if sessions[0].SessionID != "test-session-1" {
			t.Errorf("expected session ID test-session-1, got %s", sessions[0].SessionID)
		}
		if sessions[0].CWD != "/tmp" {
			t.Errorf("expected CWD /tmp, got %s", sessions[0].CWD)
		}
	})

	t.Run("auto-generates session ID when empty", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, ".", "")
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		sessions := sm.ListSessions("")
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		if sessions[0].SessionID == "" {
			t.Error("expected auto-generated session ID, got empty")
		}
	})

	t.Run("replaces existing session with same ID", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, "/tmp", "dup")
		if err != nil {
			t.Fatalf("first CreateSession failed: %v", err)
		}
		_, err = sm.CreateSession(ctx, "/home", "dup")
		if err != nil {
			t.Fatalf("second CreateSession failed: %v", err)
		}

		sessions := sm.ListSessions("")
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session after replacement, got %d", len(sessions))
		}
		if sessions[0].CWD != "/home" {
			t.Errorf("expected CWD /home after replacement, got %s", sessions[0].CWD)
		}
	})

	t.Run("defaults cwd to current dir when empty", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, "", "cwd-empty")
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		sessions := sm.ListSessions("")
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		abs, _ := filepath.Abs(".")
		if sessions[0].CWD != abs {
			t.Errorf("expected CWD %s, got %s", abs, sessions[0].CWD)
		}
	})

	t.Run("uses factory default model and mode", func(t *testing.T) {
		factory := &stubAgentFactory{
			defaultModel: "gpt-4",
			defaultMode:  "primary",
		}
		sm := newTestManager(t, factory, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, ".", "model-test")
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		if factory.createCount != 1 {
			t.Errorf("expected 1 agent creation, got %d", factory.createCount)
		}
	})

	t.Run("fails when agent factory errors", func(t *testing.T) {
		factory := &stubAgentFactory{createError: io.ErrUnexpectedEOF}
		sm := newTestManager(t, factory, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, ".", "")
		if err == nil {
			t.Fatal("expected error from CreateSession")
		}
	})
}

func TestGetSession(t *testing.T) {
	t.Run("returns session for existing ID", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, ".", "existing")
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		state := sm.GetSession("existing")
		if state == nil {
			t.Fatal("expected non-nil session state")
		}
	})

	t.Run("returns nil for non-existent ID", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		state := sm.GetSession("nonexistent")
		if state != nil {
			t.Fatal("expected nil for non-existent session")
		}
	})

	t.Run("returns nil after cleanup", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		sm.CreateSession(ctx, ".", "to-clean")
		sm.Cleanup()

		state := sm.GetSession("to-clean")
		if state != nil {
			t.Fatal("expected nil after cleanup")
		}
	})
}

func TestForkSession(t *testing.T) {
	t.Run("forks from existing session", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, "/tmp", "original")
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		_, err = sm.ForkSession(ctx, "original", "/home")
		if err != nil {
			t.Fatalf("ForkSession failed: %v", err)
		}

		sessions := sm.ListSessions("")
		if len(sessions) != 2 {
			t.Fatalf("expected 2 sessions after fork, got %d", len(sessions))
		}

		var forked *acp.SessionInfo
		for i := range sessions {
			if sessions[i].SessionID != "original" {
				forked = &sessions[i]
				break
			}
		}
		if forked == nil {
			t.Fatal("expected forked session to have different ID")
		}
		if forked.SessionID == "" {
			t.Error("forked session ID should not be empty")
		}
	})

	t.Run("returns error when original session does not exist", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.ForkSession(ctx, "nonexistent", "/tmp")
		if err == nil {
			t.Fatal("expected error when forking non-existent session")
		}
	})
}

func TestRestoreSession(t *testing.T) {
	t.Run("returns existing in-memory session", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, ".", "mem-sess")
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		state, err := sm.RestoreSession(ctx, "mem-sess")
		if err != nil {
			t.Fatalf("RestoreSession failed: %v", err)
		}
		if state == nil {
			t.Fatal("expected non-nil session from restore")
		}
	})

	t.Run("restores from session store when not in memory", func(t *testing.T) {
		store := &stubSessionStore{}
		store.SaveSessionMeta(acp.SessionMeta{
			SessionID: "stored-sess",
			CWD:       "/tmp",
			Model:     "gpt-4",
			Mode:      "primary",
		})

		factory := &stubAgentFactory{
			defaultModel: "test-model",
			defaultMode:  "test-mode",
		}
		sm := newTestManager(t, factory, store)
		ctx := context.Background()

		state, err := sm.RestoreSession(ctx, "stored-sess")
		if err != nil {
			t.Fatalf("RestoreSession failed: %v", err)
		}
		if state == nil {
			t.Fatal("expected non-nil session")
		}

		same := sm.GetSession("stored-sess")
		if same == nil {
			t.Fatal("session should be accessible via GetSession after restore")
		}
	})

	t.Run("returns error when session is not stored", func(t *testing.T) {
		sm := newTestManager(t, nil, &stubSessionStore{})
		ctx := context.Background()

		_, err := sm.RestoreSession(ctx, "not-stored")
		if err == nil {
			t.Fatal("expected error when restoring non-existent session")
		}
	})
}

func TestLoadPersistedSessions(t *testing.T) {
	t.Run("loads persisted sessions from store on init", func(t *testing.T) {
		store := &stubSessionStore{}
		store.SaveSessionMeta(acp.SessionMeta{
			SessionID: "persisted-1",
			CWD:       "/tmp",
			Model:     "gpt-4",
			Mode:      "primary",
		})
		store.SaveSessionMeta(acp.SessionMeta{
			SessionID: "persisted-2",
			CWD:       "/home",
			Model:     "gpt-4",
			Mode:      "primary",
		})

		factory := &stubAgentFactory{}
		sm := newTestManager(t, factory, store)

		sessions := sm.ListSessions("")
		if len(sessions) != 2 {
			t.Fatalf("expected 2 persisted sessions loaded, got %d", len(sessions))
		}

		ids := make(map[string]bool)
		for _, s := range sessions {
			ids[s.SessionID] = true
		}
		if !ids["persisted-1"] {
			t.Error("persisted-1 not found in loaded sessions")
		}
		if !ids["persisted-2"] {
			t.Error("persisted-2 not found in loaded sessions")
		}
	})

	t.Run("returns empty when store has no sessions", func(t *testing.T) {
		sm := newTestManager(t, nil, &stubSessionStore{})

		sessions := sm.ListSessions("")
		if len(sessions) != 0 {
			t.Errorf("expected 0 sessions from empty store, got %d", len(sessions))
		}
	})

	t.Run("counts agent creations correctly", func(t *testing.T) {
		store := &stubSessionStore{}
		store.SaveSessionMeta(acp.SessionMeta{
			SessionID: "valid-1",
			CWD:       "/tmp",
			Model:     "good-model",
			Mode:      "primary",
		})
		store.SaveSessionMeta(acp.SessionMeta{
			SessionID: "valid-2",
			CWD:       "/home",
			Model:     "good-model",
			Mode:      "primary",
		})

		factory := &stubAgentFactory{}
		_ = newTestManager(t, factory, store)

		if factory.createCount != 2 {
			t.Errorf("expected 2 agent creations (both persisted), got %d", factory.createCount)
		}
	})
}

func TestUpdateCWD(t *testing.T) {
	t.Run("updates existing session", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, "/tmp", "updatable")
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		state := sm.UpdateCWD("updatable", "/new/path")
		if state == nil {
			t.Fatal("expected non-nil session state")
		}

		sessions := sm.ListSessions("")
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessions))
		}
		if sessions[0].CWD != "/new/path" {
			t.Errorf("expected CWD /new/path, got %s", sessions[0].CWD)
		}
	})

	t.Run("returns nil for non-existent session", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		state := sm.UpdateCWD("nonexistent", "/tmp")
		if state != nil {
			t.Fatal("expected nil for non-existent session")
		}
	})
}

func TestUpdateModel(t *testing.T) {
	t.Run("updates existing session", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, ".", "model-upd")
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		if err := sm.UpdateModel("model-upd", "gpt-5"); err != nil {
			t.Fatalf("UpdateModel failed: %v", err)
		}
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		err := sm.UpdateModel("nonexistent", "gpt-5")
		if err == nil {
			t.Fatal("expected error for non-existent session")
		}
	})
}

func TestUpdateMode(t *testing.T) {
	t.Run("updates existing session", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, ".", "mode-upd")
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		if err := sm.UpdateMode("mode-upd", "architect"); err != nil {
			t.Fatalf("UpdateMode failed: %v", err)
		}
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		err := sm.UpdateMode("nonexistent", "architect")
		if err == nil {
			t.Fatal("expected error for non-existent session")
		}
	})
}

func TestListSessions(t *testing.T) {
	t.Run("lists all sessions when cwd is empty", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		sm.CreateSession(ctx, "/tmp", "s1")
		sm.CreateSession(ctx, "/home", "s2")

		sessions := sm.ListSessions("")
		if len(sessions) != 2 {
			t.Fatalf("expected 2 sessions, got %d", len(sessions))
		}
	})

	t.Run("filters by cwd", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		sm.CreateSession(ctx, "/tmp", "s1")
		sm.CreateSession(ctx, "/home", "s2")

		sessions := sm.ListSessions("/tmp")
		if len(sessions) != 1 {
			t.Fatalf("expected 1 session with CWD /tmp, got %d", len(sessions))
		}
		if sessions[0].SessionID != "s1" {
			t.Errorf("expected s1, got %s", sessions[0].SessionID)
		}
	})

	t.Run("returns empty when no sessions match cwd", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		sm.CreateSession(ctx, "/tmp", "s1")

		sessions := sm.ListSessions("/nonexistent")
		if len(sessions) != 0 {
			t.Errorf("expected 0 sessions, got %d", len(sessions))
		}
	})

	t.Run("returns empty when no sessions exist", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		sessions := sm.ListSessions("")
		if len(sessions) != 0 {
			t.Errorf("expected 0 sessions, got %d", len(sessions))
		}
	})
}

func TestSetRunningAndIdle(t *testing.T) {
	t.Run("SetRunning and SetIdle do not panic on missing session", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)

		sm.SetRunning("missing", nil)
		sm.SetIdle("missing")
	})

	t.Run("SetRunning and SetIdle work on existing session", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		_, err := sm.CreateSession(ctx, ".", "running-test")
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		runningCtx, cancel := context.WithCancel(context.Background())
		sm.SetRunning("running-test", cancel)
		_ = runningCtx
		sm.SetIdle("running-test")
		cancel()
	})
}

func TestCleanup(t *testing.T) {
	t.Run("removes all sessions", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		ctx := context.Background()

		sm.CreateSession(ctx, "/tmp", "s1")
		sm.CreateSession(ctx, "/home", "s2")
		sm.CreateSession(ctx, "/var", "s3")

		sm.Cleanup()

		sessions := sm.ListSessions("")
		if len(sessions) != 0 {
			t.Errorf("expected 0 sessions after cleanup, got %d", len(sessions))
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		sm := newTestManager(t, nil, nil)
		sm.Cleanup()
		sm.Cleanup()
	})
}

// ==============================
// Server Tests (via JSON-RPC over bytes.Buffer)
// ==============================

func TestServerInitialize(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"
	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %s", resps[0].Error.Message)
	}

	var initResult acp.InitializeResult
	if err := json.Unmarshal(resps[0].Result, &initResult); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if initResult.ProtocolVersion != 1 {
		t.Errorf("expected ProtocolVersion 1, got %d", initResult.ProtocolVersion)
	}
	if !initResult.AgentCapabilities.LoadSession {
		t.Error("expected LoadSession capability to be true")
	}
	if initResult.AgentCapabilities.PromptCapabilities == nil {
		t.Error("expected PromptCapabilities")
	}
	if initResult.AgentCapabilities.SessionCapabilities == nil {
		t.Error("expected SessionCapabilities")
	}
}

func TestServerInitializeWithClientInfo(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientInfo":{"name":"zed","version":"1.0"}}}` + "\n"
	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %s", resps[0].Error.Message)
	}
}

func TestServerNewSession(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/tmp"}}` + "\n"
	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}

	if resps[1].Error != nil {
		t.Fatalf("unexpected error on session/new: %s", resps[1].Error.Message)
	}

	var newSess acp.NewSessionResult
	if err := json.Unmarshal(resps[1].Result, &newSess); err != nil {
		t.Fatalf("unmarshal new session result: %v", err)
	}
	if newSess.SessionID == "" {
		t.Error("expected non-empty session ID")
	}
	if newSess.Models == nil {
		t.Error("expected Models in session result")
	}
	if newSess.Modes == nil {
		t.Error("expected Modes in session result")
	}
}

func TestServerNewSessionDefaultCWD(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"session/new"}` + "\n"
	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}

	if resps[1].Error != nil {
		t.Fatalf("unexpected error: %s", resps[1].Error.Message)
	}

	var newSess acp.NewSessionResult
	if err := json.Unmarshal(resps[1].Result, &newSess); err != nil {
		t.Fatalf("unmarshal new session result: %v", err)
	}
	if newSess.SessionID == "" {
		t.Error("expected non-empty session ID")
	}
}

func TestServerSessionNotFound(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"session/load","params":{"sessionId":"nonexistent","cwd":"/tmp"}}` + "\n"

	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error == nil {
		t.Fatal("expected error for non-existent session")
	}
	if resps[0].Error.Code != -32002 {
		t.Errorf("expected error code -32002, got %d", resps[0].Error.Code)
	}
}

func TestServerListSessions(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/tmp"}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"session/list"}` + "\n"
	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(resps))
	}

	if resps[2].Error != nil {
		t.Fatalf("unexpected error on session/list: %s", resps[2].Error.Message)
	}

	var listResult acp.ListSessionsResult
	if err := json.Unmarshal(resps[2].Result, &listResult); err != nil {
		t.Fatalf("unmarshal list result: %v", err)
	}
	if len(listResult.Sessions) != 1 {
		t.Fatalf("expected 1 session in list, got %d", len(listResult.Sessions))
	}
	if listResult.Sessions[0].CWD != "/tmp" {
		t.Errorf("expected CWD /tmp, got %s", listResult.Sessions[0].CWD)
	}
}

func TestServerCancelSession(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/tmp"}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"session/cancel","params":{"sessionId":"cancel-me"}}` + "\n"
	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(resps))
	}

	if resps[2].Error != nil {
		t.Fatalf("unexpected error on session/cancel: %s", resps[2].Error.Message)
	}
}

func TestServerFSCapabilitiesRejected(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientCapabilities":{"fs":{"readTextFile":true}}}}` + "\n"

	factory := &stubAgentFactory{}
	sm := newTestManager(t, factory, nil)
	output := &bytes.Buffer{}
	srv := acp.NewServer(acp.ServerConfig{
		SessionManager:        sm,
		AgentInfo:             acp.AgentInfo{Name: "test", Version: "1.0"},
		Reader:                bytes.NewReader([]byte(input)),
		Writer:                output,
		AllowedFSCapabilities: nil,
	})

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error == nil {
		t.Fatal("expected error when FS caps are rejected")
	}
	if !strings.Contains(resps[0].Error.Message, "not allowed") &&
		!strings.Contains(resps[0].Error.Message, "rejected") {
		t.Errorf("expected rejection error, got: %s", resps[0].Error.Message)
	}
}

func TestServerFSCapabilitiesAccepted(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientCapabilities":{"fs":{"readTextFile":true}}}}` + "\n"

	factory := &stubAgentFactory{}
	sm := newTestManager(t, factory, nil)
	output := &bytes.Buffer{}
	srv := acp.NewServer(acp.ServerConfig{
		SessionManager: sm,
		AgentInfo:      acp.AgentInfo{Name: "test", Version: "1.0"},
		Reader:         bytes.NewReader([]byte(input)),
		Writer:         output,
		AllowedFSCapabilities: map[string]bool{
			"FS.ReadTextFile": true,
		},
	})

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %s", resps[0].Error.Message)
	}
}

func TestServerInvalidJSONRPC(t *testing.T) {
	input := `{"jsonrpc":"1.0","id":1,"method":"initialize"}` + "\n"
	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error == nil {
		t.Fatal("expected error for invalid jsonrpc version")
	}
	if resps[0].Error.Code != -32600 {
		t.Errorf("expected error code -32600, got %d", resps[0].Error.Code)
	}
}

func TestServerParseError(t *testing.T) {
	input := `not json at all` + "\n"
	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error == nil {
		t.Fatal("expected parse error")
	}
	if resps[0].Error.Code != -32700 {
		t.Errorf("expected error code -32700, got %d", resps[0].Error.Code)
	}
}

func TestServerMethodNotFound(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"unknown_method"}` + "\n"
	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error == nil {
		t.Fatal("expected method not found error")
	}
	if resps[0].Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", resps[0].Error.Code)
	}
}

func TestServerResumeSession(t *testing.T) {
	store := &stubSessionStore{}
	store.SaveSessionMeta(acp.SessionMeta{
		SessionID: "resume-me",
		CWD:       "/tmp",
		Model:     "test-model",
		Mode:      "test-mode",
	})

	input := `{"jsonrpc":"2.0","id":1,"method":"session/resume","params":{"sessionId":"resume-me","cwd":"/tmp"}}` + "\n"

	factory := &stubAgentFactory{}
	sm := newTestManager(t, factory, store)
	output := &bytes.Buffer{}
	srv := acp.NewServer(acp.ServerConfig{
		SessionManager: sm,
		AgentInfo:      acp.AgentInfo{Name: "test", Version: "1.0"},
		Reader:         bytes.NewReader([]byte(input)),
		Writer:         output,
	})

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %s", resps[0].Error.Message)
	}

	var resumeResult acp.ResumeSessionResult
	if err := json.Unmarshal(resps[0].Result, &resumeResult); err != nil {
		t.Fatalf("unmarshal resume result: %v", err)
	}
	if resumeResult.Models == nil {
		t.Error("expected Models in resume result")
	}
	if resumeResult.Modes == nil {
		t.Error("expected Modes in resume result")
	}
}

func TestServerResumeSessionFallback(t *testing.T) {
	store := &stubSessionStore{}

	input := `{"jsonrpc":"2.0","id":1,"method":"session/resume","params":{"sessionId":"unknown","cwd":"/tmp"}}` + "\n"

	factory := &stubAgentFactory{}
	sm := newTestManager(t, factory, store)
	output := &bytes.Buffer{}
	srv := acp.NewServer(acp.ServerConfig{
		SessionManager: sm,
		AgentInfo:      acp.AgentInfo{Name: "test", Version: "1.0"},
		Reader:         bytes.NewReader([]byte(input)),
		Writer:         output,
	})

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("resume fallback should create new session: %s", resps[0].Error.Message)
	}
}

func TestServerSetModeOnMissingSession(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"session/set_mode","params":{"sessionId":"absent","modeId":"architect"}}` + "\n"

	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error == nil {
		t.Fatal("expected error for non-existent session")
	}
	if resps[0].Error.Code != -32002 {
		t.Errorf("expected error code -32002, got %d", resps[0].Error.Code)
	}
}

func TestServerSetModelOnMissingSession(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"session/set_model","params":{"sessionId":"absent","modelId":"gpt-5"}}` + "\n"

	srv, output := newTestServer(t, input, nil)

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("Server.Run failed: %v", err)
	}

	resps := parseServerResponses(t, output.Bytes())
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error == nil {
		t.Fatal("expected error for non-existent session")
	}
	if resps[0].Error.Code != -32002 {
		t.Errorf("expected error code -32002, got %d", resps[0].Error.Code)
	}
}

// ==============================
// Protocol / Utility Tests
// ==============================

func TestToolKind(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"read_file", "read"},
		{"write_file", "edit"},
		{"patch", "edit"},
		{"search_files", "search"},
		{"terminal", "execute"},
		{"process", "execute"},
		{"execute_code", "execute"},
		{"web_search", "fetch"},
		{"web_extract", "fetch"},
		{"browser_navigate", "fetch"},
		{"browser_click", "execute"},
		{"browser_type", "execute"},
		{"browser_snapshot", "read"},
		{"browser_vision", "read"},
		{"browser_get_images", "read"},
		{"browser_scroll", "execute"},
		{"browser_press", "execute"},
		{"browser_back", "execute"},
		{"_thinking", "think"},
		{"delegate_task", "execute"},
		{"vision_analyze", "read"},
		{"image_generate", "execute"},
		{"todo", "other"},
		{"skill_view", "read"},
		{"skills_list", "read"},
		{"skill_manage", "edit"},
		{"unknown_tool", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := acp.ToolKind(tt.name)
			if got != tt.want {
				t.Errorf("ToolKind(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestBuildToolTitle(t *testing.T) {
	t.Run("terminal truncates long commands", func(t *testing.T) {
		short := acp.BuildToolTitle("terminal", map[string]any{"command": "ls -la"})
		if short != "terminal: ls -la" {
			t.Errorf("expected 'terminal: ls -la', got %q", short)
		}

		longCmd := strings.Repeat("x", 100)
		long := acp.BuildToolTitle("terminal", map[string]any{"command": longCmd})
		if len(long) > 96 {
			t.Errorf("title too long: %d chars", len(long))
		}
		if !strings.Contains(long, "...") {
			t.Errorf("expected truncated title containing ..., got %q", long)
		}
	})

	t.Run("read_file shows path", func(t *testing.T) {
		title := acp.BuildToolTitle("read_file", map[string]any{"path": "/foo/bar.go"})
		if title != "read: /foo/bar.go" {
			t.Errorf("expected 'read: /foo/bar.go', got %q", title)
		}
	})

	t.Run("write_file shows path", func(t *testing.T) {
		title := acp.BuildToolTitle("write_file", map[string]any{"path": "/foo/bar.go"})
		if title != "write: /foo/bar.go" {
			t.Errorf("expected 'write: /foo/bar.go', got %q", title)
		}
	})

	t.Run("web_search shows query", func(t *testing.T) {
		title := acp.BuildToolTitle("web_search", map[string]any{"query": "golang testing"})
		if title != "web search: golang testing" {
			t.Errorf("expected 'web search: golang testing', got %q", title)
		}
	})

	t.Run("web_extract with single URL", func(t *testing.T) {
		title := acp.BuildToolTitle("web_extract", map[string]any{"urls": []any{"https://example.com"}})
		if title != "extract: https://example.com" {
			t.Errorf("expected 'extract: https://example.com', got %q", title)
		}
	})

	t.Run("web_extract with multiple URLs", func(t *testing.T) {
		title := acp.BuildToolTitle("web_extract", map[string]any{"urls": []any{"https://example.com", "https://other.com"}})
		expected := "extract: https://example.com (+1)"
		if title != expected {
			t.Errorf("expected %q, got %q", expected, title)
		}
	})

	t.Run("web_extract with empty urls", func(t *testing.T) {
		title := acp.BuildToolTitle("web_extract", map[string]any{})
		if title != "web extract" {
			t.Errorf("expected 'web extract', got %q", title)
		}
	})

	t.Run("delegate_task shows goal", func(t *testing.T) {
		title := acp.BuildToolTitle("delegate_task", map[string]any{"goal": "analyze this code"})
		if title != "delegate: analyze this code" {
			t.Errorf("expected 'delegate: analyze this code', got %q", title)
		}
	})

	t.Run("delegate_task truncates long goal", func(t *testing.T) {
		longGoal := strings.Repeat("x", 100)
		title := acp.BuildToolTitle("delegate_task", map[string]any{"goal": longGoal})
		if len(title) > 76 {
			t.Errorf("title too long: %d chars", len(title))
		}
		if !strings.Contains(title, "...") {
			t.Errorf("expected truncated title containing ..., got %q", title)
		}
	})

	t.Run("delegate_task with empty goal", func(t *testing.T) {
		title := acp.BuildToolTitle("delegate_task", map[string]any{"goal": ""})
		if title != "delegate task" {
			t.Errorf("expected 'delegate task', got %q", title)
		}
	})

	t.Run("execute_code shows first non-empty line", func(t *testing.T) {
		title := acp.BuildToolTitle("execute_code", map[string]any{"code": "print('hello world')"})
		if title != "python: print('hello world')" {
			t.Errorf("expected 'python: print(hello world)', got %q", title)
		}
	})

	t.Run("execute_code with empty code", func(t *testing.T) {
		title := acp.BuildToolTitle("execute_code", map[string]any{})
		if title != "python code" {
			t.Errorf("expected 'python code', got %q", title)
		}
	})

	t.Run("browser_navigate shows URL", func(t *testing.T) {
		title := acp.BuildToolTitle("browser_navigate", map[string]any{"url": "https://example.com"})
		if title != "navigate: https://example.com" {
			t.Errorf("expected 'navigate: https://example.com', got %q", title)
		}
	})

	t.Run("patch shows mode and path", func(t *testing.T) {
		title := acp.BuildToolTitle("patch", map[string]any{"mode": "replace", "path": "/foo.go"})
		if title != "patch (replace): /foo.go" {
			t.Errorf("expected 'patch (replace): /foo.go', got %q", title)
		}
	})

	t.Run("unknown tool returns name", func(t *testing.T) {
		title := acp.BuildToolTitle("custom_tool", map[string]any{})
		if title != "custom_tool" {
			t.Errorf("expected 'custom_tool', got %q", title)
		}
	})

	t.Run("vision_analyze shows question", func(t *testing.T) {
		title := acp.BuildToolTitle("vision_analyze", map[string]any{"question": "What is in this image?"})
		if title != "analyze image: What is in this image?" {
			t.Errorf("expected 'analyze image: What is in this image?', got %q", title)
		}
	})

	t.Run("image_generate uses prompt or description", func(t *testing.T) {
		title := acp.BuildToolTitle("image_generate", map[string]any{"prompt": "a cat"})
		if title != "generate image: a cat" {
			t.Errorf("expected 'generate image: a cat', got %q", title)
		}

		title2 := acp.BuildToolTitle("image_generate", map[string]any{"description": "a dog"})
		if title2 != "generate image: a dog" {
			t.Errorf("expected 'generate image: a dog', got %q", title2)
		}

		title3 := acp.BuildToolTitle("image_generate", map[string]any{})
		if title3 != "generate image" {
			t.Errorf("expected 'generate image', got %q", title3)
		}
	})

	t.Run("browser_snapshot", func(t *testing.T) {
		title := acp.BuildToolTitle("browser_snapshot", map[string]any{})
		if title != "browser snapshot" {
			t.Errorf("expected 'browser snapshot', got %q", title)
		}
	})

	t.Run("browser_get_images", func(t *testing.T) {
		title := acp.BuildToolTitle("browser_get_images", map[string]any{})
		if title != "browser images" {
			t.Errorf("expected 'browser images', got %q", title)
		}
	})
}

func TestDefaultPermissionOptions(t *testing.T) {
	opts := acp.DefaultPermissionOptions()
	if len(opts) != 3 {
		t.Fatalf("expected 3 options, got %d", len(opts))
	}

	expected := []struct {
		id   string
		name string
	}{
		{"allow_once", "Allow"},
		{"allow_always", "Always allow"},
		{"reject_once", "Reject"},
	}
	for i, exp := range expected {
		if opts[i].OptionID != exp.id {
			t.Errorf("option %d: expected OptionID %s, got %s", i, exp.id, opts[i].OptionID)
		}
		if opts[i].Name != exp.name {
			t.Errorf("option %d: expected Name %s, got %s", i, exp.name, opts[i].Name)
		}
	}
}

func TestSessionStoreRoundTrip(t *testing.T) {
	store := &stubSessionStore{}
	meta := acp.SessionMeta{
		SessionID: "rt-1",
		CWD:       "/tmp",
		Model:     "gpt-4",
		Mode:      "primary",
		Title:     "test session",
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err := store.SaveSessionMeta(meta); err != nil {
		t.Fatalf("SaveSessionMeta failed: %v", err)
	}

	loaded, err := store.LoadSessionMeta("rt-1")
	if err != nil {
		t.Fatalf("LoadSessionMeta failed: %v", err)
	}
	if loaded.SessionID != "rt-1" || loaded.CWD != "/tmp" {
		t.Errorf("loaded meta does not match: %+v", loaded)
	}

	list := store.ListSessions("")
	if len(list) != 1 {
		t.Errorf("expected 1 session in list, got %d", len(list))
	}
}

func TestSessionStoreFilterByCWD(t *testing.T) {
	store := &stubSessionStore{}
	store.SaveSessionMeta(acp.SessionMeta{SessionID: "s1", CWD: "/tmp"})
	store.SaveSessionMeta(acp.SessionMeta{SessionID: "s2", CWD: "/home"})

	list := store.ListSessions("/tmp")
	if len(list) != 1 || list[0].SessionID != "s1" {
		t.Errorf("expected [s1], got %+v", list)
	}
}

func TestEnsureHomeDir(t *testing.T) {
	t.Run("creates directory with given path", func(t *testing.T) {
		tmpDir := t.TempDir()
		homeDir := filepath.Join(tmpDir, ".acp-test")

		result, err := acp.EnsureHomeDir(homeDir)
		if err != nil {
			t.Fatalf("EnsureHomeDir failed: %v", err)
		}
		if result != homeDir {
			t.Errorf("expected %s, got %s", homeDir, result)
		}
		if _, err := os.Stat(homeDir); os.IsNotExist(err) {
			t.Error("directory was not created")
		}
	})

	t.Run("returns existing path without error", func(t *testing.T) {
		tmpDir := t.TempDir()

		result, err := acp.EnsureHomeDir(tmpDir)
		if err != nil {
			t.Fatalf("EnsureHomeDir failed: %v", err)
		}
		if result != tmpDir {
			t.Errorf("expected %s, got %s", tmpDir, result)
		}
	})
}

// ==============================
// Edge Case: Concurrent Access
// ==============================

func TestSessionManagerConcurrentCreate(t *testing.T) {
	sm := newTestManager(t, nil, nil)
	ctx := context.Background()
	const numSessions = 30

	var wg sync.WaitGroup
	wg.Add(numSessions)

	for i := 0; i < numSessions; i++ {
		i := i // capture
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("conc-%d", i)
			_, err := sm.CreateSession(ctx, "/tmp", id)
			if err != nil {
				t.Errorf("CreateSession(%q) failed: %v", id, err)
			}
			sm.GetSession(id)
		}()
	}

	wg.Wait()

	sessions := sm.ListSessions("")
	if len(sessions) != numSessions {
		t.Errorf("expected %d sessions, got %d", numSessions, len(sessions))
	}

	sm.Cleanup()
	sessions = sm.ListSessions("")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after cleanup, got %d", len(sessions))
	}
}
