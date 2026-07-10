package domains

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
)

const (
	// maxCachedProjects 是 AgentPool 最大缓存的案件 Agent 数。
	maxCachedProjects = 50

	// defaultProjectIdleTTL 是案件 Agent 的空闲超时时间。
	defaultProjectIdleTTL = 30 * time.Minute
)

// ProjectAgentEntry 是 AgentPool 中的一个缓存条目。
type ProjectAgentEntry struct {
	Agent     *agentcore.Agent
	LastUsed  time.Time
	CreatedAt time.Time
	HitCount  int64
}

// AgentPool 管理案件专属 Agent 实例的生命周期。
//
// 设计要点：
// - 惰性创建：首次 Handoff 到某案件时才创建 Agent
// - 空闲超时：超时未使用的 Agent 被自动清理
// - 并发安全：通过读写锁保护 agent map
// - 上限保护：最多缓存 maxCachedProjects 个 Agent；超出上限时驱逐最旧的条目
type AgentPool struct {
	mu     sync.RWMutex
	agents map[string]*ProjectAgentEntry
	base   agentcore.Config

	idleTTL time.Duration
	stopCh  chan struct{}
	closed  atomic.Bool
}

// NewAgentPool 创建一个 AgentPool。
// base 是 Agent 的基础配置，BuildProjectAgent 会在其上叠加案件特化配置。
func NewAgentPool(base agentcore.Config) *AgentPool {
	p := &AgentPool{
		agents:  make(map[string]*ProjectAgentEntry),
		base:    base,
		idleTTL: defaultProjectIdleTTL,
		stopCh:  make(chan struct{}),
	}
	go p.reaperLoop()
	return p
}

// Close 关闭 AgentPool，释放所有 Agent 资源。
// 先收集所有 agent，再在锁外逐个关闭，避免持锁时间过长。
func (p *AgentPool) Close() {
	if !p.closed.CompareAndSwap(false, true) {
		return // 已经关闭
	}
	close(p.stopCh)

	// 在锁内收集所有 agent
	p.mu.Lock()
	agents := make([]*agentcore.Agent, 0, len(p.agents))
	for id, entry := range p.agents {
		agents = append(agents, entry.Agent)
		delete(p.agents, id)
	}
	p.mu.Unlock()

	// 在锁外逐个关闭，避免 agent.Close() 阻塞时锁住整个池
	for _, agent := range agents {
		agent.Close()
	}
}

// GetOrCreate 获取或创建指定案件的 Agent。
// rec 是案件记录，用于 BuildProjectAgent 构建特化 Agent。
//
// 返回的 Agent 由 AgentPool 持有生命周期，调用方不需、也不应调用 Close。
// Agent 可被后续同案件的 Handoff 安全复用。
func (p *AgentPool) GetOrCreate(rec ProjectRecord) (*agentcore.Agent, error) {
	// 已存在且未超时
	p.mu.RLock()
	if entry, ok := p.agents[rec.ProjectID]; ok {
		entry.LastUsed = time.Now()
		atomic.AddInt64(&entry.HitCount, 1)
		p.mu.RUnlock()
		return entry.Agent, nil
	}
	p.mu.RUnlock()

	// 创建新 Agent
	cfg := BuildProjectAgent(rec, p.base)
	agent := agentcore.New(cfg)

	p.mu.Lock()
	defer p.mu.Unlock()

	// 竞态检查：其他 goroutine 可能已先创建
	if existing, ok := p.agents[rec.ProjectID]; ok {
		// 在锁内收集待关闭 agent，在锁外执行关闭
		dup := agent
		p.mu.Unlock()
		dup.Close()
		p.mu.Lock()

		existing.LastUsed = time.Now()
		atomic.AddInt64(&existing.HitCount, 1)
		return existing.Agent, nil
	}

	// 超出上限时驱逐最旧的条目，保持池大小可控
	if len(p.agents) >= maxCachedProjects {
		var oldestID string
		var oldestTime time.Time
		for id, entry := range p.agents {
			if oldestID == "" || entry.LastUsed.Before(oldestTime) {
				oldestID = id
				oldestTime = entry.LastUsed
			}
		}
		if oldestID != "" {
			if entry, ok := p.agents[oldestID]; ok {
				old := entry.Agent
				delete(p.agents, oldestID)
				p.mu.Unlock()
				old.Close()
				p.mu.Lock()
			}
		}
	}

	entry := &ProjectAgentEntry{
		Agent:     agent,
		LastUsed:  time.Now(),
		CreatedAt: time.Now(),
	}
	p.agents[rec.ProjectID] = entry
	return agent, nil
}

// Evict 主动移除并关闭指定案件的 Agent。
func (p *AgentPool) Evict(projectID string) {
	p.mu.Lock()
	entry, ok := p.agents[projectID]
	if ok {
		delete(p.agents, projectID)
	}
	p.mu.Unlock()

	if ok {
		entry.Agent.Close()
	}
}

// Stats 返回池的统计信息。
func (p *AgentPool) Stats() AgentPoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	totalHitCount := int64(0)
	entries := make([]ProjectAgentInfo, 0, len(p.agents))
	for id, entry := range p.agents {
		totalHitCount += atomic.LoadInt64(&entry.HitCount)
		entries = append(entries, ProjectAgentInfo{
			ProjectID: id,
			Age:       time.Since(entry.CreatedAt),
			IdleTime:  time.Since(entry.LastUsed),
			HitCount:  atomic.LoadInt64(&entry.HitCount),
		})
	}
	return AgentPoolStats{
		TotalAgents:   len(p.agents),
		TotalHitCount: totalHitCount,
		Agents:        entries,
	}
}

// reaperLoop 定期清理超时的 Agent。
func (p *AgentPool) reaperLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.reap()
		case <-p.stopCh:
			return
		}
	}
}

func (p *AgentPool) reap() {
	now := time.Now()

	// 在锁内收集过期 agent，在锁外关闭
	p.mu.Lock()
	var expired []*agentcore.Agent
	for id, entry := range p.agents {
		if now.Sub(entry.LastUsed) > p.idleTTL {
			expired = append(expired, entry.Agent)
			delete(p.agents, id)
		}
	}
	p.mu.Unlock()

	for _, agent := range expired {
		agent.Close()
	}
}

// --- 类型定义 ---

// AgentPoolStats 是 AgentPool 的统计快照。
type AgentPoolStats struct {
	TotalAgents   int                `json:"total_agents"`
	TotalHitCount int64              `json:"total_hit_count"`
	Agents        []ProjectAgentInfo `json:"agents,omitempty"`
}

// ProjectAgentInfo 是单个案件 Agent 的信息。
type ProjectAgentInfo struct {
	ProjectID string        `json:"project_id"`
	Age       time.Duration `json:"age"`
	IdleTime  time.Duration `json:"idle_time"`
	HitCount  int64         `json:"hit_count"`
}

// NewTestAgentPool 创建测试用 AgentPool（空闲超时 1 秒）。
func NewTestAgentPool(base agentcore.Config) *AgentPool {
	p := &AgentPool{
		agents:  make(map[string]*ProjectAgentEntry),
		base:    base,
		idleTTL: 1 * time.Second,
		stopCh:  make(chan struct{}),
	}
	go p.reaperLoop()
	return p
}
