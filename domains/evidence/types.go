package evidence

import (
	"time"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

// EvidenceType 对证据判断规则所适用的证据类型进行分类。
type EvidenceType string

const (
	EvTypeGeneral         EvidenceType = "general"
	EvTypeForeignLang     EvidenceType = "foreign_language"
	EvTypeOverseas        EvidenceType = "overseas"
	EvTypeElectronic      EvidenceType = "electronic"
	EvTypeWitness         EvidenceType = "witness_testimony"
	EvTypeExpert          EvidenceType = "expert_opinion"
	EvTypeCommonKnowledge EvidenceType = "common_knowledge"
	EvTypeNotarial        EvidenceType = "notarial_certificate"
	EvTypeBurden          EvidenceType = "burden_of_proof"
	EvTypeStandard        EvidenceType = "standard_of_proof"
	EvTypePriorArtDate    EvidenceType = "prior_art_date"
	EvTypeProcedural      EvidenceType = "procedural"
)

func (t EvidenceType) Valid() bool {
	switch t {
	case EvTypeGeneral, EvTypeForeignLang, EvTypeOverseas, EvTypeElectronic,
		EvTypeWitness, EvTypeExpert, EvTypeCommonKnowledge, EvTypeNotarial,
		EvTypeBurden, EvTypeStandard, EvTypePriorArtDate, EvTypeProcedural:
		return true
	default:
		return false
	}
}

type CredibilityLevel string

const (
	CredHigh       CredibilityLevel = "high"
	CredMediumHigh CredibilityLevel = "medium_high"
	CredMedium     CredibilityLevel = "medium"
	CredLow        CredibilityLevel = "low"
)

type AssessmentType string

const (
	AssessTripleAttr  AssessmentType = "triple-attribute"
	AssessBinary      AssessmentType = "binary"
	AssessScored      AssessmentType = "scored"
	AssessMultiCond   AssessmentType = "multi_condition"
	AssessCredScaled  AssessmentType = "credibility_scaled"
	AssessConditional AssessmentType = "conditional"
)

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

type RuleCheck struct {
	Type       string   `yaml:"type" json:"type"`
	Method     string   `yaml:"method" json:"method"`
	Principles []string `yaml:"principles,omitempty" json:"principles,omitempty"`
	Rules      []string `yaml:"rules,omitempty" json:"rules,omitempty"`
	Conditions []string `yaml:"conditions,omitempty" json:"conditions,omitempty"`
}

type EvidenceAssessment struct {
	AssessmentType      AssessmentType        `yaml:"assessmentType" json:"assessment_type"`
	Dimensions          []AssessmentDimension `yaml:"dimensions,omitempty" json:"dimensions,omitempty"`
	PlatformCredibility map[string]ScoreLabel `yaml:"platformCredibility,omitempty" json:"platform_credibility,omitempty"`
	Exemptions          []string              `yaml:"exemptions,omitempty" json:"exemptions,omitempty"`
	Conditions          map[string]string     `yaml:"conditions,omitempty" json:"conditions,omitempty"`
}

type AssessmentDimension struct {
	Name   string       `yaml:"name" json:"name"`
	Weight float64      `yaml:"weight" json:"weight"`
	Levels []ScoreLevel `yaml:"levels" json:"levels"`
}

type ScoreLevel struct {
	Value       string  `yaml:"value" json:"value"`
	Score       float64 `yaml:"score" json:"score"`
	Description string  `yaml:"description,omitempty" json:"description,omitempty"`
}

type ScoreLabel struct {
	Score float64 `yaml:"score" json:"score"`
	Label string  `yaml:"label" json:"label"`
}

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

type DimensionJudgment struct {
	Dimension string  `json:"dimension"`
	Score     float64 `json:"score"`
	Level     string  `json:"level"`
	Reasoning string  `json:"reasoning"`
}

type TypeSpecificJudgment struct {
	EvidenceType        EvidenceType       `json:"evidence_type"`
	PlatformCredibility *CredibilityLevel  `json:"platform_credibility,omitempty"`
	TranslationStatus   string             `json:"translation_status,omitempty"`
	NotarizationStatus  string             `json:"notarization_status,omitempty"`
	ExemptionApplied    string             `json:"exemption_applied,omitempty"`
	WitnessCredibility  string             `json:"witness_credibility,omitempty"`
	DateDetermination   *DateDetermination `json:"date_determination,omitempty"`
	DeadlineStatus      string             `json:"deadline_status,omitempty"`
}

type DateDetermination struct {
	SourceDate string `json:"source_date"`
	Determined string `json:"determined"`
	Method     string `json:"method"`
	IsPriorArt bool   `json:"is_prior_art"`
	FilingDate string `json:"filing_date"`
}

type JudgmentIssue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

type BurdenDetermination struct {
	BurdenHolder string `json:"burden_holder"`
	Standard     string `json:"standard"`
	HasShifted   bool   `json:"has_shifted"`
	ShiftReason  string `json:"shift_reason,omitempty"`
	Reasoning    string `json:"reasoning"`
}

type ProofStandardResult struct {
	Met                bool     `json:"met"`
	Standard           string   `json:"standard"`
	Confidence         float64  `json:"confidence"`
	SupportingCount    int      `json:"supporting_count"`
	ContradictingCount int      `json:"contradicting_count"`
	Reasoning          string   `json:"reasoning"`
	Gaps               []string `json:"gaps,omitempty"`
}

type EvidenceJudgmentEngine interface {
	Judge(span agentcore_evidence.EvidenceSpan) (*EvidenceJudgment, error)
	BatchJudge(spans []agentcore_evidence.EvidenceSpan) ([]*EvidenceJudgment, error)
	AssessBurdenOfProof(caseType string, context map[string]string) (*BurdenDetermination, error)
	AssessProofStandard(judgments []*EvidenceJudgment, standard string) (*ProofStandardResult, error)
	LoadRules(yamlBytes []byte) error
	GetRules() []EvidenceRule
	GetRulesByType(evType EvidenceType) []EvidenceRule
}
