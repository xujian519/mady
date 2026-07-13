package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/domains/reasoning"
)

func testStore(t *testing.T) *SQLiteCheckpointStore {
	t.Helper()
	dir := t.TempDir()
	store, err := NewCheckpointStore(filepath.Join(dir, "checkpoints.db"))
	if err != nil {
		t.Fatalf("NewCheckpointStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func makeCheckpoint(id, caseID string) *reasoning.StageCheckpoint {
	return &reasoning.StageCheckpoint{
		CheckpointID: id,
		CaseID:       caseID,
		CaseType:     reasoning.CasePatentability,
		CurrentStage: 3,
		Blackboard:   reasoning.NewFactBlackboard(caseID, reasoning.CasePatentability, "G06F"),
		Metadata:     map[string]any{"workflow_id": "wf-1"},
	}
}

func TestSaveAndLoad(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	cp := makeCheckpoint("cp-1", "case-1")
	if err := s.Save(ctx, cp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := s.Load(ctx, "cp-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.CheckpointID != "cp-1" {
		t.Fatalf("CheckpointID: got %q", loaded.CheckpointID)
	}
	if loaded.CaseID != "case-1" {
		t.Fatalf("CaseID: got %q", loaded.CaseID)
	}
	if loaded.CurrentStage != 3 {
		t.Fatalf("CurrentStage: got %d", loaded.CurrentStage)
	}
	if loaded.Blackboard == nil {
		t.Fatal("Blackboard should not be nil")
	}
	if loaded.Blackboard.CaseID != "case-1" {
		t.Fatalf("Blackboard.CaseID: got %q", loaded.Blackboard.CaseID)
	}
}

func TestLoadNotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_, err := s.Load(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent checkpoint")
	}
}

func TestDelete(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	cp := makeCheckpoint("cp-del", "case-del")
	s.Save(ctx, cp)

	if err := s.Delete(ctx, "cp-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Load(ctx, "cp-del")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestSaveReplace(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	cp1 := makeCheckpoint("cp-rep", "case-1")
	cp1.CurrentStage = 2
	s.Save(ctx, cp1)

	cp2 := makeCheckpoint("cp-rep", "case-1")
	cp2.CurrentStage = 4
	s.Save(ctx, cp2)

	loaded, _ := s.Load(ctx, "cp-rep")
	if loaded.CurrentStage != 4 {
		t.Fatalf("CurrentStage after replace: got %d, want 4", loaded.CurrentStage)
	}
}

func TestListByCase(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	s.Save(ctx, makeCheckpoint("cp-a1", "case-A"))
	s.Save(ctx, makeCheckpoint("cp-a2", "case-A"))
	s.Save(ctx, makeCheckpoint("cp-b1", "case-B"))

	ids, err := s.ListByCase(ctx, "case-A")
	if err != nil {
		t.Fatalf("ListByCase: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 checkpoints for case-A, got %d", len(ids))
	}

	idsB, _ := s.ListByCase(ctx, "case-B")
	if len(idsB) != 1 {
		t.Fatalf("expected 1 checkpoint for case-B, got %d", len(idsB))
	}

	idsEmpty, _ := s.ListByCase(ctx, "case-C")
	if len(idsEmpty) != 0 {
		t.Fatalf("expected 0 checkpoints for case-C, got %d", len(idsEmpty))
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")
	ctx := context.Background()

	store1, err := NewCheckpointStore(dbPath)
	if err != nil {
		t.Fatalf("NewCheckpointStore: %v", err)
	}
	store1.Save(ctx, makeCheckpoint("cp-persist", "case-persist"))
	store1.Close()

	store2, err := NewCheckpointStore(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { store2.Close() })

	loaded, err := store2.Load(ctx, "cp-persist")
	if err != nil {
		t.Fatalf("Load after reopen: %v", err)
	}
	if loaded.CaseID != "case-persist" {
		t.Fatalf("CaseID after reopen: got %q", loaded.CaseID)
	}
}
