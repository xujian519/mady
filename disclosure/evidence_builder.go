package disclosure

import (
	"fmt"

	"github.com/xujian519/mady/agentcore/evidence"
)

// BuildEvidencePackage 从分析报告构建证据包裹。
// 每个技术特征作为一个 Claim，关联对应 NoveltyResult 中的评估作为 EvidenceSpan。
func BuildEvidencePackage(report *AnalysisReport, sessionID string) *evidence.ClaimBinding {
	cb := evidence.NewClaimBinding()
	if report == nil || report.Extraction == nil {
		return cb
	}

	// Create evidence spans from extraction results.
	for _, feature := range report.Extraction.Features {
		var direction evidence.EvidenceDirection
		switch feature.PriorArtStatus {
		case "known":
			direction = evidence.DirectionContradicting
		case "unknown":
			direction = evidence.DirectionSupporting
		default:
			direction = evidence.DirectionNeutral
		}

		span := evidence.EvidenceSpan{
			ID:        fmt.Sprintf("ev_feat_%s", feature.ID),
			Snippet:   feature.Description,
			Direction: direction,
			ClaimRefs: []string{fmt.Sprintf("claim_feat_%s", feature.ID)},
		}
		if report.Document != nil {
			span.SourceURI = fmt.Sprintf("disclosure://%s", report.Document.ID)
		}
		cb.RegisterSpan(span)
	}

	// Create evidence spans from novelty assessment.
	if report.Novelty != nil && report.Novelty.Assessed {
		span := evidence.EvidenceSpan{
			ID:        "ev_novelty_conclusion",
			Snippet:   report.Novelty.Conclusion,
			Direction: evidence.DirectionNeutral,
		}
		if report.Document != nil {
			span.SourceURI = fmt.Sprintf("disclosure://%s", report.Document.ID)
		}
		cb.RegisterSpan(span)
	}

	return cb
}
