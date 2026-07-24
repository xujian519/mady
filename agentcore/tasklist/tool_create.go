package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// TaskCreateToolName 是任务创建工具的名称。
const TaskCreateToolName = "task_create"

// taskCreateTool 创建新的待办任务。
type taskCreateTool struct {
	store Store
	mu    *sync.Mutex
	agent *agentcore.Agent
}

func newCreateTool(store Store, mu *sync.Mutex, agent *agentcore.Agent) *agentcore.Tool {
	t := &taskCreateTool{store: store, mu: mu, agent: agent}
	return &agentcore.Tool{
		Name:        TaskCreateToolName,
		Description: taskCreateDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subject": map[string]any{
					"type":        "string",
					"description": "A brief, actionable title in imperative form (e.g., \"Search prior art\")",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Detailed description of what needs to be done, including context",
				},
				"priority": map[string]any{
					"type":        "string",
					"enum":        []string{"urgent", "high", "normal", "low"},
					"description": "Task priority (default: normal)",
				},
				"active_form": map[string]any{
					"type":        "string",
					"description": "Present continuous form for progress indicator (e.g., \"Searching prior art\")",
				},
			},
			"required":             []string{"subject", "description"},
			"additionalProperties": false,
		},
		Func: t.Run,
	}
}

type taskCreateArgs struct {
	Subject    string `json:"subject"`
	Desc       string `json:"description"`
	Priority   string `json:"priority"`
	ActiveForm string `json:"active_form"`
}

// Run 执行任务创建。
func (t *taskCreateTool) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p taskCreateArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if p.Subject == "" {
		return nil, fmt.Errorf("缺少必填字段 'subject'")
	}
	if p.Desc == "" {
		return nil, fmt.Errorf("缺少必填字段 'description'")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	id, err := t.store.NextID(ctx)
	if err != nil {
		return nil, fmt.Errorf("分配任务 ID 失败: %w", err)
	}

	priority := agentcore.TaskPriorityNormal
	if p.Priority != "" {
		priority = agentcore.TaskPriority(p.Priority)
		if !isValidPriority(priority) {
			return nil, fmt.Errorf("无效的优先级 %q（可选值: urgent/high/normal/low）", p.Priority)
		}
	}

	now := time.Now()
	task := &agentcore.Task{
		ID:          id,
		Subject:     p.Subject,
		Description: p.Desc,
		Status:      agentcore.TaskPending,
		Priority:    priority,
		ActiveForm:  p.ActiveForm,
		Blocks:      []string{},
		BlockedBy:   []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := t.store.Create(ctx, task); err != nil {
		return nil, err
	}

	// 发射事件供 TUI 实时刷新
	if t.agent != nil {
		t.agent.EmitEvent(agentcore.NewTaskCreatedEvent(task))
	}

	return fmt.Sprintf("任务 #%s 已创建: %s（优先级: %s）", id, p.Subject, priority), nil
}

func isValidPriority(p agentcore.TaskPriority) bool {
	switch p {
	case agentcore.TaskPriorityUrgent, agentcore.TaskPriorityHigh,
		agentcore.TaskPriorityNormal, agentcore.TaskPriorityLow:
		return true
	default:
		return false
	}
}
