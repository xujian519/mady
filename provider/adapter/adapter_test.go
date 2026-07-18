package adapter

import (
	"context"
	"strings"
	"testing"
)

// mockAdapter implements AgentAdapter for testing.
type mockAdapter struct {
	name         string
	desc         string
	available    bool
	canEdit      bool
	maxTokens    int
	models       []string
	detectError  error
	spawnSession AgentSession
	spawnError   error
}

func (m mockAdapter) Name() string                           { return m.name }
func (m mockAdapter) Description() string                    { return m.desc }
func (m mockAdapter) Detect(_ context.Context) (bool, error) { return m.available, m.detectError }
func (m mockAdapter) Capabilities() AgentCapabilities {
	return AgentCapabilities{
		CanEdit:          m.canEdit,
		MaxContextTokens: m.maxTokens,
		Models:           m.models,
	}
}
func (m mockAdapter) Spawn(_ context.Context, _ SpawnConfig) (AgentSession, error) {
	return m.spawnSession, m.spawnError
}

// mockSession implements AgentSession for testing.
type mockSession struct {
	output string
	err    error
	closed bool
}

func (s *mockSession) Send(_ context.Context, _ string) (string, error) { return s.output, s.err }
func (s *mockSession) Stream(ctx context.Context, _ string) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{Content: s.output, Done: true, Error: s.err}
	close(ch)
	return ch, nil
}
func (s *mockSession) Close() error { s.closed = true; return nil }

func TestRegisterAndLookupAdapter(t *testing.T) {
	// Verify built-in adapters are registered.
	for _, name := range []string{"claude", "codex"} {
		a := LookupAdapter(name)
		if a == nil {
			t.Fatalf("adapter %q not found", name)
		}
		if a.Name() != name {
			t.Fatalf("adapter %q: Name() = %q", name, a.Name())
		}
	}
}

func TestListAdapters(t *testing.T) {
	all := ListAdapters()
	if len(all) < 2 {
		t.Fatalf("expected at least 2 adapters, got %d", len(all))
	}
	names := map[string]bool{}
	for _, a := range all {
		names[a.Name()] = true
	}
	if !names["claude"] || !names["codex"] {
		t.Fatalf("missing expected adapters: %v", names)
	}
}

func TestDetectAll(t *testing.T) {
	// Register a mock adapter to test DetectAll.
	mock := mockAdapter{name: "test-detect", desc: "test", available: true}
	defer func() {
		adapterMu.Lock()
		delete(adapters, "test-detect")
		adapterMu.Unlock()
	}()
	RegisterAdapter(mock)

	results := DetectAll(context.Background())
	if r, ok := results["test-detect"]; !ok {
		t.Fatal("test-detect missing from DetectAll")
	} else if !r.Available {
		t.Fatal("expected available=true")
	}
}

func TestAdapterIndex(t *testing.T) {
	mock := mockAdapter{name: "test-idx", desc: "test idx adapter", available: true}
	defer func() {
		adapterMu.Lock()
		delete(adapters, "test-idx")
		adapterMu.Unlock()
	}()
	RegisterAdapter(mock)

	idx := AdapterIndex(context.Background())
	if !strings.Contains(idx, "test-idx") {
		t.Fatalf("AdapterIndex missing test-idx: %s", idx)
	}
	if !strings.Contains(idx, "test idx adapter") {
		t.Fatalf("AdapterIndex missing description: %s", idx)
	}
}

func TestAdapterCapabilities(t *testing.T) {
	claude := LookupAdapter("claude")
	if claude == nil {
		t.Fatal("claude not found")
	}
	caps := claude.Capabilities()
	if !caps.CanEdit {
		t.Error("claude should be able to edit")
	}
	if caps.MaxContextTokens < 100_000 {
		t.Errorf("claude context too small: %d", caps.MaxContextTokens)
	}
	if len(caps.Models) == 0 {
		t.Error("claude should have models")
	}

	codex := LookupAdapter("codex")
	if codex == nil {
		t.Fatal("codex not found")
	}
	caps2 := codex.Capabilities()
	if len(caps2.Models) == 0 {
		t.Error("codex should have models")
	}
}

func TestMockSession(t *testing.T) {
	sess := &mockSession{output: "hello world"}
	out, err := sess.Send(context.Background(), "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello world" {
		t.Fatalf("output = %q", out)
	}

	ch, err := sess.Stream(context.Background(), "prompt")
	if err != nil {
		t.Fatal(err)
	}
	chunk := <-ch
	if chunk.Content != "hello world" {
		t.Fatalf("stream chunk = %q", chunk.Content)
	}
	if !chunk.Done {
		t.Fatal("expected done=true")
	}
	if _, ok := <-ch; ok {
		t.Fatal("channel should be closed")
	}

	sess.Close()
	if !sess.closed {
		t.Fatal("session should be closed")
	}
}

func TestMockAdapter(t *testing.T) {
	sess := &mockSession{output: "result"}
	mock := mockAdapter{
		name:         "test-mock",
		desc:         "test mock adapter",
		available:    true,
		canEdit:      true,
		maxTokens:    4096,
		models:       []string{"test-model"},
		spawnSession: sess,
	}

	if mock.Name() != "test-mock" {
		t.Fatalf("name = %q", mock.Name())
	}

	ok, err := mock.Detect(context.Background())
	if err != nil || !ok {
		t.Fatalf("Detect = %v, %v", ok, err)
	}

	caps := mock.Capabilities()
	if caps.MaxContextTokens != 4096 {
		t.Fatalf("tokens = %d", caps.MaxContextTokens)
	}

	spawned, err := mock.Spawn(context.Background(), SpawnConfig{})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := spawned.Send(context.Background(), "test")
	if out != "result" {
		t.Fatalf("spawn output = %q", out)
	}
}
