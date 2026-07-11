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

func (b *FactBlackboard) checkNotLocked() {
	if b.Locked {
		panic("factBlackboard: attempt to mutate a locked blackboard")
	}
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
func (b *FactBlackboard) Plan() *ExecutionPlan {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.plan
}

// SetPlan stores an execution plan. Panics if locked.
func (b *FactBlackboard) SetPlan(p ExecutionPlan) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checkNotLocked()
	b.plan = &p
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
	return nil
}
