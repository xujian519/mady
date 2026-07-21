package evaluate

import (
	"strings"
)

// ============================================================================
// JudgeConsistency — AI 裁决与人工参考结论的一致性
// ============================================================================

// JudgeFunc determines whether a prediction agrees with a reference judgment.
// It returns true if the two are consistent, false otherwise.
// Phase 3: heuristic implementation; Phase 4+: LLM-based judge.
type JudgeFunc func(prediction, reference string) bool

// JudgeConsistency measures how consistently the AI's output aligns with the
// human reference judgment. It wraps an optional JudgeFunc for LLM-based
// comparison; when no JudgeFunc is set, a keyword-overlap heuristic is used.
//
// Score: 1.0 = agree, 0.0 = disagree.
type JudgeConsistency struct {
	// Judge is the optional comparison function. When nil, a heuristic
	// based on keyword recall is used (≥60% keyword overlap = agree).
	Judge JudgeFunc

	// MinOverlap is the keyword overlap threshold for the heuristic
	// fallback (default 0.6).
	MinOverlap float64
}

func (JudgeConsistency) Name() string { return "judge_consistency" }

func (j JudgeConsistency) Compute(prediction, reference string) float64 {
	if j.Judge != nil {
		if j.Judge(prediction, reference) {
			return 1.0
		}
		return 0.0
	}

	threshold := j.MinOverlap
	if threshold <= 0 {
		threshold = 0.6
	}

	overlap := keywordOverlap(prediction, reference)
	if overlap >= threshold {
		return 1.0
	}
	return 0.0
}

// keywordOverlap computes the fraction of reference keywords found in the
// prediction. This is a deliberately simple heuristic for Phase 3.
func keywordOverlap(prediction, reference string) float64 {
	keywords := ExtractKeywords(reference)
	if len(keywords) == 0 {
		return 1.0
	}
	lower := strings.ToLower(prediction)
	hit := 0
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			hit++
		}
	}
	return float64(hit) / float64(len(keywords))
}

// ============================================================================
// GuardrailFalseNegativeRate — 护栏漏报率（聚合指标）
// ============================================================================

// GuardrailFalseNegativeRate measures the proportion of high-risk outputs
// that were NOT flagged by the guardrail system. This is an aggregate metric
// computed from guardrail evaluation logs, not a per-case Metric.
//
// Design principle: false negatives (漏报) are weighted more heavily than
// false positives (误报). "过于严格多问几句"的代价远小于"该拦的没拦住"。
type GuardrailFalseNegativeRate struct {
	TotalHighRisk   int // 实际高风险输出数（人工标注）
	FlaggedHighRisk int // 护栏标记为高风险的数量
}

// Name returns the aggregate metric identifier. NOTE: GuardrailFalseNegativeRate
// does NOT implement the Metric interface (it has no Compute method); it is an
// aggregate computed from guardrail logs. Use Rate()/Score() instead.
func (GuardrailFalseNegativeRate) Name() string { return "guardrail_false_negative_rate" }

// Rate returns the false negative rate in [0, 1]. Lower is better.
func (g GuardrailFalseNegativeRate) Rate() float64 {
	if g.TotalHighRisk == 0 {
		return 0
	}
	missed := g.TotalHighRisk - g.FlaggedHighRisk
	if missed < 0 {
		return 0
	}
	return float64(missed) / float64(g.TotalHighRisk)
}

// Score returns 1 - Rate, so higher is better (consistent with Metric convention).
func (g GuardrailFalseNegativeRate) Score() float64 {
	return 1.0 - g.Rate()
}

// ============================================================================
// AdoptionRate — 人工复核采纳率（聚合指标）
// ============================================================================

// AdoptionRate measures the proportion of human review decisions that fully
// adopted the AI's output without modification. This is an aggregate metric
// computed from ApprovalGate review records.
//
// Three categories: Adopted (全部采纳), Modified (部分修改), Rejected (拒绝).
type AdoptionRate struct {
	Adopted  int // 全部采纳
	Modified int // 部分修改
	Rejected int // 拒绝
}

// Name returns the aggregate metric identifier. NOTE: AdoptionRate does NOT
// implement the Metric interface (it has no Compute method); it is an aggregate
// computed from ApprovalGate review records. Use FullyAdopted()/Accepted() etc.
func (AdoptionRate) Name() string { return "adoption_rate" }

// Total returns the total number of reviewed outputs.
func (a AdoptionRate) Total() int { return a.Adopted + a.Modified + a.Rejected }

// FullyAdopted returns the proportion of outputs fully adopted (no changes).
func (a AdoptionRate) FullyAdopted() float64 {
	if a.Total() == 0 {
		return 0
	}
	return float64(a.Adopted) / float64(a.Total())
}

// Accepted returns the proportion of outputs accepted (adopted + modified).
// This measures how often the AI output was useful, even if not perfect.
func (a AdoptionRate) Accepted() float64 {
	if a.Total() == 0 {
		return 0
	}
	return float64(a.Adopted+a.Modified) / float64(a.Total())
}

// RejectedRate returns the proportion of outputs rejected.
func (a AdoptionRate) RejectedRate() float64 {
	if a.Total() == 0 {
		return 0
	}
	return float64(a.Rejected) / float64(a.Total())
}
