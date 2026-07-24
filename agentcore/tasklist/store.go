// Package tasklist 提供结构化任务管理工具集（TaskCreate/TaskGet/TaskUpdate/TaskList），
// 让 LLM 在处理复杂多步骤任务时自行规划和追踪进度。
//
// 作为 agentcore.Extension 注入，通过 ToolProvider 接口贡献工具，
// 与 evidence/planmode/filecheckpoint 等扩展使用相同的装配机制。
package tasklist

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/xujian519/mady/agentcore"
)

// Store 是任务持久化的抽象接口。
// 实现可以是文件系统（FileStore）或内存（MemoryStore，用于测试）。
// 所有方法必须安全并发调用。
type Store interface {
	// Create 将新任务写入存储。t.ID 必须已由 NextID 分配。
	Create(ctx context.Context, t *agentcore.Task) error
	// Get 按 ID 读取单个任务。不存在时返回错误。
	Get(ctx context.Context, id string) (*agentcore.Task, error)
	// Update 覆盖写入已有任务。
	Update(ctx context.Context, t *agentcore.Task) error
	// List 返回所有非归档任务，按优先级降序 + ID 升序排列。
	// includeArchived 为 true 时也返回归档任务。
	List(ctx context.Context, includeArchived bool) ([]*agentcore.Task, error)
	// Delete 按 ID 删除任务（仅用于内部清理，工具层使用 archived 状态）。
	Delete(ctx context.Context, id string) error
	// NextID 返回下一个单调递增的 ID 字符串（"1"、"2"、…）。
	NextID(ctx context.Context) (string, error)
}

// MemoryStore 是基于内存的 Store 实现，用于测试和单进程场景。
// 设置 MaxTasks 限制最大任务数（0 表示无限），超出时最旧任务自动归档。
type MemoryStore struct {
	mu       sync.Mutex
	tasks    map[string]*agentcore.Task
	nextID   atomic.Int64
	MaxTasks int
}

// NewMemoryStore 创建一个空的内存存储。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasks: make(map[string]*agentcore.Task)}
}

func (m *MemoryStore) Create(_ context.Context, t *agentcore.Task) error {
	if t.ID == "" {
		return fmt.Errorf("tasklist: task ID is empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.tasks[t.ID]; exists {
		return fmt.Errorf("tasklist: task #%s already exists", t.ID)
	}
	m.tasks[t.ID] = t.Clone()
	return nil
}

func (m *MemoryStore) Get(_ context.Context, id string) (*agentcore.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, fmt.Errorf("tasklist: task #%s not found", id)
	}
	return t.Clone(), nil
}

func (m *MemoryStore) Update(_ context.Context, t *agentcore.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.tasks[t.ID]; !exists {
		return fmt.Errorf("tasklist: task #%s not found", t.ID)
	}
	m.tasks[t.ID] = t.Clone()
	return nil
}

func (m *MemoryStore) List(_ context.Context, includeArchived bool) ([]*agentcore.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*agentcore.Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		if !includeArchived && t.Status == agentcore.TaskArchived {
			continue
		}
		result = append(result, t.Clone())
	}
	sortTasks(result)
	return result, nil
}

func (m *MemoryStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.tasks[id]; !exists {
		return fmt.Errorf("tasklist: task #%s not found", id)
	}
	delete(m.tasks, id)
	return nil
}

func (m *MemoryStore) NextID(_ context.Context) (string, error) {
	id := m.nextID.Add(1)
	return fmt.Sprintf("%d", id), nil
}

// sortTasks 按优先级降序 + ID 升序排列。
func sortTasks(tasks []*agentcore.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		pi := tasks[i].Priority.Order()
		pj := tasks[j].Priority.Order()
		if pi != pj {
			return pi > pj
		}
		return tasks[i].ID < tasks[j].ID
	})
}
