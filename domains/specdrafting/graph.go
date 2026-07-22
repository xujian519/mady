package specdrafting

import (
	"fmt"

	"github.com/xujian519/mady/agentcore"
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
// 每个撰写节点有两种模式：
//   - LLM 模式：provider 可用时，使用 LLM Agent（Temperature=0.2, MaxTurns=1）
//   - 降级模式：provider 为 nil 时，使用 SpecBuilder 模板填充
//
// validate 和 score 节点使用规则引擎和评分器对生成结果进行校验。
// 可通过 GraphOption 注入自定义的规则引擎和评分器。
func BuildSpecificationGraph(provider agentcore.Provider, engine *RuleEngine, scorer *SpecScorer) (*graph.CompiledPregelGraph, error) {
	pg := graph.NewPregelGraph()

	// 默认引擎
	if engine == nil {
		engine = NewRuleEngine()
		RegisterDefaultRules(engine)
	}
	if scorer == nil {
		scorer = NewSpecScorer(engine)
	}

	nodes := map[string]graph.PregelNode{
		"load_input":       loadInputNode(),
		"classify_domain":  classifyDomainNode(provider),
		"draft_title":      draftTitleNode(provider),
		"draft_tech_field": draftTechFieldNode(provider),
		"draft_background": draftBackgroundNode(provider),
		"draft_content":    draftContentNode(provider),
		"draft_drawings":   draftDrawingsNode(provider),
		"draft_embodiment": draftEmbodimentNode(provider),
		"draft_abstract":   draftAbstractNode(provider),
		"validate":         validateNode(engine),
		"score":            scoreNode(scorer),
		"finalize":         finalizeNode(),
	}

	for name, node := range nodes {
		if err := pg.AddNode(name, node); err != nil {
			return nil, fmt.Errorf("specdrafting: add node %q: %w", name, err)
		}
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

	for _, e := range edges {
		if err := pg.AddEdge(e[0], e[1]); err != nil {
			return nil, fmt.Errorf("specdrafting: add edge %q→%q: %w", e[0], e[1], err)
		}
	}

	return pg.Compile("load_input", 30)
}

// GraphOption 配置可选的图参数（预留扩展）。
type GraphOption func(*graphConfig)

type graphConfig struct {
	engine *RuleEngine
	scorer *SpecScorer
}

// WithRuleEngine 注入自定义规则引擎。
func WithRuleEngine(engine *RuleEngine) GraphOption {
	return func(c *graphConfig) {
		c.engine = engine
	}
}

// WithScorer 注入自定义评分器。
func WithScorer(scorer *SpecScorer) GraphOption {
	return func(c *graphConfig) {
		c.scorer = scorer
	}
}

// BuildSpecGraphWithOpts 使用选项构建图。
func BuildSpecGraphWithOpts(provider agentcore.Provider, opts ...GraphOption) (*graph.CompiledPregelGraph, error) {
	cfg := &graphConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return BuildSpecificationGraph(provider, cfg.engine, cfg.scorer)
}
