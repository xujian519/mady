package reasoning

import (
	"context"
	"fmt"
	"log/slog"
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

// RuleEngineSource queries a deterministic rule engine (domains/rules YAML)
// for rules matching a case type. This is the highest-authority source in the
// Stage ② rule-acquisition hierarchy: its output is code-curated law-article
// mappings (e.g. NOV-001~004 novelty rules), not retrieved corpus fragments.
type RuleEngineSource interface {
	// MatchRules returns deterministic rules applicable to the given case type
	// and query context. The query context carries keywords/tech_field that
	// implementations may use to select the rule domain (e.g. mapping a
	// novelty query to the "patent_novelty" rule domain).
	MatchRules(ctx context.Context, caseType string, queryCtx map[string]string) ([]RetrievedRule, error)
}

// IPCStandardSource queries IPC-based examination standards (宝宸知识库 cards).
// This is an optional knowledge source that provides IPC field × legal article
// examination practice summaries for the reasoning framework.
type IPCStandardSource interface {
	// MatchByIPC returns examination standards matching the given IPC section
	// (e.g. "G" for physics/computing) and optional legal article. Returns
	// results as RetrievedRules so they integrate seamlessly with the existing
	// Stage ② rule acquisition pipeline.
	MatchByIPC(ctx context.Context, ipcSection, article string, queryCtx map[string]string) ([]RetrievedRule, error)
}

// MultiSourceRetriever queries up to five rule sources in parallel and
// aggregates the results according to the Manifest. It can optionally
// extract workflow topology from the knowledge graph for plan generation.
type MultiSourceRetriever struct {
	walker      *ReasoningWalker
	vectorStore RuleVectorStore
	skillReader RuleSkillReader
	ruleEngine  RuleEngineSource
	ipcSource   IPCStandardSource  // optional — IPC 审查标准
	topologyExt *TopologyExtractor // optional — enables topology extraction
}

// NewMultiSourceRetriever creates a retriever. Any source may be nil,
// in which case it is skipped during retrieval.
func NewMultiSourceRetriever(walker *ReasoningWalker, vs RuleVectorStore, sr RuleSkillReader, re RuleEngineSource) *MultiSourceRetriever {
	return &MultiSourceRetriever{
		walker:      walker,
		vectorStore: vs,
		skillReader: sr,
		ruleEngine:  re,
	}
}

// WithTopologyExtractor attaches a topology extractor for KG-driven
// workflow generation. When set, RetrieveWithTopology can derive ordered
// WorkflowSteps from the knowledge graph for the requested case type.
func (r *MultiSourceRetriever) WithTopologyExtractor(ext *TopologyExtractor) *MultiSourceRetriever {
	r.topologyExt = ext
	return r
}

// WithIPCSource attaches an IPC examination standards source. When set,
// the retriever can query IPC standards as part of Stage ② rule acquisition.
// This is optional and does not affect existing sources.
func (r *MultiSourceRetriever) WithIPCSource(src IPCStandardSource) *MultiSourceRetriever {
	r.ipcSource = src
	return r
}

// Retrieve runs all configured sources in parallel and returns an aggregated,
// deduplicated list of RetrievedRules sorted by priority.
func (r *MultiSourceRetriever) Retrieve(ctx context.Context, manifest RuleRetrievalManifest, facts []FactEntry, techField string) ([]RetrievedRule, error) {
	if len(manifest.Sources) == 0 {
		return nil, nil
	}

	// Build query context from facts and technical field.
	queryCtx := buildQueryContext(facts, techField)
	// case_type lets the deterministic-rules source select the matching rule
	// domain (e.g. novelty → patent_novelty). Other sources ignore it.
	queryCtx["case_type"] = string(manifest.CaseType)

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
	case RuleSourceRules:
		return r.queryRules(ctx, src, queryCtx)
	case RuleSourceIPC:
		return r.queryIPC(ctx, src, queryCtx)
	default:
		return nil, fmt.Errorf("unknown rule source: %s", src.Source)
	}
}

// queryRules fetches deterministic rules from the rule engine. The case type
// (passed through from the manifest) lets the engine select the matching rule
// domain; queryCtx keywords are available as a secondary signal.
func (r *MultiSourceRetriever) queryRules(ctx context.Context, src RuleSourceCfg, queryCtx map[string]string) ([]RetrievedRule, error) {
	if r.ruleEngine == nil {
		return nil, nil
	}
	caseType := queryCtx["case_type"]
	rules, err := r.ruleEngine.MatchRules(ctx, caseType, queryCtx)
	if err != nil {
		return nil, fmt.Errorf("deterministic rules: %w", err)
	}
	maxPerSource := src.MaxPerSource
	if maxPerSource > 0 && len(rules) > maxPerSource {
		rules = rules[:maxPerSource]
	}
	return rules, nil
}

// queryIPC fetches IPC examination standards. The source uses the tech_field
// from query context to determine which IPC section(s) to query, and the
// case_type to infer the relevant legal article. When the IPC source is not
// configured, it returns nil (not an error), making it a purely optional source.
func (r *MultiSourceRetriever) queryIPC(ctx context.Context, src RuleSourceCfg, queryCtx map[string]string) ([]RetrievedRule, error) {
	if r.ipcSource == nil {
		return nil, nil
	}

	// Derive IPC section from tech_field if available; otherwise use "ALL".
	ipcSection := queryCtx["tech_field"]
	if ipcSection == "" {
		ipcSection = "ALL"
	}

	// Derive legal article from case_type.
	article := caseTypeToArticle(queryCtx["case_type"])

	rules, err := r.ipcSource.MatchByIPC(ctx, ipcSection, article, queryCtx)
	if err != nil {
		return nil, fmt.Errorf("IPC standards: %w", err)
	}
	maxPerSource := src.MaxPerSource
	if maxPerSource > 0 && len(rules) > maxPerSource {
		rules = rules[:maxPerSource]
	}
	return rules, nil
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

// RetrieveWithTopology runs all configured sources in parallel (like Retrieve)
// and additionally extracts workflow topology from the knowledge graph. The
// topology provides ordered WorkflowSteps that encode the CITES/APPLIES/RELATED_TO
// structure around GuidelineRule nodes matching the case type.
//
// The topology is best-effort: it returns nil when no topology extractor is
// configured or when the KG has no matching GuidelineRule nodes for the case type.
// In that case, callers should fall back to the flat rule list alone.
func (r *MultiSourceRetriever) RetrieveWithTopology(ctx context.Context, manifest RuleRetrievalManifest, facts []FactEntry, techField string) ([]RetrievedRule, *WorkflowTopology, error) {
	rules, err := r.Retrieve(ctx, manifest, facts, techField)
	if err != nil {
		return nil, nil, err
	}
	if r.topologyExt == nil || !r.topologyExt.HasStore() {
		return rules, nil, nil
	}

	topology, err := r.topologyExt.ExtractByCaseType(ctx, manifest.CaseType, 2, 15)
	if err != nil {
		// Topology extraction is best-effort; don't fail the whole retrieval.
		slog.Error("retrieve: topology extraction failed", "caseType", manifest.CaseType, "err", err)
		return rules, nil, nil
	}
	return rules, topology, nil
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

// caseTypeToArticle maps a CaseType to the most likely legal article ID for
// IPC standards matching. This enables the IPC source to pre-filter standards
// by the relevant patent law article without requiring explicit article input.
func caseTypeToArticle(caseType string) string {
	switch CaseType(caseType) {
	case CaseNoveltySearch, CasePatentability:
		return "patent-law-a22.2"
	case CaseRejection, CaseReexamination, CaseInvalidation:
		return "patent-law-a22.3"
	case CaseDrafting:
		return "patent-law-a26.3"
	case CaseOAResponse:
		return "patent-law-a22.3"
	default:
		return ""
	}
}
