package memory

import (
	"sort"
)

// RRFConfig 控制 Reciprocal Rank Fusion 的行为。
type RRFConfig struct {
	// K 是 RRF 平滑参数，默认 60（标准值）。
	K int `json:"k"`
}

// DefaultRRFConfig 返回标准 RRF 配置。
func DefaultRRFConfig() RRFConfig {
	return RRFConfig{K: 60}
}

// RRFFusion 对稠密检索和稀疏检索的结果进行 Reciprocal Rank Fusion。
//
// 融合公式：RRF_score(d) = Σ_{r in rankings} 1 / (K + rank_dr)
// 其中 rank_dr 是文档 d 在排名 r 中的位置（1-indexed），K 是平滑参数。
//
// 两个结果集可以为空（但不会同时为空），融合后按 RRF 分数降序排列。
func RRFFusion(dense, sparse []ScoredMemory, cfg RRFConfig) []ScoredMemory {
	if len(dense) == 0 && len(sparse) == 0 {
		return nil
	}

	k := float64(cfg.K)
	if k <= 0 {
		k = 60
	}

	// 构建 entryID → 融合分数 的 map
	rrfScores := make(map[string]float64)
	// 保留原始 ScoredMemory 引用（取 dense 和 sparse 中首次出现的）
	entries := make(map[string]*ScoredMemory)

	// 处理 dense 排名
	for i, sm := range dense {
		rank := float64(i + 1) // 1-indexed
		eid := sm.Entry.ID
		rrfScores[eid] += 1.0 / (k + rank)
		if _, exists := entries[eid]; !exists {
			cp := sm
			entries[eid] = &cp
		}
	}

	// 处理 sparse 排名
	for i, sm := range sparse {
		rank := float64(i + 1)
		eid := sm.Entry.ID
		rrfScores[eid] += 1.0 / (k + rank)
		if _, exists := entries[eid]; !exists {
			cp := sm
			entries[eid] = &cp
		}
	}

	// 按 RRF 分数降序排列
	type fused struct {
		entry *ScoredMemory
		score float64
	}
	fusedList := make([]fused, 0, len(rrfScores))
	for eid, score := range rrfScores {
		fusedList = append(fusedList, fused{entry: entries[eid], score: score})
	}

	sort.Slice(fusedList, func(i, j int) bool {
		return fusedList[i].score > fusedList[j].score
	})

	// 设置 Composite 为 RRF 分数（后续仍可叠加复合评分）
	result := make([]ScoredMemory, len(fusedList))
	for i, f := range fusedList {
		result[i] = *f.entry
		result[i].Composite = f.score
		result[i].Rank = i
	}

	return result
}
