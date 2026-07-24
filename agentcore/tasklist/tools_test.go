package tasklist

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// --- task_create tests ---

func TestTaskCreate_Basic(t *testing.T) {
	store := NewMemoryStore()
	ext := NewExtensionWithStore(store)
	tools := ext.Tools()

	var createTool *agentcore.Tool
	for _, tool := range tools {
		if tool.Name == TaskCreateToolName {
			createTool = tool
			break
		}
	}
	if createTool == nil {
		t.Fatal("task_create tool not found")
	}

	args, _ := json.Marshal(map[string]string{
		"subject":     "Search prior art",
		"description": "Find relevant prior art for claim 1",
		"priority":    "high",
	})
	result, err := createTool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("task_create failed: %v", err)
	}
	str, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}
	if !strings.Contains(str, "已创建") {
		t.Errorf("result %q does not contain '已创建'", str)
	}

	// Verify task was stored
	tasks, _ := store.List(context.Background(), false)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Subject != "Search prior art" {
		t.Errorf("subject = %q", tasks[0].Subject)
	}
	if tasks[0].Priority != agentcore.TaskPriorityHigh {
		t.Errorf("priority = %q", tasks[0].Priority)
	}
}

func TestTaskCreate_MissingSubject(t *testing.T) {
	ext := NewExtensionWithStore(NewMemoryStore())
	tool := findTool(ext.Tools(), TaskCreateToolName)

	args, _ := json.Marshal(map[string]string{"description": "No subject"})
	if _, err := tool.Func(context.Background(), args); err == nil {
		t.Fatal("expected error for missing subject")
	}
}

func TestTaskCreate_InvalidPriority(t *testing.T) {
	ext := NewExtensionWithStore(NewMemoryStore())
	tool := findTool(ext.Tools(), TaskCreateToolName)

	args, _ := json.Marshal(map[string]string{
		"subject":     "Test",
		"description": "Desc",
		"priority":    "invalid",
	})
	if _, err := tool.Func(context.Background(), args); err == nil {
		t.Fatal("expected error for invalid priority")
	}
}

func TestTaskCreate_DefaultPriority(t *testing.T) {
	store := NewMemoryStore()
	ext := NewExtensionWithStore(store)
	tool := findTool(ext.Tools(), TaskCreateToolName)

	args, _ := json.Marshal(map[string]string{
		"subject":     "Test",
		"description": "Desc",
	})
	tool.Func(context.Background(), args)

	tasks, _ := store.List(context.Background(), false)
	if tasks[0].Priority != agentcore.TaskPriorityNormal {
		t.Errorf("default priority = %q, want normal", tasks[0].Priority)
	}
}

// --- task_get tests ---

func TestTaskGet_Basic(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	store.Create(ctx, &agentcore.Task{
		ID:          "1",
		Subject:     "Test task",
		Description: "A detailed description",
		Status:      agentcore.TaskInProgress,
		Priority:    agentcore.TaskPriorityHigh,
	})

	ext := NewExtensionWithStore(store)
	tool := findTool(ext.Tools(), TaskGetToolName)

	args, _ := json.Marshal(map[string]string{"task_id": "1"})
	result, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("task_get failed: %v", err)
	}
	str := result.(string)
	if !strings.Contains(str, "Test task") {
		t.Errorf("result missing subject: %q", str)
	}
	if !strings.Contains(str, "in_progress") {
		t.Errorf("result missing status: %q", str)
	}
}

func TestTaskGet_NotFound(t *testing.T) {
	ext := NewExtensionWithStore(NewMemoryStore())
	tool := findTool(ext.Tools(), TaskGetToolName)

	args, _ := json.Marshal(map[string]string{"task_id": "999"})
	if _, err := tool.Func(context.Background(), args); err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

// --- task_list tests ---

func TestTaskList_Empty(t *testing.T) {
	ext := NewExtensionWithStore(NewMemoryStore())
	tool := findTool(ext.Tools(), TaskListToolName)

	args, _ := json.Marshal(map[string]any{})
	result, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("task_list failed: %v", err)
	}
	str := result.(string)
	if !strings.Contains(str, "暂无任务") {
		t.Errorf("expected empty message, got %q", str)
	}
}

func TestTaskList_WithTasks(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Task A", Status: agentcore.TaskPending, Priority: agentcore.TaskPriorityNormal})
	store.Create(ctx, &agentcore.Task{ID: "2", Subject: "Task B", Status: agentcore.TaskInProgress, Priority: agentcore.TaskPriorityHigh})

	ext := NewExtensionWithStore(store)
	tool := findTool(ext.Tools(), TaskListToolName)

	args, _ := json.Marshal(map[string]any{})
	result, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("task_list failed: %v", err)
	}
	str := result.(string)
	if !strings.Contains(str, "总计 2") {
		t.Errorf("expected count in result: %q", str)
	}
	// High priority (Task B) should come first
	idxB := strings.Index(str, "Task B")
	idxA := strings.Index(str, "Task A")
	if idxB < 0 || idxA < 0 || idxB > idxA {
		t.Errorf("expected Task B before Task A in: %q", str)
	}
}

// --- task_update tests ---

func TestTaskUpdate_StatusChange(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Test", Status: agentcore.TaskPending})

	ext := NewExtensionWithStore(store)
	tool := findTool(ext.Tools(), TaskUpdateToolName)

	args, _ := json.Marshal(map[string]string{"task_id": "1", "status": "in_progress"})
	result, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("task_update failed: %v", err)
	}
	str := result.(string)
	if !strings.Contains(str, "status") {
		t.Errorf("expected status in updated fields: %q", str)
	}

	task, _ := store.Get(ctx, "1")
	if task.Status != agentcore.TaskInProgress {
		t.Errorf("status = %q, want in_progress", task.Status)
	}
}

func TestTaskUpdate_PriorityChange(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Test", Status: agentcore.TaskPending})

	ext := NewExtensionWithStore(store)
	tool := findTool(ext.Tools(), TaskUpdateToolName)

	args, _ := json.Marshal(map[string]string{"task_id": "1", "priority": "urgent"})
	_, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("task_update failed: %v", err)
	}

	task, _ := store.Get(ctx, "1")
	if task.Priority != agentcore.TaskPriorityUrgent {
		t.Errorf("priority = %q, want urgent", task.Priority)
	}
}

func TestTaskUpdate_AddBlocks_Bidirectional(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Blocker", Status: agentcore.TaskPending})
	store.Create(ctx, &agentcore.Task{ID: "2", Subject: "Blocked", Status: agentcore.TaskPending})

	ext := NewExtensionWithStore(store)
	tool := findTool(ext.Tools(), TaskUpdateToolName)

	// Task 1 blocks Task 2
	args, _ := json.Marshal(map[string]any{
		"task_id":    "1",
		"add_blocks": []string{"2"},
	})
	if _, err := tool.Func(context.Background(), args); err != nil {
		t.Fatalf("task_update add_blocks failed: %v", err)
	}

	// Verify task 1 has "2" in blocks
	t1, _ := store.Get(ctx, "1")
	found := false
	for _, b := range t1.Blocks {
		if b == "2" {
			found = true
		}
	}
	if !found {
		t.Error("task 1 blocks does not contain '2'")
	}

	// Verify task 2 has "1" in blockedBy (bidirectional maintenance)
	t2, _ := store.Get(ctx, "2")
	found = false
	for _, b := range t2.BlockedBy {
		if b == "1" {
			found = true
		}
	}
	if !found {
		t.Error("task 2 blockedBy does not contain '1' (bidirectional not maintained)")
	}
}

func TestTaskUpdate_CyclicDependency(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	store.Create(ctx, &agentcore.Task{ID: "1", Subject: "A", Status: agentcore.TaskPending})
	store.Create(ctx, &agentcore.Task{ID: "2", Subject: "B", Status: agentcore.TaskPending})

	ext := NewExtensionWithStore(store)
	tool := findTool(ext.Tools(), TaskUpdateToolName)

	// 1 blocks 2
	args, _ := json.Marshal(map[string]any{"task_id": "1", "add_blocks": []string{"2"}})
	tool.Func(context.Background(), args)

	// Try 2 blocks 1 — should fail (cycle)
	args, _ = json.Marshal(map[string]any{"task_id": "2", "add_blocks": []string{"1"}})
	if _, err := tool.Func(context.Background(), args); err == nil {
		t.Fatal("expected cyclic dependency error")
	}
}

func TestTaskUpdate_SelfBlock(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Self", Status: agentcore.TaskPending})

	ext := NewExtensionWithStore(store)
	tool := findTool(ext.Tools(), TaskUpdateToolName)

	args, _ := json.Marshal(map[string]any{"task_id": "1", "add_blocks": []string{"1"}})
	if _, err := tool.Func(context.Background(), args); err == nil {
		t.Fatal("expected error for self-blocking")
	}
}

func TestTaskUpdate_InvalidStatus(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	store.Create(ctx, &agentcore.Task{ID: "1", Subject: "Test", Status: agentcore.TaskPending})

	ext := NewExtensionWithStore(store)
	tool := findTool(ext.Tools(), TaskUpdateToolName)

	args, _ := json.Marshal(map[string]string{"task_id": "1", "status": "invalid"})
	if _, err := tool.Func(context.Background(), args); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestTaskUpdate_NotFound(t *testing.T) {
	ext := NewExtensionWithStore(NewMemoryStore())
	tool := findTool(ext.Tools(), TaskUpdateToolName)

	args, _ := json.Marshal(map[string]string{"task_id": "999", "status": "completed"})
	if _, err := tool.Func(context.Background(), args); err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

// --- ReadOnly flag tests ---

func TestReadOnlyFlags(t *testing.T) {
	ext := NewExtensionWithStore(NewMemoryStore())
	tools := ext.Tools()

	for _, tool := range tools {
		switch tool.Name {
		case TaskGetToolName, TaskListToolName:
			if !tool.ReadOnly {
				t.Errorf("%s should be ReadOnly", tool.Name)
			}
		case TaskCreateToolName, TaskUpdateToolName:
			if tool.ReadOnly {
				t.Errorf("%s should NOT be ReadOnly", tool.Name)
			}
		}
	}
}

// --- Helper ---

func findTool(tools []*agentcore.Tool, name string) *agentcore.Tool {
	for _, t := range tools {
		if t.Name == name {
			return t
		}
	}
	panic("tool not found: " + name)
}
