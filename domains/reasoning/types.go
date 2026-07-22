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

// FactCollectorID identifies which collector produced a fact (Stage ①).
type FactCollectorID string

const (
	CollectorUserInput FactCollectorID = "user_input" // 用户输入提取
	CollectorDocuments FactCollectorID = "documents"  // 文档解析
	CollectorKnowledge FactCollectorID = "knowledge"  // KG/知识库检索
	CollectorDerived   FactCollectorID = "derived"    // LLM 推理衍生
)

// FactCategory groups facts for filtering (Stage ① → Stage ③).
type FactCategory string

const (
	FactCategoryTechnical  FactCategory = "technical"  // 技术特征
	FactCategoryLegal      FactCategory = "legal"      // 法律要件
	FactCategoryProcedural FactCategory = "procedural" // 程序性事实
	FactCategoryTemporal   FactCategory = "temporal"   // 期限/日期
)

// FactEntry is a single extracted fact on the blackboard.
type FactEntry struct {
	ID          string          `json:"id"`
	Source      string          `json:"source"` // user_text | file | cnipa_query | manual
	Content     string          `json:"content"`
	FilePath    string          `json:"file_path,omitempty"`
	Confidence  float64         `json:"confidence"`
	ExtractedAt string          `json:"extracted_at"`
	DiscardedAt string          `json:"discarded_at,omitempty"`
	CollectorID FactCollectorID `json:"collector_id,omitempty"` // Stage ① 来源追溯
	Category    FactCategory    `json:"category,omitempty"`     // 事实分类
	Tags        []string        `json:"tags,omitempty"`         // 自由标签
	ArtifactRef string          `json:"artifact_ref,omitempty"` // 关联文件引用
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

// RuleConfirmation records the human operator's verdict on a retrieved rule.
type RuleConfirmation string

const (
	RuleConfirmed RuleConfirmation = "confirmed" // adopted verbatim
	RuleModified  RuleConfirmation = "modified"  // adopted with edits
	RuleRejected  RuleConfirmation = "rejected"  // rejected, excluded from plan
)

// ConfirmedRuleEntry pairs a retrieved rule with its human-confirmation status.
// For Status == RuleModified, Modified holds the edited version that downstream
// stages consume in place of the original Rule.
type ConfirmedRuleEntry struct {
	Rule        RuleConstraint   `json:"rule"`
	Status      RuleConfirmation `json:"status"`
	Modified    *RuleConstraint  `json:"modified,omitempty"`
	Feedback    string           `json:"feedback,omitempty"`
	ConfirmedAt string           `json:"confirmed_at,omitempty"`
}

// ConfirmedRuleSet is the immutable snapshot of human-confirmed rules produced
// after Stage ② retrieval. Plan/Execute/Check consume only entries with Status
// confirmed or modified; rejected entries are retained for audit but isolated.
// (对齐 docs/specs/design-rule-acquisition-stage.md 第五节 ConfirmedRuleSet.)
type ConfirmedRuleSet struct {
	Entries []ConfirmedRuleEntry `json:"entries"`
	Locked  bool                 `json:"locked"`
}

// ActiveConstraints returns the RuleConstraints that downstream stages should
// consume: confirmed entries use their original Rule, modified entries use
// the Modified revision. Rejected entries are excluded.
func (rs *ConfirmedRuleSet) ActiveConstraints() []RuleConstraint {
	if rs == nil {
		return nil
	}
	out := make([]RuleConstraint, 0, len(rs.Entries))
	for _, e := range rs.Entries {
		switch e.Status {
		case RuleConfirmed:
			out = append(out, e.Rule)
		case RuleModified:
			if e.Modified != nil {
				out = append(out, *e.Modified)
			} else {
				out = append(out, e.Rule) // fallback if Modified missing
			}
		}
	}
	return out
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
//
// Deprecated: Use Plan for new code. ExecutionPlan remains for backward
// compatibility with existing workflows. Will be removed in v0.6.0.
type ExecutionPlan struct {
	Steps     []ExecutionPlanStep `json:"steps"`
	Artifacts []string            `json:"artifacts"`
}

// RuleSource identifies which retrieval source produced a rule (Stage ②).
type RuleSource string

const (
	RuleSourceKG     RuleSource = "knowledge_graph"     // 知识图谱
	RuleSourceVector RuleSource = "vector_db"           // 向量数据库
	RuleSourceSkill  RuleSource = "skill_md"            // SKILL.md 规则文档
	RuleSourceRules  RuleSource = "deterministic_rules" // 确定性规则引擎（domains/rules YAML）
)

// RetrievedRule is the Stage ② output — a RuleConstraint enriched with
// retrieval metadata (source, priority, authority score, confidence).
type RetrievedRule struct {
	Rule           RuleConstraint `json:"rule"`
	Source         RuleSource     `json:"source"`
	Priority       int            `json:"priority"`          // 1=high, 3=low
	AuthorityScore float64        `json:"authority_score"`   // 0-1 权威度
	Confidence     float64        `json:"confidence"`        // 0-1 检索置信度
	Baggage        string         `json:"baggage,omitempty"` // 附带原始文本
}

// StrategyType defines how a PlanStep is executed (Stage ④).
type StrategyType string

const (
	StrategyReact           StrategyType = "react"
	StrategyChain           StrategyType = "chain"
	StrategyMultiHypothesis StrategyType = "multi_hypothesis"
)

// WorkflowRelation describes the edge relation type that connects a workflow
// step to its parent GuidelineRule in the knowledge graph topology.
// These mirror knowledge/graph relation constants but are defined here to
// keep the domains/reasoning layer free of knowledge/graph import cycles.
type WorkflowRelation string

const (
	WorkflowRelCites     WorkflowRelation = "CITES"      // 引用法条 — 必须检查的实体要求
	WorkflowRelApplies   WorkflowRelation = "APPLIES"    // 适用 — 需要案例对比/正反方辩论
	WorkflowRelRelatedTo WorkflowRelation = "RELATED_TO" // 关联 — 相关联的审查步骤
	WorkflowRelContains  WorkflowRelation = "CONTAINS"   // 包含 — 子规则/子步骤
)

// WorkflowStep is a single step derived from the knowledge graph topology.
// Unlike PlanStep (which is LLM-generated), WorkflowStep is extracted from
// the KG edge structure around a GuidelineRule seed node.
type WorkflowStep struct {
	// ArticleID is the KG node ID of the rule/law/article this step references.
	ArticleID string `json:"article_id"`
	// NodeType is the KG node type (LawArticle, GuidelineRule, Case, etc.).
	NodeType string `json:"node_type"`
	// Name is the human-readable title from the KG node.
	Name string `json:"name"`
	// Content is a truncated preview of the rule content from the KG node.
	Content string `json:"content,omitempty"`
	// Relation is the edge type from the parent GuidelineRule to this node.
	Relation WorkflowRelation `json:"relation"`
	// Strategy is the recommended execution strategy for this step.
	Strategy StrategyType `json:"strategy"`
	// Priority ranks the step's importance (1=highest/must-do).
	Priority int `json:"priority"`
	// AuthorityWeight is the KG node's authority weight (0-1).
	AuthorityWeight float64 `json:"authority_weight"`
}

// WorkflowTopology represents the workflow structure derived from a
// knowledge graph traversal around a GuidelineRule seed node.
// It encodes the ordered steps and their dependencies as encoded by
// the KG edge topology (CITES/APPLIES/RELATED_TO chains).
type WorkflowTopology struct {
	// CaseType is the business-process type that triggered this topology.
	CaseType CaseType `json:"case_type"`
	// RootRule is the ID of the seed GuidelineRule node.
	RootRule string `json:"root_rule"`
	// Steps are the extracted steps, ordered by topological priority.
	Steps []WorkflowStep `json:"steps"`
	// Dependencies encodes the step dependency matrix:
	// Dependencies[i] contains the indices of steps that step i depends on.
	Dependencies [][]int `json:"dependencies,omitempty"`
	// AuthorityScore aggregates the authority weights across all steps.
	AuthorityScore float64 `json:"authority_score"`
	// Gaps describes any missing coverage noted during extraction.
	Gaps []string `json:"gaps,omitempty"`
}

// PlanIntent describes the cognitive mode of a Plan (Stage ③).
type PlanIntent string

const (
	PlanIntentSimple          PlanIntent = "simple"           // 单步，模板驱动
	PlanIntentChain           PlanIntent = "chain"            // 多步链式，模板驱动
	PlanIntentReAct           PlanIntent = "react"            // LLM ReAct
	PlanIntentMultiHypothesis PlanIntent = "multi_hypothesis" // 正反方+Judge
)

// PlanHypothesis is a thesis/argument in a multi-hypothesis Plan (Stage ③ → ④).
type PlanHypothesis struct {
	ID         string   `json:"id"`
	Label      string   `json:"label"`      // "专利权人主张" / "无效请求人主张"
	Thesis     string   `json:"thesis"`     // 核心论点
	UsedFacts  []string `json:"used_facts"` // 引用 Fact.ID 列表
	UsedRules  []string `json:"used_rules"` // 引用 Rule.ID 列表
	Confidence float64  `json:"confidence"`
}

// PlanStep is one action in a Plan — replaces ExecutionPlanStep for new code.
type PlanStep struct {
	Order          int            `json:"order"`
	Description    string         `json:"description"`
	Strategy       StrategyType   `json:"strategy"` // react | chain | multi_hypothesis
	ToolName       string         `json:"tool_name,omitempty"`
	ToolInput      map[string]any `json:"tool_input,omitempty"`
	ExpectedOutput string         `json:"expected_output"`
	DependsOn      []string       `json:"depends_on,omitempty"`     // 前置 StepID 列表
	RequiredFacts  []string       `json:"required_facts,omitempty"` // Fact.ID 列表
	RequiredRules  []string       `json:"required_rules,omitempty"` // Rule.ID 列表
}

// Plan is the Stage ③ output — a structured execution plan that carries
// references back to the facts (Stage ①) and rules (Stage ②) it depends on.
type Plan struct {
	PlanID       string           `json:"plan_id"`
	Intent       PlanIntent       `json:"intent"`
	CaseType     CaseType         `json:"case_type"`
	Steps        []PlanStep       `json:"steps"`
	Hypotheses   []PlanHypothesis `json:"hypotheses,omitempty"`
	UsedFacts    []string         `json:"used_facts"`              // Stage ① 回溯
	UsedRules    []string         `json:"used_rules"`              // Stage ② 回溯
	LLMReasoning string           `json:"llm_reasoning,omitempty"` // LLM 生成 Plan 时的 reasoning trace
}

// ValidationGap describes a reasoning gap between UsedFacts+UsedRules and the
// conclusion. Produced by Stage ⑤.
type ValidationGap struct {
	Description string `json:"description"`
	Severity    string `json:"severity"` // "hard" | "soft"
	Suggestion  string `json:"suggestion,omitempty"`
}

// CheckReport is the Stage ⑤ output — validates that conclusions follow from
// premises through the syllogism chain.
type CheckReport struct {
	PlanID         string          `json:"plan_id"`
	Passed         bool            `json:"passed"`
	Syllogisms     []Syllogism     `json:"syllogisms"`             // 三段论链
	UsedFacts      []string        `json:"used_facts"`             // 实际用到的 Facts
	UsedRules      []string        `json:"used_rules"`             // 实际用到的 Rules
	UnusedFacts    []string        `json:"unused_facts,omitempty"` // 未使用的 Facts
	UnusedRules    []string        `json:"unused_rules,omitempty"` // 未使用的 Rules
	Gaps           []ValidationGap `json:"gaps,omitempty"`         // 逻辑缺口
	Confidence     float64         `json:"confidence"`
	LLMExplanation string          `json:"llm_explanation,omitempty"`
}

// CollectResult is the return value of a FactCollector (Stage ①).
type CollectResult struct {
	CollectorID FactCollectorID `json:"collector_id"`
	FactCount   int             `json:"fact_count"`
	Confidence  float64         `json:"confidence"`
	Gaps        []string        `json:"gaps,omitempty"`
	Artifacts   []string        `json:"artifacts,omitempty"`
}

// =============================================================================
// Multi-Hypothesis types (Stage ④ multi_hypothesis strategy)
// =============================================================================

// HypothesisSpec defines one side of a debate.
type HypothesisSpec struct {
	ID    string `json:"id"`    // "pro" | "con"
	Claim string `json:"claim"` // e.g. "该技术特征相对于对比文件是显而易见的"
}

// Argument is the output of an Advocate node — a structured case for one side.
type Argument struct {
	HypothesisID              string   `json:"hypothesis_id"`
	Claim                     string   `json:"claim"`
	SupportingFacts           []string `json:"supporting_facts"`           // FactID
	SupportingRules           []string `json:"supporting_rules"`           // RuleID
	Reasoning                 string   `json:"reasoning"`                  // 论证过程
	AcknowledgedCounterpoints string   `json:"acknowledged_counterpoints"` // 主动承认的最强反对理由
}

// Verdict is the Judge's resolution of a multi-hypothesis debate.
type Verdict struct {
	Resolved          bool    `json:"resolved"`
	WinningHypothesis string  `json:"winning_hypothesis,omitempty"` // empty when !Resolved
	Confidence        float64 `json:"confidence"`
	Rationale         string  `json:"rationale"`
	DissentNotes      string  `json:"dissent_notes,omitempty"`     // 败方合理部分
	UnresolvedReason  string  `json:"unresolved_reason,omitempty"` // 无法裁决的原因
}

// =============================================================================
// 缺口 3：ArbitrationConfig — 多 LLM 仲裁配置
// =============================================================================

// JudgeLLMConfig 定义一个仲裁 LLM 实例。
type JudgeLLMConfig struct {
	Name   string  `json:"name"`   // "deepseek", "claude", "gpt4o"
	Model  string  `json:"model"`  // model identifier（如 "deepseek-v4-pro"）
	Weight float64 `json:"weight"` // 0-1 voting weight，仲裁时加权
}

// ArbitrationConfig 定义多 LLM 仲裁配置。
// 可选配置：当仲裁配置存在时，BuildMultiHypothesisSubgraph 使用多模型 Advocate。
type ArbitrationConfig struct {
	Judges       []JudgeLLMConfig `json:"judges"`
	MinAgreement float64          `json:"min_agreement"` // 最低加权一致阈值 (0.5-1.0)
	Strategy     string           `json:"strategy"`      // "weighted_vote" | "best_of_n"
}

// JudgeVote 记录单个 LLM 的裁决意见，由仲裁时收集使用。
type JudgeVote struct {
	JudgeName string  `json:"judge_name"`
	Verdict   Verdict `json:"verdict"`
	Score     float64 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

// NowISO returns the current UTC time in RFC3339 for timestamps.
func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// nowISO is the internal alias for backward compatibility.
func nowISO() string { return NowISO() }
