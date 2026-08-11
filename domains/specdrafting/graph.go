package specdrafting

import (
	"fmt"

	"github.com/xujian519/mady/domains/analysiskit"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Pregel 图构建
// =============================================================================

// BuildSpecificationGraph 构建专利说明书撰写的独立 Pregel 子图。
//
// 图拓扑:
//
//	load_input
//	  → classify_domain
//	  → draft_title
//	  → draft_tech_field
//	  → draft_background
//	  → draft_content
//	  → draft_drawings
//	  → draft_embodiment
//	  → draft_abstract
//	  → validate
//	  → score
//	  → finalize → __end__
//
// 所有撰写节点使用 SpecBuilder 模板填充生成内容。
// validate 和 score 节点使用规则引擎和评分器对生成结果进行校验。
// engine 和 scorer 为 nil 时使用默认实现。
func BuildSpecificationGraph(engine *RuleEngine, scorer *SpecScorer) (*graph.CompiledPregelGraph, error) {
	// 默认引擎
	if engine == nil {
		engine = NewRuleEngine()
		RegisterDefaultRules(engine)
	}
	if scorer == nil {
		scorer = NewSpecScorer(engine)
	}

	// 共享 Builder 实例（避免各节点重复创建）
	builder := NewSpecBuilder(nil)

	nodes := map[string]graph.PregelNode{
		"load_input":       loadInputNode(),
		"classify_domain":  classifyDomainNode(),
		"draft_title":      draftTitleNode(builder),
		"draft_tech_field": draftTechFieldNode(builder),
		"draft_background": draftBackgroundNode(builder),
		"draft_content":    draftContentNode(builder),
		"draft_drawings":   draftDrawingsNode(builder),
		"draft_embodiment": draftEmbodimentNode(builder),
		"draft_abstract":   draftAbstractNode(builder),
		"validate":         validateNode(engine),
		"score":            scoreNode(scorer),
		"finalize":         finalizeNode(),
	}

	edges := [][2]string{
		{"load_input", "classify_domain"},
		{"classify_domain", "draft_title"},
		{"draft_title", "draft_tech_field"},
		{"draft_tech_field", "draft_background"},
		{"draft_background", "draft_content"},
		{"draft_content", "draft_drawings"},
		{"draft_drawings", "draft_embodiment"},
		{"draft_embodiment", "draft_abstract"},
		{"draft_abstract", "validate"},
		{"validate", "score"},
		{"score", "finalize"},
		{"finalize", graph.PregelEnd},
	}

	pg, err := analysiskit.AssemblePregel(nodes, edges)
	if err != nil {
		return nil, fmt.Errorf("specdrafting: %w", err)
	}

	return pg.Compile("load_input", 30)
}
