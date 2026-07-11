package reasoning

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// KgNode is a node in the knowledge graph.
type KgNode struct {
	ID       string `json:"id"`
	NodeType string `json:"node_type"`
	Name     string `json:"name"`
	Content  string `json:"content,omitempty"`
}

// KgEdge is a directed, weighted edge in the knowledge graph.
type KgEdge struct {
	TargetID string  `json:"target_id"`
	Relation string  `json:"relation"`
	Weight   float64 `json:"weight"`
}

// KgNodeDetail is a node together with its outgoing and incoming edges.
type KgNodeDetail struct {
	Node     KgNode   `json:"node"`
	Outgoing []KgEdge `json:"outgoing"`
	Incoming []KgEdge `json:"incoming"`
}

// KnowledgeGraphStore is the storage interface for multi-hop reasoning
// traversal. A concrete implementation is provided by knowledge/graph/ (Week 5).
type KnowledgeGraphStore interface {
	SearchNodes(keyword, nodeType string, limit int) ([]KgNode, error)
	GetNodeDetail(nodeID string) (*KgNodeDetail, error)
}

// LlmMessage is a single chat message for the LlmClient interface.
type LlmMessage struct {
	Role    string
	Content string
}

// LlmClient is a minimal LLM interface used for keyword extraction and
// search-direction suggestion. It decouples the walker from any specific
// provider SDK.
type LlmClient interface {
	Chat(ctx context.Context, messages []LlmMessage) (string, error)
}

// ReasoningWalkInput configures a single walk() run.
type ReasoningWalkInput struct {
	Facts     []string
	CaseType  CaseType
	MaxDepth  int // default 4
	MaxChains int // default 5
}

// ReasoningWalkResult is the outcome of walk().
type ReasoningWalkResult struct {
	Chains    []ReasoningChain `json:"chains"`
	SeedNodes []KgNode         `json:"seed_nodes"`
	Coverage  float64          `json:"coverage"`
	Gaps      []string         `json:"gaps,omitempty"`
}

// CollectAllInput configures a collectAll() run.
type CollectAllInput struct {
	Facts          []string
	CaseType       CaseType
	TechnicalField string
}

// CollectAllResult is the outcome of collectAll().
type CollectAllResult struct {
	Constraints      []RuleConstraint `json:"constraints"`
	RelatedArticles  []string         `json:"related_articles"`
	SearchDirections []string         `json:"search_directions"`
	Coverage         float64          `json:"coverage"`
	Gaps             []string         `json:"gaps,omitempty"`
}

// factTypeStrategies maps a fact category to the KG relations and target node
// types that should be followed when walking from that kind of fact.
var factTypeStrategies = map[string]struct {
	relations   []string
	targetTypes []string
}{
	"technical": {
		relations:   []string{"SIMILAR_TO", "CITES"},
		targetTypes: []string{"Case", "Concept"},
	},
	"legal": {
		relations:   []string{"APPLIES", "INTERPRETED_BY", "DEFINES"},
		targetTypes: []string{"LawArticle", "GuidelineRule"},
	},
	"procedural": {
		relations:   []string{"IMPLEMENTED_BY", "CONTAINS"},
		targetTypes: []string{"ImplementationRule", "Chapter"},
	},
	"precedent": {
		relations:   []string{"HAS_PRECEDENT", "CITED_BY"},
		targetTypes: []string{"Judgment", "SupremeCourtJudgment"},
	},
}

// ReasoningWalker performs multi-hop reasoning traversal over a knowledge graph.
//
// Walk runs in judgment mode: facts → KG multi-hop traversal → reasoning chains.
// CollectAll runs in flexible-plan mode: gathers all relevant rule constraints.
type ReasoningWalker struct {
	store KnowledgeGraphStore
	llm   LlmClient
}

// NewReasoningWalker creates a walker bound to a KG store and an LLM client.
// Both may be nil: a nil store makes Walk/CollectAll return empty results, and
// a nil LLM makes the walker fall back to heuristic keyword extraction.
func NewReasoningWalker(store KnowledgeGraphStore, llm LlmClient) *ReasoningWalker {
	return &ReasoningWalker{store: store, llm: llm}
}

// Walk performs judgment-mode multi-hop reasoning over the KG.
func (w *ReasoningWalker) Walk(ctx context.Context, in ReasoningWalkInput) (ReasoningWalkResult, error) {
	maxDepth := in.MaxDepth
	if maxDepth == 0 {
		maxDepth = 4
	}
	maxChains := in.MaxChains
	if maxChains == 0 {
		maxChains = 5
	}

	if len(in.Facts) == 0 || w.store == nil {
		return ReasoningWalkResult{Chains: []ReasoningChain{}, Coverage: 0, Gaps: []string{}}, nil
	}

	keywords := w.extractKeywords(ctx, in.Facts)

	// Layer 2: KG search for seed nodes.
	seedNodes := w.searchSeedNodes(keywords)

	// Layer 3: multi-hop BFS traversal.
	chains := make([]ReasoningChain, 0, maxChains)
	factRef := ""
	if len(in.Facts) > 0 {
		factRef = truncate(in.Facts[0], 30)
	}

	for _, seed := range seedNodes {
		if len(chains) >= maxChains {
			break
		}
		chainNodes := w.traverseFrom(ctx, seed, maxDepth)
		chains = append(chains, ReasoningChain{
			ID:         fmtChainID(len(chains) + 1),
			FactRef:    factRef,
			Nodes:      chainNodes,
			LegalBasis: extractLegalBasis(chainNodes),
			Confidence: confidenceFor(len(chainNodes)),
			Gaps:       gapsFor(seed.Name, len(chainNodes)),
		})
	}

	var gaps []string
	for _, c := range chains {
		gaps = append(gaps, c.Gaps...)
	}
	coverage := 0.0
	if len(in.Facts) > 0 && len(chains) > 0 {
		coverage = min1(float64(len(chains)) / float64(len(in.Facts)))
	}

	return ReasoningWalkResult{
		Chains:    chains,
		SeedNodes: seedNodes,
		Coverage:  coverage,
		Gaps:      gaps,
	}, nil
}

// CollectAll gathers rule constraints across all fact-category strategies.
func (w *ReasoningWalker) CollectAll(ctx context.Context, in CollectAllInput) (CollectAllResult, error) {
	if len(in.Facts) == 0 || w.store == nil {
		return CollectAllResult{Constraints: []RuleConstraint{}, Coverage: 0, Gaps: []string{}}, nil
	}

	keywords := w.extractKeywords(ctx, in.Facts)

	var constraints []RuleConstraint
	articleIDs := make(map[string]struct{})

	for _, strategyType := range sortedStrategyTypes() {
		strategy := factTypeStrategies[strategyType]
		for _, kw := range keywords {
			nodes, err := w.store.SearchNodes(kw, "", 5)
			if err != nil {
				continue
			}
			for _, node := range nodes {
				if !contains(strategy.targetTypes, node.NodeType) {
					continue
				}
				req := classifyRequirement(node.NodeType)
				constraints = append(constraints, RuleConstraint{
					ArticleID:   node.ID,
					ArticleName: node.Name,
					Requirement: req,
					Description: node.Content,
				})
				if req == ReqMust {
					articleIDs[node.ID] = struct{}{}
				}
			}
		}
	}

	// LLM-suggested search directions (best-effort).
	var directions []string
	if w.llm != nil {
		if out, err := w.llm.Chat(ctx, []LlmMessage{{
			Role:    "user",
			Content: "基于以下事实，建议专利检索方向：" + strings.Join(in.Facts, "; "),
		}}); err == nil {
			for _, line := range strings.Split(out, "\n") {
				if line = strings.TrimSpace(line); line != "" {
					directions = append(directions, line)
				}
			}
		}
	}

	// Sort constraints: must < should < note.
	sort.SliceStable(constraints, func(i, j int) bool {
		return requirementOrder(constraints[i].Requirement) < requirementOrder(constraints[j].Requirement)
	})

	coverage := 0.0
	gaps := []string{}
	if len(constraints) == 0 {
		gaps = []string{"未找到相关规则约束"}
	} else {
		coverage = 1
	}

	return CollectAllResult{
		Constraints:      constraints,
		RelatedArticles:  mapKeys(articleIDs),
		SearchDirections: directions,
		Coverage:         coverage,
		Gaps:             gaps,
	}, nil
}

// --- private helpers ---

func (w *ReasoningWalker) extractKeywords(ctx context.Context, facts []string) []string {
	if w.llm != nil {
		if out, err := w.llm.Chat(ctx, []LlmMessage{{
			Role:    "user",
			Content: "从以下事实中提取 3-5 个搜索关键词（用于专利知识图谱搜索），用逗号分隔：\n" + strings.Join(facts, "\n"),
		}}); err == nil {
			var kws []string
			for _, part := range strings.FieldsFunc(out, func(r rune) bool {
				return r == ',' || r == '，'
			}) {
				if part = strings.TrimSpace(part); part != "" {
					kws = append(kws, part)
				}
			}
			if len(kws) > 0 {
				return kws[:min1int(5, len(kws))]
			}
		}
	}
	// Heuristic fallback: split on whitespace / punctuation.
	var kws []string
	for _, word := range strings.FieldsFunc(strings.Join(facts, " "), func(r rune) bool {
		return r == ' ' || r == ',' || r == '，' || r == '。' || r == '；' || r == ';'
	}) {
		if len([]rune(word)) >= 2 {
			kws = append(kws, word)
		}
		if len(kws) >= 5 {
			break
		}
	}
	return kws
}

func (w *ReasoningWalker) searchSeedNodes(keywords []string) []KgNode {
	var all []KgNode
	seen := make(map[string]struct{})
	for _, kw := range keywords {
		nodes, err := w.store.SearchNodes(kw, "", 10)
		if err != nil {
			continue
		}
		for _, n := range nodes {
			if _, ok := seen[n.ID]; ok {
				continue
			}
			seen[n.ID] = struct{}{}
			all = append(all, n)
		}
	}
	return all
}

func (w *ReasoningWalker) traverseFrom(ctx context.Context, seed KgNode, maxDepth int) []ReasoningChainNode {
	chainNodes := []ReasoningChainNode{{
		KgNodeID: seed.ID,
		NodeType: seed.NodeType,
		Name:     seed.Name,
		Relation: "SEED",
		Excerpt:  seed.Content,
	}}

	visited := map[string]struct{}{seed.ID: {}}
	currentID := seed.ID

	for depth := 0; depth < maxDepth; depth++ {
		detail, err := w.store.GetNodeDetail(currentID)
		if err != nil || detail == nil || len(detail.Outgoing) == 0 {
			break
		}
		best := selectBestEdge(detail.Outgoing, CaseInvalidation) // legal-leaning by default
		if best == nil {
			break
		}
		if _, ok := visited[best.TargetID]; ok {
			break
		}
		visited[best.TargetID] = struct{}{}

		var name, ntype, excerpt string
		if td, err := w.store.GetNodeDetail(best.TargetID); err == nil && td != nil {
			name = td.Node.Name
			ntype = td.Node.NodeType
			excerpt = td.Node.Content
		}
		chainNodes = append(chainNodes, ReasoningChainNode{
			KgNodeID: best.TargetID,
			NodeType: ntype,
			Name:     name,
			Relation: best.Relation,
			Excerpt:  excerpt,
		})
		currentID = best.TargetID
	}

	return chainNodes
}

func selectBestEdge(edges []KgEdge, caseType CaseType) *KgEdge {
	if len(edges) == 0 {
		return nil
	}
	strategyType := "technical"
	if caseType == CaseInvalidation || caseType == CasePatentability {
		strategyType = "legal"
	}
	strategy := factTypeStrategies[strategyType]

	var preferred []KgEdge
	for _, e := range edges {
		if contains(strategy.relations, e.Relation) {
			preferred = append(preferred, e)
		}
	}
	pool := preferred
	if len(pool) == 0 {
		pool = edges
	}

	best := &pool[0]
	for i := 1; i < len(pool); i++ {
		if pool[i].Weight > best.Weight {
			best = &pool[i]
		}
	}
	return best
}

func extractLegalBasis(nodes []ReasoningChainNode) LegalBasis {
	var lb LegalBasis
	for _, n := range nodes {
		switch n.NodeType {
		case "LawArticle":
			if lb.LawArticle == "" {
				lb.LawArticle = n.Name
			}
		case "GuidelineRule":
			if lb.GuidelineRule == "" {
				lb.GuidelineRule = n.Name
			}
		case "Judgment", "SupremeCourtJudgment":
			if lb.PrecedentCase == "" {
				lb.PrecedentCase = n.Name
			}
		}
	}
	return lb
}

func classifyRequirement(nodeType string) Requirement {
	switch nodeType {
	case "LawArticle", "GuidelineRule":
		return ReqMust
	case "IPC", "ConceptDetail":
		return ReqNote
	default:
		return ReqShould
	}
}

func requirementOrder(r Requirement) int {
	switch r {
	case ReqMust:
		return 0
	case ReqShould:
		return 1
	case ReqNote:
		return 2
	default:
		return 99
	}
}

// --- small utils ---

func contains(slice []string, v string) bool {
	for _, s := range slice {
		if s == v {
			return true
		}
	}
	return false
}

func sortedStrategyTypes() []string {
	out := make([]string, 0, len(factTypeStrategies))
	for k := range factTypeStrategies {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func mapKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func min1(x float64) float64 {
	if x > 1 {
		return 1
	}
	return x
}

func min1int(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func confidenceFor(nodeCount int) float64 {
	if nodeCount >= 2 {
		return 0.8
	}
	return 0.3
}

func gapsFor(seedName string, nodeCount int) []string {
	if nodeCount < 2 {
		return []string{"从 \"" + seedName + "\" 出发的推理链不足 2 跳"}
	}
	return nil
}

func fmtChainID(n int) string {
	return "chain_" + strconv.Itoa(n)
}
