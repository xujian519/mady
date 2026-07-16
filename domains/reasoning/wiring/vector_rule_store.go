// Package wiring adapts infrastructure-layer knowledge backends (SQLite FTS,
// Obsidian wiki cards) to the reasoning package's rule-source interfaces.
//
// The reasoning package itself stays free of infrastructure imports
// (ADR-0001: domain layer depends only on agentcore/graph). This subpackage
// is the composition root that stitches concrete backends to the
// RuleVectorStore / RuleSkillReader interfaces declared in the parent.
package wiring

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/retrieval"
)

// FTSSearcher is the minimal slice of knowledge.KnowledgeBackend that
// VectorRuleStore needs. Declared here (rather than importing knowledge) so
// the wiring package depends on a narrow contract; *sqlite.SQLiteStore and
// knowledge.KnowledgeBackend both satisfy it structurally.
type FTSSearcher interface {
	FTSSearch(query string, topK int) ([]retrieval.ScoredChunk, error)
}

// VectorRuleStore adapts an FTS-backed knowledge store as a reasoning
// RuleVectorStore. It is one of the three rule-acquisition sources in
// Stage ② (获取规则): it surfaces patent-law / examination-guide corpus
// fragments as RetrievedRules with AuthorityScore reflecting their
// "normative reference" authority tier (below deterministic rules, above
// wiki experience cards).
type VectorRuleStore struct {
	fts FTSSearcher
}

// NewVectorRuleStore binds an FTS searcher. Returns nil if fts is nil so
// callers can write `NewMultiSourceRetriever(walker, NewVectorRuleStore(x), ...)`
// and let a nil adapter simply disable the vector lane.
func NewVectorRuleStore(fts FTSSearcher) *VectorRuleStore {
	if fts == nil {
		return nil
	}
	return &VectorRuleStore{fts: fts}
}

// SearchRules queries the FTS index and maps each hit to a RetrievedRule.
// The raw fragment text is preserved in Baggage so Stage ③/④ can cite it
// and Stage ⑤ can check rule compliance against the original wording.
func (v *VectorRuleStore) SearchRules(ctx context.Context, query string, topK int) ([]reasoning.RetrievedRule, error) {
	if v == nil || v.fts == nil {
		return nil, nil
	}
	chunks, err := v.fts.FTSSearch(query, topK)
	if err != nil {
		return nil, fmt.Errorf("vector rule search: %w", err)
	}
	out := make([]reasoning.RetrievedRule, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, reasoning.RetrievedRule{
			Rule: reasoning.RuleConstraint{
				ArticleID:   c.DocID, // reuse DocID as the locatable anchor
				ArticleName: firstLine(c.Content),
				// Requirement defaults to ReqNote; the deterministic-rules
				// lane (domains/rules) is the authority on Must/Should.
				Requirement: reasoning.ReqNote,
				Description: truncate(c.Content, 300),
			},
			Source:         reasoning.RuleSourceVector,
			Priority:       2,   // vector lane: Should-tier
			AuthorityScore: 0.7, // normative reference, below deterministic rules
			Confidence:     c.Score,
			Baggage:        c.Content,
		})
	}
	return out, nil
}

// firstLine returns the first non-empty line of s, capped at 80 runes, for
// use as a human-readable ArticleName when no structured title exists.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return truncate(line, 80)
		}
	}
	return ""
}

// truncate caps s to at most n runes, appending an ellipsis when truncated.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
