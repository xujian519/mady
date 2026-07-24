package claimdrafting

import (
	"fmt"

	"github.com/xujian519/mady/graph"
)

// BuildClaimGraph 构建权利要求撰写的独立 Pregel 子图。
//
// 图拓扑:
//
//	load_input
//	  → classify_features
//	  → draft_primary
//	  → draft_parallel
//	  → draft_dependents
//	  → validate
//	  → score
//	  → finalize → __end__
//
// engine 和 scorer 为 nil 时使用默认实现。
func BuildClaimGraph(engine *RuleEngine, scorer *ClaimScorer) (*graph.CompiledPregelGraph, error) {
	pg := graph.NewPregelGraph()

	if engine == nil {
		engine = NewRuleEngine()
		RegisterDefaultRules(engine)
	}
	if scorer == nil {
		scorer = NewClaimScorer(engine)
	}

	builder := NewClaimBuilder(DomainGeneral, "")

	nodes := map[string]graph.PregelNode{
		"load_input":        loadInputNode(),
		"classify_features": classifyFeaturesNode(),
		"draft_primary":     draftPrimaryNode(builder),
		"draft_parallel":    draftParallelNode(builder),
		"draft_dependents":  draftDependentsNode(builder),
		"validate":          validateNode(engine),
		"score":             scoreNode(scorer),
		"finalize":          finalizeNode(),
	}

	for name, node := range nodes {
		if err := pg.AddNode(name, node); err != nil {
			return nil, fmt.Errorf("claimdrafting: add node %q: %w", name, err)
		}
	}

	edges := [][2]string{
		{"load_input", "classify_features"},
		{"classify_features", "draft_primary"},
		{"draft_primary", "draft_parallel"},
		{"draft_parallel", "draft_dependents"},
		{"draft_dependents", "validate"},
		{"validate", "score"},
		{"score", "finalize"},
		{"finalize", graph.PregelEnd},
	}

	for _, e := range edges {
		if err := pg.AddEdge(e[0], e[1]); err != nil {
			return nil, fmt.Errorf("claimdrafting: add edge %q→%q: %w", e[0], e[1], err)
		}
	}

	return pg.Compile("load_input", 30)
}
