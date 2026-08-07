package evaluate

// EvidenceJudgmentAccuracy 评估证据判断的准确性。
// 分数为预测结论与参考结论一致的比例。
type EvidenceJudgmentAccuracy struct {
	// TotalJudgments 总判断次数
	TotalJudgments int
	// CorrectJudgments 正确判断次数
	CorrectJudgments int
}

// Name returns "evidence_judgment_accuracy".
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

// Name returns "evidence_type_coverage".
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

// Name returns "evidence_reasoning_completeness".
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
