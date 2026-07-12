package reasoning

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// RuleRetrievalManifest defines the Stage ② rule acquisition strategy.
// It specifies which sources to query, how to aggregate results, and
// the maximum number of rules to retain.
//
// A Manifest is typically loaded from YAML (Phase 3), but can also be
// constructed programmatically for testing or simple cases.
type RuleRetrievalManifest struct {
	ManifestID  string          `json:"manifest_id"`
	CaseType    CaseType        `json:"case_type"`
	Name        string          `json:"name"`
	Sources     []RuleSourceCfg `json:"sources"`
	Aggregation string          `json:"aggregation"` // "merge" | "priority" | "intersect"
	MaxRules    int             `json:"max_rules"`
}

// RuleSourceCfg configures a single rule source query.
type RuleSourceCfg struct {
	Source         RuleSource `json:"source"`
	QueryTemplates []string   `json:"query_templates"` // {{.keywords}} 占位符
	MaxPerSource   int        `json:"max_per_source"`
	Weight         float64    `json:"weight"` // 聚合权重
}

// RuleVectorStore searches rules in the vector database.
type RuleVectorStore interface {
	SearchRules(ctx context.Context, query string, topK int) ([]RetrievedRule, error)
}

// RuleSkillReader reads rules from SKILL.md documents.
type RuleSkillReader interface {
	ReadRules(ctx context.Context, domain string) ([]RetrievedRule, error)
}

// MultiSourceRetriever queries three rule sources in parallel and
// aggregates the results according to the Manifest.
type MultiSourceRetriever struct {
	walker      *ReasoningWalker
	vectorStore RuleVectorStore
	skillReader RuleSkillReader
}

// NewMultiSourceRetriever creates a retriever. Any source may be nil,
// in which case it is skipped during retrieval.
func NewMultiSourceRetriever(walker *ReasoningWalker, vs RuleVectorStore, sr RuleSkillReader) *MultiSourceRetriever {
	return &MultiSourceRetriever{
		walker:      walker,
		vectorStore: vs,
		skillReader: sr,
	}
}

// Retrieve runs all configured sources in parallel and returns an aggregated,
// deduplicated list of RetrievedRules sorted by priority.
func (r *MultiSourceRetriever) Retrieve(ctx context.Context, manifest RuleRetrievalManifest, facts []FactEntry, techField string) ([]RetrievedRule, error) {
	if len(manifest.Sources) == 0 {
		return nil, nil
	}

	// Build query context from facts and technical field.
	queryCtx := buildQueryContext(facts, techField)

	var (
		mu       sync.Mutex
		all      []RetrievedRule
		wg       sync.WaitGroup
		firstErr error
	)

	for _, src := range manifest.Sources {
		src := src // capture
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("source %s panicked: %v", src.Source, r)
					}
					mu.Unlock()
				}
			}()
			rules, err := r.querySource(ctx, src, queryCtx)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			all = append(all, rules...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Don't fail entirely on partial source errors — return what we have.
	if len(all) == 0 && firstErr != nil {
		return nil, fmt.Errorf("rule retrieval: all sources failed: %w", firstErr)
	}

	// Deduplicate by ArticleID, keeping highest Priority.
	deduped := deduplicateRules(all)

	// Sort by Priority ascending (lower = more important).
	sort.SliceStable(deduped, func(i, j int) bool {
		return deduped[i].Priority < deduped[j].Priority
	})

	// Truncate if needed.
	maxRules := manifest.MaxRules
	if maxRules <= 0 {
		maxRules = 15
	}
	if len(deduped) > maxRules {
		deduped = deduped[:maxRules]
	}

	return deduped, nil
}

// querySource dispatches a single source query.
func (r *MultiSourceRetriever) querySource(ctx context.Context, src RuleSourceCfg, queryCtx map[string]string) ([]RetrievedRule, error) {
	switch src.Source {
	case RuleSourceKG:
		return r.queryKG(ctx, src, queryCtx)
	case RuleSourceVector:
		return r.queryVector(ctx, src, queryCtx)
	case RuleSourceSkill:
		return r.querySkill(ctx, src)
	default:
		return nil, fmt.Errorf("unknown rule source: %s", src.Source)
	}
}

func (r *MultiSourceRetriever) queryKG(ctx context.Context, src RuleSourceCfg, queryCtx map[string]string) ([]RetrievedRule, error) {
	if r.walker == nil || r.walker.store == nil {
		return nil, nil
	}

	// Build facts from query context for Walker.CollectAll.
	facts := []string{queryCtx["keywords"]}
	if queryCtx["tech_field"] != "" {
		facts = append(facts, queryCtx["tech_field"])
	}

	result, err := r.walker.CollectAll(ctx, CollectAllInput{
		Facts:          facts,
		TechnicalField: queryCtx["tech_field"],
	})
	if err != nil {
		return nil, fmt.Errorf("KG query: %w", err)
	}

	maxPerSource := src.MaxPerSource
	if maxPerSource <= 0 {
		maxPerSource = 10
	}

	var rules []RetrievedRule
	for i, c := range result.Constraints {
		if i >= maxPerSource {
			break
		}
		rules = append(rules, RetrievedRule{
			Rule:           c,
			Source:         RuleSourceKG,
			Priority:       priorityForRequirement(c.Requirement),
			AuthorityScore: authorityForLevel(c.ArticleID),
			Confidence:     result.Coverage,
		})
	}
	return rules, nil
}

func (r *MultiSourceRetriever) queryVector(ctx context.Context, src RuleSourceCfg, queryCtx map[string]string) ([]RetrievedRule, error) {
	if r.vectorStore == nil {
		return nil, nil
	}

	query := queryCtx["keywords"]
	maxPerSource := src.MaxPerSource
	if maxPerSource <= 0 {
		maxPerSource = 5
	}

	rules, err := r.vectorStore.SearchRules(ctx, query, maxPerSource)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	return rules, nil
}

func (r *MultiSourceRetriever) querySkill(ctx context.Context, src RuleSourceCfg) ([]RetrievedRule, error) {
	if r.skillReader == nil {
		return nil, nil
	}

	// Skill reader is domain-scoped — no query needed.
	rules, err := r.skillReader.ReadRules(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("skill reader: %w", err)
	}
	return rules, nil
}

// --- helpers ---

// buildQueryContext creates a template context from facts.
func buildQueryContext(facts []FactEntry, techField string) map[string]string {
	kw := techField
	for _, f := range facts {
		if len(kw) < 500 {
			kw += " " + f.Content
		}
	}
	return map[string]string{
		"keywords":   truncate(kw, 500),
		"tech_field": techField,
	}
}

func priorityForRequirement(r Requirement) int {
	switch r {
	case ReqMust:
		return 1
	case ReqShould:
		return 2
	default:
		return 3
	}
}

// authorityForLevel returns an authority score based on the article name.
func authorityForLevel(articleID string) float64 {
	// Simple heuristic: shorter article IDs are typically higher-level laws.
	if len(articleID) <= 5 {
		return 0.9
	}
	return 0.7
}

// deduplicateRules removes duplicate rules by ArticleID, keeping highest Priority.
func deduplicateRules(rules []RetrievedRule) []RetrievedRule {
	seen := make(map[string]int) // ArticleID → index in result
	deduped := make([]RetrievedRule, 0, len(rules))

	for _, r := range rules {
		idx, ok := seen[r.Rule.ArticleID]
		if !ok {
			seen[r.Rule.ArticleID] = len(deduped)
			deduped = append(deduped, r)
			continue
		}
		// Keep the one with higher priority (lower number).
		if r.Priority < deduped[idx].Priority {
			deduped[idx] = r
		}
	}
	return deduped
}
