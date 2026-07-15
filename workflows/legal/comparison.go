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
	"time"

	"github.com/xujian519/mady/domains/reasoning"
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
	"专利法":     {"专利", "权利要求", "发明", "实用新型", "外观设计", "新颖性", "创造性", "侵权"},
	"民法典":     {"合同", "违约", "侵权责任", "不当得利", "无因管理", "人格权"},
	"商标法":     {"商标", "驰名商标", "商标侵权", "商标异议", "注册商标"},
	"著作权法":    {"著作权", "版权", "作品", "表演", "录音录像", "广播"},
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

	// Extract key terms from facts for case similarity.
	terms := extractKeyTerms(facts)
	query := strings.Join(terms, " ")

	// Placeholder case search results; query is surfaced for future integration
	// with a real case database (e.g., knowledge.Store FTS).
	cases := []string{
		fmt.Sprintf("类似判例检索（查询：%s）", query),
		"类似判例检索结果",
	}

	out := graph.PregelState{
		StateCaseFacts:    facts,
		StateSimilarCases: cases,
	}
	if statutes != nil {
		out[StateStatutes] = statutes
	}

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
			fmt.Fprintf(&comparison, "- %s\n", s)
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
			fmt.Fprintf(&comparison, "- %s\n", c)
		}
	}

	return graph.PregelState{
		StateComparison:   comparison.String(),
		StateCaseFacts:    facts,
		StateStatutes:     statutes,
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
			fmt.Fprintf(&report, "- 根据%s相关规定，建议进一步检索具体法条和司法解释\n", s)
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

// StateReasoningChains holds the auditable syllogism chains (markdown) produced
// by the reasoning-aware comparison graph.
const StateReasoningChains = "reasoning_chains"

// reasoningContext carries a FactBlackboard through the Pregel nodes so that
// compare/conclude can produce auditable syllogism chains rather than opaque
// template strings.
type reasoningContext struct {
	bb *reasoning.FactBlackboard
}

// BuildComparisonGraphWithReasoning builds the comparison graph backed by a
// FactBlackboard. The statute node records case facts and identified laws on
// the blackboard; the compare node builds validated syllogisms
// (大前提:法条 → 小前提:案件事实 → 结论) for every applicable law; the conclude node
// renders the auditable reasoning trace. The returned FactBlackboard can be
// inspected after invocation to audit facts, rule constraints, and chains.
func BuildComparisonGraphWithReasoning(caseID string, caseType reasoning.CaseType) (*graph.CompiledPregelGraph, *reasoning.FactBlackboard, error) {
	bb := reasoning.NewFactBlackboard(caseID, caseType, "")
	rc := &reasoningContext{bb: bb}

	g := graph.NewPregelGraph()
	g.AddNode("statute", rc.statuteNode)
	g.AddNode("case_search", rc.caseSearchNode)
	g.AddNode("compare", rc.compareNode)
	g.AddNode("conclude", rc.concludeNode)
	g.AddEdge("statute", "case_search")
	g.AddEdge("case_search", "compare")
	g.AddEdge("compare", "conclude")
	g.AddEdge("conclude", graph.PregelEnd)

	compiled, err := g.Compile("statute", 10)
	if err != nil {
		return nil, nil, err
	}
	return compiled, bb, nil
}

// filterStatutes drops the placeholder and empty entries.
func filterStatutes(statutes []string) []string {
	var out []string
	for _, s := range statutes {
		if s != "需进一步检索适用法律" && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// detectTechnicalField makes a best-effort guess at the technical/legal domain
// from the case facts, recorded on the blackboard for downstream auditing.
func detectTechnicalField(facts string) string {
	for _, kw := range []string{"专利", "发明", "实用新型", "外观设计"} {
		if strings.Contains(facts, kw) {
			return "专利"
		}
	}
	if strings.Contains(facts, "商标") {
		return "商标"
	}
	for _, kw := range []string{"著作权", "版权"} {
		if strings.Contains(facts, kw) {
			return "著作权"
		}
	}
	if strings.Contains(facts, "商业秘密") {
		return "反不正当竞争"
	}
	return ""
}

func (rc *reasoningContext) statuteNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	facts := state.GetString(StateCaseFacts)
	if facts == "" {
		return nil, fmt.Errorf("legal: case facts are empty")
	}

	rc.bb.AddFact(reasoning.FactEntry{
		ID:          "case_facts",
		Source:      "case_description",
		Content:     facts,
		Confidence:  0.9,
		ExtractedAt: time.Now().UTC().Format(time.RFC3339),
	})
	rc.bb.TechnicalField = detectTechnicalField(facts)

	stats := identifyStatutes(facts)
	for _, law := range stats {
		if law == "需进一步检索适用法律" {
			continue
		}
		rc.bb.AddRuleConstraint(reasoning.RuleConstraint{
			ArticleID:        law,
			ArticleName:      law,
			Requirement:      reasoning.ReqMust,
			Description:      "案件识别出的适用法律",
			ApplicableStages: []string{string(rc.bb.CaseType)},
		})
	}

	return graph.PregelState{
		StateStatutes:  stats,
		StateCaseFacts: facts,
	}, nil
}

func (rc *reasoningContext) caseSearchNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	return caseSearchNode(ctx, state)
}

func (rc *reasoningContext) compareNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	facts := state.GetString(StateCaseFacts)
	statutes, _ := state[StateStatutes].([]string)
	cases, _ := state[StateSimilarCases].([]string)

	var comparison strings.Builder
	comparison.WriteString("## 法律分析\n\n### 适用法律\n\n")
	applicable := filterStatutes(statutes)
	if len(applicable) == 0 {
		comparison.WriteString("需进一步检索确定适用法律。\n")
	} else {
		for _, s := range applicable {
			fmt.Fprintf(&comparison, "- %s\n", s)
		}
	}

	comparison.WriteString("\n### 案件事实摘要\n\n")
	excerpt := facts
	if r := []rune(excerpt); len(r) > 300 {
		excerpt = string(r[:300]) + "..."
	}
	comparison.WriteString(excerpt)
	comparison.WriteString("\n\n### 类似判例参考\n\n")
	for _, c := range cases {
		fmt.Fprintf(&comparison, "- %s\n", c)
	}

	// Build auditable syllogisms (大前提→小前提→结论) for every applicable law.
	var chainsMarkdown strings.Builder
	chainsMarkdown.WriteString("## 三段论推理链（可审计）\n\n")
	for i, law := range applicable {
		s, err := reasoning.NewSyllogismBuilder(fmt.Sprintf("syllogism-%s-%d", law, i+1)).
			Major("适用"+law+"相关规定", law, law+"的相关规定构成判断的法律依据").
			Minor("案件事实", "case_facts", "案件事实符合"+law+"所规制的情形").
			ConclusionText("根据"+law+"的规定，结合案件事实，需进一步比对具体法条要件", 0.7).
			Build(rc.bb)
		if err != nil {
			// If validation fails (e.g. statute not on the blackboard), record
			// the gap instead of aborting the whole analysis.
			fmt.Fprintf(&chainsMarkdown, "- ⚠️ %s：三段论校验未通过 — %v\n", law, err)
			continue
		}
		// Record the validated chain on the blackboard as a ReasoningChain.
		rc.bb.AddReasoningChain(reasoning.ReasoningChain{
			ID:         s.ID,
			FactRef:    s.FactRef,
			LegalBasis: reasoning.LegalBasis{LawArticle: s.ArticleRef},
			Confidence: s.Confidence,
		})
		fmt.Fprintf(&chainsMarkdown, "### %d. %s\n", i+1, law)
		fmt.Fprintf(&chainsMarkdown, "- 大前提（法条）：%s\n", s.MajorPremise.Content)
		fmt.Fprintf(&chainsMarkdown, "- 小前提（案件事实）：%s\n", s.MinorPremise.Content)
		fmt.Fprintf(&chainsMarkdown, "- 结论：%s\n", s.Conclusion)
		fmt.Fprintf(&chainsMarkdown, "- 置信度：%.2f  ✓ 已校验\n\n", s.Confidence)
	}
	chainsStr := chainsMarkdown.String()
	comparison.WriteString("\n\n" + chainsStr)

	return graph.PregelState{
		StateComparison:      comparison.String(),
		StateReasoningChains: chainsStr,
		StateCaseFacts:       facts,
		StateStatutes:        statutes,
		StateSimilarCases:    cases,
	}, nil
}

func (rc *reasoningContext) concludeNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	comparison := state.GetString(StateComparison)
	chains := state.GetString(StateReasoningChains)
	statutes, _ := state[StateStatutes].([]string)

	// Lock the blackboard: reasoning is complete, no further mutation.
	rc.bb.Lock()

	var report strings.Builder
	report.WriteString("# 法律分析报告\n\n")
	report.WriteString(comparison)
	report.WriteString("\n\n## 初步结论\n\n")
	report.WriteString("> ⚠️ 本分析由 AI 辅助生成，不构成正式法律意见。")
	report.WriteString("法律判断和决策应由具备执业资格的律师确认。\n\n")

	applicable := filterStatutes(statutes)
	if len(applicable) > 0 {
		report.WriteString("### 法律适用建议\n\n")
		for _, s := range applicable {
			fmt.Fprintf(&report, "- 根据%s相关规定，建议进一步检索具体法条和司法解释\n", s)
		}
	}

	// Audit summary: how many syllogisms validated on the blackboard.
	validated := rc.bb.ReasoningChains()
	if len(validated) > 0 {
		fmt.Fprintf(&report, "\n### 推理审计\n\n- 已校验三段论链：%d 条\n", len(validated))
		for _, c := range validated {
			fmt.Fprintf(&report, "  - %s（法条：%s，置信度 %.2f）\n", c.ID, c.LegalBasis.LawArticle, c.Confidence)
		}
	}

	if chains != "" {
		report.WriteString("\n" + chains)
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
