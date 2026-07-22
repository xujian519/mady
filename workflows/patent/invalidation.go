// Package patent provides a Pregel-based patent invalidation analysis workflow.
//
// The invalidation workflow analyzes a target patent against prior-art grounds
// under Chinese patent law (A22.2 novelty / A22.3 inventiveness / A26.3 disclosure
// sufficiency / A26.4 claim clarity & support / A33 amendment scope).
//
// Graph structure (without retriever):
//
//	parse_patent → identify_grounds → analyze_grounds → conclude → __end__
//
// Graph structure (with retriever):
//
//	parse_patent → identify_grounds → gather_evidence → analyze_grounds → conclude → __end__
//
// Each node is deterministic (no LLM calls) — the graph produces a structured
// invalidation analysis skeleton that a patent attorney reviews and finalizes.
// The deterministic rule engine validates the analysis for legal completeness.
package patent

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/retrieval/domain"
)

// State keys used by the invalidation workflow.
const (
	InvStateInput       = "inv_input"        // original input (claims + requester grounds)
	InvStateClaims      = "inv_claims"       // parsed patent claims text
	InvStateClaimTree   = "inv_claim_tree"   // []InvClaimNode: parsed claim structure
	InvStateGrounds     = "inv_grounds"      // []InvGround: identified invalidation grounds
	InvStateEvidence    = "inv_evidence"     // []string: retrieved evidence references
	InvStateAnalysis    = "inv_analysis"     // per-ground analysis text
	InvStateRuleCheck   = "inv_rule_check"   // rule engine check report
	InvStateRuleVerdict = "inv_rule_verdict" // aggregate verdict
	InvStateConclusion  = "inv_conclusion"   // final conclusion
	InvStateOutput      = "inv_output"       // final output text
)

// InvalidationGroundType identifies the legal basis for invalidation.
type InvalidationGroundType string

const (
	GroundNovelty       InvalidationGroundType = "A22.2_novelty"
	GroundInventiveness InvalidationGroundType = "A22.3_inventiveness"
	GroundDisclosure    InvalidationGroundType = "A26.3_disclosure"
	GroundClaimClarity  InvalidationGroundType = "A26.4_clarity"
	GroundAmendment     InvalidationGroundType = "A33_amendment"
)

// InvClaimNode represents a single parsed claim from the target patent.
type InvClaimNode struct {
	Number        int    // claim number (1, 2, 3...)
	IsIndependent bool   // true if independent claim
	Type          string // "method", "apparatus", "system", "compound", etc.
	Text          string // raw claim text
}

// InvGround represents one identified invalidation ground.
type InvGround struct {
	Type        InvalidationGroundType
	Article     string // legal article reference
	Description string // human-readable description
	ClaimRefs   []int  // affected claim numbers
}

// =============================================================================
// Pregel Nodes
// =============================================================================

// parsePatentNode parses the target patent's claims and the requester's
// invalidation grounds from the input text. It extracts the claim structure
// (independent vs. dependent, claim numbers) and identifies which invalidation
// grounds are being asserted.
func parsePatentNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	input := state.GetString(InvStateInput)
	if input == "" {
		return nil, fmt.Errorf("invalidation: input is empty")
	}

	claims := extractClaimsFromText(input)
	grounds := identifyInvalidationGrounds(input)

	return graph.PregelState{
		InvStateInput:     input,
		InvStateClaims:    input,
		InvStateClaimTree: claims,
		InvStateGrounds:   grounds,
	}, nil
}

// extractClaimsFromText parses claim text, identifying individual claims and
// whether they are independent or dependent.
func extractClaimsFromText(text string) []InvClaimNode {
	var claims []InvClaimNode

	// Split by standard claim numbering patterns: "1.", "2.", "权利要求1"
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 10 {
			continue
		}

		// Match "1." or "权利要求1" at the start.
		claimNum := 0
		if strings.HasPrefix(line, "权利要求") {
			// Extract number after "权利要求"
			rest := line[12:] // len("权利要求") in bytes (3 chars × 3 bytes + possible space)
			rest = strings.TrimLeft(rest, " ：:、")
			if _, err := fmt.Sscanf(rest, "%d", &claimNum); err != nil {
				continue
			}
		} else if len(line) > 2 && line[0] >= '1' && line[0] <= '9' && (line[1] == '.' || line[1] == ' ' || strings.HasPrefix(line[1:], "、")) {
			if _, err := fmt.Sscanf(line, "%d", &claimNum); err != nil {
				continue
			}
		}

		if claimNum > 0 {
			isIndependent := !strings.HasPrefix(line, "权利要求"+fmt.Sprintf("%d", claimNum)+"引用") &&
				!strings.Contains(line[:min(len(line), 30)], "根据") &&
				!strings.Contains(line[:min(len(line), 30)], "如权利要求")
			claims = append(claims, InvClaimNode{
				Number:        claimNum,
				IsIndependent: isIndependent,
				Text:          line,
			})
		}
	}

	// If no structured claims found, treat entire input as claim 1.
	if len(claims) == 0 {
		claims = append(claims, InvClaimNode{
			Number:        1,
			IsIndependent: true,
			Text:          truncate(text, 500),
		})
	}
	return claims
}

// invalidationGroundRules is the pattern table for invalidation ground
// identification. Order matters: earlier entries take priority on overlap.
var invalidationGroundRules = []groundPattern{
	{TypeKey: string(GroundNovelty), Article: "专利法第22条第2款",
		Desc:     "新颖性无效（不具备新颖性）",
		Patterns: []string{"22条第2款", "22.2", "新颖性", "不具备新颖"}},
	{TypeKey: string(GroundInventiveness), Article: "专利法第22条第3款",
		Desc:     "创造性无效（不具备创造性）",
		Patterns: []string{"22条第3款", "22.3", "创造性", "不具备创造"}},
	{TypeKey: string(GroundDisclosure), Article: "专利法第26条第3款",
		Desc:     "公开不充分无效",
		Patterns: []string{"26条第3款", "26.3", "公开充分", "充分公开", "能够实现"}},
	{TypeKey: string(GroundClaimClarity), Article: "专利法第26条第4款",
		Desc:     "权利要求不清楚/得不到支持无效",
		Patterns: []string{"26条第4款", "26.4", "清楚", "支持"}},
	{TypeKey: string(GroundAmendment), Article: "专利法第33条",
		Desc:     "修改超范围无效",
		Patterns: []string{"第33条", "A33", "修改超范围", "超出原"}},
}

// identifyInvalidationGrounds scans the input text for invalidation ground
// references (e.g. "第22条第2款", "A22.2", "新颖性", "创造性", "公开充分").
func identifyInvalidationGrounds(text string) []InvGround {
	matched := scanGrounds(text, invalidationGroundRules)
	var grounds []InvGround
	for _, r := range matched {
		grounds = append(grounds, InvGround{
			Type:        InvalidationGroundType(r.TypeKey),
			Article:     r.Article,
			Description: r.Desc,
		})
	}

	// If no specific grounds found, default to comprehensive analysis.
	if len(grounds) == 0 {
		grounds = append(grounds, InvGround{
			Type:        GroundNovelty,
			Article:     "专利法第22条第2款",
			Description: "新颖性无效（默认分析维度）",
		})
	}

	return grounds
}

// identifyGroundsNode refines the grounds identified during parsing, adding
// affected claim numbers based on the claim tree.
func identifyGroundsNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	grounds, _ := state[InvStateGrounds].([]InvGround)
	claims, _ := state[InvStateClaimTree].([]InvClaimNode)

	// Assign all independent claims to each ground by default.
	var indepClaims []int
	for _, c := range claims {
		if c.IsIndependent {
			indepClaims = append(indepClaims, c.Number)
		}
	}
	if len(indepClaims) == 0 {
		indepClaims = []int{1}
	}

	// Update grounds with claim references.
	for i := range grounds {
		if len(grounds[i].ClaimRefs) == 0 {
			grounds[i].ClaimRefs = indepClaims
		}
	}

	return graph.PregelState{
		InvStateInput:     state.GetString(InvStateInput),
		InvStateClaims:    state.GetString(InvStateClaims),
		InvStateClaimTree: claims,
		InvStateGrounds:   grounds,
	}, nil
}

// gatherEvidenceNode retrieves prior-art evidence for the invalidation grounds.
// With a retriever injected, it performs real prior-art search.
// Without a retriever, it marks degradation and returns empty evidence.
func gatherEvidenceNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	claims := state.GetString(InvStateClaims)
	grounds, _ := state[InvStateGrounds].([]InvGround)

	// Build search query from claims text.
	query := truncate(claims, 100)

	out := graph.PregelState{
		InvStateInput:   state.GetString(InvStateInput),
		InvStateClaims:  claims,
		InvStateGrounds: grounds,
	}
	// Also carry forward claim tree.
	if ct, ok := state[InvStateClaimTree].([]InvClaimNode); ok {
		out[InvStateClaimTree] = ct
	}

	// Mark degraded — evidence retrieval requires external data source.
	graph.MarkDegraded(out, InvStateEvidence, []string{},
		graph.DegradationNotImplemented,
		fmt.Sprintf("证据检索尚未接入真实数据库（查询：%s）。检索结果将在功能就绪后自动补充。", query))

	return out, nil
}

// newGatherEvidenceNodeWithRetriever creates an evidence-gathering node backed
// by a real domain retriever.
func newGatherEvidenceNodeWithRetriever(retriever domain.DomainRetriever) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		claims := state.GetString(InvStateClaims)
		grounds, _ := state[InvStateGrounds].([]InvGround)

		out := graph.PregelState{
			InvStateInput:   state.GetString(InvStateInput),
			InvStateClaims:  claims,
			InvStateGrounds: grounds,
		}
		if ct, ok := state[InvStateClaimTree].([]InvClaimNode); ok {
			out[InvStateClaimTree] = ct
		}

		if retriever == nil {
			graph.MarkDegraded(out, InvStateEvidence, []string{},
				graph.DegradationRetrieverNil,
				"未配置检索器，无法进行证据检索。")
			return out, nil
		}

		results, err := retriever.Search(ctx, domain.DomainQuery{
			Text:       truncate(claims, 200),
			MaxResults: 10,
		})
		if err != nil {
			graph.MarkDegraded(out, InvStateEvidence, []string{},
				graph.DegradationSearchFailed,
				fmt.Sprintf("证据检索失败: %v", err))
			return out, nil
		}

		var evidence []string
		for _, doc := range results.Documents {
			entry := fmt.Sprintf("[%s] %s", doc.ID, doc.Title)
			if doc.Snippet != "" {
				entry += ": " + doc.Snippet
			}
			evidence = append(evidence, entry)
		}
		if len(evidence) == 0 {
			evidence = append(evidence, "未检索到相关证据文献")
		}
		out[InvStateEvidence] = evidence
		return out, nil
	}
}

// analyzeGroundsNode performs per-ground invalidation analysis. Each ground is
// analyzed independently (per Chinese patent law requirement). The rule engine
// validates the analysis for completeness.
func analyzeGroundsNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	grounds, _ := state[InvStateGrounds].([]InvGround)
	evidence, _ := state[InvStateEvidence].([]string)
	claims, _ := state[InvStateClaimTree].([]InvClaimNode)

	var analysis strings.Builder
	analysis.WriteString("## 无效理由逐项分析\n\n")

	// List identified grounds.
	analysis.WriteString("### 无效理由概述\n\n")
	for i, g := range grounds {
		fmt.Fprintf(&analysis, "%d. **%s**（%s）\n", i+1, g.Description, g.Article)
		if len(g.ClaimRefs) > 0 {
			fmt.Fprintf(&analysis, "   - 涉及权利要求：%v\n", g.ClaimRefs)
		}
	}
	analysis.WriteString("\n")

	// Evidence summary.
	analysis.WriteString("### 证据材料\n\n")
	if mark := graph.GetDegradationMark(state, InvStateEvidence); mark != nil {
		fmt.Fprintf(&analysis, "> ⚠️ 证据检索降级：%s\n\n", mark.Message)
	} else if len(evidence) > 0 {
		for _, e := range evidence {
			fmt.Fprintf(&analysis, "- %s\n", e)
		}
	} else {
		analysis.WriteString("未提供对比文件证据。\n")
	}
	analysis.WriteString("\n")

	// Per-ground analysis — each must be independent.
	for _, g := range grounds {
		fmt.Fprintf(&analysis, "### %s\n\n", g.Description)
		writeGroundAnalysis(&analysis, g, claims)
	}

	// Run rule engine check.
	var checkText strings.Builder
	checkText.WriteString(analysis.String())
	for _, g := range grounds {
		checkText.WriteString("\n")
		checkText.WriteString(g.Description)
		checkText.WriteString(" ")
		checkText.WriteString(g.Article)
	}

	engine := NewRuleEngine()
	engine.RegisterRules(InvalidationRules())
	results := engine.Evaluate(engine.Rules(), checkText.String(), "patent_invalidation")
	verdict := Aggregate(results)

	ruleReport := FormatRuleResults(results, verdict)

	return graph.PregelState{
		InvStateAnalysis:    analysis.String(),
		InvStateRuleCheck:   ruleReport,
		InvStateRuleVerdict: string(verdict),
		InvStateInput:       state.GetString(InvStateInput),
		InvStateClaims:      state.GetString(InvStateClaims),
		InvStateGrounds:     grounds,
		InvStateEvidence:    evidence,
		InvStateClaimTree:   claims,
	}, nil
}

// writeGroundAnalysis writes the analysis section for a single invalidation ground.
func writeGroundAnalysis(b *strings.Builder, g InvGround, claims []InvClaimNode) {
	switch g.Type {
	case GroundNovelty:
		b.WriteString("**法律依据**：专利法第22条第2款——新颖性\n\n")
		b.WriteString("**分析要点**：\n")
		b.WriteString("- 采用**单独对比原则**，将每项权利要求与**一份**对比文件进行比对\n")
		b.WriteString("- 论证对比文件是否公开了权利要求的**全部**技术特征\n")
		b.WriteString("- 若任一技术特征未被对比文件公开，则该权利要求具备新颖性\n\n")
		b.WriteString("> ⚠️ 注意：不得将多份对比文件结合后进行新颖性判断\n\n")

	case GroundInventiveness:
		b.WriteString("**法律依据**：专利法第22条第3款——创造性\n\n")
		b.WriteString("**三步法分析框架**：\n")
		b.WriteString("1. 确定最接近的现有技术\n")
		b.WriteString("2. 确定区别技术特征及实际解决的技术问题\n")
		b.WriteString("3. 判断要求保护的发明对本领域技术人员是否显而易见\n\n")
		b.WriteString("> ⚠️ 多篇对比文件组合时，须论证**组合动机/技术启示**\n\n")

	case GroundDisclosure:
		b.WriteString("**法律依据**：专利法第26条第3款——充分公开\n\n")
		b.WriteString("**分析要点**：\n")
		b.WriteString("- 说明书记载的技术方案是否能使本领域技术人员**能够实现**\n")
		b.WriteString("- 技术问题、技术方案和有益效果是否充分公开\n")
		b.WriteString("- 是否存在仅公开功能/效果而缺少具体技术手段的情形\n\n")

	case GroundClaimClarity:
		b.WriteString("**法律依据**：专利法第26条第4款——权利要求清楚与支持\n\n")
		b.WriteString("**分析要点**：\n")
		b.WriteString("- 权利要求是否清楚、简明（术语含义是否明确）\n")
		b.WriteString("- 权利要求是否得到说明书支持（概括范围是否合理）\n\n")

	case GroundAmendment:
		b.WriteString("**法律依据**：专利法第33条——修改不超范围\n\n")
		b.WriteString("**分析要点**：\n")
		b.WriteString("- 修改后的内容是否能够从原说明书和权利要求书记载的范围中**直接且毫无疑义地**确定\n")
		b.WriteString("- 是否新增了原申请文件未记载的技术内容\n\n")
	}

	// List affected claims.
	if len(g.ClaimRefs) > 0 {
		b.WriteString("**涉及权利要求**：\n")
		for _, num := range g.ClaimRefs {
			claimText := ""
			for _, c := range claims {
				if c.Number == num {
					claimText = truncate(c.Text, 100)
					break
				}
			}
			fmt.Fprintf(b, "- 权利要求%d：%s\n", num, claimText)
		}
		b.WriteString("\n")
	}
}

// invConcludeNode generates the final invalidation analysis report.
func invConcludeNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	analysis := state.GetString(InvStateAnalysis)
	ruleCheck := state.GetString(InvStateRuleCheck)
	ruleVerdict := state.GetString(InvStateRuleVerdict)
	grounds, _ := state[InvStateGrounds].([]InvGround)

	var report strings.Builder

	// If rules blocked, prepend warning.
	if ruleVerdict == string(VerdictBlocked) {
		report.WriteString("> ⛔ **规则引擎检查未通过**：无效宣告分析存在严重缺陷，结论不宜直接采用。\n\n")
	}

	report.WriteString("# 专利无效宣告分析报告\n\n")
	report.WriteString("## 分析范围\n\n")
	fmt.Fprintf(&report, "本报告分析了 %d 项无效理由。\n\n", len(grounds))
	report.WriteString(analysis)
	report.WriteString("\n\n")
	report.WriteString(ruleCheck)

	// Conclusion.
	report.WriteString("\n## 审查结论\n\n")
	report.WriteString("基于上述逐项分析：\n")
	report.WriteString("- 各无效理由应**独立**评估，不得以「综合来看」代替逐条分析\n")
	report.WriteString("- 多篇组合时须论证**组合动机**\n")
	report.WriteString("- 对比文件公开日须**早于**涉案专利的优先权日\n\n")

	// Disclaimer.
	report.WriteString("---\n\n")
	report.WriteString("> ⚠️ **人工审核提醒**\n")
	report.WriteString("> \n")
	report.WriteString("> 本分析由 AI 辅助生成骨架，以下内容必须由专利代理师/律师逐项核实后定稿：\n")
	report.WriteString("> 1. 每项无效理由的独立论证是否完整\n")
	report.WriteString("> 2. 对比文件的公开日是否已核实\n")
	report.WriteString("> 3. 多篇组合的组合动机论证是否充分\n")
	report.WriteString("> 4. 法律依据的引用是否正确\n")
	report.WriteString("> \n")
	report.WriteString("> 本分析不构成正式法律意见。\n")

	final := report.String()
	return graph.PregelState{
		InvStateConclusion:  final,
		InvStateOutput:      final,
		InvStateAnalysis:    analysis,
		InvStateRuleCheck:   ruleCheck,
		InvStateRuleVerdict: ruleVerdict,
	}, nil
}

// =============================================================================
// Graph Builder
// =============================================================================

// InvGraphOption optionally configures the invalidation graph's dependencies.
type InvGraphOption func(*invGraphConfig)

type invGraphConfig struct {
	retriever domain.DomainRetriever
}

// WithInvRetriever injects a domain retriever, enabling real evidence retrieval.
// When not injected, evidence gathering returns degraded results.
func WithInvRetriever(r domain.DomainRetriever) InvGraphOption {
	return func(c *invGraphConfig) { c.retriever = r }
}

// BuildInvalidationGraph constructs a Pregel graph for patent invalidation analysis
// (no retriever injected, evidence gathering returns degraded results).
//
// Graph structure:
//
//	parse_patent → identify_grounds → analyze_grounds → conclude → __end__
func BuildInvalidationGraph() (*graph.CompiledPregelGraph, error) {
	return BuildInvalidationGraphWithOpts()
}

// BuildInvalidationGraphWithOpts constructs the invalidation analysis Pregel graph
// with optional dependency injection.
//
// Without retriever:
//
//	parse_patent → identify_grounds → analyze_grounds → conclude → __end__
//
// With retriever (evidence gathering node inserted):
//
//	parse_patent → identify_grounds → gather_evidence → analyze_grounds → conclude → __end__
func BuildInvalidationGraphWithOpts(opts ...InvGraphOption) (*graph.CompiledPregelGraph, error) {
	cfg := &invGraphConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	g := graph.NewPregelGraph()

	if err := g.AddNode("parse_patent", parsePatentNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("identify_grounds", identifyGroundsNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("analyze_grounds", analyzeGroundsNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("conclude", invConcludeNode); err != nil {
		return nil, err
	}

	// Conditionally insert evidence gathering node.
	hasRetriever := cfg.retriever != nil
	if hasRetriever {
		if err := g.AddNode("gather_evidence", newGatherEvidenceNodeWithRetriever(cfg.retriever)); err != nil {
			return nil, err
		}
	}

	// Build edges.
	edges := [][2]string{
		{"parse_patent", "identify_grounds"},
	}
	if hasRetriever {
		edges = append(edges, [][2]string{
			{"identify_grounds", "gather_evidence"},
			{"gather_evidence", "analyze_grounds"},
		}...)
	} else {
		edges = append(edges, [2]string{"identify_grounds", "analyze_grounds"})
	}
	edges = append(edges, [][2]string{
		{"analyze_grounds", "conclude"},
		{"conclude", graph.PregelEnd},
	}...)

	for _, edge := range edges {
		if err := g.AddEdge(edge[0], edge[1]); err != nil {
			return nil, err
		}
	}

	return g.Compile("parse_patent", 15)
}
