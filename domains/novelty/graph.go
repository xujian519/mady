package novelty

import (
	"context"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/analysiskit"
	"github.com/xujian519/mady/graph"
)

// BuildNoveltyGraph 构建新颖性分析的独立 Pregel 子图。
//
// 图拓扑:
//
//	load_input → step_prior_art_check → step_single_compare →
//	step_conflict_check → step_grace_priority
//	     │ (TechDomain非空)                   │ (TechDomain为空)
//	     └→ step_special_domain ─┐            └→ generate_conclusion
//	                             └→ generate_conclusion → __end__
//
// 每步均为单 Agent LLM 节点，输出结构化 JSON。
// 结论逻辑：HasNovelty = (现有技术未全部公开) AND (抵触申请不成立)
func BuildNoveltyGraph(provider agentcore.Provider) (*graph.CompiledPregelGraph, error) {
	nodes := map[string]graph.PregelNode{
		"load_input":           loadInputNode(),
		"step_prior_art_check": stepPriorArtCheckNode(provider),
		"step_single_compare":  stepSingleCompareNode(provider),
		"step_conflict_check":  stepConflictCheckNode(provider),
		"step_grace_priority":  stepGracePriorityNode(provider),
		"step_special_domain":  stepSpecialDomainNode(provider),
		"generate_conclusion":  generateConclusionNode(provider),
	}

	// 线性主链路 + special_domain → conclusion → end
	edges := [][2]string{
		{"load_input", "step_prior_art_check"},
		{"step_prior_art_check", "step_single_compare"},
		{"step_single_compare", "step_conflict_check"},
		{"step_conflict_check", "step_grace_priority"},
		{"step_special_domain", "generate_conclusion"},
		{"generate_conclusion", graph.PregelEnd},
	}
	pg, err := analysiskit.AssemblePregel(nodes, edges)
	if err != nil {
		return nil, fmt.Errorf("novelty: %w", err)
	}

	// 条件边：step_grace_priority 之后根据 TechDomain 分支
	if err := pg.SetConditionalEdge("step_grace_priority", func(ctx context.Context, state graph.PregelState) []string {
		input := extractInput(state)
		if input != nil && input.TechDomain != "" && input.TechDomain != "general" {
			return []string{"step_special_domain"}
		}
		return []string{"generate_conclusion"}
	}); err != nil {
		return nil, fmt.Errorf("novelty: set conditional edge step_grace_priority: %w", err)
	}

	return pg.Compile("load_input", 100)
}
