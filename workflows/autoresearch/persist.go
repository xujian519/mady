package autoresearch

import (
	"context"
	"fmt"
	"sync"
)

// ResearchStore 是自动研究任务的持久化接口。
// 将 ResearchContract 和 Heartbeat 的状态与进程生命周期解耦，
// 支持断点续跑、巡检恢复、审计追溯。
//
// 实现注意事项：
//   - 实现必须线程安全（并发读写研究合约是常态）。
//   - SaveHeartbeat 应当原子性地更新整个 Heartbeat（而非仅 LastBeat 字段），
//     以避免部分写入导致的脏数据。
//   - 未实现的存储应当返回 ErrStoreNotImplemented 而非静默失败。
type ResearchStore interface {
	// SaveContract 保存或更新研究合约。
	// 合约已存在时覆盖旧值。
	SaveContract(ctx context.Context, c *ResearchContract) error

	// LoadContract 按 ID 加载研究合约。
	// 合约不存在时返回 ErrContractNotFound。
	LoadContract(ctx context.Context, id string) (*ResearchContract, error)

	// SaveHeartbeat 保存或更新心跳。
	// ID 为空时由实现自动生成。
	SaveHeartbeat(ctx context.Context, h *Heartbeat) error

	// ListActive 列出所有运行中或已暂停的合约。
	// 用于巡检恢复和系统重启后的状态重建。
	ListActive(ctx context.Context) ([]*ResearchContract, error)

	// DeleteContract 删除指定 ID 的研究合约。
	// 合约不存在时返回 nil（幂等删除）。
	DeleteContract(ctx context.Context, id string) error
}

// Sentinel 错误，供 ResearchStore 实现返回。
var (
	ErrContractNotFound    = fmt.Errorf("autoresearch: contract not found")
	ErrStoreNotImplemented = fmt.Errorf("autoresearch: store not implemented")
)

// =============================================================================
// In-memory 实现（默认、测试用途）
// =============================================================================

// InMemoryResearchStore 是 ResearchStore 的内存实现。
// 进程重启后数据丢失，适合测试和单次会话场景。
// 生产环境应替换为 SQLite 或 JSON 文件实现。
type InMemoryResearchStore struct {
	mu         sync.RWMutex
	contracts  map[string]*ResearchContract
	heartbeats map[string]*Heartbeat
}

// NewInMemoryResearchStore 创建一个空的内存研究存储。
func NewInMemoryResearchStore() *InMemoryResearchStore {
	return &InMemoryResearchStore{
		contracts:  make(map[string]*ResearchContract),
		heartbeats: make(map[string]*Heartbeat),
	}
}

// SaveContract 保存研究合约到内存。
func (s *InMemoryResearchStore) SaveContract(_ context.Context, c *ResearchContract) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 深拷贝：防止外部修改已存储的合约。
	s.contracts[c.ID] = deepCopyContract(c)
	return nil
}

// LoadContract 从内存加载研究合约。
func (s *InMemoryResearchStore) LoadContract(_ context.Context, id string) (*ResearchContract, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.contracts[id]
	if !ok {
		return nil, ErrContractNotFound
	}
	return deepCopyContract(c), nil
}

// SaveHeartbeat 保存心跳到内存。
func (s *InMemoryResearchStore) SaveHeartbeat(_ context.Context, h *Heartbeat) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heartbeats[h.ContractID] = deepCopyHeartbeat(h)
	return nil
}

// ListActive 列出所有运行中或已暂停的合约。
func (s *InMemoryResearchStore) ListActive(_ context.Context) ([]*ResearchContract, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var active []*ResearchContract
	for _, c := range s.contracts {
		if c.Status == StatusRunning || c.Status == StatusPaused {
			active = append(active, deepCopyContract(c))
		}
	}
	// 保证返回非 nil 切片便于调用方 range 遍历。
	if active == nil {
		return []*ResearchContract{}, nil
	}
	return active, nil
}

// DeleteContract 从内存删除研究合约。
func (s *InMemoryResearchStore) DeleteContract(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.contracts, id)
	delete(s.heartbeats, id)
	return nil
}

// Count 返回当前存储中的合约总数（用于测试和监控）。
func (s *InMemoryResearchStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.contracts)
}

// =============================================================================
// 深拷贝辅助（防止外部修改已持久化的状态）
// =============================================================================

func deepCopyContract(c *ResearchContract) *ResearchContract {
	cp := &ResearchContract{
		ID:           c.ID,
		Goal:         c.Goal,
		Domain:       c.Domain,
		MaxRounds:    c.MaxRounds,
		MaxDuration:  c.MaxDuration,
		Status:       c.Status,
		CurrentRound: c.CurrentRound,
		StartedAt:    c.StartedAt,
		AbortReason:  c.AbortReason,
	}
	if c.PausedAt != nil {
		t := *c.PausedAt
		cp.PausedAt = &t
	}
	if c.CompletedAt != nil {
		t := *c.CompletedAt
		cp.CompletedAt = &t
	}
	if c.SuccessCriteria != nil {
		cp.SuccessCriteria = make([]SuccessCriterion, len(c.SuccessCriteria))
		copy(cp.SuccessCriteria, c.SuccessCriteria)
	}
	if c.Evidence != nil {
		cp.Evidence = make([]Evidence, len(c.Evidence))
		for i, ev := range c.Evidence {
			cp.Evidence[i] = deepCopyEvidence(ev)
		}
	}
	if c.Directions != nil {
		cp.Directions = make([]DirectionChange, len(c.Directions))
		copy(cp.Directions, c.Directions)
	}
	return cp
}

func deepCopyEvidence(e Evidence) Evidence {
	findings := make([]string, len(e.Findings))
	copy(findings, e.Findings)
	tools := make([]string, len(e.ToolsUsed))
	copy(tools, e.ToolsUsed)
	return Evidence{
		Round:     e.Round,
		Summary:   e.Summary,
		Findings:  findings,
		ToolsUsed: tools,
	}
}

func deepCopyHeartbeat(h *Heartbeat) *Heartbeat {
	return &Heartbeat{
		ContractID: h.ContractID,
		LastBeat:   h.LastBeat,
		Interval:   h.Interval,
		Timeout:    h.Timeout,
		BeatCount:  h.BeatCount,
		IsStale:    h.IsStale,
	}
}
