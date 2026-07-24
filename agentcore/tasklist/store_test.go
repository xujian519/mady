package tasklist

import (
	"context"
	"fmt"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// --- MemoryStore tests ---

func TestMemoryStore_CreateAndGet(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	task := &agentcore.Task{
		ID:       "1",
		Subject:  "Test task",
		Status:   agentcore.TaskPending,
		Priority: agentcore.TaskPriorityNormal,
	}
	if err := s.Create(ctx, task); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := s.Get(ctx, "1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Subject != "Test task" {
		t.Errorf("got subject %q, want %q", got.Subject, "Test task")
	}
}

func TestMemoryStore_CreateDuplicate(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	task := &agentcore.Task{ID: "1", Subject: "First"}
	if err := s.Create(ctx, task); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := s.Create(ctx, task); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	if _, err := s.Get(ctx, "999"); err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestMemoryStore_Update(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	task := &agentcore.Task{ID: "1", Subject: "Original", Status: agentcore.TaskPending}
	if err := s.Create(ctx, task); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	task.Status = agentcore.TaskInProgress
	task.Subject = "Updated"
	if err := s.Update(ctx, task); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := s.Get(ctx, "1")
	if got.Status != agentcore.TaskInProgress {
		t.Errorf("got status %q, want %q", got.Status, agentcore.TaskInProgress)
	}
	if got.Subject != "Updated" {
		t.Errorf("got subject %q, want %q", got.Subject, "Updated")
	}
}

func TestMemoryStore_UpdateNotFound(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	task := &agentcore.Task{ID: "999", Subject: "Ghost"}
	if err := s.Update(ctx, task); err == nil {
		t.Fatal("expected error for updating non-existent task")
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	task := &agentcore.Task{ID: "1", Subject: "To delete"}
	s.Create(ctx, task)

	if err := s.Delete(ctx, "1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := s.Get(ctx, "1"); err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestMemoryStore_ListExcludesArchived(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	s.Create(ctx, &agentcore.Task{ID: "1", Subject: "Active", Status: agentcore.TaskPending})
	s.Create(ctx, &agentcore.Task{ID: "2", Subject: "Archived", Status: agentcore.TaskArchived})

	tasks, err := s.List(ctx, false)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].ID != "1" {
		t.Errorf("got task ID %q, want %q", tasks[0].ID, "1")
	}

	all, _ := s.List(ctx, true)
	if len(all) != 2 {
		t.Fatalf("got %d tasks with archived, want 2", len(all))
	}
}

func TestMemoryStore_ListSortsByPriorityThenID(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	s.Create(ctx, &agentcore.Task{ID: "1", Subject: "Low", Status: agentcore.TaskPending, Priority: agentcore.TaskPriorityLow})
	s.Create(ctx, &agentcore.Task{ID: "2", Subject: "Urgent", Status: agentcore.TaskPending, Priority: agentcore.TaskPriorityUrgent})
	s.Create(ctx, &agentcore.Task{ID: "3", Subject: "High", Status: agentcore.TaskPending, Priority: agentcore.TaskPriorityHigh})
	s.Create(ctx, &agentcore.Task{ID: "4", Subject: "Urgent2", Status: agentcore.TaskPending, Priority: agentcore.TaskPriorityUrgent})

	tasks, _ := s.List(ctx, true)
	// Expected order: urgent first (ID 2, then 4), then high (3), then low (1)
	if tasks[0].ID != "2" {
		t.Errorf("task[0] ID = %q, want 2", tasks[0].ID)
	}
	if tasks[1].ID != "4" {
		t.Errorf("task[1] ID = %q, want 4", tasks[1].ID)
	}
	if tasks[2].ID != "3" {
		t.Errorf("task[2] ID = %q, want 3", tasks[2].ID)
	}
	if tasks[3].ID != "1" {
		t.Errorf("task[3] ID = %q, want 1", tasks[3].ID)
	}
}

func TestMemoryStore_NextID(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	id1, _ := s.NextID(ctx)
	id2, _ := s.NextID(ctx)
	id3, _ := s.NextID(ctx)

	if id1 != "1" || id2 != "2" || id3 != "3" {
		t.Errorf("IDs = %q, %q, %q; want 1, 2, 3", id1, id2, id3)
	}
}

func TestMemoryStore_GetReturnsClone(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	s.Create(ctx, &agentcore.Task{
		ID:       "1",
		Subject:  "Original",
		Blocks:   []string{"2"},
		Metadata: map[string]any{"key": "val"},
	})

	got, _ := s.Get(ctx, "1")
	got.Subject = "Mutated"
	got.Blocks[0] = "999"
	got.Metadata["key"] = "mutated"

	again, _ := s.Get(ctx, "1")
	if again.Subject != "Original" {
		t.Error("Get did not return a clone (subject mutated)")
	}
	if again.Blocks[0] != "2" {
		t.Error("Get did not return a clone (blocks mutated)")
	}
	if again.Metadata["key"] != "val" {
		t.Error("Get did not return a clone (metadata mutated)")
	}
}

func TestMemoryStore_CreateEmptyID(t *testing.T) {
	s := NewMemoryStore()
	task := &agentcore.Task{ID: "", Subject: "No ID"}
	if err := s.Create(context.Background(), task); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestMemoryStore_UpdateFunc_Basic(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	s.Create(ctx, &agentcore.Task{ID: "1", Subject: "Original", Status: agentcore.TaskPending})

	result, err := s.UpdateFunc(ctx, "1", func(task *agentcore.Task) error {
		task.Subject = "Mutated"
		task.Status = agentcore.TaskInProgress
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateFunc failed: %v", err)
	}
	if result.Subject != "Mutated" {
		t.Errorf("result subject = %q", result.Subject)
	}

	got, _ := s.Get(ctx, "1")
	if got.Subject != "Mutated" {
		t.Errorf("stored subject = %q, want Mutated", got.Subject)
	}
	if got.Status != agentcore.TaskInProgress {
		t.Errorf("stored status = %q", got.Status)
	}
}

func TestMemoryStore_UpdateFunc_NotFound(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.UpdateFunc(context.Background(), "999", func(task *agentcore.Task) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestMemoryStore_UpdateFunc_MutationError_AbortsWrite(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	s.Create(ctx, &agentcore.Task{ID: "1", Subject: "Original", Status: agentcore.TaskPending})

	_, err := s.UpdateFunc(ctx, "1", func(task *agentcore.Task) error {
		task.Subject = "Should not persist"
		return fmt.Errorf("deliberate failure")
	})
	if err == nil {
		t.Fatal("expected mutation error to propagate")
	}

	got, _ := s.Get(ctx, "1")
	if got.Subject != "Original" {
		t.Error("mutate error should not persist changes")
	}
}
