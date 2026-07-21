// Package patent provides Pregel-based patent analysis workflows.
//
// The novelty analysis workflow implements the standard patent examination
// process as a Pregel graph:
//
//	输入发明描述 → parse → search → analyze → conclude → ApprovalGate → 输出
//
// Each node reads from and writes to shared PregelState, enabling iterative
// refinement: the analyze phase may trigger additional search rounds if
// new prior art directions are discovered.
package patent

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/retrieval/domain"
)

// State keys used by the patent analysis workflow.
const (
	StateInput       = "input"        // original invention description
	StateFeatures    = "features"     // []string: extracted technical features
	StateSearchQuery = "search_query" // query constructed from features
	StatePriorArt    = "prior_art"    // []string: retrieved prior art summaries
	StateComparison  = "comparison"   // feature-by-feature comparison results
	StateConclusion  = "conclusion"   // final novelty/creativity assessment
	StateOutput      = "output"       // final output text
	StateRuleCheck   = "rule_check"   // rule engine check report (Markdown)
	StateRuleVerdict = "rule_verdict" // aggregate Verdict from rule check
)

// parseNode extracts technical features from the invention description.
// It identifies key claim elements from the Chinese patent description format:
// "一种...方法/装置/系统" + "其特征在于" + "包括/由...组成".
func parseNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	input := state.GetString(StateInput)
	if input == "" {
		return nil, fmt.Errorf("patent: input is empty")
	}

	features := extractFeatures(input)

	out := graph.PregelState{
		StateFeatures:    features,
		StateSearchQuery: buildSearchQuery(features),
		StateInput:       input,
	}
	return out, nil
}

// extractFeatures identifies technical features from a Chinese patent description.
func extractFeatures(text string) []string {
	var features []string

	// Split by common feature delimiters.
	markers := []string{"包括", "包含", "由...组成", "其特征在于", "所述"}
	for _, marker := range markers {
		if idx := strings.Index(text, marker); idx >= 0 {
			rest := text[idx+len(marker):]
			// Extract the phrase after the marker (up to punctuation or next marker).
			for _, part := range strings.Split(rest, "；") {
				part = strings.TrimSpace(part)
				if len(part) > 5 && len(part) < 200 {
					features = append(features, part)
				}
			}
		}
	}

	// If no structured features found, use sentence splitting.
	if len(features) == 0 {
		for _, sentence := range strings.Split(text, "。") {
			sentence = strings.TrimSpace(sentence)
			if len(sentence) > 10 {
				features = append(features, sentence)
			}
		}
	}

	// Deduplicate and limit.
	seen := make(map[string]bool)
	var result []string
	for _, f := range features {
		f = strings.TrimSpace(f)
		if !seen[f] && len(f) > 3 {
			seen[f] = true
			result = append(result, f)
		}
		if len(result) >= 10 {
			break
		}
	}
	return result
}

// buildSearchQuery constructs a prior art search query from features.
func buildSearchQuery(features []string) string {
	if len(features) == 0 {
		return ""
	}
	// Use first 3 features as primary search terms.
	n := len(features)
	if n > 3 {
		n = 3
	}
	return strings.Join(features[:n], " ")
}

// =============================================================================
// GraphOption — 可选依赖注入
// =============================================================================

// GraphOption 可选地配置 patent 分析图的依赖（如 search 节点的检索器）。
// 采用 functional option 模式，使无检索器的调用点零破坏。
type GraphOption func(*graphConfig)

type graphConfig struct {
	retriever domain.DomainRetriever
}

// WithRetriever 注入专利领域检索器，启用 search 节点的真实现有技术检索。
// 未注入时 search 节点返回占位文本，保持向后兼容。
func WithRetriever(r domain.DomainRetriever) GraphOption {
	return func(c *graphConfig) { c.retriever = r }
}

// =============================================================================
// 检索节点
// =============================================================================

// newSearchNode 创建现有技术检索的 Pregel 节点。
// retriever 为 nil 时返回占位结果（兼容旧行为）。
// retriever 非 nil 时查询专利领域检索器，产出真实现有技术列表。
func newSearchNode(retriever domain.DomainRetriever) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		query := state.GetString(StateSearchQuery)
		features, _ := state[StateFeatures].([]string)

		out := graph.PregelState{
			StateSearchQuery: query,
		}
		if features != nil {
			out[StateFeatures] = features
		}

		// 无检索器 → 返回占位结果（保持向后兼容）。
		if retriever == nil {
			out[StatePriorArt] = []string{
				"现有技术文献检索结果将在此处展示",
				"检索范围: 中国专利数据库、外国专利数据库",
			}
			return out, nil
		}

		// 真实检索：用查询文本搜索专利领域检索器。
		results, err := retriever.Search(ctx, domain.DomainQuery{
			Text:       query,
			MaxResults: 8,
		})
		if err != nil {
			// 检索失败不应阻断管线，返回占位让分析节点降级处理。
			out[StatePriorArt] = []string{
				"现有技术检索暂时不可用",
				"建议手动检索相关对比文件",
			}
			return out, nil
		}

		priorArt := make([]string, 0, len(results.Documents))
		for _, doc := range results.Documents {
			snippet := doc.Snippet
			if snippet == "" && doc.Content != "" {
				// 截取内容前 200 字作为摘要。
				runes := []rune(doc.Content)
				if len(runes) > 200 {
					snippet = string(runes[:200]) + "…"
				} else {
					snippet = doc.Content
				}
			}
			text := fmt.Sprintf("[%s] %s", doc.ID, doc.Title)
			if snippet != "" {
				text += ": " + snippet
			}
			priorArt = append(priorArt, text)
		}

		if len(priorArt) == 0 {
			priorArt = append(priorArt, "未检索到相关现有技术文献")
		}

		out[StatePriorArt] = priorArt
		return out, nil
	}
}

// analyzeNode performs feature-by-feature comparison between the invention
// and retrieved prior art. It produces a structured comparison result.
func analyzeNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	features, _ := state[StateFeatures].([]string)
	priorArt, _ := state[StatePriorArt].([]string)

	var comparison strings.Builder
	comparison.WriteString("## 技术特征比对分析\n\n")

	if len(features) == 0 {
		comparison.WriteString("未识别到明确的技术特征。\n")
	} else {
		comparison.WriteString("### 识别到的技术特征\n\n")
		for i, f := range features {
			fmt.Fprintf(&comparison, "%d. %s\n", i+1, f)
		}
	}

	comparison.WriteString("\n### 现有技术参考\n\n")
	if len(priorArt) > 0 {
		for _, art := range priorArt {
			fmt.Fprintf(&comparison, "- %s\n", art)
		}
	} else {
		comparison.WriteString("未检索到相关现有技术。\n")
	}

	return graph.PregelState{
		StateComparison: comparison.String(),
		StateFeatures:   features,
		StatePriorArt:   priorArt,
	}, nil
}

// concludeNode generates the final novelty/creativity assessment report.
// In production, this would be an LLM call using the accumulated analysis.
func concludeNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	comparison := state.GetString(StateComparison)
	input := state.GetString(StateInput)

	var report strings.Builder
	report.WriteString("# 专利分析报告\n\n")
	report.WriteString("## 发明概述\n\n")
	if len(input) > 200 {
		input = input[:200] + "..."
	}
	report.WriteString(input)
	report.WriteString("\n\n")
	report.WriteString(comparison)
	report.WriteString("\n\n## 初步结论\n\n")
	report.WriteString("> ⚠️ 本分析由 AI 辅助生成，不构成正式法律意见。")
	report.WriteString("专利性判断应由具备资质的专利代理人或律师确认。\n\n")
	report.WriteString("基于现有技术的初步检索和分析：\n")
	report.WriteString("- 建议进行更全面的专利检索以确认新颖性\n")
	report.WriteString("- 根据审查指南的规定，进一步评估创造性\n")
	report.WriteString("- 权利要求宜得到说明书的充分支持，建议由代理人核实\n")

	return graph.PregelState{
		StateConclusion: report.String(),
		StateOutput:     report.String(),
	}, nil
}

// BuildNoveltyGraph constructs a Pregel graph for patent novelty analysis
// (无检索器注入，search 节点返回占位结果）。
//
// Graph structure:
//
//	parse → search → analyze → conclude → __end__
func BuildNoveltyGraph() (*graph.CompiledPregelGraph, error) {
	return BuildNoveltyGraphWithOpts()
}

// BuildNoveltyGraphWithOpts 构造新颖性分析 Pregel 图，支持可选的依赖注入。
func BuildNoveltyGraphWithOpts(opts ...GraphOption) (*graph.CompiledPregelGraph, error) {
	cfg := &graphConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	g := graph.NewPregelGraph()

	g.AddNode("parse", parseNode)
	g.AddNode("search", newSearchNode(cfg.retriever))
	g.AddNode("analyze", analyzeNode)
	g.AddNode("conclude", concludeNode)

	// Linear flow: parse → search → analyze → conclude → end.
	g.AddEdge("parse", "search")
	g.AddEdge("search", "analyze")
	g.AddEdge("analyze", "conclude")
	g.AddEdge("conclude", graph.PregelEnd)

	return g.Compile("parse", 10) // max 10 supersteps
}

// ruleCheckNode runs the deterministic rule engine against the analysis output
// and writes a Markdown check report plus the aggregate verdict to state.
// This node sits between "analyze" and "conclude" in BuildNoveltyGraphWithRules.
func ruleCheckNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	comparison := state.GetString(StateComparison)
	features, _ := state[StateFeatures].([]string)
	priorArt, _ := state[StatePriorArt].([]string)

	// Combine all analysis text for rule checking.
	var checkText strings.Builder
	checkText.WriteString(comparison)
	for _, f := range features {
		checkText.WriteString("\n")
		checkText.WriteString(f)
	}
	for _, art := range priorArt {
		checkText.WriteString("\n")
		checkText.WriteString(art)
	}

	engine := NewRuleEngine()
	engine.RegisterRules(DefaultPatentRules())

	results := engine.Evaluate(engine.Rules(), checkText.String(), "patent_novelty")
	verdict := Aggregate(results)

	return graph.PregelState{
		StateRuleCheck:   FormatRuleResults(results, verdict),
		StateRuleVerdict: string(verdict),
		StateComparison:  comparison,
		StateFeatures:    features,
		StatePriorArt:    priorArt,
	}, nil
}

// concludeWithRulesNode is an enhanced conclude node that incorporates the
// rule engine report into the final assessment.
func concludeWithRulesNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	base, err := concludeNode(ctx, state)
	if err != nil {
		return nil, err
	}

	ruleCheck := state.GetString(StateRuleCheck)
	ruleVerdict := state.GetString(StateRuleVerdict)

	var report strings.Builder
	report.WriteString(base.GetString(StateConclusion))
	report.WriteString("\n\n")
	report.WriteString(ruleCheck)

	// If rules blocked, prepend a prominent warning.
	if ruleVerdict == string(VerdictBlocked) {
		warning := "> ⛔ **规则引擎检查未通过**：分析存在严重缺陷，结论不宜直接采用。\n\n"
		report.Reset()
		report.WriteString(warning)
		report.WriteString(base.GetString(StateConclusion))
		report.WriteString("\n\n")
		report.WriteString(ruleCheck)
	}

	final := report.String()
	return graph.PregelState{
		StateConclusion:  final,
		StateOutput:      final,
		StateRuleCheck:   ruleCheck,
		StateRuleVerdict: ruleVerdict,
	}, nil
}

// BuildNoveltyGraphWithRules constructs a Pregel graph for patent novelty
// analysis with the deterministic rule engine check inserted between analyze
// and conclude (无检索器注入，search 节点返回占位结果）。
//
// Graph structure:
//
//	parse → search → analyze → rule_check → conclude_with_rules → __end__
func BuildNoveltyGraphWithRules() (*graph.CompiledPregelGraph, error) {
	return BuildNoveltyGraphWithRulesWithOpts()
}

// BuildNoveltyGraphWithRulesWithOpts 构造带规则引擎检查的新颖性分析图，
// 支持可选的依赖注入（如检索器）。
//
// Graph structure:
//
//	parse → search → analyze → rule_check → conclude_with_rules → __end__
func BuildNoveltyGraphWithRulesWithOpts(opts ...GraphOption) (*graph.CompiledPregelGraph, error) {
	cfg := &graphConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	g := graph.NewPregelGraph()

	g.AddNode("parse", parseNode)
	g.AddNode("search", newSearchNode(cfg.retriever))
	g.AddNode("analyze", analyzeNode)
	g.AddNode("rule_check", ruleCheckNode)
	g.AddNode("conclude", concludeWithRulesNode)

	g.AddEdge("parse", "search")
	g.AddEdge("search", "analyze")
	g.AddEdge("analyze", "rule_check")
	g.AddEdge("rule_check", "conclude")
	g.AddEdge("conclude", graph.PregelEnd)

	return g.Compile("parse", 10)
}
