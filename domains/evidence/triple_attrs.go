package evidence

import (
	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

// evaluateRelevance 评估证据相关性（包级辅助函数）。
func evaluateRelevance(span agentcore_evidence.EvidenceSpan) *DimensionJudgment {
	j := &DimensionJudgment{Dimension: judgmentRelevance}
	score := 0.5
	if span.SourceURI != "" {
		score += 0.1
	}
	if len(span.ClaimRefs) > 0 {
		score += 0.2
	}
	if span.Direction == agentcore_evidence.DirectionSupporting || span.Direction == agentcore_evidence.DirectionContradicting {
		score += 0.1
	}
	if span.Snippet != "" {
		score += 0.1
	}
	if score > 1.0 {
		score = 1.0
	}
	j.Score = score
	switch {
	case score >= 0.85:
		j.Level = judgmentLevelHigh
	case score >= 0.65:
		j.Level = judgmentLevelMediumHigh
	case score >= 0.45:
		j.Level = "medium"
	default:
		j.Level = judgmentLevelLow
	}
	j.Reasoning = "相关性评估完成"
	return j
}

// evaluateAuthenticity 评估证据真实性（包级辅助函数）。
func evaluateAuthenticity(span agentcore_evidence.EvidenceSpan) *DimensionJudgment {
	j := &DimensionJudgment{Dimension: judgmentAuthenticity}
	score := 0.5
	if span.ContentHash != "" {
		score += 0.3
	}
	if span.DocVersion != "" {
		score += 0.1
	}
	if score > 1.0 {
		score = 1.0
	}
	j.Score = score
	switch {
	case score >= 0.85:
		j.Level = judgmentLevelHigh
	case score >= 0.65:
		j.Level = judgmentLevelMediumHigh
	default:
		j.Level = judgmentLevelLow
	}
	j.Reasoning = "真实性评估完成"
	return j
}

// evaluateLegality 评估证据合法性（包级辅助函数）。
func evaluateLegality(span agentcore_evidence.EvidenceSpan) *DimensionJudgment {
	j := &DimensionJudgment{Dimension: judgmentLegality}
	score := 0.7
	if span.SourceURI == "" {
		score -= 0.2
	}
	if span.ContentHash != "" {
		score += 0.2
	}
	if score > 1.0 {
		score = 1.0
	}
	if score < 0 {
		score = 0
	}
	j.Score = score
	switch {
	case score >= 0.85:
		j.Level = judgmentLevelHigh
	case score >= 0.65:
		j.Level = judgmentLevelMediumHigh
	default:
		j.Level = judgmentLevelLow
	}
	j.Reasoning = "合法性评估完成"
	return j
}
