// verdict.go 实现统一判级聚合协议（DSH checker 判级引入）：
// 把规则引擎产出的违规列表按严重度聚合成单一的 pass / needs_revision / blocked
// 判级，供工作流质量门禁与审批挂起逻辑消费。此前 claimdrafting / specdrafting
// 等规则模块各有评分口径，判级词汇不统一，工作流门禁没有一致的消费接口。
//
// 聚合规则（对齐 DSH checker 的 Must/Should/Quality 三级）：
//   - 任一 SeverityError（Must：严重违法）→ blocked
//   - SeverityWarning（Should：潜在风险）计数达 ShouldBlockedAt → blocked
//   - SeverityInfo（Quality：建议改进）计数达 InfoRevisionAt → needs_revision
//   - 其余 → pass

package rulekit

// Verdict 是规则违规聚合后的判级结论。
type Verdict string

const (
	// VerdictPass 无阻断性问题。
	VerdictPass Verdict = "pass"
	// VerdictNeedsRevision 存在足够多的质量问题，建议修订后重交。
	VerdictNeedsRevision Verdict = "needs_revision"
	// VerdictBlocked 存在阻断性问题，产出不得进入下游交付。
	VerdictBlocked Verdict = "blocked"
)

// AggregatePolicy 是判级聚合的阈值参数。
type AggregatePolicy struct {
	// ShouldBlockedAt 是 warning 级违规的阻断阈值（DSH 默认：任一 Should
	// 失败即阻断，即 1）。调大可放宽对潜在风险的容忍度。
	ShouldBlockedAt int
	// InfoRevisionAt 是 info 级违规的修订阈值（DSH 默认 3：
	// ≥3 条 Quality 失败 → needs_revision）。
	InfoRevisionAt int
}

// DefaultAggregatePolicy 返回 DSH checker 对齐的默认阈值。
func DefaultAggregatePolicy() AggregatePolicy {
	return AggregatePolicy{ShouldBlockedAt: 1, InfoRevisionAt: 3}
}

// Aggregate 把违规列表聚合成判级结论。nil/空列表 → pass。
// 阈值 ≤0 时按 DefaultAggregatePolicy 处理（防御配置错误导致全放行）。
func Aggregate(vs []Violation, p AggregatePolicy) Verdict {
	if p.ShouldBlockedAt <= 0 {
		p.ShouldBlockedAt = DefaultAggregatePolicy().ShouldBlockedAt
	}
	if p.InfoRevisionAt <= 0 {
		p.InfoRevisionAt = DefaultAggregatePolicy().InfoRevisionAt
	}

	warnings, infos := 0, 0
	for _, v := range vs {
		switch v.Severity {
		case SeverityError:
			return VerdictBlocked // 任一 Must 失败直接阻断，无需继续计数
		case SeverityWarning:
			warnings++
		case SeverityInfo:
			infos++
		}
	}

	switch {
	case warnings >= p.ShouldBlockedAt:
		return VerdictBlocked
	case infos >= p.InfoRevisionAt:
		return VerdictNeedsRevision
	default:
		return VerdictPass
	}
}

// AggregateWithDefault 用默认阈值聚合。
func AggregateWithDefault(vs []Violation) Verdict {
	return Aggregate(vs, DefaultAggregatePolicy())
}
