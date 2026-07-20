package legal

import (
	"github.com/xujian519/mady/domains/reasoning"
)

// reasoningAdapter bridges the domains/reasoning implementation into the
// local FactBlackboard interface. This single file absorbs the architectural
// dependency inversion; all other files in this package (except comparison.go)
// use only local types.
//
// NOTE: The underlying reasoning.FactBlackboard methods are void (no error
// return). The local FactBlackboard interface declares error returns as
// future-proofing — the adapter returns nil to satisfy the contract. If the
// underlying methods ever gain error returns, the adapter will propagate them.
type reasoningAdapter struct {
	bb *reasoning.FactBlackboard
}

// WrapBlackboard wraps a concrete reasoning.FactBlackboard into the local
// FactBlackboard interface for use in tool.go.
func WrapBlackboard(bb *reasoning.FactBlackboard) FactBlackboard {
	if bb == nil {
		return nil
	}
	return &reasoningAdapter{bb: bb}
}

func (a *reasoningAdapter) AddFact(entry FactEntry) error {
	a.bb.AddFact(reasoning.FactEntry{
		ID:          entry.ID,
		Content:     entry.Content,
		Source:      entry.Source,
		ExtractedAt: entry.ExtractedAt,
		Confidence:  entry.Confidence,
	})
	return nil
}

func (a *reasoningAdapter) AddRuleConstraint(rc RuleConstraint) error {
	// Local RuleConstraint only has LawArticle; reasoning.RuleConstraint
	// distinguishes ArticleID (e.g. "A22.3") from ArticleName (e.g. "创造性").
	// We store LawArticle as the ID; Name is left empty since the local type
	// doesn't carry that distinction.
	a.bb.AddRuleConstraint(reasoning.RuleConstraint{
		ArticleID:   rc.LawArticle,
		ArticleName: "",
		Description: rc.Description,
		Requirement: reasoning.Requirement(rc.Requirement),
	})
	return nil
}

func (a *reasoningAdapter) AddReasoningChain(chain ReasoningChain) error {
	// NOTE: The local ReasoningChain type carries a Conclusion field, but
	// reasoning.ReasoningChain has no corresponding field. When the
	// underlying type gains a Conclusion slot, this adapter should be
	// updated to propagate it.
	a.bb.AddReasoningChain(reasoning.ReasoningChain{
		ID:         chain.Premise,
		FactRef:    chain.Premise,
		LegalBasis: reasoning.LegalBasis{LawArticle: chain.LegalBasis.LawArticle},
		Confidence: chain.Confidence,
	})
	return nil
}
