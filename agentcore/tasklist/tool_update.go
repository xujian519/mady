package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// TaskUpdateToolName 是任务更新工具的名称。
const TaskUpdateToolName = "task_update"

// taskUpdateTool 更新已有任务（状态/优先级/依赖/owner）。
type taskUpdateTool struct {
	store Store
	mu    *sync.Mutex
	agent *agentcore.Agent
}

func newUpdateTool(store Store, mu *sync.Mutex, agent *agentcore.Agent) *agentcore.Tool {
	t := &taskUpdateTool{store: store, mu: mu, agent: agent}
	return &agentcore.Tool{
		Name:        TaskUpdateToolName,
		Description: taskUpdateDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "The ID of the task to update",
				},
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{"pending", "in_progress", "completed", "archived"},
					"description": "New status for the task",
				},
				"priority": map[string]any{
					"type":        "string",
					"enum":        []string{"urgent", "high", "normal", "low"},
					"description": "New priority for the task",
				},
				"add_blocks": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Task IDs that this task blocks (cannot start until this completes)",
				},
				"add_blocked_by": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Task IDs that block this task (must complete before this can start)",
				},
				"owner": map[string]any{
					"type":        "string",
					"description": "Agent name claiming ownership of this task",
				},
			},
			"required":             []string{"task_id"},
			"additionalProperties": false,
		},
		Func: t.Run,
	}
}

type taskUpdateArgs struct {
	TaskID       string   `json:"task_id"`
	Status       string   `json:"status"`
	Priority     string   `json:"priority"`
	AddBlocks    []string `json:"add_blocks"`
	AddBlockedBy []string `json:"add_blocked_by"`
	Owner        string   `json:"owner"`
}

// Run 执行任务更新。
func (t *taskUpdateTool) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p taskUpdateArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if !isValidTaskID(p.TaskID) {
		return nil, fmt.Errorf("无效的任务 ID %q（应为纯数字）", p.TaskID)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	task, err := t.store.Get(ctx, p.TaskID)
	if err != nil {
		return nil, err
	}

	oldStatus := task.Status
	var updatedFields []string

	// 状态变更
	if p.Status != "" {
		newStatus := agentcore.TaskStatus(p.Status)
		if !isValidStatus(newStatus) {
			return nil, fmt.Errorf("无效的状态 %q（可选值: pending/in_progress/completed/archived）", p.Status)
		}
		task.Status = newStatus
		updatedFields = append(updatedFields, "status")
	}

	// 优先级变更
	if p.Priority != "" {
		newPriority := agentcore.TaskPriority(p.Priority)
		if !isValidPriority(newPriority) {
			return nil, fmt.Errorf("无效的优先级 %q（可选值: urgent/high/normal/low）", p.Priority)
		}
		task.Priority = newPriority
		updatedFields = append(updatedFields, "priority")
	}

	// Owner 变更
	if p.Owner != "" {
		task.Owner = p.Owner
		updatedFields = append(updatedFields, "owner")
	}

	// 依赖关系变更
	if len(p.AddBlocks) > 0 || len(p.AddBlockedBy) > 0 {
		allTasks, err := t.store.List(ctx, true)
		if err != nil {
			return nil, fmt.Errorf("读取任务列表失败: %w", err)
		}
		taskMap := buildTaskMap(allTasks)

		if len(p.AddBlocks) > 0 {
			for _, blockedID := range p.AddBlocks {
				if !isValidTaskID(blockedID) {
					return nil, fmt.Errorf("无效的被阻塞任务 ID %q", blockedID)
				}
				if _, exists := taskMap[blockedID]; !exists {
					return nil, fmt.Errorf("任务 #%s 不存在", blockedID)
				}
				if hasCyclicDependency(taskMap, p.TaskID, blockedID) {
					return nil, fmt.Errorf("添加任务 #%s 到 #%s 的阻塞关系会形成循环依赖", blockedID, p.TaskID)
				}
			}
			// 双向维护：被阻塞任务添加 blockedBy
			for _, blockedID := range p.AddBlocks {
				if err := addBlockedBy(ctx, t.store, blockedID, p.TaskID); err != nil {
					return nil, err
				}
			}
			task.Blocks = appendUnique(task.Blocks, p.AddBlocks...)
			updatedFields = append(updatedFields, "blocks")
		}

		if len(p.AddBlockedBy) > 0 {
			for _, blockerID := range p.AddBlockedBy {
				if !isValidTaskID(blockerID) {
					return nil, fmt.Errorf("无效的阻塞任务 ID %q", blockerID)
				}
				if _, exists := taskMap[blockerID]; !exists {
					return nil, fmt.Errorf("任务 #%s 不存在", blockerID)
				}
				if hasCyclicDependency(taskMap, blockerID, p.TaskID) {
					return nil, fmt.Errorf("添加任务 #%s 到 #%s 的被阻塞关系会形成循环依赖", blockerID, p.TaskID)
				}
			}
			// 双向维护：阻塞任务添加 blocks
			for _, blockerID := range p.AddBlockedBy {
				if err := addBlocks(ctx, t.store, blockerID, p.TaskID); err != nil {
					return nil, err
				}
			}
			task.BlockedBy = appendUnique(task.BlockedBy, p.AddBlockedBy...)
			updatedFields = append(updatedFields, "blocked_by")
		}
	}

	task.UpdatedAt = time.Now()
	if err := t.store.Update(ctx, task); err != nil {
		return nil, err
	}

	// 发射事件供 TUI 实时刷新
	if t.agent != nil {
		t.agent.EmitEvent(agentcore.NewTaskUpdatedEvent(task, string(oldStatus), string(task.Status)))
	}

	return fmt.Sprintf("任务 #%s 已更新: %s", p.TaskID, strings.Join(updatedFields, ", ")), nil
}

func isValidStatus(s agentcore.TaskStatus) bool {
	switch s {
	case agentcore.TaskPending, agentcore.TaskInProgress,
		agentcore.TaskCompleted, agentcore.TaskArchived:
		return true
	default:
		return false
	}
}

// --- 依赖图工具函数 ---

func buildTaskMap(tasks []*agentcore.Task) map[string]*agentcore.Task {
	m := make(map[string]*agentcore.Task, len(tasks))
	for _, t := range tasks {
		m[t.ID] = t
	}
	return m
}

// hasCyclicDependency 检查在 blockerID 和 blockedID 之间添加依赖是否会形成循环。
// blockerID 是阻塞方，blockedID 是被阻塞方。
// 如果 blockedID 通过 blocks 链能到达 blockerID，则形成循环。
func hasCyclicDependency(taskMap map[string]*agentcore.Task, blockerID, blockedID string) bool {
	if blockerID == blockedID {
		return true
	}
	visited := make(map[string]bool)
	return canReach(taskMap, blockedID, blockerID, visited)
}

// canReach 通过 blocks 链检查 from 是否能到达 to。
func canReach(taskMap map[string]*agentcore.Task, fromID, toID string, visited map[string]bool) bool {
	if fromID == toID {
		return true
	}
	if visited[fromID] {
		return false
	}
	visited[fromID] = true
	fromTask, exists := taskMap[fromID]
	if !exists {
		return false
	}
	for _, nextID := range fromTask.Blocks {
		if canReach(taskMap, nextID, toID, visited) {
			return true
		}
	}
	return false
}

// addBlockedBy 在 targetID 任务的 blockedBy 列表中追加 blockerID。
func addBlockedBy(ctx context.Context, store Store, targetID, blockerID string) error {
	t, err := store.Get(ctx, targetID)
	if err != nil {
		return err
	}
	t.BlockedBy = appendUnique(t.BlockedBy, blockerID)
	t.UpdatedAt = time.Now()
	return store.Update(ctx, t)
}

// addBlocks 在 targetID 任务的 blocks 列表中追加 blockedID。
func addBlocks(ctx context.Context, store Store, targetID, blockedID string) error {
	t, err := store.Get(ctx, targetID)
	if err != nil {
		return err
	}
	t.Blocks = appendUnique(t.Blocks, blockedID)
	t.UpdatedAt = time.Now()
	return store.Update(ctx, t)
}

func appendUnique(slice []string, items ...string) []string {
	seen := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		seen[s] = struct{}{}
	}
	for _, item := range items {
		if _, exists := seen[item]; !exists {
			slice = append(slice, item)
			seen[item] = struct{}{}
		}
	}
	return slice
}
