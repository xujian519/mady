package adversary

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// 换位重跑：评估最终意见陈述书的论证强度、遗漏驳回理由与证据完整性。
// 与 adversary 计划评估互补：那边评估「计划」，这边评估「最终产物」。

// ReviewFinalDraft 从对立审查员视角评估最终意见陈述书。
func ReviewFinalDraft(input ReviewInput) ReviewReport {
	missed := DetectMissedRejections(input.FinalDraft, input.OfficeAction)
	gaps := DetectEvidenceGaps(input.FinalDraft, input.ReferenceDocuments, input.OfficeAction)
	if strings.TrimSpace(input.Claims) != "" && strings.TrimSpace(input.FinalDraft) != "" {
		gaps = append(gaps, DetectClaimsCoverage(input.FinalDraft, input.Claims)...)
	}
	disputes := IdentifyReviewDisputes(input.OfficeAction, missed, gaps)
	strength := AssessArgumentStrength(missed, gaps, disputes)
	confidence := ComputeReviewConfidence(disputes, strength)
	return ReviewReport{
		Disputes: disputes, ArgumentStrength: strength,
		MissedRejections: missed, EvidenceGaps: gaps,
		OverallConfidence: confidence,
		Summary:           BuildReviewSummary(missed, gaps, strength, confidence),
	}
}

// 驳回类型 → 回应关键词（确定性骨架；语义匹配需模型）。
var rejectionKeywords = map[string][]string{
	"novelty":       {"新颖性", "A22.2", "第二十二条第二款"},
	"inventiveness": {"创造性", "A22.3", "第二十二条第三款", "三步法"},
	"utility":       {"实用性", "A22.4", "第二十二条第四款"},
	"disclosure":    {"说明不充分", "A26.3", "第二十六条第三款"},
	"claim-clarity": {"权利要求不清楚", "A26.4", "第二十六条第四款"},
	"clarity":       {"权利要求不清楚", "A26.4", "第二十六条第四款"},
	"support":       {"不支持", "A26.4", "第二十六条第四款"},
	"amendment":     {"修改超范围", "A33", "第三十三条"},
	"scope":         {"修改超范围", "A33", "第三十三条"},
}

// DetectMissedRejections 检测最终产物未回应的驳回理由。
func DetectMissedRejections(finalDraft string, officeAction *OfficeAction) []string {
	if officeAction == nil || len(officeAction.RejectionTypes) == 0 {
		return nil
	}
	var missed []string
	for _, ground := range officeAction.RejectionTypes {
		keywords := rejectionKeywords[ground]
		if len(keywords) == 0 {
			// 未收录的类型退回按类型名本身判断。
			if !strings.Contains(finalDraft, ground) {
				missed = append(missed, ground)
			}
			continue
		}
		responded := false
		for _, keyword := range keywords {
			if strings.Contains(finalDraft, keyword) {
				responded = true
				break
			}
		}
		if !responded {
			missed = append(missed, ground)
		}
	}
	return missed
}

var (
	referencePatentNumberRE = regexp.MustCompile(`CN\d+[A-Z]`)
	referenceLongNumberRE   = regexp.MustCompile(`\b\d{4,}\b`)
	claimMarkerRE           = regexp.MustCompile(`(?:权利要求|权)\s*\d+`)
)

// DetectEvidenceGaps 检测论证缺少支撑证据。
func DetectEvidenceGaps(finalDraft, referenceDocuments string, officeAction *OfficeAction) []string {
	var gaps []string
	if strings.TrimSpace(referenceDocuments) != "" && strings.TrimSpace(finalDraft) != "" {
		keywords := extractReferenceKeywords(referenceDocuments)
		cited := 0
		for _, keyword := range keywords {
			if strings.Contains(finalDraft, keyword) {
				cited++
			}
		}
		if cited == 0 && len(keywords) > 0 {
			gaps = append(gaps, "最终产物未引用对比文件内容")
		}
	}
	if officeAction != nil && containsString(officeAction.RejectionTypes, "inventiveness") {
		if !strings.Contains(finalDraft, "创造性") && !strings.Contains(finalDraft, "A22.3") {
			gaps = append(gaps, "通知书指出创造性问题但最终产物未论证")
		}
	}
	return gaps
}

func extractReferenceKeywords(referenceDocuments string) []string {
	seen := make(map[string]bool)
	var keywords []string
	add := func(matches []string) {
		for _, match := range matches {
			if seen[match] || len(keywords) >= 5 {
				continue
			}
			seen[match] = true
			keywords = append(keywords, match)
		}
	}
	add(referencePatentNumberRE.FindAllString(referenceDocuments, -1))
	add(referenceLongNumberRE.FindAllString(referenceDocuments, -1))
	return keywords
}

// DetectClaimsCoverage 检测最终产物是否引用了权利要求编号。
func DetectClaimsCoverage(finalDraft, claims string) []string {
	markers := claimMarkerRE.FindAllString(claims, -1)
	if len(markers) == 0 {
		return nil
	}
	unique := make(map[string]bool, len(markers))
	covered := false
	for _, marker := range markers {
		if unique[marker] {
			continue
		}
		unique[marker] = true
		if strings.Contains(finalDraft, marker) {
			covered = true
		}
	}
	if covered {
		return nil
	}
	return []string{fmt.Sprintf("最终产物未引用任何权利要求编号（共 %d 项权项标记未覆盖）", len(unique))}
}

// IdentifyReviewDisputes 由遗漏理由与证据缺口生成争议点。
func IdentifyReviewDisputes(officeAction *OfficeAction, missed, gaps []string) []Dispute {
	claims := []int(nil)
	if officeAction != nil {
		claims = append(claims, officeAction.AffectedClaims...)
	}
	disputes := make([]Dispute, 0, len(missed)+len(gaps))
	for _, ground := range missed {
		statute := GroundToStatute[ground]
		if statute == "" {
			statute = "unknown"
		}
		disputes = append(disputes, Dispute{
			Statute: statute, Claims: claims,
			Point: "遗漏驳回理由：" + ground, Relation: RelationMissedByBaseline,
			Confidence: 0.9, Tag: TagHeuristic,
		})
	}
	for _, gap := range gaps {
		disputes = append(disputes, Dispute{
			Statute: "A22.3", Claims: claims,
			Point: "证据缺口：" + gap, Relation: RelationDisagreement,
			Confidence: 0.7, Tag: TagHeuristic,
		})
	}
	return disputes
}

// AssessArgumentStrength 评估论证强度：有遗漏为 insufficient；
// 多重证据缺口或高置信度争议为 weak；否则 strong。
func AssessArgumentStrength(missed, gaps []string, disputes []Dispute) ArgumentStrength {
	if len(missed) > 0 {
		return StrengthInsufficient
	}
	if len(gaps) >= 2 {
		return StrengthWeak
	}
	for _, dispute := range disputes {
		if dispute.Confidence >= 0.8 {
			return StrengthWeak
		}
	}
	return StrengthStrong
}

// ComputeReviewConfidence 计算换位审查整体置信度，并按论证强度打折。
func ComputeReviewConfidence(disputes []Dispute, strength ArgumentStrength) float64 {
	if len(disputes) == 0 {
		if strength == StrengthStrong {
			return 0.8
		}
		return 0.5
	}
	var total, weighted float64
	for _, dispute := range disputes {
		total += dispute.Confidence
		weighted += dispute.Confidence * dispute.Confidence
	}
	if total == 0 {
		return 0
	}
	weighted /= total
	factor := 1.0
	switch strength {
	case StrengthWeak:
		factor = 0.7
	case StrengthInsufficient:
		factor = 0.4
	}
	value := weighted * factor
	if value > 1 {
		return 1
	}
	if value < 0 {
		return 0
	}
	return value
}

// BuildReviewSummary 生成换位审查摘要。
func BuildReviewSummary(missed, gaps []string, strength ArgumentStrength, confidence float64) string {
	strengthLabel := map[ArgumentStrength]string{
		StrengthStrong: "充分", StrengthWeak: "部分不足", StrengthInsufficient: "严重不足",
	}[strength]
	parts := []string{"论证强度：" + strengthLabel}
	if len(missed) > 0 {
		parts = append(parts, fmt.Sprintf("遗漏 %d 项驳回理由：%s", len(missed), strings.Join(missed, "、")))
	}
	if len(gaps) > 0 {
		parts = append(parts, fmt.Sprintf("存在 %d 处证据缺口", len(gaps)))
	}
	parts = append(parts, fmt.Sprintf("整体置信度：%.1f%%", confidence*100))
	return strings.Join(parts, "；")
}

func containsString(values []string, target string) bool { return slices.Contains(values, target) }
