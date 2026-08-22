// Package patent — standardized legal reasoning patterns from the patent
// re-examination knowledge base. Each pattern encodes a canonical reasoning
// template that patent agents and examiners follow during invalidation and
// examination proceedings. Patterns are grouped by category (creativity /
// novelty / claims / other) and carry metadata about their usage frequency
// and invalidation success rate.
package patent

import "github.com/xujian519/mady/evaluate"

// ReasoningPattern represents a standardized legal reasoning template from
// the patent re-examination knowledge base. Each pattern corresponds to one
// of the 18 canonical reasoning forms used by patent examiners and agents.
type ReasoningPattern struct {
	ID               string      // unique pattern identifier (e.g. "RP-CREATIVITY-01")
	Category         string      // "creativity" / "novelty" / "claims" / "other"
	Name             string      // Chinese display name
	Frequency        float64     // occurrence frequency in practice (%)
	InvalidationRate float64     // success rate when used for invalidation (%)
	CoreLogic        string      // core reasoning logic description
	CheckRules       []CheckRule // associated deterministic check rules
	Template         string      // canonical reasoning template text (Chinese)
}

// AllPatterns returns all 18 standardized reasoning patterns ordered by
// priority batch: Creativity (1-6), Novelty (7-9), Claims (10-14), Other (15-18).
func AllPatterns() []ReasoningPattern {
	// 返回副本，防止调用方修改共享数据（数据本体见 reasoning_patterns_data.go）。
	out := make([]ReasoningPattern, len(patternsData))
	copy(out, patternsData)
	return out
}

// PatternsByCategory filters AllPatterns results by category.
// Valid categories: "creativity", "novelty", "claims", "other".
// An empty string returns all patterns.
func PatternsByCategory(category string) []ReasoningPattern {
	all := AllPatterns()
	if category == "" {
		return all
	}
	var out []ReasoningPattern
	for _, p := range all {
		if p.Category == category {
			out = append(out, p)
		}
	}
	return out
}

// PatternInfos 将本领域的推理模式摘要为 evaluate.PatternInfo 列表，
// 供组装期注入 evaluate.NewReasoningPatternCoverage。依赖方向：
// 领域层 → evaluate（评估器保持领域无关）。
func PatternInfos() []evaluate.PatternInfo {
	all := AllPatterns()
	out := make([]evaluate.PatternInfo, 0, len(all))
	for _, p := range all {
		out = append(out, evaluate.PatternInfo{
			ID:        p.ID,
			Name:      p.Name,
			Category:  p.Category,
			RuleCount: len(p.CheckRules),
		})
	}
	return out
}
