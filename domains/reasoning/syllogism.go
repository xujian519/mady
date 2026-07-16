package reasoning

import (
	"errors"
	"fmt"
)

// Premise is one premise of a categorical syllogism.
type Premise struct {
	Label   string `json:"label"`  // human-readable label
	Source  string `json:"source"` // statute | case_fact | precedent | guideline
	RefID   string `json:"ref_id"` // references a blackboard fact/article ID
	Content string `json:"content"`
}

// Premise source constants.
const (
	SourceStatute   = "statute"
	SourceCaseFact  = "case_fact"
	SourcePrecedent = "precedent"
	SourceGuideline = "guideline"
)

// Syllogism is a categorical syllogism:
//
//	大前提 (major premise: 法条)  +  小前提 (minor premise: 案件事实)  →  结论
//
// Every conclusion must be grounded: FactRef references a FactEntry on the
// blackboard and ArticleRef references a RuleConstraint/ArticleJudgment. A
// syllogism whose conclusion lacks these references is rejected by
// RuleAssertion.
type Syllogism struct {
	ID           string  `json:"id"`
	MajorPremise Premise `json:"major_premise"`
	MinorPremise Premise `json:"minor_premise"`
	Conclusion   string  `json:"conclusion"`
	FactRef      string  `json:"fact_ref"`
	ArticleRef   string  `json:"article_ref"`
	Confidence   float64 `json:"confidence"`
	Validated    bool    `json:"validated"`
}

// ErrUnreferencedConclusion is returned when a syllogism's conclusion lacks
// required references to blackboard facts and legal articles.
var ErrUnreferencedConclusion = errors.New("三段论结论缺少必要引用：必须引用黑板事实ID和法条ID")

// articleExists reports whether an article ID is known to the blackboard
// (either as a rule constraint or an article judgment).
func articleExists(bb *FactBlackboard, articleID string) bool {
	for _, c := range bb.ConfirmedRuleConstraints() {
		if c.ArticleID == articleID {
			return true
		}
	}
	if _, ok := bb.GetArticleJudgment(articleID); ok {
		return true
	}
	return false
}

// RuleAssertion validates that a syllogism conclusion is grounded in the
// blackboard. The conclusion must reference both a fact ID (minor premise)
// and an article ID (major premise); both must exist on the blackboard.
// On success the syllogism is marked Validated and nil is returned.
func RuleAssertion(bb *FactBlackboard, s *Syllogism) error {
	if s.FactRef == "" || s.ArticleRef == "" {
		return ErrUnreferencedConclusion
	}
	if _, ok := bb.GetFact(s.FactRef); !ok {
		return fmt.Errorf("三段论 %s 引用的事实 %s 不存在于黑板上", s.ID, s.FactRef)
	}
	if !articleExists(bb, s.ArticleRef) {
		return fmt.Errorf("三段论 %s 引用的法条 %s 不存在于黑板上", s.ID, s.ArticleRef)
	}
	s.Validated = true
	return nil
}

// AssertChain validates a slice of syllogisms against the blackboard. It
// returns the index and error of the first syllogism that fails validation.
func AssertChain(bb *FactBlackboard, chains []Syllogism) error {
	for i := range chains {
		if err := RuleAssertion(bb, &chains[i]); err != nil {
			return fmt.Errorf("推理链第 %d 项校验失败: %w", i+1, err)
		}
	}
	return nil
}

// SyllogismBuilder provides a fluent API for constructing a single validated
// syllogism against a blackboard.
type SyllogismBuilder struct {
	s Syllogism
}

// NewSyllogismBuilder starts building a syllogism with the given ID.
func NewSyllogismBuilder(id string) *SyllogismBuilder {
	return &SyllogismBuilder{s: Syllogism{ID: id, Confidence: 0.5}}
}

// Major sets the major premise (大前提: 法条/规则). The refID becomes ArticleRef.
func (b *SyllogismBuilder) Major(label, refID, content string) *SyllogismBuilder {
	b.s.MajorPremise = Premise{Label: label, Source: SourceStatute, RefID: refID, Content: content}
	b.s.ArticleRef = refID
	return b
}

// Minor sets the minor premise (小前提: 案件事实). The refID becomes FactRef.
func (b *SyllogismBuilder) Minor(label, refID, content string) *SyllogismBuilder {
	b.s.MinorPremise = Premise{Label: label, Source: SourceCaseFact, RefID: refID, Content: content}
	b.s.FactRef = refID
	return b
}

// ConclusionText sets the conclusion text and its confidence (0-1).
func (b *SyllogismBuilder) ConclusionText(text string, confidence float64) *SyllogismBuilder {
	b.s.Conclusion = text
	b.s.Confidence = confidence
	return b
}

// Build validates the syllogism against the blackboard and returns it.
func (b *SyllogismBuilder) Build(bb *FactBlackboard) (Syllogism, error) {
	if err := RuleAssertion(bb, &b.s); err != nil {
		return Syllogism{}, err
	}
	return b.s, nil
}
