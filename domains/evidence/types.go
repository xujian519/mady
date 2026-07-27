package evidence

import (
	"time"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

// EvidenceType 对证据判断规则所适用的证据类型进行分类。
type EvidenceType string

// Evidence type constants.
const (
	EvTypeGeneral             EvidenceType = "general"
	EvTypeForeignLang         EvidenceType = "foreign_language"
	EvTypeOverseas            EvidenceType = "overseas"
	EvTypeElectronic          EvidenceType = "electronic"
	EvTypeWitness             EvidenceType = "witness_testimony"
	EvTypeExpert              EvidenceType = "expert_opinion"
	EvTypeCommonKnowledge     EvidenceType = "common_knowledge"
	EvTypeNotarial            EvidenceType = "notarial_certificate"
	EvTypeBurden              EvidenceType = "burden_of_proof"
	EvTypeStandard            EvidenceType = "standard_of_proof"
	EvTypePriorArtDate        EvidenceType = "prior_art_date"
	EvTypeProcedural          EvidenceType = "procedural"
	EvTypeInternetPublication EvidenceType = "internet_publication" // 互联网公开
	EvTypePublicUse           EvidenceType = "public_use"           // 使用公开
	EvTypeDesignComparison    EvidenceType = "design_comparison"    // 设计对比证据
)

// Valid reports whether the evidence type is a recognized value.
func (t EvidenceType) Valid() bool {
	switch t {
	case EvTypeGeneral, EvTypeForeignLang, EvTypeOverseas, EvTypeElectronic,
		EvTypeWitness, EvTypeExpert, EvTypeCommonKnowledge, EvTypeNotarial,
		EvTypeBurden, EvTypeStandard, EvTypePriorArtDate, EvTypeProcedural,
		EvTypeInternetPublication, EvTypePublicUse, EvTypeDesignComparison:
		return true
	default:
		return false
	}
}

// CredibilityLevel rates the credibility of an evidence item or source.
type CredibilityLevel string

// Credibility level constants.
const (
	CredHigh       CredibilityLevel = "high"
	CredMediumHigh CredibilityLevel = "medium_high"
	CredMedium     CredibilityLevel = "medium"
	CredLow        CredibilityLevel = "low"
)

// AssessmentType defines the scoring methodology for a rule dimension.
type AssessmentType string

// Assessment type constants.
const (
	AssessTripleAttr  AssessmentType = "triple-attribute"
	AssessBinary      AssessmentType = "binary"
	AssessScored      AssessmentType = "scored"
	AssessMultiCond   AssessmentType = "multi_condition"
	AssessCredScaled  AssessmentType = "credibility_scaled"
	AssessConditional AssessmentType = "conditional"
)

// EvidenceRule describes a single evidence judgment rule, its
// legal basis, severity, and assessment methodology.
type EvidenceRule struct {
	RuleID             string              `yaml:"ruleId" json:"rule_id"`
	Name               string              `yaml:"name" json:"name"`
	Description        string              `yaml:"description" json:"description"`
	LegalBasis         string              `yaml:"legalBasis" json:"legal_basis"`
	Domain             string              `yaml:"domain" json:"domain"`
	Severity           string              `yaml:"severity" json:"severity"`
	Action             string              `yaml:"action" json:"action"`
	EvidenceType       EvidenceType        `yaml:"evidenceType" json:"evidence_type"`
	Check              *RuleCheck          `yaml:"check,omitempty" json:"check,omitempty"`
	EvidenceAssessment *EvidenceAssessment `yaml:"evidenceAssessment,omitempty" json:"evidence_assessment,omitempty"`
}

// RuleCheck defines the check logic for an evidence rule, including
// the check type, method, and applicable principles.
type RuleCheck struct {
	Type       string   `yaml:"type" json:"type"`
	Method     string   `yaml:"method" json:"method"`
	Principles []string `yaml:"principles,omitempty" json:"principles,omitempty"`
	Rules      []string `yaml:"rules,omitempty" json:"rules,omitempty"`
	Conditions []string `yaml:"conditions,omitempty" json:"conditions,omitempty"`
}

// EvidenceAssessment configures the assessment methodology, dimensions,
// platform credibility, and any exemptions for a rule.
type EvidenceAssessment struct {
	AssessmentType      AssessmentType        `yaml:"assessmentType" json:"assessment_type"`
	Dimensions          []AssessmentDimension `yaml:"dimensions,omitempty" json:"dimensions,omitempty"`
	PlatformCredibility map[string]ScoreLabel `yaml:"platformCredibility,omitempty" json:"platform_credibility,omitempty"`
	Exemptions          []string              `yaml:"exemptions,omitempty" json:"exemptions,omitempty"`
	Conditions          map[string]string     `yaml:"conditions,omitempty" json:"conditions,omitempty"`
}

// AssessmentDimension defines one dimension within an evidence assessment,
// with its weight and allowed score levels.
type AssessmentDimension struct {
	Name   string       `yaml:"name" json:"name"`
	Weight float64      `yaml:"weight" json:"weight"`
	Levels []ScoreLevel `yaml:"levels" json:"levels"`
}

// ScoreLevel defines one score band within a scoring dimension.
type ScoreLevel struct {
	Value       string  `yaml:"value" json:"value"`
	Score       float64 `yaml:"score" json:"score"`
	Description string  `yaml:"description,omitempty" json:"description,omitempty"`
}

// ScoreLabel maps a numeric score to a human-readable label.
type ScoreLabel struct {
	Score float64 `yaml:"score" json:"score"`
	Label string  `yaml:"label" json:"label"`
}

// EvidenceJudgment is the complete evaluation result for one evidence span,
// covering relevance, legality, authenticity, and type-specific dimensions.
type EvidenceJudgment struct {
	SpanID               string                `json:"span_id"`
	RelevanceJudgment    *DimensionJudgment    `json:"relevance_judgment,omitempty"`
	LegalityJudgment     *DimensionJudgment    `json:"legality_judgment,omitempty"`
	AuthenticityJudgment *DimensionJudgment    `json:"authenticity_judgment,omitempty"`
	TypeSpecificJudgment *TypeSpecificJudgment `json:"type_specific_judgment,omitempty"`
	OverallScore         float64               `json:"overall_score"`
	Confidence           float64               `json:"confidence"`
	Reasoning            string                `json:"reasoning"`
	FlaggedIssues        []JudgmentIssue       `json:"flagged_issues,omitempty"`
	EvaluatedAt          time.Time             `json:"evaluated_at"`
}

// DimensionJudgment stores the evaluation result for a single dimension
// (e.g. relevance, legality, authenticity).
type DimensionJudgment struct {
	Dimension string  `json:"dimension"`
	Score     float64 `json:"score"`
	Level     string  `json:"level"`
	Reasoning string  `json:"reasoning"`
}

// DateReliability 表示日期确定的可靠程度。
type DateReliability string

// Date reliability constants.
const (
	RelHigh   DateReliability = "high"
	RelMedium DateReliability = "medium"
	RelLow    DateReliability = "low"
)

// DateSourceType 表示日期来源的类型。
type DateSourceType string

// Date source type constants.
const (
	SrcExactPage      DateSourceType = "exact_page_date"     // 页面明确标注的日期
	SrcHTTPHeader     DateSourceType = "http_header"         // HTTP 响应头中的日期
	SrcWaybackMachine DateSourceType = "wayback_machine"     // Wayback Machine 记录
	SrcDomainReg      DateSourceType = "domain_registration" // 域名注册日期
	SrcClaimed        DateSourceType = "claimed_date"        // 主张方声称的日期
	SrcInferred       DateSourceType = "inferred"            // 根据上下文推断
)

// ContentIntegrityStatus 表示互联网证据内容完整性状态。
type ContentIntegrityStatus string

// Content integrity status constants.
const (
	IntegrityVerified   ContentIntegrityStatus = "verified"   // 内容完整性已验证
	IntegrityPartial    ContentIntegrityStatus = "partial"    // 部分可验证
	IntegrityUnverified ContentIntegrityStatus = "unverified" // 无法验证完整性
)

// PublicIntent 表示互联网公开的公开意图。
type PublicIntent string

// Public intent constants.
const (
	IntentPublic     PublicIntent = "public"     // 对公众开放
	IntentRestricted PublicIntent = "restricted" // 受限制访问（收费/注册墙）
)

// FourElementsResult 表示使用公开四要件的检查结果。
type FourElementsResult struct {
	TimeElement   ElementResult `json:"time_element"`   // 公开时间
	PlaceElement  ElementResult `json:"place_element"`  // 公开地点
	MethodElement ElementResult `json:"method_element"` // 公开方式
	Accessibility ElementResult `json:"accessibility"`  // 公众可获取性
}

// AllMet 检查四要件是否全部满足。
func (f *FourElementsResult) AllMet() bool {
	return f != nil && f.TimeElement.Met && f.PlaceElement.Met &&
		f.MethodElement.Met && f.Accessibility.Met
}

// OverallScore 计算四要件综合评分（0-1）。
func (f *FourElementsResult) OverallScore() float64 {
	if f == nil {
		return 0
	}
	return (f.TimeElement.Score + f.PlaceElement.Score +
		f.MethodElement.Score + f.Accessibility.Score) / 4
}

// ElementResult records the single-element outcome of a four-elements check,
// indicating whether the element is met and at what score.
type ElementResult struct {
	Met    bool    `json:"met"`
	Score  float64 `json:"score"`
	Detail string  `json:"detail"`
}

// TypeSpecificJudgment captures evidence-type-specific evaluation fields,
// such as platform credibility, translation status, and content integrity.
type TypeSpecificJudgment struct {
	EvidenceType        EvidenceType       `json:"evidence_type"`
	PlatformCredibility *CredibilityLevel  `json:"platform_credibility,omitempty"`
	TranslationStatus   string             `json:"translation_status,omitempty"`
	NotarizationStatus  string             `json:"notarization_status,omitempty"`
	ExemptionApplied    string             `json:"exemption_applied,omitempty"`
	WitnessCredibility  string             `json:"witness_credibility,omitempty"`
	DateDetermination   *DateDetermination `json:"date_determination,omitempty"`
	DeadlineStatus      string             `json:"deadline_status,omitempty"`
	// 互联网公开专用字段
	ContentIntegrity ContentIntegrityStatus `json:"content_integrity,omitempty"` // 内容完整性
	PublicIntent     PublicIntent           `json:"public_intent,omitempty"`     // 公开意图
	PlatformCategory string                 `json:"platform_category,omitempty"` // 平台分类
	// 可信度分数（0-1），由 CredibilityToScore 在类型评估时计算
	CredibilityScore *float64 `json:"credibility_score,omitempty"`
	// 使用公开专用字段
	FourElementsCheck *FourElementsResult `json:"four_elements_check,omitempty"` // 四要件检查
	BurdenDifficulty  string              `json:"burden_difficulty,omitempty"`   // 举证难度
	ChainIntegrity    string              `json:"chain_integrity,omitempty"`     // 证据链完整性
}

// DateDetermination records the determined date for an evidence item,
// including the method used and reliability assessment.
type DateDetermination struct {
	SourceDate  string          `json:"source_date"`
	Determined  string          `json:"determined"`
	Method      string          `json:"method"`
	IsPriorArt  bool            `json:"is_prior_art"`
	FilingDate  string          `json:"filing_date"`
	Reliability DateReliability `json:"reliability,omitempty"`
	SourceType  DateSourceType  `json:"source_type,omitempty"`
}

// JudgmentIssue records a flagged issue discovered during evidence evaluation.
type JudgmentIssue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// BurdenDetermination records the outcome of a burden-of-proof analysis,
// identifying who bears the burden and whether it has shifted.
type BurdenDetermination struct {
	BurdenHolder string `json:"burden_holder"`
	Standard     string `json:"standard"`
	HasShifted   bool   `json:"has_shifted"`
	ShiftReason  string `json:"shift_reason,omitempty"`
	Reasoning    string `json:"reasoning"`
}

// ProofStandardResult records whether a given standard of proof has been met,
// along with supporting and contradicting evidence counts.
type ProofStandardResult struct {
	Met                bool     `json:"met"`
	Standard           string   `json:"standard"`
	Confidence         float64  `json:"confidence"`
	SupportingCount    int      `json:"supporting_count"`
	ContradictingCount int      `json:"contradicting_count"`
	Reasoning          string   `json:"reasoning"`
	Gaps               []string `json:"gaps,omitempty"`
}

// EvidenceJudgmentEngine defines the interface for judging evidence spans
// and assessing burden of proof, proof standards, and rules.
type EvidenceJudgmentEngine interface {
	Judge(span agentcore_evidence.EvidenceSpan) (*EvidenceJudgment, error)
	BatchJudge(spans []agentcore_evidence.EvidenceSpan) ([]*EvidenceJudgment, error)
	AssessBurdenOfProof(caseType string, context map[string]string) (*BurdenDetermination, error)
	AssessProofStandard(judgments []*EvidenceJudgment, standard string) (*ProofStandardResult, error)
	LoadRules(yamlBytes []byte) error
	GetRules() []EvidenceRule
	GetRulesByType(evType EvidenceType) []EvidenceRule
}
