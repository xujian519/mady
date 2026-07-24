package tasklist

import (
	"context"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName 是任务管理扩展的注册名称。
const ExtensionName = "tasklist"

// Extension 将结构化任务管理工具集注入 Agent。
//
// 通过 ToolProvider 贡献 4 个工具：
//   - task_create: 创建待办任务
//   - task_get: 查询单个任务（只读）
//   - task_update: 更新任务状态/优先级/依赖（有副作用）
//   - task_list: 列出所有任务（只读）
//
// task_get 和 task_list 标记为 ReadOnly，在 planmode 下始终可用。
// task_create 和 task_update 不标记 ReadOnly，在 planmode 下被门控阻止。
//
// 与 evidence/planmode/filecheckpoint 等扩展使用相同的装配机制。
type Extension struct {
	store Store
	agent *agentcore.Agent
	mu    sync.Mutex
}

var (
	_ agentcore.Extension             = (*Extension)(nil)
	_ agentcore.ToolProvider          = (*Extension)(nil)
	_ agentcore.EventSnapshotProvider = (*Extension)(nil)
)

// NewExtension 创建一个基于 FileStore 的任务管理扩展。
// baseDir 是任务 JSON 文件的存储目录（通常为 ~/.mady/sessions/<id>/tasks）。
func NewExtension(baseDir string) (*Extension, error) {
	store, err := NewFileStore(baseDir)
	if err != nil {
		return nil, err
	}
	return &Extension{store: store}, nil
}

// NewExtensionWithStore 创建一个使用自定义 Store 的任务管理扩展。
// 用于测试（MemoryStore）或自定义存储后端。
func NewExtensionWithStore(store Store) *Extension {
	return &Extension{store: store}
}

// Name 实现 agentcore.Extension。
func (e *Extension) Name() string { return ExtensionName }

// Init 实现 agentcore.Extension。
func (e *Extension) Init(_ context.Context, agent *agentcore.Agent) error {
	e.agent = agent
	return nil
}

// Dispose 实现 agentcore.Extension。
func (e *Extension) Dispose() error { return nil }

// Tools 实现 agentcore.ToolProvider，返回 4 个任务管理工具。
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		newCreateTool(e.store, &e.mu, e.agent),
		newGetTool(e.store, &e.mu),
		newUpdateTool(e.store, &e.mu, e.agent),
		newListTool(e.store, &e.mu),
	}
}

// SnapshotEvents 实现 agentcore.EventSnapshotProvider，
// 供新挂载的 TUI 获取当前任务状态（将现有任务作为 TaskCreated 事件发射）。
func (e *Extension) SnapshotEvents() []agentcore.Event {
	tasks, err := e.store.List(context.Background(), false)
	if err != nil || len(tasks) == 0 {
		return nil
	}
	events := make([]agentcore.Event, len(tasks))
	for i, t := range tasks {
		events[i] = agentcore.NewTaskCreatedEvent(t)
	}
	return events
}
