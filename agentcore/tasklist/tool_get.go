package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// TaskGetToolName 是任务查询工具的名称。
const TaskGetToolName = "task_get"

// taskGetTool 查询单个任务详情。
type taskGetTool struct {
	store Store
}

func newGetTool(store Store) *agentcore.Tool {
	t := &taskGetTool{store: store}
	return &agentcore.Tool{
		Name:        TaskGetToolName,
		Description: taskGetDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "The ID of the task to retrieve",
				},
			},
			"required":             []string{"task_id"},
			"additionalProperties": false,
		},
		ReadOnly: true,
		Func:     t.Run,
	}
}

type taskGetArgs struct {
	TaskID string `json:"task_id"`
}

// Run 查询任务详情。
func (t *taskGetTool) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p taskGetArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if !isValidTaskID(p.TaskID) {
		return nil, fmt.Errorf("无效的任务 ID %q（应为纯数字）", p.TaskID)
	}

	task, err := t.store.Get(ctx, p.TaskID)
	if err != nil {
		return nil, err
	}

	return formatTaskDetail(task), nil
}

// formatTaskDetail 将任务格式化为人类可读的字符串。
func formatTaskDetail(t *agentcore.Task) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "任务 #%s: %s\n", t.ID, t.Subject)
	fmt.Fprintf(&sb, "状态: %s\n", t.Status)
	fmt.Fprintf(&sb, "优先级: %s\n", t.Priority)
	fmt.Fprintf(&sb, "描述: %s\n", t.Description)

	if len(t.BlockedBy) > 0 {
		fmt.Fprintf(&sb, "被阻塞: %s\n", formatIDs(t.BlockedBy))
	}
	if len(t.Blocks) > 0 {
		fmt.Fprintf(&sb, "阻塞: %s\n", formatIDs(t.Blocks))
	}
	if t.Owner != "" {
		fmt.Fprintf(&sb, "所有者: %s\n", t.Owner)
	}
	if t.ActiveForm != "" {
		fmt.Fprintf(&sb, "进行时文案: %s\n", t.ActiveForm)
	}
	return sb.String()
}

// formatIDs 将 ID 列表格式化为 "#1, #2" 形式。
func formatIDs(ids []string) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = "#" + id
	}
	return strings.Join(parts, ", ")
}
