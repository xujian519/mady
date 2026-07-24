package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// TaskUpdateToolName 是任务更新工具的名称。
const TaskUpdateToolName = "task_update"

// taskUpdateTool 更新已有任务（状态/优先级/依赖/owner）。
type taskUpdateTool struct {
	store Store
	agent *agentcore.Agent
}

func newUpdateTool(store Store, agent *agentcore.Agent) *agentcore.Tool {
	t := &taskUpdateTool{store: store, agent: agent}
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
//
// 原子性策略：
//   - 主任务的状态/优先级/owner 变更通过单次 UpdateFunc 原子完成。
//   - 依赖关系的反向写入（addBlockedBy/addBlocks）各自通过独立 UpdateFunc 原子完成。
//     验证全部在写入前完成——只有所有校验通过才开始写入。
//   - 主任务的 Blocks/BlockedBy 字段在同一 UpdateFunc 中与状态等一起写入。
func (t *taskUpdateTool) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p taskUpdateArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if !isValidTaskID(p.TaskID) {
		return nil, fmt.Errorf("无效的任务 ID %q（应为纯数字）", p.TaskID)
	}

	// --- 预验证阶段（无写入） ---
	if p.Status != "" {
		if !isValidStatus(agentcore.TaskStatus(p.Status)) {
			return nil, fmt.Errorf("无效的状态 %q（可选值: pending/in_progress/completed/archived）", p.Status)
		}
	}
	if p.Priority != "" {
		if !isValidPriority(agentcore.TaskPriority(p.Priority)) {
			return nil, fmt.Errorf("无效的优先级 %q（可选值: urgent/high/normal/low）", p.Priority)
		}
	}

	// 依赖关系校验
	var taskMap map[string]*agentcore.Task
	if len(p.AddBlocks) > 0 || len(p.AddBlockedBy) > 0 {
		allTasks, err := t.store.List(ctx, true)
		if err != nil {
			return nil, fmt.Errorf("读取任务列表失败: %w", err)
		}
		taskMap = buildTaskMap(allTasks)

		if _, exists := taskMap[p.TaskID]; !exists {
			return nil, fmt.Errorf("任务 #%s 不存在", p.TaskID)
		}

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
	}

	// --- 写入阶段 ---
	// 先写入反向依赖（每个原子），再写主任务。
	for _, blockedID := range p.AddBlocks {
		if _, err := t.store.UpdateFunc(ctx, blockedID, func(task *agentcore.Task) error {
			task.BlockedBy = appendUnique(task.BlockedBy, p.TaskID)
			task.UpdatedAt = time.Now()
			return nil
		}); err != nil {
			return nil, fmt.Errorf("更新任务 #%s 的 blockedBy 失败: %w", blockedID, err)
		}
	}
	for _, blockerID := range p.AddBlockedBy {
		if _, err := t.store.UpdateFunc(ctx, blockerID, func(task *agentcore.Task) error {
			task.Blocks = appendUnique(task.Blocks, p.TaskID)
			task.UpdatedAt = time.Now()
			return nil
		}); err != nil {
			return nil, fmt.Errorf("更新任务 #%s 的 blocks 失败: %w", blockerID, err)
		}
	}

	// 主任务：状态/优先级/owner/依赖字段全部在一次原子写入中完成。
	task, err := t.store.UpdateFunc(ctx, p.TaskID, func(task *agentcore.Task) error {
		if p.Status != "" {
			task.Status = agentcore.TaskStatus(p.Status)
		}
		if p.Priority != "" {
			task.Priority = agentcore.TaskPriority(p.Priority)
		}
		if p.Owner != "" {
			task.Owner = p.Owner
		}
		if len(p.AddBlocks) > 0 {
			task.Blocks = appendUnique(task.Blocks, p.AddBlocks...)
		}
		if len(p.AddBlockedBy) > 0 {
			task.BlockedBy = appendUnique(task.BlockedBy, p.AddBlockedBy...)
		}
		task.UpdatedAt = time.Now()
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 发射事件供 TUI 实时刷新
	if t.agent != nil {
		t.agent.EmitEvent(agentcore.NewTaskUpdatedEvent(task, "", string(task.Status)))
	}

	return fmt.Sprintf("任务 #%s 已更新", p.TaskID), nil
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
//
// 注意：此检查基于当前快照，不检测同一批次中新增的边之间是否形成循环。
// 对于 LLM 管理的任务列表，这种边缘情况的影响可接受。
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
