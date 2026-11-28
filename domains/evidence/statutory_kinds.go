package evidence

import "strings"

// kindDefinesLegality 报告该种类是否自带完整的合法性评分。
func kindDefinesLegality(kind EvidenceKind) bool {
	switch kind {
	case KindElectronic, KindWitnessTestimony, KindExpertOpinion:
		return true
	default:
		return false
	}
}

// kindStatutoryContributions 是法定种类轴（正交轴 1）的规则。
func kindStatutoryContributions(in StatutoryEvidenceInput) []statutoryContribution {
	switch in.Kind {
	case KindDocumentary:
		return documentaryContributions(in)
	case KindPhysical:
		return physicalContributions(in)
	case KindAudioVisual:
		return audioVisualContributions(in)
	case KindElectronic:
		return electronicContributions(in)
	case KindWitnessTestimony:
		return witnessContributions(in)
	case KindPartyStatement:
		return partyStatementContributions(in)
	case KindExpertOpinion:
		return expertContributions(in)
	case KindInspectionRecord:
		return inspectionContributions(in)
	default:
		return nil
	}
}

// kindDelta 生成一条只参与评分、不生成 finding 的种类轴贡献。
func kindDelta(dimension string, delta float64, reason string) statutoryContribution {
	return statutoryContribution{Axis: AxisKind, Dimension: dimension, Delta: delta, Reason: reason}
}

func documentaryContributions(in StatutoryEvidenceInput) []statutoryContribution {
	out := originalCheck(in, "LEGAL-EVI-001", "民事诉讼法第73条",
		judgmentCritical, "原件", 0.20, 0.05, -0.15)
	out = append(out,
		statutoryCheck{
			axis: AxisKind, dimension: judgmentAuthenticity, id: "LEGAL-EVI-002",
			legalBasis: "最高人民法院关于民事诉讼证据的若干规定第90条", severity: "warning",
			deltaTrue: 0.20, deltaFalse: -0.20,
			reasonTrue: "书证签章已核验", reasonFalse: "书证签章真实性未核验",
			reasonUnknown: "未声明书证签章是否核验",
		}.contribution(boolState(in.SignatureVerified)),
		statutoryCheck{
			axis: AxisKind, dimension: judgmentAuthenticity, id: "LEGAL-EVI-003",
			severity: "warning", deltaTrue: 0.10, deltaFalse: -0.10,
			reasonTrue: "书证形成时间明确", reasonFalse: "书证形成时间不明确",
			reasonUnknown: "未声明书证形成时间是否明确",
		}.contribution(boolState(in.FormationTimeClear)),
	)
	return out
}

func physicalContributions(in StatutoryEvidenceInput) []statutoryContribution {
	out := originalCheck(in, "LEGAL-EVI-013", "民事诉讼法第73条",
		"warning", "原物", 0.25, 0.05, -0.15)
	out = append(out, statutoryCheck{
		axis: AxisKind, dimension: judgmentAuthenticity,
		severity: "warning", deltaTrue: 0.15, deltaFalse: -0.15,
		reasonTrue: "物证保管链完整", reasonFalse: "物证保管链不完整",
		reasonUnknown: "未声明物证保管链是否完整",
	}.contribution(boolState(in.StorageIntegrityVerified)))
	return out
}

func audioVisualContributions(in StatutoryEvidenceInput) []statutoryContribution {
	var out []statutoryContribution
	if strings.TrimSpace(in.ContentHash) == "" {
		out = append(out, statutoryContribution{
			Axis: AxisKind, Dimension: judgmentAuthenticity, RuleID: "LEGAL-EVI-014",
			Severity: "warning", Delta: -0.10, Reason: "视听资料未记录内容哈希",
		})
	} else {
		out = append(out, kindDelta(judgmentAuthenticity, 0.15, "视听资料已记录内容哈希"))
		verified := statutoryCheck{
			axis: AxisKind, dimension: judgmentAuthenticity, id: "LEGAL-EVI-014",
			severity: "warning", deltaTrue: 0.15, deltaFalse: -0.10,
			reasonTrue: "视听资料内容哈希已核验", reasonFalse: "视听资料内容哈希未核验",
			reasonUnknown: "未声明视听资料内容哈希是否核验",
		}.contribution(boolState(in.HashVerified))
		// 已记录哈希的固定加分，叠加在核验结论之上。
		verified.Delta += 0.15
		out = append(out, verified)
	}
	out = append(out, statutoryCheck{
		axis: AxisKind, dimension: judgmentAuthenticity,
		severity: "warning", deltaTrue: 0.15, deltaFalse: -0.15,
		reasonTrue: "视听资料存储完整性已核验", reasonFalse: "视听资料存储完整性未核验",
		reasonUnknown: "未声明视听资料存储完整性是否核验",
	}.contribution(boolState(in.StorageIntegrityVerified)))
	if strings.TrimSpace(in.Timestamp) != "" {
		out = append(out, kindDelta(judgmentAuthenticity, 0.10, "已记录视听资料时间信息"))
	}
	return out
}

func electronicContributions(in StatutoryEvidenceInput) []statutoryContribution {
	var out []statutoryContribution
	switch boolState(in.CollectionLegal) {
	case declaredTrue:
		out = append(out, kindDelta(judgmentLegality, 0.25, "电子数据收集程序合法"))
	case declaredFalse:
		out = append(out, statutoryContribution{
			Axis: AxisKind, Dimension: judgmentLegality, RuleID: "LEGAL-EVI-GEN-002",
			LegalBasis: "最高人民法院关于民事诉讼证据的若干规定第106条",
			Severity:   judgmentCritical, Delta: -0.30,
			Reason: "电子数据收集程序合法性存疑",
		})
	}
	switch boolState(in.FormLegal) {
	case declaredTrue:
		out = append(out, kindDelta(judgmentLegality, 0.20, "电子数据形式合法"))
	case declaredFalse:
		out = append(out, kindDelta(judgmentLegality, -0.15, "电子数据形式合法性存疑"))
	}
	if strings.TrimSpace(in.Source) != "" {
		out = append(out, kindDelta(judgmentLegality, 0.10, "已记录电子数据来源"))
	}

	if strings.TrimSpace(in.ContentHash) == "" {
		out = append(out, statutoryContribution{
			Axis: AxisKind, Dimension: judgmentAuthenticity, RuleID: "LEGAL-EVI-005",
			Severity: "warning", Delta: -0.10, Reason: "电子数据未记录内容哈希",
		})
	} else {
		out = append(out, kindDelta(judgmentAuthenticity, 0.15, "电子数据已记录内容哈希"))
		verified := statutoryCheck{
			axis: AxisKind, dimension: judgmentAuthenticity, id: "LEGAL-EVI-005",
			severity: "warning", deltaTrue: 0.20, deltaFalse: -0.10,
			reasonTrue: "内容哈希已独立核验", reasonFalse: "内容哈希未独立核验",
			reasonUnknown: "未声明内容哈希是否独立核验",
		}.contribution(boolState(in.HashVerified))
		// 已记录哈希的固定加分，叠加在核验结论之上。
		verified.Delta += 0.15
		out = append(out, verified)
	}

	if strings.TrimSpace(in.Timestamp) == "" {
		out = append(out, statutoryContribution{
			Axis: AxisKind, Dimension: judgmentAuthenticity, RuleID: "LEGAL-EVI-006",
			Severity: "warning", Delta: -0.05, Reason: "电子数据未记录时间戳",
		})
	} else {
		out = append(out, statutoryContribution{
			Axis: AxisKind, Dimension: judgmentAuthenticity, RuleID: "LEGAL-EVI-006",
			Severity: "warning", Passed: true, Delta: 0.10,
			Reason: "电子数据已记录时间戳",
		})
	}

	out = append(out, statutoryCheck{
		axis: AxisKind, dimension: judgmentAuthenticity, id: "LEGAL-EVI-004",
		legalBasis: "最高人民法院关于民事诉讼证据的若干规定第93条",
		severity:   judgmentCritical, deltaTrue: 0.15, deltaFalse: -0.15,
		reasonTrue: "电子数据存储完整性已核验", reasonFalse: "电子数据存储完整性未核验",
		reasonUnknown: "未声明电子数据存储完整性是否核验",
	}.contribution(boolState(in.StorageIntegrityVerified)))
	return out
}

func witnessContributions(in StatutoryEvidenceInput) []statutoryContribution {
	var out []statutoryContribution
	switch boolState(in.CollectionLegal) {
	case declaredTrue:
		out = append(out, kindDelta(judgmentLegality, 0.25, "证人证言收集程序合法"))
	case declaredFalse:
		out = append(out, statutoryContribution{
			Axis: AxisKind, Dimension: judgmentLegality, RuleID: "LEGAL-EVI-GEN-002",
			LegalBasis: "最高人民法院关于民事诉讼证据的若干规定第106条",
			Severity:   judgmentCritical, Delta: -0.30,
			Reason: "证人证言收集程序合法性存疑",
		})
	}
	switch boolState(in.FormLegal) {
	case declaredTrue:
		out = append(out, kindDelta(judgmentLegality, 0.20, "证人证言形式合法"))
	case declaredFalse:
		out = append(out, kindDelta(judgmentLegality, -0.15, "证人证言形式合法性存疑"))
	}
	if strings.TrimSpace(in.Source) != "" {
		out = append(out, kindDelta(judgmentLegality, 0.05, "已记录证人身份"))
	}

	conflict := statutoryContribution{
		Axis: AxisKind, Dimension: judgmentAuthenticity, RuleID: "LEGAL-EVI-007",
		LegalBasis: "民事诉讼法第75条", Severity: judgmentCritical,
	}
	switch boolState(in.WitnessHasConflict) {
	case declaredTrue:
		if boolState(in.ConflictDisclosed) == declaredTrue {
			conflict.Passed = true
			conflict.Delta = 0.05
			conflict.Reason = "证人与案件有利害关系，但已披露"
		} else {
			conflict.Delta = -0.20
			conflict.Reason = "证人与案件有利害关系且未披露"
		}
	case declaredFalse:
		conflict.Passed = true
		conflict.Delta = 0.15
		conflict.Reason = "证人与案件无利害关系"
	default:
		conflict.Reason = "未声明证人与案件是否存在利害关系"
	}
	out = append(out, conflict)

	out = append(out, statutoryCheck{
		axis: AxisKind, dimension: judgmentAuthenticity, id: "LEGAL-EVI-008",
		severity: "warning", deltaTrue: 0.20, deltaFalse: -0.15,
		reasonTrue: "证人证言有其他证据佐证", reasonFalse: "证人证言缺乏其他证据佐证",
		reasonUnknown: "未声明证人证言是否有其他证据佐证",
	}.contribution(boolState(in.Corroborated)))
	if strings.TrimSpace(in.Content) != "" {
		out = append(out, kindDelta(judgmentAuthenticity, 0.10, "已记录证言内容"))
	}
	return out
}

func partyStatementContributions(in StatutoryEvidenceInput) []statutoryContribution {
	out := []statutoryContribution{statutoryCheck{
		axis: AxisKind, dimension: judgmentAuthenticity, id: "LEGAL-EVI-012",
		legalBasis: "民事诉讼法第78条", severity: "warning",
		deltaTrue: 0.20, deltaFalse: -0.15,
		reasonTrue: "当事人陈述有其他证据佐证", reasonFalse: "当事人陈述缺乏其他证据佐证",
		reasonUnknown: "未声明当事人陈述是否有其他证据佐证",
	}.contribution(boolState(in.Corroborated))}
	if strings.TrimSpace(in.Content) != "" {
		out = append(out, kindDelta(judgmentAuthenticity, 0.15, "已记录陈述内容"))
	}
	return out
}

func expertContributions(in StatutoryEvidenceInput) []statutoryContribution {
	var out []statutoryContribution
	// 鉴定意见的合法性以资质与鉴定程序为主口径，收集与形式只在明确违法时扣分：
	// 两者未声明不扣分，也不额外加分，避免与资质、程序重复计分。
	if boolState(in.CollectionLegal) == declaredFalse {
		out = append(out, statutoryContribution{
			Axis: AxisKind, Dimension: judgmentLegality, RuleID: "LEGAL-EVI-GEN-002",
			LegalBasis: "最高人民法院关于民事诉讼证据的若干规定第106条",
			Severity:   judgmentCritical, Delta: -0.20,
			Reason: "鉴定意见收集合法性存疑",
		})
	}
	if boolState(in.FormLegal) == declaredFalse {
		out = append(out, kindDelta(judgmentLegality, -0.15, "鉴定意见形式合法性存疑"))
	}
	out = append(out,
		statutoryCheck{
			axis: AxisKind, dimension: judgmentLegality, id: "LEGAL-EVI-009",
			legalBasis: "民事诉讼法第79条", severity: judgmentCritical,
			deltaTrue: 0.30, deltaFalse: -0.30,
			reasonTrue: "鉴定机构/鉴定人资质已核验", reasonFalse: "鉴定机构/鉴定人资质未核验",
			reasonUnknown: "未声明鉴定机构/鉴定人资质是否核验",
		}.contribution(boolState(in.ExpertQualified)),
		statutoryCheck{
			axis: AxisKind, dimension: judgmentLegality, id: "LEGAL-EVI-010",
			severity: judgmentCritical, deltaTrue: 0.20, deltaFalse: -0.25,
			reasonTrue: "鉴定程序合法", reasonFalse: "鉴定程序合法性存疑",
			reasonUnknown: "未声明鉴定程序是否合法",
		}.contribution(boolState(in.ExpertProcedureLegal)),
		statutoryCheck{
			axis: AxisKind, dimension: judgmentAuthenticity, id: "LEGAL-EVI-011",
			severity: "warning", deltaTrue: 0.25, deltaFalse: -0.20,
			reasonTrue: "鉴定结论明确", reasonFalse: "鉴定结论不明确",
			reasonUnknown: "未声明鉴定结论是否明确",
		}.contribution(boolState(in.ExpertConclusionClear)),
	)
	if strings.TrimSpace(in.Content) != "" {
		out = append(out, kindDelta(judgmentAuthenticity, 0.10, "已记录鉴定材料"))
	}
	if boolState(in.ExpertQualified) == declaredTrue {
		out = append(out, kindDelta(judgmentAuthenticity, 0.15, "鉴定资质已核验"))
	}
	return out
}

func inspectionContributions(in StatutoryEvidenceInput) []statutoryContribution {
	return []statutoryContribution{
		statutoryCheck{
			axis: AxisKind, dimension: judgmentAuthenticity, id: "LEGAL-EVI-015",
			legalBasis: "民事诉讼法第82条", severity: "warning",
			deltaTrue: 0.20, deltaFalse: -0.15,
			reasonTrue: "勘验有见证人在场", reasonFalse: "勘验无见证人在场",
			reasonUnknown: "未声明勘验是否有见证人在场",
		}.contribution(boolState(in.InspectionHasWitness)),
		statutoryCheck{
			axis: AxisKind, dimension: judgmentAuthenticity, id: "LEGAL-EVI-016",
			severity: "warning", deltaTrue: 0.20, deltaFalse: -0.15,
			reasonTrue: "勘验记录完整", reasonFalse: "勘验记录不完整",
			reasonUnknown: "未声明勘验记录是否完整",
		}.contribution(boolState(in.InspectionRecordComplete)),
	}
}

// originalCheck 处理"应提供原件/原物"这一类检查：提供、未提供但说明原因、
// 未提供且未说明、未声明，四种情况的分值不同。
func originalCheck(in StatutoryEvidenceInput, ruleID, legalBasis, severity, noun string,
	deltaTrue, deltaExplained, deltaAbsent float64) []statutoryContribution {
	c := statutoryContribution{
		Axis:       AxisKind,
		Dimension:  judgmentAuthenticity,
		RuleID:     ruleID,
		LegalBasis: legalBasis,
		Severity:   severity,
	}
	switch boolState(in.HasOriginal) {
	case declaredTrue:
		c.Passed = true
		c.Delta = deltaTrue
		c.Reason = "已提供" + noun
	case declaredFalse:
		if strings.TrimSpace(in.OriginalAbsenceReason) != "" {
			c.Delta = deltaExplained
			c.Reason = "未提供" + noun + "，但已说明原因：" + in.OriginalAbsenceReason
		} else {
			c.Delta = deltaAbsent
			c.Reason = "未提供" + noun + "且未说明原因"
		}
	default:
		c.Reason = "未声明是否提供" + noun
	}
	return []statutoryContribution{c}
}
