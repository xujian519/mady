package sqlite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestEventStore(t *testing.T) *SQLEventStore {
	t.Helper()
	dir := t.TempDir()
	s, err := NewEventStore(filepath.Join(dir, "events.db"))
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestEventStore_AppendAndListBySession verifies the core write + read path.
func TestEventStore_AppendAndListBySession(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()

	id, err := s.Append(ctx, "agent_start", "session_1", "patent", json.RawMessage(`{"x":1}`))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if id <= 0 {
		t.Fatalf("Append id = %d, want > 0", id)
	}
	if _, err := s.Append(ctx, "agent_end", "session_1", "patent", json.RawMessage(`{"x":2}`)); err != nil {
		t.Fatalf("Append 2: %v", err)
	}
	// A different session must not leak into session_1.
	if _, err := s.Append(ctx, "agent_start", "session_2", "legal", json.RawMessage(`{"x":3}`)); err != nil {
		t.Fatalf("Append 3: %v", err)
	}

	recs, err := s.ListBySession(ctx, "session_1", 10)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("ListBySession len = %d, want 2", len(recs))
	}
	if recs[0].EventType != "agent_end" {
		t.Errorf("newest first: got %q", recs[0].EventType)
	}
	if recs[0].AgentName != "patent" || recs[0].SessionID != "session_1" {
		t.Errorf("unexpected record: %+v", recs[0])
	}
	if string(recs[0].Payload) != `{"x":2}` {
		t.Errorf("payload = %q", string(recs[0].Payload))
	}
	if recs[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

// TestEventStore_ListByType verifies filtering by event type.
func TestEventStore_ListByType(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()

	for _, et := range []string{"agent_start", "agent_end", "agent_start"} {
		if _, err := s.Append(ctx, et, "session_1", "patent", json.RawMessage(`{}`)); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	recs, err := s.ListByType(ctx, "agent_start", 10)
	if err != nil {
		t.Fatalf("ListByType: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("ListByType len = %d, want 2", len(recs))
	}
}

// TestEventStore_Prune verifies pruning deletes old events and keeps recent ones.
func TestEventStore_Prune(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()

	if _, err := s.Append(ctx, "agent_start", "session_1", "patent", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Prune everything before now+1h → deletes all.
	n, err := s.Prune(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("Prune deleted %d, want 1", n)
	}
	recs, err := s.ListBySession(ctx, "session_1", 10)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected empty after prune, got %d", len(recs))
	}
}

// TestEventStore_InterfaceContract verifies the store.CaseStore contract
// methods (CaseID / RunID / Version / Migrate).
func TestEventStore_InterfaceContract(t *testing.T) {
	s := newTestEventStore(t)
	ctx := context.Background()

	if s.CaseID() != "" {
		t.Errorf("CaseID = %q, want empty", s.CaseID())
	}
	if s.RunID() != "" {
		t.Errorf("RunID = %q, want empty", s.RunID())
	}
	if s.Version() != 1 {
		t.Errorf("Version = %d, want 1", s.Version())
	}
	v, err := s.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if v != 1 {
		t.Errorf("Migrate version = %d, want 1", v)
	}
}

// TestEventStore_ReopenPersists verifies events survive close + reopen.
func TestEventStore_ReopenPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")

	s, err := NewEventStore(path)
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	if _, err := s.Append(context.Background(), "agent_start", "session_1", "patent", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen the same file.
	s2, err := NewEventStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	recs, err := s2.ListBySession(context.Background(), "session_1", 10)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 persisted event, got %d", len(recs))
	}
}

// TestEventStore_FileCreated verifies the DB file is actually created on disk.
func TestEventStore_FileCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")
	s, err := NewEventStore(path)
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	defer s.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected db file to exist: %v", err)
	}
}
