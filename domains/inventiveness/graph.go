// Package inventiveness 提供完全独立的创造性分析 Pregel 子图。
//
// 子图不依赖 disclosure 管线，只通过 input state 获取数据。它实现三步法评估：
//
//	Step 1: 确定最接近的现有技术
//	Step 2: 确定区别特征和发明实际解决的技术问题
//	Step 3: 判断现有技术整体上是否存在技术启示
//	Step 4: 判断发明是否具有显著的进步（有益技术效果）
//
// 使用方式（通过 EventBus 接力）：
//
//	disclosure 管线完成后发射 DisclosureCompletedEvent →
//	InventivenessTrigger 接收事件 → 填充 PregelState → 运行子图 →
//	结果写回 session store / emit InventivenessCompletedEvent
//
// 使用方式（作为 Agent 工具）：
//
//	tool := NewInventivenessTool(WithProvider(provider))
//	// 注册到 Patent Agent，LLM 可主动调用 evaluate_inventiveness 分析创造性
//
// # 关键词
//
//	专利法第22条第3款、22.3、A22.3、创造性、三步法、inventive step、
//	非显而易见性、技术启示、TSM test、obviousness、显著的进步
package inventiveness

import (
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Pregel 图构建
// =============================================================================

// BuildInventivenessGraph 构建创造性分析的独立 Pregel 子图。
//
// 图拓扑:
//
//	load_input → step1_closest_prior_art → step2_distinguishing_features →
//	step3_technical_suggestion → step4_significant_progress → generate_conclusion → __end__
//
// 每步均为单 Agent LLM 节点，输出结构化 JSON。
// 结论逻辑：IsInventive = (Step3: 非显而易见) AND (Step4: 具有显著进步)
func BuildInventivenessGraph(provider agentcore.Provider) (*graph.CompiledPregelGraph, error) {
	pg := graph.NewPregelGraph()

	nodes := map[string]graph.PregelNode{
		"load_input":                    loadInputNode(),
		"step1_closest_prior_art":       step1ClosestPriorArtNode(provider),
		"step2_distinguishing_features": step2DistinguishingFeaturesNode(provider),
		"step3_technical_suggestion":    step3TechnicalSuggestionNode(provider),
		"step4_significant_progress":    step4SignificantProgressNode(provider),
		"generate_conclusion":           generateConclusionNode(provider),
	}

	for name, node := range nodes {
		if err := pg.AddNode(name, node); err != nil {
			return nil, fmt.Errorf("inventiveness: add node %q: %w", name, err)
		}
	}

	edges := [][2]string{
		{"load_input", "step1_closest_prior_art"},
		{"step1_closest_prior_art", "step2_distinguishing_features"},
		{"step2_distinguishing_features", "step3_technical_suggestion"},
		{"step3_technical_suggestion", "step4_significant_progress"},
		{"step4_significant_progress", "generate_conclusion"},
		{"generate_conclusion", graph.PregelEnd},
	}
	for _, e := range edges {
		if err := pg.AddEdge(e[0], e[1]); err != nil {
			return nil, fmt.Errorf("inventiveness: add edge %q→%q: %w", e[0], e[1], err)
		}
	}

	return pg.Compile("load_input", 100)
}
