package adversary

// Relation 标记争议点与基线答复计划的关系。
type Relation string

// Relation constants.
const (
	RelationAgreement        Relation = "agreement"
	RelationDisagreement     Relation = "disagreement"
	RelationMissedByBaseline Relation = "missed_by_baseline"
)

// Tag 标记争议点来源：unverified = 模型不可用时的占位；heuristic = 确定性规则识别。
type Tag string

// Tag constants.
const (
	TagUnverified Tag = "unverified"
	TagHeuristic  Tag = "heuristic"
)

// Dispute 是一条争议点。
type Dispute struct {
	Statute          string   `json:"statute"`
	Claims           []int    `json:"claims"`
	Point            string   `json:"point"`
	Relation         Relation `json:"relation"`
	DivergenceReason string   `json:"divergence_reason,omitempty"`
	Confidence       float64  `json:"confidence"`
	Tag              Tag      `json:"tag"`
}

// OfficeAction 是对立审查员读取的通知书结构化字段（最小契约）。
type OfficeAction struct {
	RejectionTypes    []string `json:"rejection_types"`
	AffectedClaims    []int    `json:"affected_claims"`
	ExaminerArguments []string `json:"examiner_arguments,omitempty"`
}

// PlanEntry 是答复计划的一条策略条目（最小契约）。
type PlanEntry struct {
	Ground   string   `json:"ground"`
	Strategy string   `json:"strategy"`
	Claims   []int    `json:"claims,omitempty"`
	Sections []string `json:"sections,omitempty"`
}

// Report 是对立审查员聚合报告。
type Report struct {
	Disputes          []Dispute `json:"disputes"`
	Agreements        []Dispute `json:"agreements"`
	Disagreements     []Dispute `json:"disagreements"`
	MissedByBaseline  []Dispute `json:"missed_by_baseline"`
	Unverified        []Dispute `json:"unverified"`
	OverallConfidence float64   `json:"overall_confidence"`
	Summary           string    `json:"summary"`
}

// ReviewInput 是换位审查输入。
type ReviewInput struct {
	FinalDraft         string        `json:"final_draft"`
	ReferenceDocuments string        `json:"reference_documents"`
	Claims             string        `json:"claims"`
	OfficeAction       *OfficeAction `json:"office_action,omitempty"`
}

// ArgumentStrength 是论证强度。
type ArgumentStrength string

// Argument strength constants.
const (
	StrengthStrong       ArgumentStrength = "strong"
	StrengthWeak         ArgumentStrength = "weak"
	StrengthInsufficient ArgumentStrength = "insufficient"
)

// ReviewReport 是换位审查报告。
type ReviewReport struct {
	Disputes          []Dispute        `json:"disputes"`
	ArgumentStrength  ArgumentStrength `json:"argument_strength"`
	MissedRejections  []string         `json:"missed_rejections"`
	EvidenceGaps      []string         `json:"evidence_gaps"`
	OverallConfidence float64          `json:"overall_confidence"`
	Summary           string           `json:"summary"`
}
