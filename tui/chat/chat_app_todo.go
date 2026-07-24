package chat

// chat_app_todo.go — Task list (TodoPanel) event handlers and overlay management.
//
// When the agent creates or updates a task via task_create/task_update tools,
// a TaskCreated/TaskUpdated event propagates through the EventBus → adapter
// → ChatEvent pipeline. These handlers maintain a local task cache
// (a.tasks) and refresh the TodoPanel so the user sees a live task list.
//
// The user opens the panel with Ctrl+T (ToggleTodoPanel); Esc closes it.

import (
	"sort"

	"github.com/xujian519/mady/tui/component"
)

// todoPriorityRank maps priority strings to sort weights for the TUI display.
// This mirrors agentcore.TaskPriority.Order() but stays in the TUI layer
// to avoid importing agentcore directly (TaskInfo is the boundary type).
var todoPriorityRank = map[string]int{
	"urgent": 4,
	"high":   3,
	"normal": 2,
	"low":    1,
}

// onTaskCreated handles a TaskCreatedChatEvent by adding the task to the
// local cache and refreshing the TodoPanel.
func (a *ChatApp) onTaskCreated(e ChatEvent) {
	ev, ok := e.(TaskCreatedChatEvent)
	if !ok || ev.Task == nil {
		return
	}
	a.mu.Lock()
	a.tasks[ev.Task.ID] = taskToTodoItem(ev.Task)
	a.mu.Unlock()
	a.todoPanel.Reload()
}

// onTaskUpdated handles a TaskUpdatedChatEvent by updating the task in the
// local cache and refreshing the TodoPanel.
func (a *ChatApp) onTaskUpdated(e ChatEvent) {
	ev, ok := e.(TaskUpdatedChatEvent)
	if !ok || ev.Task == nil {
		return
	}
	a.mu.Lock()
	a.tasks[ev.Task.ID] = taskToTodoItem(ev.Task)
	a.mu.Unlock()
	a.todoPanel.Reload()
}

// collectTodoItems returns the current tasks as a sorted slice of TodoItems
// for the TodoPanel data provider. Archived tasks are excluded from the
// main view to keep the list focused on actionable work.
func (a *ChatApp) collectTodoItems() []component.TodoItem {
	a.mu.Lock()
	items := make([]component.TodoItem, 0, len(a.tasks))
	for _, item := range a.tasks {
		if item.Status == "archived" {
			continue
		}
		items = append(items, item)
	}
	a.mu.Unlock()

	// Sort by priority DESC (urgent first), then ID ASC.
	sort.Slice(items, func(i, j int) bool {
		pi, pj := todoPriorityRank[items[i].Priority], todoPriorityRank[items[j].Priority]
		if pi != pj {
			return pi > pj
		}
		return items[i].ID < items[j].ID
	})
	return items
}

// taskToTodoItem converts a TaskInfo to a component.TodoItem.
func taskToTodoItem(t *TaskInfo) component.TodoItem {
	return component.TodoItem{
		ID:       t.ID,
		Content:  t.Subject,
		Status:   string(t.Status),
		Priority: string(t.Priority),
	}
}

// ToggleTodoPanel opens or closes the TodoPanel overlay.
// Lock discipline: a.mu is NOT held during PushOverlay/RemoveOverlay
// (see ToggleKeyHelp for the lock-ordering rationale).
func (a *ChatApp) ToggleTodoPanel() OverlayRef {
	a.mu.Lock()
	if a.todoOverlay != nil {
		ov := a.todoOverlay
		a.todoOverlay = nil
		editor := a.editor
		a.mu.Unlock()
		a.host.RemoveOverlay(ov)
		a.host.Focus(editor)
		return nil
	}

	panel := a.todoPanel
	panel.SetTitle("任务列表 (↑↓ 导航 · Esc 关闭)")
	panel.SetOnClose(func() { a.CloseTodoPanel() })
	panel.Reload()
	ov := &overlayHandle{
		content:       panel,
		focus:         true,
		dimBackground: true,
		category:      OverlayCatSystem,
		widthPct:      60,
		heightPct:     70,
	}
	a.todoOverlay = ov
	a.mu.Unlock()
	a.host.PushOverlay(ov)
	return ov
}

// CloseTodoPanel closes the TodoPanel overlay if it is open.
func (a *ChatApp) CloseTodoPanel() {
	a.mu.Lock()
	ov := a.todoOverlay
	a.todoOverlay = nil
	a.mu.Unlock()
	if ov != nil {
		a.host.RemoveOverlay(ov)
		a.host.Focus(a.editor)
	}
}
