package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// TaskListToolName 是任务列表工具的名称。
const TaskListToolName = "task_list"

// taskListTool 列出所有任务。
type taskListTool struct {
	store Store
	mu    *sync.Mutex
}

func newListTool(store Store, mu *sync.Mutex) *agentcore.Tool {
	t := &taskListTool{store: store, mu: mu}
	return &agentcore.Tool{
		Name:        TaskListToolName,
		Description: taskListDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"include_archived": map[string]any{
					"type":        "boolean",
					"description": "Set to true to include archived tasks (default: false)",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		ReadOnly: true,
		Func:     t.Run,
	}
}

type taskListArgs struct {
	IncludeArchived bool `json:"include_archived"`
}

// Run 列出所有任务。
func (t *taskListTool) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p taskListArgs
	// 参数可为空对象，忽略解析错误
	_ = json.Unmarshal(args, &p)

	t.mu.Lock()
	defer t.mu.Unlock()

	tasks, err := t.store.List(ctx, p.IncludeArchived)
	if err != nil {
		return nil, err
	}

	if len(tasks) == 0 {
		return "暂无任务。", nil
	}

	var sb strings.Builder
	// 统计摘要
	pending, inProgress, completed, archived := countByStatus(tasks)
	fmt.Fprintf(&sb, "总计 %d 个任务 | 待处理: %d | 进行中: %d | 已完成: %d", len(tasks), pending, inProgress, completed)
	if archived > 0 {
		fmt.Fprintf(&sb, " | 已归档: %d", archived)
	}
	sb.WriteString("\n\n")

	for i, task := range tasks {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "#%s [%s] %s (优先级: %s)", task.ID, task.Status, task.Subject, task.Priority)
		if task.Owner != "" {
			fmt.Fprintf(&sb, " [owner: %s]", task.Owner)
		}
		if len(task.BlockedBy) > 0 {
			fmt.Fprintf(&sb, " [被阻塞: %s]", formatIDs(task.BlockedBy))
		}
	}

	return sb.String(), nil
}

func countByStatus(tasks []*agentcore.Task) (pending, inProgress, completed, archived int) {
	for _, t := range tasks {
		switch t.Status {
		case agentcore.TaskPending:
			pending++
		case agentcore.TaskInProgress:
			inProgress++
		case agentcore.TaskCompleted:
			completed++
		case agentcore.TaskArchived:
			archived++
		}
	}
	return
}
