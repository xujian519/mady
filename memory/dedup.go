package memory

import (
	"context"
	"fmt"
)

// DedupAction 表示去重判定的动作类型。
type DedupAction string

const (
	DedupAdd    DedupAction = "add"    // 新增一条独立记忆
	DedupUpdate DedupAction = "update" // 更新已有记忆内容
	DedupDelete DedupAction = "delete" // 删除已有记忆（新信息表明旧信息不再正确）
	DedupNoop   DedupAction = "noop"   // 无操作（新信息与已有记忆完全相同）
)

// DedupResult 是去重判定的结果。
type DedupResult struct {
	Action     DedupAction `json:"action"`
	ExistingID string      `json:"existing_id,omitempty"` // 当 Action 为 update/delete/noop 时，目标记忆 ID
	NewID      string      `json:"new_id,omitempty"`      // 当 Action 为 add 时，新记忆 ID
	Reason     string      `json:"reason"`                // 判定原因
}

// DedupDecider 是去重判定的 LLM 接口。
// 给定新事实和已有记忆列表，判断应采取的动作。
type DedupDecider interface {
	Decide(ctx context.Context, newFact string, existing []ScoredMemory) (DedupAction, string, error)
}

// DedupConfig 控制去重行为。
type DedupConfig struct {
	// SimilarityThreshold 语义相似度阈值。新事实与已有记忆的最高相似度低于此值时，
	// 直接 ADD（不调 LLM）。默认为 0.85。
	SimilarityThreshold float64 `json:"similarity_threshold"`

	// NoopThreshold 在无 LLM 判定的 fallback 模式下，相似度高于此值时 NOOP（不新增）。
	// 默认为 0.9。
	NoopThreshold float64 `json:"noop_threshold"`

	// UpdateThreshold 在无 LLM 判定的 fallback 模式下，相似度在此区间时 UPDATE。
	// 即 (SimilarityThreshold, NoopThreshold] 区间。默认为使用 SimilarityThreshold。
	UpdateThreshold float64 `json:"update_threshold"`

	// Decider 是 LLM 判定器。为 nil 时使用基于相似度的规则 fallback。
	Decider DedupDecider `json:"-"`
}

// DefaultDedupConfig 返回默认去重配置。
func DefaultDedupConfig() DedupConfig {
	return DedupConfig{
		SimilarityThreshold: 0.85,
		NoopThreshold:       0.90,
		UpdateThreshold:     0.85,
	}
}

// ---------------------------------------------------------------------------
// Manager 去重方法
// ---------------------------------------------------------------------------

// SetDedupConfig 配置 Manager 的去重行为。
func (m *Manager) SetDedupConfig(cfg DedupConfig) {
	m.dedupCfg = cfg
}

// Deduplicate 检查新事实是否与已有记忆重复，并执行相应的 ADD/UPDATE/DELETE/NOOP 操作。
//
// 流程：
//  1. 检索 top-3 相似记忆
//  2. 若最高相似度 < SimilarityThreshold → 直接 ADD
//  3. 若有 DedupDecider → 调 LLM 判定动作
//  4. 否则 → fallback 规则判定
//  5. 根据判定结果执行操作并返回 DedupResult
func (m *Manager) Deduplicate(ctx context.Context, content string, scope MemoryScope, layer MemoryLayer) (DedupResult, error) {
	if content == "" {
		return DedupResult{}, fmt.Errorf("memory: cannot deduplicate empty content")
	}

	cfg := m.dedupCfg

	// 1. 检索相似记忆
	filter := MemoryFilter{
		UserID:  scope.UserID,
		AgentID: scope.AgentID,
		Layer:   layer,
		TopK:    3,
	}
	results, err := m.store.Recall(ctx, content, filter)
	if err != nil {
		return DedupResult{}, fmt.Errorf("memory: dedup recall: %w", err)
	}

	// 2. 无相似记忆 → 直接 ADD
	if len(results) == 0 || results[0].Semantic < cfg.SimilarityThreshold {
		id, err := m.store.Remember(ctx, content, scope, layer, nil)
		if err != nil {
			return DedupResult{}, fmt.Errorf("memory: dedup add: %w", err)
		}
		return DedupResult{
			Action: DedupAdd,
			NewID:  id,
			Reason: fmt.Sprintf("无相似记忆（最高相似度 %.2f < 阈值 %.2f）",
				func() float64 {
					if len(results) == 0 {
						return 0.0
					}
					return results[0].Semantic
				}(), cfg.SimilarityThreshold),
		}, nil
	}

	// 3. 调 LLM 判定（若可用）
	if cfg.Decider != nil {
		action, reason, err := cfg.Decider.Decide(ctx, content, results)
		if err != nil {
			// LLM 判定失败时 fallback 到规则判定
			action, reason = ruleBasedDedup(results, cfg)
		}
		return m.applyDedupAction(ctx, action, reason, content, scope, layer, results[0].Entry.ID)
	}

	// 4. Fallback 规则判定
	action, reason := ruleBasedDedup(results, cfg)
	return m.applyDedupAction(ctx, action, reason, content, scope, layer, results[0].Entry.ID)
}

// applyDedupAction 执行去重判定的操作。
func (m *Manager) applyDedupAction(ctx context.Context, action DedupAction, reason, content string, scope MemoryScope, layer MemoryLayer, existingID string) (DedupResult, error) {
	result := DedupResult{
		Action:     action,
		ExistingID: existingID,
		Reason:     reason,
	}

	switch action {
	case DedupAdd:
		id, err := m.store.Remember(ctx, content, scope, layer, nil)
		if err != nil {
			return result, fmt.Errorf("memory: dedup add: %w", err)
		}
		result.NewID = id

	case DedupUpdate:
		if err := m.store.Update(ctx, existingID, content); err != nil {
			return result, fmt.Errorf("memory: dedup update: %w", err)
		}

	case DedupDelete:
		if err := m.store.Forget(ctx, existingID); err != nil {
			return result, fmt.Errorf("memory: dedup delete: %w", err)
		}

	case DedupNoop:
		// 无需操作
	}

	return result, nil
}

// ruleBasedDedup 在无 LLM 时使用基于相似度的规则判定。
//
// 规则：
//   - 相似度 > NoopThreshold → NOOP（几乎相同）
//   - 相似度 > SimilarityThreshold → UPDATE（相似但可能有更新）
//   - 否则 → ADD（fallthrough，理论上不会走到这里因为调用方已检查阈值）
func ruleBasedDedup(results []ScoredMemory, cfg DedupConfig) (DedupAction, string) {
	top := results[0]

	if top.Semantic > cfg.NoopThreshold {
		return DedupNoop,
			fmt.Sprintf("规则判定: 相似度 %.2f > %.2f → 无操作",
				top.Semantic, cfg.NoopThreshold)
	}

	return DedupUpdate,
		fmt.Sprintf("规则判定: 相似度 %.2f 在 (%.2f, %.2f] 区间 → 更新已有记忆 %s",
			top.Semantic, cfg.SimilarityThreshold, cfg.NoopThreshold, top.Entry.ID)
}
