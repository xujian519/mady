package evidence

import (
	"fmt"
	"strings"
)

// 本文件实现《民事诉讼法》第 66 条八类法定证据的二维三性模型。
//
// 两条轴彼此独立：
//   - EvidenceKind：法定种类（第 66 条列举的八类）。
//   - AcquisitionContext：取证场景，复用本包既有的 15 类 EvidenceType。
//
// 两轴各自出规则、分数按增量叠加：法定种类回答"这类证据在形式上是否齐备"，
// 取证场景回答"它如何取得、取得过程是否留有痕迹"。二者互不推导——互联网
// 公开的内容可能是电子数据，也可能是视听资料，公开时间与可及性的判断不因
// 法定种类而改变。因此本包不提供 EvidenceType ↔ EvidenceKind 的映射表。
//
// 未声明的事实按"未通过"记入 finding，但不改变分数（分数只对已声明的事实用
// 增量修正）。这样漏填字段不会凭空抬高评分，也不会替调用方推定结论。

// StatutoryToolName 是 evaluate_legal_evidence 工具对模型可见的名称。
const StatutoryToolName = "evaluate_legal_evidence"

// 二维模型的评分基线、三性权重与轴标识。
const (
	statutoryBaseScore          = 0.5
	statutoryWeightRelevance    = 0.35
	statutoryWeightLegality     = 0.30
	statutoryWeightAuthenticity = 0.35

	// AxisCommon 是两轴共用的基础检查；AxisKind/AxisAcquisition 分别对应
	// 法定种类轴与取证场景轴。
	AxisCommon      = "common"
	AxisKind        = "kind"
	AxisAcquisition = "acquisition"
)

// EvidenceKind 是《民事诉讼法》第 66 条规定的法定证据种类（正交轴 1）。
//
//nolint:revive // stutter: 与方案文档 §6.5 的 EvidenceKind 命名一致，也与包内既有 EvidenceType/EvidenceRule 同风格
type EvidenceKind string

// Statutory evidence kind constants.
const (
	KindPartyStatement   EvidenceKind = "party_statement"   // 当事人的陈述
	KindDocumentary      EvidenceKind = "documentary"       // 书证
	KindPhysical         EvidenceKind = "physical"          // 物证
	KindAudioVisual      EvidenceKind = "audio_visual"      // 视听资料
	KindElectronic       EvidenceKind = "electronic"        // 电子数据
	KindWitnessTestimony EvidenceKind = "witness_testimony" // 证人证言
	KindExpertOpinion    EvidenceKind = "expert_opinion"    // 鉴定意见
	KindInspectionRecord EvidenceKind = "inspection_record" // 勘验笔录
)

// Valid reports whether the kind is one of the eight statutory evidence kinds.
func (k EvidenceKind) Valid() bool {
	switch k {
	case KindPartyStatement, KindDocumentary, KindPhysical, KindAudioVisual,
		KindElectronic, KindWitnessTestimony, KindExpertOpinion, KindInspectionRecord:
		return true
	default:
		return false
	}
}

// Label returns the Chinese name used in reasoning text.
func (k EvidenceKind) Label() string {
	switch k {
	case KindPartyStatement:
		return "当事人陈述"
	case KindDocumentary:
		return "书证"
	case KindPhysical:
		return "物证"
	case KindAudioVisual:
		return "视听资料"
	case KindElectronic:
		return "电子数据"
	case KindWitnessTestimony:
		return "证人证言"
	case KindExpertOpinion:
		return "鉴定意见"
	case KindInspectionRecord:
		return "勘验笔录"
	default:
		return string(k)
	}
}

// AcquisitionContext 是取证场景轴（正交轴 2）。
//
// 复用本包既有的 15 类 EvidenceType 作为场景标识：它们描述证据"如何取得或
// 形成"，与法定种类互不推导。只有真正涉及取得过程的场景（互联网公开、
// 使用公开、域外取得、外文形成、公证）带规则，其余场景只参与基础检查。
type AcquisitionContext = EvidenceType

// StatutoryEvidenceInput 是一条待评估证据的声明式输入。
//
// 布尔字段用指针区分"未声明"与"明确为否"：未声明只记一条未通过 finding，
// 不加不减分数；明确为否才按规则扣分。工具层直接从 JSON 反序列化本结构。
type StatutoryEvidenceInput struct {
	EvidenceID  string             `json:"evidence_id"`
	Kind        EvidenceKind       `json:"kind"`
	Acquisition AcquisitionContext `json:"acquisition_context"`
	FactToProve string             `json:"fact_to_prove"`
	Content     string             `json:"content"`
	Source      string             `json:"source"`

	HasOriginal           *bool  `json:"has_original"`
	OriginalAbsenceReason string `json:"original_absence_reason"`
	SignatureVerified     *bool  `json:"signature_verified"`
	FormationTimeClear    *bool  `json:"formation_time_clear"`

	ContentHash              string `json:"content_hash"`
	HashVerified             *bool  `json:"hash_verified"`
	Timestamp                string `json:"timestamp"`
	StorageIntegrityVerified *bool  `json:"storage_integrity_verified"`

	WitnessHasConflict *bool `json:"witness_has_conflict"`
	ConflictDisclosed  *bool `json:"conflict_disclosed"`
	Corroborated       *bool `json:"corroborated"`

	ExpertQualified       *bool `json:"expert_qualified"`
	ExpertProcedureLegal  *bool `json:"expert_procedure_legal"`
	ExpertConclusionClear *bool `json:"expert_conclusion_clear"`

	InspectionHasWitness     *bool `json:"inspection_has_witness"`
	InspectionRecordComplete *bool `json:"inspection_record_complete"`

	CollectionLegal *bool `json:"collection_legal"`
	FormLegal       *bool `json:"form_legal"`

	// 取证场景轴专用声明。
	Notarized             *bool `json:"notarized"`
	Translated            *bool `json:"translated"`
	PublicDateClear       *bool `json:"public_date_clear"`
	PubliclyAccessible    *bool `json:"publicly_accessible"`
	PublicUseFourElements *bool `json:"public_use_four_elements"`
}

// StatutoryFinding 是一条规则检查结果。Passed=false 且 Reason 含"未声明"
// 表示该事实未被声明，结论据此保留。Delta 是该检查对所属维度的分值修正。
type StatutoryFinding struct {
	RuleID     string  `json:"rule_id"`
	Axis       string  `json:"axis"`
	Dimension  string  `json:"dimension"`
	Passed     bool    `json:"passed"`
	Severity   string  `json:"severity"`
	LegalBasis string  `json:"legal_basis,omitempty"`
	Delta      float64 `json:"delta"`
	Reason     string  `json:"reason"`
}

// StatutoryAssessment 是单条证据的二维三性评估结果。
type StatutoryAssessment struct {
	EvidenceID   string             `json:"evidence_id"`
	Kind         EvidenceKind       `json:"kind"`
	KindLabel    string             `json:"kind_label"`
	Acquisition  AcquisitionContext `json:"acquisition_context"`
	Relevance    DimensionJudgment  `json:"relevance"`
	Legality     DimensionJudgment  `json:"legality"`
	Authenticity DimensionJudgment  `json:"authenticity"`
	Overall      float64            `json:"overall"`
	OverallLevel string             `json:"overall_level"`
	Findings     []StatutoryFinding `json:"findings"`
	Reasoning    string             `json:"reasoning"`
}

// statutoryContribution 是一条检查对评分与 finding 的贡献。
// RuleID 为空时只参与评分，不生成 finding。
type statutoryContribution struct {
	Axis       string
	Dimension  string
	RuleID     string
	LegalBasis string
	Severity   string
	Passed     bool
	Delta      float64
	Reason     string
}

// declaredBool 表示三值声明：未声明 / 是 / 否。
type declaredBool int

const (
	declaredUnknown declaredBool = iota
	declaredTrue
	declaredFalse
)

func boolState(p *bool) declaredBool {
	switch {
	case p == nil:
		return declaredUnknown
	case *p:
		return declaredTrue
	default:
		return declaredFalse
	}
}

// statutoryCheck 是一条带规则编号的三值检查。
type statutoryCheck struct {
	axis          string
	dimension     string
	id            string
	legalBasis    string
	severity      string
	deltaTrue     float64
	deltaFalse    float64
	reasonTrue    string
	reasonFalse   string
	reasonUnknown string
}

// contribution 把声明状态翻译成一条贡献：未声明不加分也不扣分，但记未通过。
func (c statutoryCheck) contribution(state declaredBool) statutoryContribution {
	out := statutoryContribution{
		Axis:       c.axis,
		Dimension:  c.dimension,
		RuleID:     c.id,
		LegalBasis: c.legalBasis,
		Severity:   c.severity,
	}
	switch state {
	case declaredTrue:
		out.Passed = true
		out.Delta = c.deltaTrue
		out.Reason = c.reasonTrue
	case declaredFalse:
		out.Delta = c.deltaFalse
		out.Reason = c.reasonFalse
	default:
		out.Reason = c.reasonUnknown
	}
	return out
}

// AssessStatutoryEvidence 对单条证据做二维三性评估。
//
// 未知的法定种类或取证场景直接报错，不降级为通用评分——静默套用通用规则会
// 让"用错了种类"看起来像一次正常评估。
func AssessStatutoryEvidence(in StatutoryEvidenceInput) (StatutoryAssessment, error) {
	if !in.Kind.Valid() {
		return StatutoryAssessment{}, fmt.Errorf("未知的法定证据种类 %q，可选值：%s",
			in.Kind, strings.Join(evidenceKindNames(), "、"))
	}
	if in.Acquisition == "" {
		in.Acquisition = EvTypeGeneral
	}
	if !in.Acquisition.Valid() {
		return StatutoryAssessment{}, fmt.Errorf("未知的取证场景 %q，可选值：%s",
			in.Acquisition, strings.Join(evidenceTypeNames(), "、"))
	}

	contributions := statutoryContributions(in)
	scores := map[string]float64{
		judgmentRelevance:    statutoryBaseScore,
		judgmentLegality:     statutoryBaseScore,
		judgmentAuthenticity: statutoryBaseScore,
	}
	var findings []StatutoryFinding
	for _, c := range contributions {
		scores[c.Dimension] += c.Delta
		if c.RuleID == "" {
			continue
		}
		findings = append(findings, StatutoryFinding{
			RuleID:     c.RuleID,
			Axis:       c.Axis,
			Dimension:  c.Dimension,
			Passed:     c.Passed,
			Severity:   c.Severity,
			LegalBasis: c.LegalBasis,
			Delta:      c.Delta,
			Reason:     c.Reason,
		})
	}
	for dim, score := range scores {
		scores[dim] = clampScore(score)
	}

	relevance := statutoryDimension(judgmentRelevance, scores[judgmentRelevance], contributions)
	legality := statutoryDimension(judgmentLegality, scores[judgmentLegality], contributions)
	authenticity := statutoryDimension(judgmentAuthenticity, scores[judgmentAuthenticity], contributions)
	overall := clampScore(relevance.Score*statutoryWeightRelevance +
		legality.Score*statutoryWeightLegality +
		authenticity.Score*statutoryWeightAuthenticity)

	if findings == nil {
		findings = []StatutoryFinding{}
	}
	return StatutoryAssessment{
		EvidenceID:   in.EvidenceID,
		Kind:         in.Kind,
		KindLabel:    in.Kind.Label(),
		Acquisition:  in.Acquisition,
		Relevance:    relevance,
		Legality:     legality,
		Authenticity: authenticity,
		Overall:      overall,
		OverallLevel: statutoryLevel(overall),
		Findings:     findings,
		Reasoning:    statutoryReasoning(in, relevance, legality, authenticity, findings),
	}, nil
}

// statutoryContributions 按轴收集贡献：基础 → 法定种类 → 取证场景。
// 轴之间只做加法，任何一个轴的结论都不会覆盖另一个轴。
func statutoryContributions(in StatutoryEvidenceInput) []statutoryContribution {
	out := commonStatutoryContributions(in)
	out = append(out, kindStatutoryContributions(in)...)
	out = append(out, acquisitionStatutoryContributions(in)...)
	return out
}

// commonStatutoryContributions 是两轴共用的基础检查。
//
// 电子数据、证人证言、鉴定意见的合法性要点由种类轴单独给出（扣分幅度不同），
// 此时跳过通用合法性，避免同一事实被扣两次。
func commonStatutoryContributions(in StatutoryEvidenceInput) []statutoryContribution {
	var out []statutoryContribution
	add := func(dimension string, delta float64, reason string) {
		out = append(out, statutoryContribution{
			Axis: AxisCommon, Dimension: dimension, Delta: delta, Reason: reason,
		})
	}

	if strings.TrimSpace(in.FactToProve) != "" {
		add(judgmentRelevance, 0.15, "已明确待证事实")
	}
	if strings.TrimSpace(in.Content) != "" {
		add(judgmentRelevance, 0.15, "已提供证据内容")
	}
	if strings.TrimSpace(in.Source) != "" {
		add(judgmentRelevance, 0.10, "已记录证据来源")
	}

	if kindDefinesLegality(in.Kind) {
		return out
	}
	if strings.TrimSpace(in.Source) != "" {
		add(judgmentLegality, 0.05, "已记录证据来源")
	}
	switch boolState(in.CollectionLegal) {
	case declaredTrue:
		add(judgmentLegality, 0.25, "证据收集合法")
	case declaredFalse:
		out = append(out, statutoryContribution{
			Axis:       AxisCommon,
			Dimension:  judgmentLegality,
			RuleID:     "LEGAL-EVI-GEN-002",
			LegalBasis: "最高人民法院关于民事诉讼证据的若干规定第106条",
			Severity:   judgmentCritical,
			Delta:      -0.20,
			Reason:     "证据收集合法性存疑",
		})
	}
	switch boolState(in.FormLegal) {
	case declaredTrue:
		add(judgmentLegality, 0.20, "证据形式合法")
	case declaredFalse:
		add(judgmentLegality, -0.15, "证据形式合法性存疑")
	}
	return out
}

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

// acquisitionStatutoryContributions 是取证场景轴（正交轴 2）的规则。
func acquisitionStatutoryContributions(in StatutoryEvidenceInput) []statutoryContribution {
	var out []statutoryContribution
	check := func(c statutoryCheck, state declaredBool) {
		out = append(out, c.contribution(state))
	}

	switch in.Acquisition {
	case EvTypeInternetPublication:
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentAuthenticity, id: "LEGAL-EVI-ACQ-001",
			legalBasis: "专利审查指南第二部分第三章2.1.2.2", severity: "warning",
			deltaTrue: 0.10, deltaFalse: -0.10,
			reasonTrue: "互联网公开时间明确", reasonFalse: "互联网公开时间不明确",
			reasonUnknown: "未声明互联网公开时间是否明确",
		}, boolState(in.PublicDateClear))
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentAuthenticity, id: "LEGAL-EVI-ACQ-002",
			legalBasis: "专利审查指南第二部分第三章2.1.2.2", severity: "warning",
			deltaTrue: 0.10, deltaFalse: -0.10,
			reasonTrue: "内容处于公众可获取状态", reasonFalse: "内容不处于公众可获取状态",
			reasonUnknown: "未声明内容是否处于公众可获取状态",
		}, boolState(in.PubliclyAccessible))

	case EvTypePublicUse:
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentAuthenticity, id: "LEGAL-EVI-ACQ-003",
			legalBasis: "专利审查指南第二部分第三章2.1.2.3", severity: "warning",
			deltaTrue: 0.15, deltaFalse: -0.15,
			reasonTrue: "使用公开四要件齐备", reasonFalse: "使用公开四要件不齐备",
			reasonUnknown: "未声明使用公开四要件是否齐备",
		}, boolState(in.PublicUseFourElements))

	case EvTypeOverseas:
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentLegality, id: "LEGAL-EVI-ACQ-004",
			legalBasis: "最高人民法院关于民事诉讼证据的若干规定第16条",
			severity:   judgmentCritical, deltaTrue: 0.20, deltaFalse: -0.20,
			reasonTrue: "域外证据已办理公证认证手续", reasonFalse: "域外证据未办理公证认证手续",
			reasonUnknown: "未声明域外证据是否办理公证认证手续",
		}, boolState(in.Notarized))

	case EvTypeForeignLang:
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentLegality, id: "LEGAL-EVI-ACQ-005",
			legalBasis: "民事诉讼法第73条", severity: "warning",
			deltaTrue: 0.15, deltaFalse: -0.20,
			reasonTrue: "外文证据附有中文译本", reasonFalse: "外文证据未附中文译本",
			reasonUnknown: "未声明外文证据是否附有中文译本",
		}, boolState(in.Translated))

	case EvTypeNotarial:
		check(statutoryCheck{
			axis: AxisAcquisition, dimension: judgmentAuthenticity, id: "LEGAL-EVI-ACQ-006",
			legalBasis: "民事诉讼法第72条", severity: "warning",
			deltaTrue: 0.20, deltaFalse: -0.15,
			reasonTrue: "已经法定程序公证证明", reasonFalse: "未经法定程序公证证明",
			reasonUnknown: "未声明是否经过公证证明",
		}, boolState(in.Notarized))
	}
	return out
}

// statutoryDimension 汇总单个维度上的贡献，生成判定与理由。
func statutoryDimension(dimension string, score float64, contributions []statutoryContribution) DimensionJudgment {
	var reasons []string
	for _, c := range contributions {
		if c.Dimension == dimension && c.Reason != "" {
			reasons = append(reasons, c.Reason)
		}
	}
	reasoning := "基础评分（未声明补充事实）"
	if len(reasons) > 0 {
		reasoning = strings.Join(reasons, "；")
	}
	return DimensionJudgment{
		Dimension: dimension,
		Score:     score,
		Level:     statutoryLevel(score),
		Reasoning: reasoning,
	}
}

// statutoryReasoning 生成综合推理说明。
func statutoryReasoning(in StatutoryEvidenceInput, relevance, legality, authenticity DimensionJudgment, findings []StatutoryFinding) string {
	parts := []string{
		fmt.Sprintf("证据种类[%s]×取证场景[%s]", in.Kind.Label(), in.Acquisition),
		fmt.Sprintf("关联性[%s %.2f]：%s", relevance.Level, relevance.Score, relevance.Reasoning),
		fmt.Sprintf("合法性[%s %.2f]：%s", legality.Level, legality.Score, legality.Reasoning),
		fmt.Sprintf("真实性[%s %.2f]：%s", authenticity.Level, authenticity.Score, authenticity.Reasoning),
	}
	var critical []string
	for _, f := range findings {
		if !f.Passed && f.Severity == judgmentCritical {
			critical = append(critical, f.Reason)
		}
	}
	if len(critical) > 0 {
		parts = append(parts, fmt.Sprintf("存在 %d 项需优先处理的问题：%s",
			len(critical), strings.Join(critical, "；")))
	} else {
		parts = append(parts, "未发现需优先处理的问题（结论以已声明事实为限）")
	}
	return strings.Join(parts, "；")
}

func statutoryLevel(score float64) string {
	switch {
	case score >= 0.85:
		return judgmentLevelHigh
	case score >= 0.65:
		return judgmentLevelMediumHigh
	case score >= 0.45:
		return "medium"
	default:
		return judgmentLevelLow
	}
}

func clampScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

// evidenceKindNames 返回全部法定种类的字符串表示，用于参数校验报错。
func evidenceKindNames() []string {
	return []string{
		string(KindPartyStatement), string(KindDocumentary), string(KindPhysical),
		string(KindAudioVisual), string(KindElectronic), string(KindWitnessTestimony),
		string(KindExpertOpinion), string(KindInspectionRecord),
	}
}

// evidenceTypeNames 返回全部取证场景（既有 15 类证据类型）。
func evidenceTypeNames() []string {
	return []string{
		string(EvTypeGeneral), string(EvTypeForeignLang), string(EvTypeOverseas),
		string(EvTypeElectronic), string(EvTypeWitness), string(EvTypeExpert),
		string(EvTypeCommonKnowledge), string(EvTypeNotarial), string(EvTypeBurden),
		string(EvTypeStandard), string(EvTypePriorArtDate), string(EvTypeProcedural),
		string(EvTypeInternetPublication), string(EvTypePublicUse), string(EvTypeDesignComparison),
	}
}
