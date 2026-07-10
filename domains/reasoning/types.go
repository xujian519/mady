package reasoning

import "time"

// CaseType identifies a patent/legal business-process transaction type.
type CaseType string

const (
	CaseNoveltySearch CaseType = "novelty_search"
	CasePatentability CaseType = "patentability"
	CaseDrafting      CaseType = "drafting"
	CaseOAResponse    CaseType = "oa_response"
	CaseRejection     CaseType = "rejection_response"
	CaseReexamination CaseType = "reexamination"
	CaseInvalidation  CaseType = "invalidation"
	CaseInfringement  CaseType = "infringement"
	CaseFTO           CaseType = "fto"
	CaseValidity      CaseType = "validity"
	CaseLegalStatus   CaseType = "legal_status"
	CaseGeneralLegal  CaseType = "general_legal"
)

// FactEntry is a single extracted fact on the blackboard.
type FactEntry struct {
	ID          string  `json:"id"`
	Source      string  `json:"source"` // user_text | file | cnipa_query | manual
	Content     string  `json:"content"`
	FilePath    string  `json:"file_path,omitempty"`
	Confidence  float64 `json:"confidence"`
	ExtractedAt string  `json:"extracted_at"`
	DiscardedAt string  `json:"discarded_at,omitempty"`
}

// IsDiscarded reports whether the fact has been soft-discarded (for backtracking).
func (f FactEntry) IsDiscarded() bool { return f.DiscardedAt != "" }

// Requirement is the enforcement level of a rule constraint.
type Requirement string

const (
	ReqMust   Requirement = "must"
	ReqShould Requirement = "should"
	ReqNote   Requirement = "note"
)

// RuleConstraint is a rule pulled from statutes/guidelines that a plan must satisfy.
type RuleConstraint struct {
	ArticleID        string      `json:"article_id"`
	ArticleName      string      `json:"article_name"`
	Requirement      Requirement `json:"requirement"`
	Description      string      `json:"description"`
	ApplicableStages []string    `json:"applicable_stages,omitempty"`
}

// ReasoningChainNode is one hop in a multi-hop reasoning chain.
type ReasoningChainNode struct {
	KgNodeID string `json:"kg_node_id"`
	NodeType string `json:"node_type"`
	Name     string `json:"name"`
	Relation string `json:"relation"`
	Excerpt  string `json:"excerpt"`
}

// LegalBasis captures the legal references backing a reasoning chain.
type LegalBasis struct {
	LawArticle    string `json:"law_article,omitempty"`
	GuidelineRule string `json:"guideline_rule,omitempty"`
	PrecedentCase string `json:"precedent_case,omitempty"`
}

// ReasoningChain is a complete reasoning path from fact to legal basis.
type ReasoningChain struct {
	ID         string               `json:"id"`
	FactRef    string               `json:"fact_ref"`
	Nodes      []ReasoningChainNode `json:"nodes"`
	LegalBasis LegalBasis           `json:"legal_basis"`
	Confidence float64              `json:"confidence"`
	Gaps       []string             `json:"gaps,omitempty"`
}

// ArticleStepResult records the outcome of one step in a multi-step article judgment.
type ArticleStepResult struct {
	StepID    string         `json:"step_id"`
	Completed bool           `json:"completed"`
	Output    map[string]any `json:"output"`
	LLMTrace  string         `json:"llm_trace,omitempty"`
}

// ConfidenceLevel rates the reliability of an article judgment.
type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

// ArticleJudgment is the verdict on whether an article (法条) applies to the case.
type ArticleJudgment struct {
	ArticleID   string                       `json:"article_id"`
	ArticleName string                       `json:"article_name"`
	StepResults map[string]ArticleStepResult `json:"step_results"`
	Conclusion  map[string]any               `json:"conclusion"`
	Confidence  ConfidenceLevel              `json:"confidence"`
	Timestamp   string                       `json:"timestamp"`
}

// ExecutionPlanStep is one action in the execution plan.
type ExecutionPlanStep struct {
	Order          int            `json:"order"`
	Description    string         `json:"description"`
	ToolName       string         `json:"tool_name"`
	ToolInput      map[string]any `json:"tool_input"`
	ExpectedOutput string         `json:"expected_output"`
}

// ExecutionPlan is the action plan derived from the blackboard.
type ExecutionPlan struct {
	Steps     []ExecutionPlanStep `json:"steps"`
	Artifacts []string            `json:"artifacts"`
}

// nowISO returns the current UTC time in RFC3339 for timestamps.
func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
