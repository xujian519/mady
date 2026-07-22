package reasoning

import (
	"context"
	"log/slog"
	"sort"
)

// TopologyExtractor extracts workflow topology from the knowledge graph by
// traversing the edge structure around GuidelineRule seed nodes. It translates
// the KG's CITES/APPLIES/RELATED_TO chains into ordered, dependency-aware
// WorkflowSteps that the Planner can consume.
//
// The extractor is the bridge between Stage ② (flat rule retrieval) and
// Stage ③ (structured plan generation): it adds topological ordering that
// the KG already encodes but the flat []RetrievedRule list discards.
type TopologyExtractor struct {
	store KnowledgeGraphStore
}

// NewTopologyExtractor creates an extractor bound to a knowledge graph store.
// A nil store makes all methods return empty results.
func NewTopologyExtractor(store KnowledgeGraphStore) *TopologyExtractor {
	return &TopologyExtractor{store: store}
}

// HasStore reports whether a non-nil store is configured.
func (e *TopologyExtractor) HasStore() bool { return e.store != nil }

// ExtractByCaseType queries the KG for GuidelineRule nodes matching the given
// case type, traverses their outgoing edges (CITES/APPLIES/RELATED_TO) up to
// maxDepth hops, and returns a WorkflowTopology with ordered steps.
//
// Steps are ordered: CITES (must-check law articles) first, then APPLIES
// (case/precedent comparisons), then RELATED_TO (associated procedures).
// Within each group, steps are sorted by authority weight descending.
//
// maxDepth defaults to 2 when <= 0. MaxSteps defaults to 15 when <= 0.
func (e *TopologyExtractor) ExtractByCaseType(ctx context.Context, caseType CaseType, maxDepth, maxSteps int) (*WorkflowTopology, error) {
	if e.store == nil {
		return &WorkflowTopology{CaseType: caseType, Gaps: []string{"知识图谱不可用"}}, nil
	}
	if maxDepth <= 0 {
		maxDepth = 2
	}
	if maxSteps <= 0 {
		maxSteps = 15
	}

	// Step 1: search for GuidelineRule or Rule seed nodes matching the case type.
	keywords := caseTypeKeywords(caseType)
	seedNodes := e.searchSeedGuidelineRules(keywords)

	if len(seedNodes) == 0 {
		return &WorkflowTopology{
			CaseType: caseType,
			Gaps:     []string{"未找到匹配的审查指南规则"},
		}, nil
	}

	// Step 2: traverse edges from each seed node, collect topology steps.
	allSteps := e.collectTopologySteps(seedNodes, maxDepth, maxSteps)

	if len(allSteps) == 0 {
		return &WorkflowTopology{
			CaseType: caseType,
			RootRule: seedNodes[0].ID,
			Gaps:     []string{"种子节点无有效出边"},
		}, nil
	}

	// Step 3: sort by topological order (once — the sorted result is reused
	// for both the Steps output and the dependency matrix).
	ordered := e.orderSteps(allSteps)

	// Step 4: compute aggregate authority score.
	authScore := 0.0
	for _, s := range ordered {
		authScore += s.AuthorityWeight
	}
	if len(ordered) > 0 {
		authScore /= float64(len(ordered))
	}

	return &WorkflowTopology{
		CaseType:       caseType,
		RootRule:       seedNodes[0].ID,
		Steps:          ordered,
		Dependencies:   e.buildDependencyMatrix(ordered), // pre-sorted input, no re-sort
		AuthorityScore: authScore,
	}, nil
}

// --- private helpers ---

// searchSeedGuidelineRules searches the KG for GuidelineRule/Rule nodes
// matching any of the given keywords.
func (e *TopologyExtractor) searchSeedGuidelineRules(keywords []string) []KgNode {
	seen := make(map[string]bool)
	var seeds []KgNode
	for _, kw := range keywords {
		nodes, err := e.store.SearchNodes(kw, "", 20)
		if err != nil {
			slog.Error("topology: SearchNodes failed", "keyword", kw, "err", err)
			continue
		}
		for _, n := range nodes {
			if seen[n.ID] {
				continue
			}
			// Only accept GuidelineRule or Rule node types as seeds.
			if n.NodeType != "GuidelineRule" && n.NodeType != "Rule" {
				continue
			}
			seen[n.ID] = true
			seeds = append(seeds, n)
		}
	}
	return seeds
}

// collectTopologySteps traverses outward from seed nodes, collecting
// WorkflowSteps from CITES/APPLIES/RELATED_TO edges.
func (e *TopologyExtractor) collectTopologySteps(seeds []KgNode, maxDepth, maxSteps int) []WorkflowStep {
	visited := make(map[string]bool)
	var allSteps []WorkflowStep

	for _, seed := range seeds {
		if len(allSteps) >= maxSteps {
			break
		}
		visited[seed.ID] = true
		frontier := []KgNodeDetail{{Node: toKgNode(seed)}}
		// Fetch full detail for the seed to get its outgoing edges.
		if detail, err := e.store.GetNodeDetail(seed.ID); err == nil && detail != nil {
			frontier[0] = *detail
		} else if err != nil {
			slog.Error("topology: GetNodeDetail seed", "seedID", seed.ID, "err", err)
		}

		for depth := 0; depth < maxDepth && len(allSteps) < maxSteps; depth++ {
			var nextFrontier []KgNodeDetail
			for _, current := range frontier {
				detail := &current
				if detail.Node.ID != seed.ID {
					if d, err := e.store.GetNodeDetail(current.Node.ID); err == nil && d != nil {
						detail = d
					} else if err != nil {
						slog.Error("topology: GetNodeDetail frontier", "nodeID", current.Node.ID, "err", err)
					}
				}
				for _, edge := range detail.Outgoing {
					if len(allSteps) >= maxSteps {
						break
					}
					relation := WorkflowRelation(edge.Relation)
					if !isTopologyRelation(relation) {
						continue
					}
					if visited[edge.TargetID] {
						continue
					}
					visited[edge.TargetID] = true

					// Fetch target node detail for name and type.
					targetDetail, err := e.store.GetNodeDetail(edge.TargetID)
					if err != nil || targetDetail == nil {
						if err != nil {
							slog.Error("topology: GetNodeDetail target", "targetID", edge.TargetID, "err", err)
						}
						continue
					}

					step := WorkflowStep{
						ArticleID:       edge.TargetID,
						NodeType:        targetDetail.Node.NodeType,
						Name:            targetDetail.Node.Name,
						Content:         truncate(targetDetail.Node.Content, 200),
						Relation:        relation,
						Strategy:        relationToStrategy(relation, targetDetail.Node.NodeType),
						Priority:        priorityForNodeType(targetDetail.Node.NodeType),
						AuthorityWeight: edge.Weight,
					}
					allSteps = append(allSteps, step)

					// Add target to next frontier if we should go deeper.
					if depth+1 < maxDepth {
						nextFrontier = append(nextFrontier, *targetDetail)
					}
				}
			}
			frontier = nextFrontier
		}
	}
	return allSteps
}

// orderSteps sorts steps by: CITES first → APPLIES → RELATED_TO/CONTAINS.
// Within each group, higher authority weight comes first.
func (e *TopologyExtractor) orderSteps(steps []WorkflowStep) []WorkflowStep {
	out := make([]WorkflowStep, len(steps))
	copy(out, steps)
	sort.SliceStable(out, func(i, j int) bool {
		oi := relationOrder(out[i].Relation)
		oj := relationOrder(out[j].Relation)
		if oi != oj {
			return oi < oj
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].AuthorityWeight > out[j].AuthorityWeight
	})
	return out
}

// buildDependencyMatrix creates a simple dependency matrix.
// Precondition: steps must already be sorted by orderSteps (topological order).
// CITES steps: independent (no dependencies).
// APPLIES steps: depend on all preceding CITES steps they apply to.
// RELATED_TO/CONTAINS steps: depend on all preceding steps.
func (e *TopologyExtractor) buildDependencyMatrix(steps []WorkflowStep) [][]int {
	deps := make([][]int, len(steps))

	var citesIndices []int
	for i, s := range steps {
		switch s.Relation {
		case WorkflowRelCites:
			deps[i] = nil
			citesIndices = append(citesIndices, i)
		case WorkflowRelApplies:
			// APPLIES steps depend on all preceding CITES steps.
			if len(citesIndices) > 0 {
				deps[i] = append([]int(nil), citesIndices...)
			}
		default:
			// RELATED_TO/CONTAINS depend on all preceding steps.
			all := make([]int, i)
			for j := range i {
				all[j] = j
			}
			deps[i] = all
		}
	}
	return deps
}

// --- mapping helpers ---

// caseTypeKeywords maps a business-process case type to KG search keywords.
// These are used to find relevant GuidelineRule seed nodes in the graph.
func caseTypeKeywords(caseType CaseType) []string {
	switch caseType {
	case CaseNoveltySearch:
		return []string{"新颖性", "新颖性审查", "单独对比"}
	case CasePatentability:
		return []string{"可专利性", "新颖性", "创造性", "三步法"}
	case CaseOAResponse:
		return []string{"答复", "意见陈述", "驳回"}
	case CaseRejection:
		return []string{"驳回", "驳回决定"}
	case CaseReexamination:
		return []string{"复审", "前置审查"}
	case CaseInvalidation:
		return []string{"无效", "宣告无效"}
	case CaseInfringement:
		return []string{"侵权", "全面覆盖", "等同"}
	case CaseFTO:
		return []string{"FTO", "自由实施"}
	case CaseValidity:
		return []string{"有效性", "专利有效"}
	case CaseDrafting:
		return []string{"撰写", "申请文件"}
	case CaseLegalStatus, CaseGeneralLegal:
		return []string{"法律状态", ""}
	default:
		return []string{string(caseType)}
	}
}

// isTopologyRelation returns true for edge relations that contribute to
// workflow topology. CONTAINS edges (sub-rule scaffolding) are included
// but receive a lower priority in ordering.
func isTopologyRelation(r WorkflowRelation) bool {
	switch r {
	case WorkflowRelCites, WorkflowRelApplies,
		WorkflowRelRelatedTo, WorkflowRelContains:
		return true
	default:
		return false
	}
}

// relationToStrategy maps a KG edge relation to the most suitable
// PlanStep execution strategy.
func relationToStrategy(relation WorkflowRelation, _ string) StrategyType {
	switch relation {
	case WorkflowRelCites:
		return StrategyChain
	case WorkflowRelApplies:
		return StrategyMultiHypothesis
	case WorkflowRelRelatedTo:
		return StrategyChain
	case WorkflowRelContains:
		return StrategyChain
	default:
		return StrategyChain
	}
}

// priorityForNodeType maps a KG node type to a numeric priority.
// LawArticle and GuidelineRule get highest priority (must-check);
// Case/Judgment are supportive (should-check);
// Everything else is informational.
func priorityForNodeType(nodeType string) int {
	switch nodeType {
	case "LawArticle", "GuidelineRule", "Rule":
		return 1
	case "Case", "Judgment", "SupremeCourtJudgment", "Evidence":
		return 2
	default:
		return 3
	}
}

// relationOrder defines the sort order for topology steps.
const (
	orderCITES    = 0
	orderAPPLIES  = 1
	orderRELATED  = 2
	orderCONTAINS = 3
	orderOther    = 4
)

func relationOrder(r WorkflowRelation) int {
	switch r {
	case WorkflowRelCites:
		return orderCITES
	case WorkflowRelApplies:
		return orderAPPLIES
	case WorkflowRelRelatedTo:
		return orderRELATED
	case WorkflowRelContains:
		return orderCONTAINS
	default:
		return orderOther
	}
}

// toKgNode creates a KgNode from a KgNode embedded in a detail.
// This is used to seed the frontier when we only have a search result.
func toKgNode(n KgNode) KgNode { return n }

// No compile-time assertion here: TopologyExtractor has a store field of type
// KnowledgeGraphStore rather than implementing it directly. The assertion would
// be tautological (nil type assertion always compiles). The adapter assertion
// in knowledge/graph/adapter.go covers the concrete implementation.
