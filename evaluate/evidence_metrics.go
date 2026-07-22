package evaluate

import (
	"math"
)

// EvidenceJudgmentAccuracy 评估证据判断的准确性。
// 分数为预测结论与参考结论一致的比例。
type EvidenceJudgmentAccuracy struct {
	// TotalJudgments 总判断次数
	TotalJudgments int
	// CorrectJudgments 正确判断次数
	CorrectJudgments int
}

func (m *EvidenceJudgmentAccuracy) Name() string {
	return "evidence_judgment_accuracy"
}

// Compute 计算判断准确率。
func (m *EvidenceJudgmentAccuracy) Compute(prediction, reference string) float64 {
	if m.TotalJudgments == 0 {
		return 0
	}
	if m.CorrectJudgments > m.TotalJudgments {
		m.CorrectJudgments = m.TotalJudgments
	}
	return float64(m.CorrectJudgments) / float64(m.TotalJudgments)
}

// AddResult 添加一次判断结果。
func (m *EvidenceJudgmentAccuracy) AddResult(correct bool) {
	m.TotalJudgments++
	if correct {
		m.CorrectJudgments++
	}
}

// EvidenceTypeCoverage 评估证据类型覆盖的完整度。
// 覆盖全部 12 种证据类型时得 1 分，否则按比例扣分。
type EvidenceTypeCoverage struct {
	CoveredTypes []string
	AllTypes     []string
}

func NewEvidenceTypeCoverage() *EvidenceTypeCoverage {
	return &EvidenceTypeCoverage{
		AllTypes: []string{
			"general", "foreign_language", "overseas", "electronic",
			"witness_testimony", "expert_opinion", "common_knowledge",
			"notarial_certificate", "burden_of_proof", "standard_of_proof",
			"prior_art_date", "procedural",
		},
	}
}

func (m *EvidenceTypeCoverage) Name() string {
	return "evidence_type_coverage"
}

// Compute 计算证据类型覆盖完整度。
func (m *EvidenceTypeCoverage) Compute(prediction, reference string) float64 {
	all := m.AllTypes
	if len(all) == 0 {
		return 1
	}

	covered := make(map[string]bool)
	for _, t := range m.CoveredTypes {
		covered[t] = true
	}

	hit := 0
	for _, t := range all {
		if covered[t] {
			hit++
		}
	}

	return float64(hit) / float64(len(all))
}

// EvidenceReasoningCompleteness 评估证据推理过程的完整性。
// 根据推理过程中是否包含三性审查、类型特定评估、举证责任分析和证明标准判断来衡量。
type EvidenceReasoningCompleteness struct {
	// RequiredSections 推理过程中应包含的章节
	RequiredSections []string
	// FoundSections 实际包含的章节
	FoundSections []string
}

func NewEvidenceReasoningCompleteness() *EvidenceReasoningCompleteness {
	return &EvidenceReasoningCompleteness{
		RequiredSections: []string{
			"relevance",     // 相关性审查
			"legality",      // 合法性审查
			"authenticity",  // 真实性审查
			"type_specific", // 类型特定评估
			"burden",        // 举证责任分析
			"standard",      // 证明标准判断
			"conclusion",    // 综合结论
		},
	}
}

func (m *EvidenceReasoningCompleteness) Name() string {
	return "evidence_reasoning_completeness"
}

// Compute 计算推理完整度。
func (m *EvidenceReasoningCompleteness) Compute(prediction, reference string) float64 {
	if len(m.RequiredSections) == 0 {
		return 1
	}

	found := make(map[string]bool)
	for _, s := range m.FoundSections {
		found[s] = true
	}

	hit := 0
	for _, s := range m.RequiredSections {
		if found[s] {
			hit++
		}
	}

	return float64(hit) / float64(len(m.RequiredSections))
}

// SetSectionsFound 设置已发现的推理章节。
func (m *EvidenceReasoningCompleteness) SetSectionsFound(sections []string) {
	m.FoundSections = sections
}

// F1ForEvidenceJudgment 计算证据判断的 F1 分数。
// 用于评估召回证据和参考证据之间的重叠程度。
func F1ForEvidenceJudgment(predicted, reference []string) float64 {
	if len(predicted) == 0 && len(reference) == 0 {
		return 1
	}
	if len(predicted) == 0 || len(reference) == 0 {
		return 0
	}

	refSet := make(map[string]int)
	for _, r := range reference {
		refSet[r]++
	}

	var overlap int
	predCounts := make(map[string]int)
	for _, p := range predicted {
		predCounts[p]++
	}
	for p, pc := range predCounts {
		if rc := refSet[p]; rc < pc {
			overlap += rc
		} else {
			overlap += pc
		}
	}
	if overlap == 0 {
		return 0
	}

	precision := float64(overlap) / float64(len(predicted))
	recall := float64(overlap) / float64(len(reference))
	if precision+recall == 0 {
		return 0
	}
	return 2 * precision * recall / (precision + recall)
}

// EvidenceWeightedScore 计算加权综合评分。
// weights 为各维度权重（需归一化）。
func EvidenceWeightedScore(scores map[string]float64, weights map[string]float64) float64 {
	var totalWeight, weightedSum float64
	for dim, score := range scores {
		w := weights[dim]
		if w <= 0 {
			w = 0.1 // 默认权重
		}
		weightedSum += score * w
		totalWeight += w
	}
	if totalWeight == 0 {
		return 0
	}
	raw := weightedSum / totalWeight
	if raw > 1 {
		raw = 1
	}
	if raw < 0 {
		raw = 0
	}
	return math.Round(raw*100) / 100
}
