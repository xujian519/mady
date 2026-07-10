package reasoning

import "encoding/json"

// FactBlackboard is the shared memory where specialized engines and the LLM
// engine both read and write during a legal/patent reasoning run.
//
// Discovery phase writes facts; step 2 writes reasoning chains / rule
// constraints; step 3 writes article judgments; step 4 writes the plan.
type FactBlackboard struct {
	CaseID         string   `json:"case_id"`
	CaseType       CaseType `json:"case_type"`
	TechnicalField string   `json:"technical_field"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	Locked         bool     `json:"locked"`

	facts            []FactEntry
	reasoningChains  []ReasoningChain
	ruleConstraints  []RuleConstraint
	articleJudgments map[string]ArticleJudgment
	plan             *ExecutionPlan
}

// NewFactBlackboard creates an empty blackboard for a case.
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

// touch bumps the updated timestamp.
func (b *FactBlackboard) touch() { b.UpdatedAt = nowISO() }

// Lock freezes the blackboard against further mutation.
func (b *FactBlackboard) Lock() { b.Locked = true; b.touch() }

// --- facts ---

// Facts returns all fact entries (including discarded ones).
func (b *FactBlackboard) Facts() []FactEntry { return b.facts }

// ActiveFacts returns only non-discarded fact entries.
func (b *FactBlackboard) ActiveFacts() []FactEntry {
	out := b.facts[:0:0]
	for _, f := range b.facts {
		if !f.IsDiscarded() {
			out = append(out, f)
		}
	}
	return out
}

// AddFact appends a fact to the blackboard.
func (b *FactBlackboard) AddFact(f FactEntry) {
	b.facts = append(b.facts, f)
	b.touch()
}

// GetFact looks up a fact by ID (ok=false if absent).
func (b *FactBlackboard) GetFact(id string) (FactEntry, bool) {
	for i := range b.facts {
		if b.facts[i].ID == id {
			return b.facts[i], true
		}
	}
	return FactEntry{}, false
}

// DiscardFact soft-discards a fact by ID (used for backtracking).
func (b *FactBlackboard) DiscardFact(id string) {
	for i := range b.facts {
		if b.facts[i].ID == id && b.facts[i].DiscardedAt == "" {
			b.facts[i].DiscardedAt = nowISO()
			b.touch()
			return
		}
	}
}

// --- reasoning chains ---

// ReasoningChains returns the accumulated reasoning chains.
func (b *FactBlackboard) ReasoningChains() []ReasoningChain { return b.reasoningChains }

// AddReasoningChain appends a reasoning chain.
func (b *FactBlackboard) AddReasoningChain(c ReasoningChain) {
	b.reasoningChains = append(b.reasoningChains, c)
	b.touch()
}

// ClearReasoningChains drops all reasoning chains (e.g. before a re-walk).
func (b *FactBlackboard) ClearReasoningChains() {
	b.reasoningChains = nil
	b.touch()
}

// --- rule constraints ---

// RuleConstraints returns the accumulated rule constraints.
func (b *FactBlackboard) RuleConstraints() []RuleConstraint { return b.ruleConstraints }

// AddRuleConstraint appends a rule constraint.
func (b *FactBlackboard) AddRuleConstraint(c RuleConstraint) {
	b.ruleConstraints = append(b.ruleConstraints, c)
	b.touch()
}

// SetRuleConstraints replaces the whole rule-constraint set.
func (b *FactBlackboard) SetRuleConstraints(cs []RuleConstraint) {
	b.ruleConstraints = cs
	b.touch()
}

// --- article judgments ---

// ArticleJudgments returns the map of article-ID → judgment.
func (b *FactBlackboard) ArticleJudgments() map[string]ArticleJudgment {
	return b.articleJudgments
}

// SetArticleJudgment records or replaces the judgment for an article.
func (b *FactBlackboard) SetArticleJudgment(id string, j ArticleJudgment) {
	b.articleJudgments[id] = j
	b.touch()
}

// GetArticleJudgment looks up a judgment by article ID.
func (b *FactBlackboard) GetArticleJudgment(id string) (ArticleJudgment, bool) {
	j, ok := b.articleJudgments[id]
	return j, ok
}

// --- plan ---

// Plan returns the execution plan, or nil if none has been set.
func (b *FactBlackboard) Plan() *ExecutionPlan { return b.plan }

// SetPlan records the execution plan on the blackboard.
func (b *FactBlackboard) SetPlan(p ExecutionPlan) {
	b.plan = &p
	b.touch()
}

// --- serialization ---

// factBlackboardJSON is the serializable projection of FactBlackboard.
// The unexported slices/maps are projected explicitly because encoding/json
// cannot see them.
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

// MarshalJSON implements json.Marshaler.
func (b *FactBlackboard) MarshalJSON() ([]byte, error) {
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

// UnmarshalJSON implements json.Unmarshaler.
func (b *FactBlackboard) UnmarshalJSON(data []byte) error {
	var s factBlackboardJSON
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
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
