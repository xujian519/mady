package registry

import (
	"fmt"
	"sync"
	"time"
)

// Registration 是 Agent 注册信息，描述一个 Agent 的网络位置、能力和元数据。
type Registration struct {
	Name         string            `json:"name"`
	Version      string            `json:"version,omitempty"`
	URL          string            `json:"url"`
	Capabilities []string          `json:"capabilities,omitempty"` // "streaming", "push", "chat", "patent" 等
	Description  string            `json:"description,omitempty"`
	Skills       []string          `json:"skills,omitempty"`   // 技能列表
	HeartbeatAt  time.Time         `json:"heartbeat_at"`       // 最后心跳时间
	Metadata     map[string]string `json:"metadata,omitempty"` // 附加元数据
}

// Registry 是 Agent 注册表的接口抽象。
// 支持按名称注册/注销，以及按能力和技能进行高效查询。
// New() 返回 *InMemoryRegistry（默认内存实现），
// 可替换为基于 etcd/Redis 的分布式实现。
type Registry interface {
	Register(reg *Registration) error
	Deregister(name string)
	Get(name string) (*Registration, bool)
	List() []*Registration
	ListByCapability(capability string) []*Registration
	ListBySkill(skill string) []*Registration
	Count() int
}

// InMemoryRegistry 是 Registry 的线程安全内存实现。
type InMemoryRegistry struct {
	mu      sync.RWMutex
	agents  map[string]*Registration            // key = name
	bySkill map[string]map[string]*Registration // skill → name → reg
}

// New 创建一个空的内存注册表。
func New() *InMemoryRegistry {
	return &InMemoryRegistry{
		agents:  make(map[string]*Registration),
		bySkill: make(map[string]map[string]*Registration),
	}
}

// Register 注册一个 Agent。Name 字段必填，否则返回错误。
// 如果同名 Agent 已存在，则覆盖旧记录并更新 bySkill 索引。
func (r *InMemoryRegistry) Register(reg *Registration) error {
	if reg.Name == "" {
		return fmt.Errorf("registry: registration name is required")
	}
	if reg.URL == "" {
		return fmt.Errorf("registry: registration url is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 清理旧的 skill 索引
	if old, ok := r.agents[reg.Name]; ok {
		for _, skill := range old.Skills {
			delete(r.bySkill[skill], reg.Name)
			if len(r.bySkill[skill]) == 0 {
				delete(r.bySkill, skill)
			}
		}
	}

	reg.HeartbeatAt = time.Now()

	// 浅拷贝，避免外部修改影响注册表
	entry := *reg
	r.agents[reg.Name] = &entry

	// 更新 bySkill 索引
	for _, skill := range reg.Skills {
		if r.bySkill[skill] == nil {
			r.bySkill[skill] = make(map[string]*Registration)
		}
		r.bySkill[skill][reg.Name] = &entry
	}

	return nil
}

// Deregister 从注册表移除指定名称的 Agent，同时清理相关索引。
func (r *InMemoryRegistry) Deregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	old, ok := r.agents[name]
	if !ok {
		return
	}

	for _, skill := range old.Skills {
		delete(r.bySkill[skill], name)
		if len(r.bySkill[skill]) == 0 {
			delete(r.bySkill, skill)
		}
	}

	delete(r.agents, name)
}

// List 返回注册表中所有 Agent 的副本列表。
func (r *InMemoryRegistry) List() []*Registration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Registration, 0, len(r.agents))
	for _, reg := range r.agents {
		cp := *reg
		result = append(result, &cp)
	}
	return result
}

// Get 按名称查询 Agent。返回注册信息的副本和安全布尔值。
func (r *InMemoryRegistry) Get(name string) (*Registration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, ok := r.agents[name]
	if !ok {
		return nil, false
	}
	cp := *reg
	return &cp, true
}

// ListByCapability 返回所有具备指定能力的 Agent 列表。
// 遍历所有 Agent 进行匹配。
func (r *InMemoryRegistry) ListByCapability(capability string) []*Registration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Registration
	for _, reg := range r.agents {
		for _, cap := range reg.Capabilities {
			if cap == capability {
				cp := *reg
				result = append(result, &cp)
				break
			}
		}
	}
	return result
}

// ListBySkill 返回所有具备指定技能的 Agent 列表。
// 使用二级索引实现 O(1) 查询。
func (r *InMemoryRegistry) ListBySkill(skill string) []*Registration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills, ok := r.bySkill[skill]
	if !ok {
		return nil
	}

	result := make([]*Registration, 0, len(skills))
	for _, reg := range skills {
		cp := *reg
		result = append(result, &cp)
	}
	return result
}

// Count 返回当前注册的 Agent 数量。
func (r *InMemoryRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.agents)
}
