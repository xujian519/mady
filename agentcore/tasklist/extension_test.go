package tasklist

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// --- Extension lifecycle tests ---

func TestExtension_Name(t *testing.T) {
	ext := NewExtensionWithStore(NewMemoryStore())
	if ext.Name() != ExtensionName {
		t.Errorf("Name = %q, want %q", ext.Name(), ExtensionName)
	}
}

func TestExtension_ToolsCount(t *testing.T) {
	ext := NewExtensionWithStore(NewMemoryStore())
	tools := ext.Tools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, expected := range []string{TaskCreateToolName, TaskGetToolName, TaskUpdateToolName, TaskListToolName} {
		if !names[expected] {
			t.Errorf("missing tool: %s", expected)
		}
	}
}

func TestExtension_InitDispose(t *testing.T) {
	ext := NewExtensionWithStore(NewMemoryStore())
	if err := ext.Init(context.Background(), nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if err := ext.Dispose(); err != nil {
		t.Fatalf("Dispose failed: %v", err)
	}
}

func TestExtension_SnapshotEvents_Empty(t *testing.T) {
	ext := NewExtensionWithStore(NewMemoryStore())
	events := ext.SnapshotEvents()
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestExtension_SnapshotEvents_WithTasks(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Task A", Status: agentcore.TaskPending})
	store.Create(ctx, &agentcore.Task{ID: "2", Subject: "Task B", Status: agentcore.TaskInProgress})

	ext := NewExtensionWithStore(store)
	events := ext.SnapshotEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	for _, ev := range events {
		if ev.EventKind() != agentcore.EventTaskCreated {
			t.Errorf("event type = %q, want %q", ev.EventKind(), agentcore.EventTaskCreated)
		}
	}
}

func TestNewExtension_FileStore(t *testing.T) {
	dir := t.TempDir()
	ext, err := NewExtension(dir)
	if err != nil {
		t.Fatalf("NewExtension failed: %v", err)
	}
	if ext.Name() != ExtensionName {
		t.Errorf("Name = %q", ext.Name())
	}

	// Verify directory was created
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("base dir not created: %v", err)
	}
}

func TestNewExtension_EmptyDir(t *testing.T) {
	if _, err := NewExtension(""); err == nil {
		t.Fatal("expected error for empty baseDir")
	}
}

// --- FileStore tests ---

func TestFileStore_CreateAndGet(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}
	ctx := context.Background()

	task := &agentcore.Task{ID: "1", Subject: "File task", Status: agentcore.TaskPending}
	if err := store.Create(ctx, task); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, "1.json")); err != nil {
		t.Errorf("task file not created: %v", err)
	}

	got, err := store.Get(ctx, "1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Subject != "File task" {
		t.Errorf("subject = %q", got.Subject)
	}
}

func TestFileStore_NextID_Persists(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store1, _ := NewFileStore(dir)
	id1, _ := store1.NextID(ctx)
	id2, _ := store1.NextID(ctx)
	if id1 != "1" || id2 != "2" {
		t.Fatalf("IDs = %q, %q; want 1, 2", id1, id2)
	}

	// Recreate store — should resume from persisted counter
	store2, _ := NewFileStore(dir)
	id3, _ := store2.NextID(ctx)
	if id3 != "3" {
		t.Errorf("after reload, ID = %q, want 3", id3)
	}
}

func TestFileStore_NextID_InfersFromFiles(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Pre-create task files without .nextid
	task1 := &agentcore.Task{ID: "5", Subject: "Existing", Status: agentcore.TaskPending}
	data := []byte(`{"id":"5","subject":"Existing","status":"pending","priority":"normal"}`)
	os.WriteFile(filepath.Join(dir, "5.json"), data, 0644)
	// Also write a non-task file that should be ignored
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore"), 0644)

	store, _ := NewFileStore(dir)
	id, _ := store.NextID(ctx)
	if id != "6" {
		t.Errorf("inferred ID = %q, want 6", id)
	}

	_ = task1 // keep linter happy
}

func TestFileStore_UpdateAndDelete(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileStore(dir)
	ctx := context.Background()

	store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Original", Status: agentcore.TaskPending})

	// Update
	task, _ := store.Get(ctx, "1")
	task.Subject = "Updated"
	store.Update(ctx, task)

	got, _ := store.Get(ctx, "1")
	if got.Subject != "Updated" {
		t.Errorf("subject = %q, want Updated", got.Subject)
	}

	// Delete
	store.Delete(ctx, "1")
	if _, err := store.Get(ctx, "1"); err == nil {
		t.Error("expected error after delete")
	}
}

func TestFileStore_List(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileStore(dir)
	ctx := context.Background()

	store.Create(ctx, &agentcore.Task{ID: "1", Subject: "A", Status: agentcore.TaskPending, Priority: agentcore.TaskPriorityLow})
	store.Create(ctx, &agentcore.Task{ID: "2", Subject: "B", Status: agentcore.TaskArchived, Priority: agentcore.TaskPriorityNormal})

	// Default excludes archived
	tasks, _ := store.List(ctx, false)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 non-archived task, got %d", len(tasks))
	}

	// Include archived
	all, _ := store.List(ctx, true)
	if len(all) != 2 {
		t.Fatalf("expected 2 tasks total, got %d", len(all))
	}
}

func TestTaskClone_DeepCopy(t *testing.T) {
	original := &agentcore.Task{
		ID:       "1",
		Blocks:   []string{"2", "3"},
		Metadata: map[string]any{"key": "value"},
	}
	clone := original.Clone()

	clone.Blocks[0] = "999"
	clone.Metadata["key"] = "mutated"

	if original.Blocks[0] != "2" {
		t.Error("Clone did not deep-copy Blocks")
	}
	if original.Metadata["key"] != "value" {
		t.Error("Clone did not deep-copy Metadata")
	}
}

func TestTaskClone_Nil(t *testing.T) {
	if nil != (*agentcore.Task)(nil).Clone() {
		t.Error("Clone of nil should return nil")
	}
}

func TestPriorityOrder(t *testing.T) {
	tests := []struct {
		p    agentcore.TaskPriority
		want int
	}{
		{agentcore.TaskPriorityUrgent, 4},
		{agentcore.TaskPriorityHigh, 3},
		{agentcore.TaskPriorityNormal, 2},
		{agentcore.TaskPriorityLow, 1},
		{agentcore.TaskPriority("invalid"), 0},
	}
	for _, tt := range tests {
		if got := tt.p.Order(); got != tt.want {
			t.Errorf("%s.Order() = %d, want %d", tt.p, got, tt.want)
		}
	}
}

func TestFileStore_UpdateFunc(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileStore(dir)
	ctx := context.Background()
	store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Original", Status: agentcore.TaskPending})

	result, err := store.UpdateFunc(ctx, "1", func(task *agentcore.Task) error {
		task.Subject = "Updated"
		task.Status = agentcore.TaskCompleted
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateFunc failed: %v", err)
	}
	if result.Subject != "Updated" {
		t.Errorf("result subject = %q", result.Subject)
	}

	got, _ := store.Get(ctx, "1")
	if got.Subject != "Updated" || got.Status != agentcore.TaskCompleted {
		t.Errorf("stored = %+v", got)
	}
}

func TestFileStore_UpdateFunc_NotFound(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileStore(dir)
	_, err := store.UpdateFunc(context.Background(), "999", func(task *agentcore.Task) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}
