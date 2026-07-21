package reasoning

import (
	"encoding/json"
	"sync"
)

// FactBlackboard is the shared memory structure for legal reasoning.
// It stores facts, reasoning chains, rule constraints, article judgments,
// and an execution plan. All methods are goroutine-safe.
// Once Locked, any attempt to mutate the blackboard panics.
type FactBlackboard struct {
	CaseID         string   `json:"case_id"`
	CaseType       CaseType `json:"case_type"`
	TechnicalField string   `json:"technical_field"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	Locked         bool     `json:"locked"`

	mu               sync.RWMutex
	facts            []FactEntry
	reasoningChains  []ReasoningChain
	ruleConstraints  []RuleConstraint
	articleJudgments map[string]ArticleJudgment
	plan             *ExecutionPlan
	// Stage ①-⑤ workflow fields.
	rplan          *Plan             // Stage ③ Plan (replaces ExecutionPlan for new code)
	checkReport    *CheckReport      // Stage ⑤ output
	stageOutputs   map[string]any    // per-stage outputs, keyed by "stage1".."stage5"
	workflowID     string            // current WorkflowManifest ID
	currentStage   int               // current stage (1-5)
	confirmedRules *ConfirmedRuleSet // Stage ② 后人工确认的规则集（nil=未确认）
}

// NewFactBlackboard creates an empty blackboard for the given case.
func NewFactBlackboard(caseID string, caseType CaseType, technicalField string) *FactBlackboard {
	ts := nowISO()
	return &FactBlackboard{
		CaseID:           caseID,
		CaseType:         caseType,
		TechnicalField:   technicalField,
		CreatedAt:        ts,
		UpdatedAt:        ts,
		articleJudgments: make(map[string]ArticleJudgment),
	}
}

func (b *FactBlackboard) touch() { b.UpdatedAt = nowISO() }

// checkNotLocked panics if the blackboard is locked. This is an intentional
// programmer-error guard — mutating a locked blackboard indicates a bug in the
// caller. Callers must check IsLocked() before calling any mutating method.
func (b *FactBlackboard) checkNotLocked() {
	if b.Locked {
		panic("factBlackboard: attempt to mutate a locked blackboard")
	}
}

// IsLocked returns true if the blackboard has been locked. Prefer this over
// reading the exported Locked field directly, which may race without the mutex.
func (b *FactBlackboard) IsLocked() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Locked
}

// Lock freezes the blackboard. After locking, any mutating method panics.
func (b *FactBlackboard) Lock() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Locked = true
	b.touch()
}

// Facts returns a shallow copy of all fact entries.
func (b *FactBlackboard) Facts() []FactEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.facts
}

// ActiveFacts returns only non-discarded fact entries.
func (b *FactBlackboard) ActiveFacts() []FactEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := b.facts[:0:0]
	for _, f := range b.facts {
		if !f.IsDiscarded() {
			out = append(out, f)
		}
	}
	return out
}

// AddFact appends a fact entry. Panics if the blackboard is locked.
func (b *FactBlackboard) AddFact(f FactEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.facts = append(b.facts, f)
	b.touch()
}

// GetFact looks up a fact by ID.
func (b *FactBlackboard) GetFact(id string) (FactEntry, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for i := range b.facts {
		if b.facts[i].ID == id {
			return b.facts[i], true
		}
	}
	return FactEntry{}, false
}

// DiscardFact marks a fact as discarded by setting DiscardedAt. Panics if locked.
func (b *FactBlackboard) DiscardFact(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	for i := range b.facts {
		if b.facts[i].ID == id && b.facts[i].DiscardedAt == "" {
			b.facts[i].DiscardedAt = nowISO()
			b.touch()
			return
		}
	}
}

// ReasoningChains returns all stored reasoning chains.
func (b *FactBlackboard) ReasoningChains() []ReasoningChain {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.reasoningChains
}

// AddReasoningChain appends a reasoning chain. Panics if locked.
func (b *FactBlackboard) AddReasoningChain(c ReasoningChain) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.reasoningChains = append(b.reasoningChains, c)
	b.touch()
}

// ClearReasoningChains removes all reasoning chains. Panics if locked.
func (b *FactBlackboard) ClearReasoningChains() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.reasoningChains = nil
	b.touch()
}

// RuleConstraints returns all rule constraints.
func (b *FactBlackboard) RuleConstraints() []RuleConstraint {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.ruleConstraints
}

// AddRuleConstraint appends a rule constraint. Panics if locked.
func (b *FactBlackboard) AddRuleConstraint(c RuleConstraint) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.ruleConstraints = append(b.ruleConstraints, c)
	b.touch()
}

// SetRuleConstraints replaces all rule constraints. Panics if locked.
func (b *FactBlackboard) SetRuleConstraints(cs []RuleConstraint) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.ruleConstraints = cs
	b.touch()
}

// ConfirmedRules returns the human-confirmed rule set, or nil if Stage ②
// output has not yet been confirmed.
func (b *FactBlackboard) ConfirmedRules() *ConfirmedRuleSet {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.confirmedRules
}

// SetConfirmedRules stores the human-confirmed rule set. Panics if locked.
// Once set, ConfirmedRuleConstraints (and thus Plan/Execute/Check) will only
// consume confirmed/modified entries, isolating rejected rules.
func (b *FactBlackboard) SetConfirmedRules(rs ConfirmedRuleSet) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.confirmedRules = &rs
	b.touch()
}

// ConfirmedRuleConstraints returns the rule constraints that downstream stages
// (Plan/Execute/Check) should consume. When a ConfirmedRuleSet is present, it
// returns only confirmed/modified entries (modified uses the edited version).
// When no confirmation has occurred (confirmedRules == nil), it falls back to
// the raw retrieved RuleConstraints — preserving backward compatibility for
// workflows that haven't integrated the confirmation gate yet.
func (b *FactBlackboard) ConfirmedRuleConstraints() []RuleConstraint {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.confirmedRules != nil {
		return b.confirmedRules.ActiveConstraints()
	}
	return b.ruleConstraints
}

// ArticleJudgments returns the map of article judgments.
func (b *FactBlackboard) ArticleJudgments() map[string]ArticleJudgment {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.articleJudgments
}

// SetArticleJudgment sets or updates a judgment for the given article ID. Panics if locked.
func (b *FactBlackboard) SetArticleJudgment(id string, j ArticleJudgment) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.articleJudgments[id] = j
	b.touch()
}

// GetArticleJudgment retrieves a judgment by article ID.
func (b *FactBlackboard) GetArticleJudgment(id string) (ArticleJudgment, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	j, ok := b.articleJudgments[id]
	return j, ok
}

// Plan returns the current execution plan, or nil if none has been set.
//
// Deprecated: Use PlanV2 for new code. Plan relies on the legacy ExecutionPlan
// type and is only retained for backward compatibility. Will be removed in v0.6.0.
func (b *FactBlackboard) Plan() *ExecutionPlan {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.plan
}

// PlanV2 returns the Stage ③ Plan, or nil if none has been set.
func (b *FactBlackboard) PlanV2() *Plan {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.rplan
}

// SetPlanV2 stores a Stage ③ Plan. Panics if locked.
func (b *FactBlackboard) SetPlanV2(p Plan) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.rplan = &p
	b.touch()
}

// CheckReport returns the Stage ⑤ check report, or nil if not yet generated.
func (b *FactBlackboard) CheckReport() *CheckReport {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.checkReport
}

// SetCheckReport stores the Stage ⑤ check report. Panics if locked.
func (b *FactBlackboard) SetCheckReport(r CheckReport) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.checkReport = &r
	b.touch()
}

// StageOutput returns the output stored for a given stage key (e.g. "stage1").
// Returns nil and false if no output exists.
func (b *FactBlackboard) StageOutput(phase string) (any, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.stageOutputs[phase]
	return v, ok
}

// SetStageOutput stores an output for a stage. Panics if locked.
func (b *FactBlackboard) SetStageOutput(phase string, v any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	if b.stageOutputs == nil {
		b.stageOutputs = make(map[string]any)
	}
	b.stageOutputs[phase] = v
	b.touch()
}

// WorkflowID returns the current WorkflowManifest ID.
func (b *FactBlackboard) WorkflowID() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.workflowID
}

// SetWorkflowID sets the WorkflowManifest ID. Panics if locked.
func (b *FactBlackboard) SetWorkflowID(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.workflowID = id
	b.touch()
}

// CurrentStage returns the currently executing stage number (1-5), 0 if not started.
func (b *FactBlackboard) CurrentStage() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentStage
}

// SetCurrentStage advances the stage counter. Panics if locked.
func (b *FactBlackboard) SetCurrentStage(s int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.currentStage = s
	b.touch()
}

type factBlackboardJSON struct {
	CaseID           string                     `json:"case_id"`
	CaseType         CaseType                   `json:"case_type"`
	TechnicalField   string                     `json:"technical_field"`
	CreatedAt        string                     `json:"created_at"`
	UpdatedAt        string                     `json:"updated_at"`
	Locked           bool                       `json:"locked"`
	Facts            []FactEntry                `json:"facts"`
	ReasoningChains  []ReasoningChain           `json:"reasoning_chains"`
	RuleConstraints  []RuleConstraint           `json:"rule_constraints"`
	ArticleJudgments map[string]ArticleJudgment `json:"article_judgments"`
	Plan             *ExecutionPlan             `json:"plan"`
	Rplan            *Plan                      `json:"rplan,omitempty"`
	CheckReport      *CheckReport               `json:"check_report,omitempty"`
	StageOutputs     map[string]any             `json:"stage_outputs,omitempty"`
	WorkflowID       string                     `json:"workflow_id,omitempty"`
	CurrentStage     int                        `json:"current_stage"`
	ConfirmedRules   *ConfirmedRuleSet          `json:"confirmed_rules,omitempty"`
}

func (b *FactBlackboard) MarshalJSON() ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return json.Marshal(factBlackboardJSON{
		CaseID:           b.CaseID,
		CaseType:         b.CaseType,
		TechnicalField:   b.TechnicalField,
		CreatedAt:        b.CreatedAt,
		UpdatedAt:        b.UpdatedAt,
		Locked:           b.Locked,
		Facts:            b.facts,
		ReasoningChains:  b.reasoningChains,
		RuleConstraints:  b.ruleConstraints,
		ArticleJudgments: b.articleJudgments,
		Plan:             b.plan,
		Rplan:            b.rplan,
		CheckReport:      b.checkReport,
		StageOutputs:     b.stageOutputs,
		WorkflowID:       b.workflowID,
		CurrentStage:     b.currentStage,
		ConfirmedRules:   b.confirmedRules,
	})
}

func (b *FactBlackboard) UnmarshalJSON(data []byte) error {
	var s factBlackboardJSON
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.CaseID = s.CaseID
	b.CaseType = s.CaseType
	b.TechnicalField = s.TechnicalField
	b.CreatedAt = s.CreatedAt
	b.UpdatedAt = s.UpdatedAt
	b.Locked = s.Locked
	b.facts = s.Facts
	b.reasoningChains = s.ReasoningChains
	b.ruleConstraints = s.RuleConstraints
	b.articleJudgments = s.ArticleJudgments
	if b.articleJudgments == nil {
		b.articleJudgments = make(map[string]ArticleJudgment)
	}
	b.plan = s.Plan
	b.rplan = s.Rplan
	b.checkReport = s.CheckReport
	b.stageOutputs = s.StageOutputs
	b.workflowID = s.WorkflowID
	b.currentStage = s.CurrentStage
	b.confirmedRules = s.ConfirmedRules
	return nil
}
