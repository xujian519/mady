package evidence

import (
	"fmt"
	"strings"
)

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
