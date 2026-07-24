package agentcore

import "time"

// =============================================================================
// Task 数据模型 — 定义在 agentcore 包中以避免循环导入。
// tasklist 子包引用这些类型，事件类型也直接引用（同包）。
// =============================================================================

// TaskStatus 表示任务的生命周期状态。
type TaskStatus string

const (
	// TaskPending 是初始状态：任务已创建但尚未开始。
	TaskPending TaskStatus = "pending"
	// TaskInProgress 表示任务正在执行中。
	TaskInProgress TaskStatus = "in_progress"
	// TaskCompleted 表示任务已完成。
	TaskCompleted TaskStatus = "completed"
	// TaskArchived 表示任务已归档（保留审计留痕，不物理删除）。
	// List 默认不返回 archived 任务，但可通过参数查询。
	TaskArchived TaskStatus = "archived"
)

// TaskPriority 表示任务的优先级。
type TaskPriority string

const (
	// TaskPriorityLow 是最低优先级。
	TaskPriorityLow TaskPriority = "low"
	// TaskPriorityNormal 是默认优先级。
	TaskPriorityNormal TaskPriority = "normal"
	// TaskPriorityHigh 表示较高优先级。
	TaskPriorityHigh TaskPriority = "high"
	// TaskPriorityUrgent 是最高优先级。
	TaskPriorityUrgent TaskPriority = "urgent"
)

// Task 表示一个结构化待办事项。
// 由 tasklist 工具集（TaskCreate/TaskGet/TaskUpdate/TaskList）管理，
// 供 LLM 在处理复杂多步骤任务时自行规划和追踪进度。
type Task struct {
	// ID 是单调递增的任务标识符（"1"、"2"、…）。
	ID string `json:"id"`
	// Subject 是祈使句形式的简短标题（如"检索现有技术"）。
	Subject string `json:"subject"`
	// Description 是详细描述，包含上下文和验收标准。
	Description string `json:"description"`
	// Status 是当前生命周期状态。
	Status TaskStatus `json:"status"`
	// Priority 控制任务在列表中的排序（urgent > high > normal > low）。
	Priority TaskPriority `json:"priority"`
	// Blocks 列出本任务阻塞的任务 ID（这些任务在本任务完成前不能开始）。
	Blocks []string `json:"blocks,omitempty"`
	// BlockedBy 列出阻塞本任务的任务 ID（这些任务完成后本任务才能开始）。
	BlockedBy []string `json:"blocked_by,omitempty"`
	// ActiveForm 是 in_progress 状态下在进度指示器中显示的现在进行时文案
	// （如"正在检索现有技术"）。
	ActiveForm string `json:"active_form,omitempty"`
	// Owner 是认领该任务的 Agent 名称（多 Agent 协作时使用）。
	Owner string `json:"owner,omitempty"`
	// Metadata 是自由扩展字段（如领域标签、关联文件等）。
	Metadata map[string]any `json:"metadata,omitempty"`
	// CreatedAt 是任务创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 是任务最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// Clone 返回 Task 的深拷贝，包括切片和 map。
func (t *Task) Clone() *Task {
	if t == nil {
		return nil
	}
	cp := *t
	if t.Blocks != nil {
		cp.Blocks = append([]string(nil), t.Blocks...)
	}
	if t.BlockedBy != nil {
		cp.BlockedBy = append([]string(nil), t.BlockedBy...)
	}
	if t.Metadata != nil {
		cp.Metadata = make(map[string]any, len(t.Metadata))
		for k, v := range t.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}

// PriorityOrder 返回优先级的数值用于排序（越大越优先）。
func (p TaskPriority) Order() int {
	switch p {
	case TaskPriorityUrgent:
		return 4
	case TaskPriorityHigh:
		return 3
	case TaskPriorityNormal:
		return 2
	case TaskPriorityLow:
		return 1
	default:
		return 0
	}
}
