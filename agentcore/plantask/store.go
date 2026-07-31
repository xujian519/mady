package plantask

import (
	"context"
	"fmt"
	"sync"
)

// Store 管理 PlanTaskSession 的持久化。
// 生命周期语义同 ApprovalGate 的 PendingStore：活动状态可更新，已决只读。
type Store interface {
	// Save 创建或覆盖写入会话。会话 ID 必须非空。
	Save(ctx context.Context, s *PlanTaskSession) error
	// Load 按 ID 读取会话（返回 Clone）。不存在时返回错误。
	Load(ctx context.Context, id string) (*PlanTaskSession, error)
	// ListPending 列出所有未终态（非 Finished/Canceled/Expired）会话，供启动恢复。
	ListPending(ctx context.Context) ([]*PlanTaskSession, error)
	// ListByCase 列出某案件的会话（含已决，按创建时间升序）。
	ListByCase(ctx context.Context, caseID string) ([]*PlanTaskSession, error)
	// Delete 删除会话（仅内部清理用）。
	Delete(ctx context.Context, id string) error
}

// MemoryStore 是基于内存的 Store 实现，用于测试和单进程场景。
type MemoryStore struct {
	mu       sync.Mutex
	sessions map[string]*PlanTaskSession
}

// NewMemoryStore 创建一个空的内存存储。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: make(map[string]*PlanTaskSession)}
}

// Save stores a new or updated session. The ID must be non-empty.
func (m *MemoryStore) Save(_ context.Context, s *PlanTaskSession) error {
	if s.ID == "" {
		return fmt.Errorf("plantask: session ID is empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s.Clone()
	return nil
}

// Load reads a cloned session by ID.
func (m *MemoryStore) Load(_ context.Context, id string) (*PlanTaskSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("plantask: session %q not found", id)
	}
	return s.Clone(), nil
}

// ListPending returns all non-terminal sessions, ordered by creation time.
func (m *MemoryStore) ListPending(_ context.Context) ([]*PlanTaskSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*PlanTaskSession
	for _, s := range m.sessions {
		if !isTerminal(s.Status) {
			out = append(out, s.Clone())
		}
	}
	sortSessions(out)
	return out, nil
}

// ListByCase returns all sessions for a case, ordered by creation time.
func (m *MemoryStore) ListByCase(_ context.Context, caseID string) ([]*PlanTaskSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*PlanTaskSession
	for _, s := range m.sessions {
		if s.CaseID == caseID {
			out = append(out, s.Clone())
		}
	}
	sortSessions(out)
	return out, nil
}

// Delete removes a session by ID.
func (m *MemoryStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return fmt.Errorf("plantask: session %q not found", id)
	}
	delete(m.sessions, id)
	return nil
}

// isTerminal 报告状态是否为终态（Finished/Canceled/Expired）。
func isTerminal(s Status) bool {
	return s == StatusFinished || s == StatusCanceled || s == StatusExpired
}

// sortSessions 按创建时间升序排列。
func sortSessions(ss []*PlanTaskSession) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j].CreatedAt.Before(ss[j-1].CreatedAt); j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
