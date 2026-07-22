package enablement

import (
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// State keys
// =============================================================================

const (
	stateKeyInput  = "enablement_input"
	stateKeyResult = "enablement_result"
	stateKeyStep1  = "step1_completeness"
	stateKeyStep2  = "step2_clarity"
	stateKeyStep3  = "step3_enablement"
	stateKeyDomain = "tech_domain"
)

// =============================================================================
// Pregel 图构建
// =============================================================================

// BuildEnablementGraph 构建专利法第26条第3款（充分公开/可实现性）评估的独立 Pregel 子图。
//
// 图拓扑:
//
//	load_input → step1_completeness → step2_clarity →
//	step3_enablement → generate_conclusion → __end__
//
// 每步均为单 Agent LLM 节点（Temperature=0.2, MaxTurns=1），
// 输出结构化 JSON，最终汇总为 EnablementResult。
func BuildEnablementGraph(provider agentcore.Provider) (*graph.CompiledPregelGraph, error) {
	pg := graph.NewPregelGraph()

	nodes := map[string]graph.PregelNode{
		"load_input":          loadInputNode(),
		"step1_completeness":  step1CompletenessNode(provider),
		"step2_clarity":       step2ClarityNode(provider),
		"step3_enablement":    step3EnablementNode(provider),
		"generate_conclusion": generateConclusionNode(provider),
	}

	for name, node := range nodes {
		if err := pg.AddNode(name, node); err != nil {
			return nil, fmt.Errorf("enablement: add node %q: %w", name, err)
		}
	}

	edges := [][2]string{
		{"load_input", "step1_completeness"},
		{"step1_completeness", "step2_clarity"},
		{"step2_clarity", "step3_enablement"},
		{"step3_enablement", "generate_conclusion"},
		{"generate_conclusion", graph.PregelEnd},
	}
	for _, e := range edges {
		if err := pg.AddEdge(e[0], e[1]); err != nil {
			return nil, fmt.Errorf("enablement: add edge %q→%q: %w", e[0], e[1], err)
		}
	}

	return pg.Compile("load_input", 100)
}
