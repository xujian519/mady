// Package legal provides Pregel-based legal analysis workflows.
//
// Reasoning types defined here avoid importing domains/reasoning directly,
// satisfying the layered architecture constraint that infrastructure (workflows)
// should not depend on domain implementation details.
package legal

// CaseType mirrors reasoning.CaseType for dependency inversion.
type CaseType string

const (
	CaseInvalidation CaseType = "invalidation"
	CaseInfringement CaseType = "infringement"
	CaseNovelty      CaseType = "novelty"
	CaseGeneralLegal CaseType = "general_legal"
)

// FactEntry represents a single fact in the reasoning blackboard.
type FactEntry struct {
	ID          string
	Content     string
	Source      string
	ExtractedAt string
	Confidence  float64
}

// RuleConstraint describes a legal constraint applied during reasoning.
type RuleConstraint struct {
	LawArticle  string
	Description string
	Requirement string // "must", "should", "must_not"
}

// LegalBasis records the legal foundation of a reasoning step.
type LegalBasis struct {
	LawArticle string
}

// ReasoningChain records one syllogism-based reasoning step.
type ReasoningChain struct {
	Premise    string
	Conclusion string
	LegalBasis LegalBasis
	Confidence float64
}

// SyllogismBuilder constructs formal syllogisms for legal reasoning.
type SyllogismBuilder struct {
	ArticleRef string
	Major      string
	Minor      string
	Conclusion string
}

// FactBlackboard defines the interface for the reasoning fact store.
// domains/reasoning.FactBlackboard satisfies this interface.
type FactBlackboard interface {
	AddFact(entry FactEntry) error
	AddRuleConstraint(rc RuleConstraint) error
	AddReasoningChain(chain ReasoningChain) error
}
