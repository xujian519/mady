package tasklist

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// TestNewExtension_FileStore verifies that NewExtension creates the base
// directory and returns a working Extension.
func TestNewExtension_FileStore(t *testing.T) {
	dir := t.TempDir()
	ext, err := NewExtension(dir)
	if err != nil {
		t.Fatalf("NewExtension failed: %v", err)
	}
	if ext.Name() != ExtensionName {
		t.Errorf("Name = %q, want %q", ext.Name(), ExtensionName)
	}

	// Verify directory was created
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("base dir not created: %v", err)
	}
}

// TestNewExtension_EmptyDir verifies that NewExtension rejects an empty
// base directory.
func TestNewExtension_EmptyDir(t *testing.T) {
	if _, err := NewExtension(""); err == nil {
		t.Fatal("expected error for empty baseDir")
	}
}

// TestExtension_InitDispose verifies the extension lifecycle methods.
func TestExtension_InitDispose(t *testing.T) {
	dir := t.TempDir()
	ext, err := NewExtension(dir)
	if err != nil {
		t.Fatalf("NewExtension failed: %v", err)
	}
	if err := ext.Init(context.Background(), nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if err := ext.Dispose(); err != nil {
		t.Fatalf("Dispose failed: %v", err)
	}
}

// TestExtension_ToolsCount verifies the extension provides the expected
// set of task management tools.
func TestExtension_ToolsCount(t *testing.T) {
	dir := t.TempDir()
	ext, err := NewExtension(dir)
	if err != nil {
		t.Fatalf("NewExtension failed: %v", err)
	}
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

// TestExtension_SnapshotEvents_Empty verifies SnapshotEvents returns an
// empty slice when no tasks exist.
func TestExtension_SnapshotEvents_Empty(t *testing.T) {
	dir := t.TempDir()
	ext, err := NewExtension(dir)
	if err != nil {
		t.Fatalf("NewExtension failed: %v", err)
	}
	events := ext.SnapshotEvents()
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

// TestExtension_SnapshotEvents_WithTasks verifies that tasks stored in the
// underlying FileStore are reflected in SnapshotEvents.
func TestExtension_SnapshotEvents_WithTasks(t *testing.T) {
	dir := t.TempDir()
	ext, err := NewExtension(dir)
	if err != nil {
		t.Fatalf("NewExtension failed: %v", err)
	}

	// Create tasks directly through the store.
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}
	ctx := context.Background()
	if err := store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Task A", Status: agentcore.TaskPending}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := store.Create(ctx, &agentcore.Task{ID: "2", Subject: "Task B", Status: agentcore.TaskInProgress}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

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

// TestFileStore_CreateAndGet verifies basic CRUD operations.
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

// TestFileStore_NextID_Persists verifies that counter persists across store
// instances.
func TestFileStore_NextID_Persists(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store1, _ := NewFileStore(dir)
	id1, _ := store1.NextID(ctx)
	id2, _ := store1.NextID(ctx)
	if id1 != "1" || id2 != "2" {
		t.Fatalf("IDs = %q, %q; want 1, 2", id1, id2)
	}

	store2, _ := NewFileStore(dir)
	id3, _ := store2.NextID(ctx)
	if id3 != "3" {
		t.Errorf("after reload, ID = %q, want 3", id3)
	}
}

// TestFileStore_NextID_InfersFromFiles verifies that NextID correctly infers
// the next ID from existing task files.
func TestFileStore_NextID_InfersFromFiles(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Pre-create task files
	data := []byte(`{"id":"5","subject":"Existing","status":"pending","priority":"normal"}`)
	if err := os.WriteFile(filepath.Join(dir, "5.json"), data, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	// Also write a non-task file that should be ignored
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	store, _ := NewFileStore(dir)
	id, _ := store.NextID(ctx)
	if id != "6" {
		t.Errorf("inferred ID = %q, want 6", id)
	}
}

// TestFileStore_UpdateAndDelete verifies update and delete operations.
func TestFileStore_UpdateAndDelete(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileStore(dir)
	ctx := context.Background()

	if err := store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Original", Status: agentcore.TaskPending}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	task, _ := store.Get(ctx, "1")
	task.Subject = "Updated"
	if err := store.Update(ctx, task); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := store.Get(ctx, "1")
	if got.Subject != "Updated" {
		t.Errorf("subject = %q, want Updated", got.Subject)
	}

	if err := store.Delete(ctx, "1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := store.Get(ctx, "1"); err == nil {
		t.Error("expected error after delete")
	}
}

// TestFileStore_List verifies listing with archive filtering.
func TestFileStore_List(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileStore(dir)
	ctx := context.Background()

	if err := store.Create(ctx, &agentcore.Task{ID: "1", Subject: "A", Status: agentcore.TaskPending, Priority: agentcore.TaskPriorityLow}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := store.Create(ctx, &agentcore.Task{ID: "2", Subject: "B", Status: agentcore.TaskArchived, Priority: agentcore.TaskPriorityNormal}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

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

// TestPriorityOrder verifies the priority ordering.
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

// TestFileStore_UpdateFunc verifies atomic read-modify-write.
func TestFileStore_UpdateFunc(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFileStore(dir)
	ctx := context.Background()
	if err := store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Original", Status: agentcore.TaskPending}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

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

// TestFileStore_UpdateFunc_NotFound verifies error on unknown task.
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
