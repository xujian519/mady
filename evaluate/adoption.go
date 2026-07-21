package evaluate

// ============================================================================
// ApprovalRecord — 最小审批记录接口
// ============================================================================

// ApprovalRecord 是最小审批记录接口。
// 用于从外部审批系统（如 domains.ApprovalRecord）接入采纳率统计，
// 避免 agentcore/evaluate 包反向依赖领域层。
//
// Decision 返回审批结果，取值为 "adopted" | "modified" | "rejected"。
type ApprovalRecord interface {
	// Decision 返回审批决策标识。
	Decision() string
}

// ApprovalRecordFunc 是将已有审批记录适配为 ApprovalRecord 接口的辅助类型。
// 适用于结构体值无法直接实现接口的场景（如 domains.ApprovalRecord 的
// Decision 是字段而非方法）。
type ApprovalRecordFunc func() string

// Decision 实现 ApprovalRecord 接口。
func (f ApprovalRecordFunc) Decision() string { return f() }

// ============================================================================
// AdoptionRateMetric
// ============================================================================

// AdoptionRateMetric 将人工审批采纳率包装为 Metric 接口实现。
//
// 数据来源：ApprovalGate 累积的 AdoptionRecord（Adopted / Modified / Rejected）。
// 总体采纳率定义为 (Adopted + Modified) / Total，反映 AI 输出对人类的
// 实际帮助程度——即使不完全准确，只要有用（被采纳或部分修改）即计入采纳。
//
// 作为 Metric 接口，prediction 和 reference 参数被忽略，使用内部计数
// 计算聚合采纳率。这使得 AdoptionRateMetric 适合作为 Evaluator 的补充
// 指标：可在 Evaluate() 中为每条用例返回同一聚合值，或在 EvaluateBatch()
// 后单独调用 Compute() 获取全局采纳率。
type AdoptionRateMetric struct {
	Adopted  int
	Modified int
	Rejected int
}

// Name 返回指标标识符 "adoption_rate"。
func (m *AdoptionRateMetric) Name() string { return "adoption_rate" }

// Compute 返回总体采纳率 (Adopted + Modified) / Total。
// prediction 和 reference 参数被忽略，使用内部计数器。
// 当 Total == 0 时返回 1.0（无数据视为满分，表示尚无负面记录）。
func (m *AdoptionRateMetric) Compute(prediction, reference string) float64 {
	total := m.Adopted + m.Modified + m.Rejected
	if total == 0 {
		return 1.0
	}
	return float64(m.Adopted+m.Modified) / float64(total)
}

// Total 返回审批记录总数。
func (m *AdoptionRateMetric) Total() int {
	return m.Adopted + m.Modified + m.Rejected
}

// FullyAdopted 返回完全采纳比例：Adopted / Total。
// 当 Total == 0 时返回 0。
func (m *AdoptionRateMetric) FullyAdopted() float64 {
	total := m.Total()
	if total == 0 {
		return 0
	}
	return float64(m.Adopted) / float64(total)
}

// Accepted 返回有用输出比例：(Adopted + Modified) / Total。
// 与 Compute 返回值相同，但类型安全且不实现 Metric 接口。
func (m *AdoptionRateMetric) Accepted() float64 {
	total := m.Total()
	if total == 0 {
		return 0
	}
	return float64(m.Adopted+m.Modified) / float64(total)
}

// RejectedRate 返回拒绝率：Rejected / Total。
func (m *AdoptionRateMetric) RejectedRate() float64 {
	total := m.Total()
	if total == 0 {
		return 0
	}
	return float64(m.Rejected) / float64(total)
}

// Record 记录一次审批结果。
// decision 取值 "adopted" / "modified" / "rejected"，大小写不敏感。
// 未知值静默忽略。
func (m *AdoptionRateMetric) Record(decision string) {
	switch decision {
	case "adopted":
		m.Adopted++
	case "modified":
		m.Modified++
	case "rejected":
		m.Rejected++
	}
}

// FromApprovalRecords 从 ApprovalRecord 列表批量导入审批记录。
// 返回一个新的 AdoptionRateMetric，不会修改原指标。
func FromApprovalRecords(records []ApprovalRecord) *AdoptionRateMetric {
	m := &AdoptionRateMetric{}
	for _, r := range records {
		m.Record(r.Decision())
	}
	return m
}
