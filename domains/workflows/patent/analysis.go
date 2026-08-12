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
	StateRiskReport  = "risk_report"  // risk scan report (Markdown), if scanner injected
)

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

	if err := g.AddNode("parse", parseNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("search", newSearchNode(cfg.retriever)); err != nil {
		return nil, err
	}
	if err := g.AddNode("analyze", analyzeNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("conclude", concludeNode); err != nil {
		return nil, err
	}

	// Linear flow: parse → search → analyze → conclude → end.
	if err := g.AddEdge("parse", "search"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("search", "analyze"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("analyze", "conclude"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("conclude", graph.PregelEnd); err != nil {
		return nil, err
	}

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
// rule engine report and optional risk scan into the final assessment.
func concludeWithRulesNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	base, err := concludeNode(ctx, state)
	if err != nil {
		return nil, err
	}

	ruleCheck := state.GetString(StateRuleCheck)
	ruleVerdict := state.GetString(StateRuleVerdict)
	riskReport := state.GetString(StateRiskReport)

	var report strings.Builder

	// If rules blocked, prepend a prominent warning.
	if ruleVerdict == string(VerdictBlocked) {
		report.WriteString("> ⛔ **规则引擎检查未通过**：分析存在严重缺陷，结论不宜直接采用。\n\n")
	}

	report.WriteString(base.GetString(StateConclusion))
	report.WriteString("\n\n")
	report.WriteString(ruleCheck)

	// Append risk scan report if available.
	if riskReport != "" {
		report.WriteString("\n\n")
		report.WriteString(riskReport)
	}

	final := report.String()
	return graph.PregelState{
		StateConclusion:  final,
		StateOutput:      final,
		StateRuleCheck:   ruleCheck,
		StateRuleVerdict: ruleVerdict,
		StateRiskReport:  riskReport,
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

// newRiskScanNode creates a Pregel node that scans extracted technical features
// for historical invalidation risk. scanner 为 nil 时返回 no-op（跳过风险扫描）。
// scanner 非 nil 时调用 ScanByFeatures，将 Markdown 报告写入 StateRiskReport。
func newRiskScanNode(scanner FeatureRiskScanner) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		features, _ := state[StateFeatures].([]string)

		out := graph.PregelState{}
		if features != nil {
			out[StateFeatures] = features
		}

		if scanner == nil || len(features) == 0 {
			return out, nil
		}

		result, err := scanner.ScanByFeatures(ctx, features)
		if err != nil {
			// 风险扫描失败不阻断管线，仅标记降级。
			graph.MarkDegraded(out, StateRiskReport, "",
				graph.DegradationSearchFailed,
				fmt.Sprintf("风险扫描失败: %v", err))
			return out, nil
		}

		if result != nil && result.Markdown != "" {
			out[StateRiskReport] = result.Markdown
		}
		return out, nil
	}
}

// BuildNoveltyGraphWithRulesWithOpts 构造带规则引擎检查的新颖性分析图，
// 支持可选的依赖注入（如检索器、风险扫描器）。
//
// 无 scanner 时图结构：
//
//	parse → search → analyze → rule_check → conclude_with_rules → __end__
//
// 有 scanner 时图结构（在 rule_check 和 conclude 之间插入 risk_scan）：
//
//	parse → search → analyze → rule_check → risk_scan → conclude_with_rules → __end__
func BuildNoveltyGraphWithRulesWithOpts(opts ...GraphOption) (*graph.CompiledPregelGraph, error) {
	cfg := &graphConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	g := graph.NewPregelGraph()

	if err := g.AddNode("parse", parseNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("search", newSearchNode(cfg.retriever)); err != nil {
		return nil, err
	}
	if err := g.AddNode("analyze", analyzeNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("rule_check", ruleCheckNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("conclude", concludeWithRulesNode); err != nil {
		return nil, err
	}

	// Conditionally add risk_scan node.
	hasScanner := cfg.scanner != nil
	if hasScanner {
		if err := g.AddNode("risk_scan", newRiskScanNode(cfg.scanner)); err != nil {
			return nil, err
		}
	}

	// Build edges — risk_scan is inserted between rule_check and conclude.
	if err := g.AddEdge("parse", "search"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("search", "analyze"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("analyze", "rule_check"); err != nil {
		return nil, err
	}
	if hasScanner {
		if err := g.AddEdge("rule_check", "risk_scan"); err != nil {
			return nil, err
		}
		if err := g.AddEdge("risk_scan", "conclude"); err != nil {
			return nil, err
		}
	} else {
		if err := g.AddEdge("rule_check", "conclude"); err != nil {
			return nil, err
		}
	}
	if err := g.AddEdge("conclude", graph.PregelEnd); err != nil {
		return nil, err
	}

	return g.Compile("parse", 10)
}
