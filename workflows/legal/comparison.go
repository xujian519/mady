// Package legal provides Pregel-based legal analysis workflows.
//
// The case comparison workflow mirrors the legal reasoning process:
//
//	案件事实 → statute → case_search → compare → conclude → ApprovalGate → 输出
//
// Each node reads from and writes to shared PregelState.
package legal

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
)

// State keys for the legal case comparison workflow.
const (
	StateCaseFacts    = "case_facts"    // original case description
	StateStatutes     = "statutes"      // []string: applicable laws found
	StateSimilarCases = "similar_cases" // []string: similar precedent cases
	StateComparison   = "comparison"    // case comparison analysis
	StateConclusion   = "conclusion"    // final assessment
	StateOutput       = "output"        // final output text
)

// Key legal keywords for statute identification.
var legalKeywords = map[string][]string{
	"专利法":    {"专利", "权利要求", "发明", "实用新型", "外观设计", "新颖性", "创造性", "侵权"},
	"民法典":    {"合同", "违约", "侵权责任", "不当得利", "无因管理", "人格权"},
	"商标法":    {"商标", "驰名商标", "商标侵权", "商标异议", "注册商标"},
	"著作权法":  {"著作权", "版权", "作品", "表演", "录音录像", "广播"},
	"反不正当竞争法": {"商业秘密", "不正当竞争", "混淆", "虚假宣传", "商业诋毁"},
}

// statuteNode identifies applicable laws from the case facts.
func statuteNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	facts := state.GetString(StateCaseFacts)
	if facts == "" {
		return nil, fmt.Errorf("legal: case facts are empty")
	}

	stats := identifyStatutes(facts)

	return graph.PregelState{
		StateStatutes:  stats,
		StateCaseFacts: facts,
	}, nil
}

// identifyStatutes matches case description against legal keyword sets.
func identifyStatutes(facts string) []string {
	lower := strings.ToLower(facts)
	var matched []string
	for law, keywords := range legalKeywords {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				matched = append(matched, law)
				break
			}
		}
	}
	if len(matched) == 0 {
		matched = append(matched, "需进一步检索适用法律")
	}
	return matched
}

// caseSearchNode searches for similar precedent cases.
// In production, this would query the knowledge.Store or a case database.
func caseSearchNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	facts := state.GetString(StateCaseFacts)
	statutes := state[StateStatutes]

	// Simulate case search results.
	cases := []string{
		"类似判例检索结果",
	}

	// Extract key terms from facts for case similarity.
	terms := extractKeyTerms(facts)
	query := strings.Join(terms, " ")

	out := graph.PregelState{
		StateCaseFacts:    facts,
		StateSimilarCases: cases,
	}
	if statutes != nil {
		out[StateStatutes] = statutes
	}
	_ = query // used in production for actual search

	return out, nil
}

// extractKeyTerms identifies key legal terms from case facts.
func extractKeyTerms(facts string) []string {
	seen := make(map[string]bool)
	var terms []string
	// Strip common legal punctuation.
	for _, word := range strings.FieldsFunc(facts, func(r rune) bool {
		return r == '，' || r == '。' || r == '；' || r == '、' || r == ' '
	}) {
		word = strings.TrimSpace(word)
		if len(word) >= 4 && !seen[word] {
			seen[word] = true
			terms = append(terms, word)
		}
	}
	return terms
}

// compareNode performs case comparison analysis.
func compareNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	facts := state.GetString(StateCaseFacts)
	statutes, _ := state[StateStatutes].([]string)
	cases, _ := state[StateSimilarCases].([]string)

	var comparison strings.Builder
	comparison.WriteString("## 法律分析\n\n")

	comparison.WriteString("### 适用法律\n\n")
	if len(statutes) > 0 {
		for _, s := range statutes {
			comparison.WriteString(fmt.Sprintf("- %s\n", s))
		}
	} else {
		comparison.WriteString("需进一步检索确定适用法律。\n")
	}

	comparison.WriteString("\n### 案件事实摘要\n\n")
	if len(facts) > 300 {
		facts = facts[:300] + "..."
	}
	comparison.WriteString(facts)
	comparison.WriteString("\n")

	comparison.WriteString("\n### 类似判例参考\n\n")
	if len(cases) > 0 {
		for _, c := range cases {
			comparison.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	return graph.PregelState{
		StateComparison: comparison.String(),
		StateCaseFacts:  facts,
		StateStatutes:   statutes,
		StateSimilarCases: cases,
	}, nil
}

// concludeNode generates the final legal assessment with mandatory disclaimer.
func concludeNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	comparison := state.GetString(StateComparison)

	var report strings.Builder
	report.WriteString("# 法律分析报告\n\n")
	report.WriteString(comparison)
	report.WriteString("\n\n## 初步结论\n\n")
	report.WriteString("> ⚠️ 本分析由 AI 辅助生成，不构成正式法律意见。")
	report.WriteString("法律判断和决策应由具备执业资格的律师确认。\n\n")

	// Assessment based on applicable laws.
	statutes, _ := state[StateStatutes].([]string)
	if len(statutes) > 0 {
		report.WriteString("### 法律适用建议\n\n")
		for _, s := range statutes {
			report.WriteString(fmt.Sprintf("- 根据%s相关规定，建议进一步检索具体法条和司法解释\n", s))
		}
	}

	report.WriteString("\n### 诉讼策略考量\n\n")
	report.WriteString("- 建议收集并保全相关证据材料\n")
	report.WriteString("- 考虑先行协商或调解解决争议\n")
	report.WriteString("- 如进入诉讼程序，应准备充分的法条依据和判例支持\n")

	return graph.PregelState{
		StateConclusion: report.String(),
		StateOutput:     report.String(),
	}, nil
}

// BuildComparisonGraph constructs a Pregel graph for legal case comparison.
//
// Graph structure:
//
//	statute → case_search → compare → conclude → __end__
func BuildComparisonGraph() (*graph.CompiledPregelGraph, error) {
	g := graph.NewPregelGraph()

	g.AddNode("statute", statuteNode)
	g.AddNode("case_search", caseSearchNode)
	g.AddNode("compare", compareNode)
	g.AddNode("conclude", concludeNode)

	g.AddEdge("statute", "case_search")
	g.AddEdge("case_search", "compare")
	g.AddEdge("compare", "conclude")
	g.AddEdge("conclude", graph.PregelEnd)

	return g.Compile("statute", 10)
}
