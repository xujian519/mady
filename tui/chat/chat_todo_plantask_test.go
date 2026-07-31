package chat

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/component"
)

// TestOnTaskCreatedUpdated verifies task event handlers populate the cache
// and refresh the panel.
func TestOnTaskCreatedUpdated(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})

	app.onTaskCreated(TaskCreatedChatEvent{Task: &TaskInfo{ID: "t1", Subject: "撰写权利要求", Status: "in_progress", Priority: "high"}})
	app.mu.Lock()
	items := app.tasks
	app.mu.Unlock()
	if len(items) != 1 {
		t.Fatalf("expected 1 cached task, got %d", len(items))
	}
	if items["t1"].Content != "撰写权利要求" || items["t1"].Priority != "high" {
		t.Fatalf("cached item = %+v", items["t1"])
	}

	// Update the same task.
	app.onTaskUpdated(TaskUpdatedChatEvent{Task: &TaskInfo{ID: "t1", Subject: "撰写权利要求（已修订）", Status: "done", Priority: "low"}})
	app.mu.Lock()
	items = app.tasks
	app.mu.Unlock()
	if items["t1"].Content != "撰写权利要求（已修订）" || items["t1"].Status != "done" {
		t.Fatalf("updated item = %+v", items["t1"])
	}

	// Nil tasks are ignored.
	before := len(items)
	app.onTaskCreated(TaskCreatedChatEvent{})
	app.onTaskUpdated(TaskUpdatedChatEvent{})
	app.mu.Lock()
	items = app.tasks
	app.mu.Unlock()
	if len(items) != before {
		t.Fatal("nil task events must be no-ops")
	}
}

// TestCollectTodoItemsSorting verifies priority sorting and archived exclusion.
func TestCollectTodoItemsSorting(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.onTaskCreated(TaskCreatedChatEvent{Task: &TaskInfo{ID: "a", Subject: "low task", Priority: "low"}})
	app.onTaskCreated(TaskCreatedChatEvent{Task: &TaskInfo{ID: "b", Subject: "urgent task", Priority: "urgent"}})
	app.onTaskCreated(TaskCreatedChatEvent{Task: &TaskInfo{ID: "c", Subject: "archived", Status: "archived", Priority: "urgent"}})
	app.onTaskCreated(TaskCreatedChatEvent{Task: &TaskInfo{ID: "d", Subject: "normal task", Priority: "normal"}})

	items := app.collectTodoItems()
	if len(items) != 3 {
		t.Fatalf("archived should be excluded, got %d items", len(items))
	}
	if items[0].ID != "b" || items[1].ID != "d" || items[2].ID != "a" {
		t.Fatalf("sort order = %v, want b(d urgent), d, a", ids(items))
	}
}

func ids(items []component.TodoItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

// TestTaskToTodoItem verifies the TaskInfo projection.
func TestTaskToTodoItem(t *testing.T) {
	item := taskToTodoItem(&TaskInfo{ID: "x", Subject: "s", Status: "st", Priority: "p"})
	if item.ID != "x" || item.Content != "s" || item.Status != "st" || item.Priority != "p" {
		t.Fatalf("projection = %+v", item)
	}
}

// TestToggleTodoPanelClosePath verifies the todo panel close paths.
//
// NOTE: the OPEN path of ToggleTodoPanel is untestable without deadlocking:
// it calls panel.Reload() while holding a.mu, and Reload → collectTodoItems
// re-locks a.mu (self-deadlock). This is a pre-existing production bug
// (chat_app_todo.go) — source edits are out of scope for this task.
func TestToggleTodoPanelClosePath(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	host := app.host.(*testAppHost)

	// Seed an open overlay directly, then verify the close path removes it.
	ov := &overlayHandle{
		content:       app.todoPanel,
		focus:         true,
		dimBackground: true,
		category:      OverlayCatSystem,
		widthPct:      60,
		heightPct:     70,
	}
	app.mu.Lock()
	app.todoOverlay = ov
	app.mu.Unlock()

	if got := app.ToggleTodoPanel(); got != nil {
		t.Fatal("ToggleTodoPanel with an open panel should close and return nil")
	}
	if len(host.overlays) != 0 {
		t.Fatal("todo panel should be removed")
	}

	// CloseTodoPanel with nothing open is a no-op.
	app.CloseTodoPanel()

	// CloseTodoPanel with a seeded overlay removes it and focuses the editor.
	app.mu.Lock()
	app.todoOverlay = ov
	app.mu.Unlock()
	app.CloseTodoPanel()
	if len(host.overlays) != 0 {
		t.Fatal("CloseTodoPanel should remove the panel")
	}
}

// TestTodoPanelReloadFromCache verifies Reload picks up cached tasks.
func TestTodoPanelReloadFromCache(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.onTaskCreated(TaskCreatedChatEvent{Task: &TaskInfo{ID: "p1", Subject: "专利分析", Priority: "high"}})
	out := strings.Join(app.todoPanel.Render(60), "\n")
	if !strings.Contains(out, "专利分析") {
		t.Fatalf("todo panel should render the cached task, got %q", out)
	}
}

// TestOnPlanTaskStatusChanged verifies HCL status change messages.
func TestOnPlanTaskStatusChanged(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.onPlanTaskStatusChanged(PlanTaskStatusChangedChatEvent{
		SessionID: "s1", CaseID: "CASE-001", FromStatus: "drafting", ToStatus: "review",
	})
	msgs := app.History().Messages()
	if len(msgs) != 1 || !strings.Contains(msgs[0].Text, "CASE-001") {
		t.Fatalf("expected status message, got %+v", msgs)
	}
	if !strings.Contains(msgs[0].Text, "drafting") || !strings.Contains(msgs[0].Text, "review") {
		t.Fatalf("status message should include transition, got %q", msgs[0].Text)
	}

	// Wrong event type is a no-op.
	app.onPlanTaskStatusChanged(AgentStartChatEvent{})
	if len(app.History().Messages()) != 1 {
		t.Fatal("wrong event type must be a no-op")
	}
}

// TestOnPlanTaskFeedbackAdded verifies feedback injection messages.
func TestOnPlanTaskFeedbackAdded(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.onPlanTaskFeedbackAdded(PlanTaskFeedbackAddedChatEvent{SessionID: "s1", Text: "补充技术效果数据"})
	msgs := app.History().Messages()
	if len(msgs) != 1 || !strings.Contains(msgs[0].Text, "补充技术效果数据") {
		t.Fatalf("expected feedback message, got %+v", msgs)
	}
}

// TestOnPlanTaskInterrupted verifies interruption messages with and without
// an explicit reason.
func TestOnPlanTaskInterrupted(t *testing.T) {
	t.Run("with reason", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.onPlanTaskInterrupted(PlanTaskInterruptedChatEvent{SessionID: "s1", StepID: "st1", Reason: "用户要求暂停"})
		msgs := app.History().Messages()
		if len(msgs) != 1 || !strings.Contains(msgs[0].Text, "用户要求暂停") {
			t.Fatalf("expected interrupt message, got %+v", msgs)
		}
		if !strings.Contains(msgs[0].Text, "/resume") {
			t.Fatalf("message should mention /resume, got %q", msgs[0].Text)
		}
	})

	t.Run("default reason", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.onPlanTaskInterrupted(PlanTaskInterruptedChatEvent{})
		msgs := app.History().Messages()
		if len(msgs) != 1 || !strings.Contains(msgs[0].Text, "用户请求暂停") {
			t.Fatalf("expected default reason, got %+v", msgs)
		}
	})
}
