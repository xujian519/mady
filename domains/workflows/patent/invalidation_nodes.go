package patent

import (
	"context"
	"fmt"
	"strings"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/domains/evidence"
	"github.com/xujian519/mady/domains/ipc"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/retrieval/domain"
)

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

// newGatherEvidenceNodeWithRetriever creates an evidence-gathering node backed
// by a domain retriever. retriever 为 nil 时标记 DegradationRetrieverNil；
// 非 nil 时执行真实证据检索，检索失败标记 DegradationSearchFailed。
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
// validates the analysis for completeness. IPC classification is automatically
// performed to inject domain-specific analysis hints.
func analyzeGroundsNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	grounds, _ := state[InvStateGrounds].([]InvGround)
	evidence, _ := state[InvStateEvidence].([]string)
	claims, _ := state[InvStateClaimTree].([]InvClaimNode)

	// IPC 分类识别——从专利文本中自动判定技术领域
	inputText := state.GetString(InvStateInput)
	ipcSection, ipcConfidence := ipc.Classify(inputText)

	var analysis strings.Builder
	analysis.WriteString("## 无效理由逐项分析\n\n")

	// IPC 技术领域信息
	analysis.WriteString("### 技术领域识别\n\n")
	fmt.Fprintf(&analysis, "- IPC 大类：%s（%s）\n", ipcSection, ipcSection.SectionOf())
	fmt.Fprintf(&analysis, "- 分类置信度：%.0f%%\n", ipcConfidence*100)
	if ipc.IsHighConfidence(ipcConfidence) {
		analysis.WriteString("- 分类结果可信，以下将应用领域特化分析规则。\n")
	} else {
		analysis.WriteString("- 分类置信度较低，以下以通用规则为主，领域特化规则仅供参考。\n")
	}
	analysis.WriteString("\n")

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
		writeGroundAnalysis(&analysis, g, claims, ipcSection)
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
// The ipcSection parameter enables domain-specific analysis hints.
func writeGroundAnalysis(b *strings.Builder, g InvGround, claims []InvClaimNode, ipcSection ipc.IPCSection) {
	switch g.Type {
	case GroundNovelty:
		b.WriteString("**法律依据**：专利法第22条第2款——新颖性\n\n")
		b.WriteString("**分析要点**：\n")
		b.WriteString("- 采用**单独对比原则**，将每项权利要求与**一份**对比文件进行比对\n")
		b.WriteString("- 论证对比文件是否公开了权利要求的**全部**技术特征\n")
		b.WriteString("- 若任一技术特征未被对比文件公开，则该权利要求具备新颖性\n\n")
		b.WriteString("> ⚠️ 注意：不得将多份对比文件结合后进行新颖性判断\n\n")
		// Append IPC-specific novelty hints.
		if hint := ipc.GetNoveltyHints(ipcSection); hint != "" {
			b.WriteString(hint)
			b.WriteString("\n")
		}

	case GroundInventiveness:
		b.WriteString("**法律依据**：专利法第22条第3款——创造性\n\n")
		b.WriteString("**三步法分析框架**：\n")
		b.WriteString("1. 确定最接近的现有技术\n")
		b.WriteString("2. 确定区别技术特征及实际解决的技术问题\n")
		b.WriteString("3. 判断要求保护的发明对本领域技术人员是否显而易见\n\n")
		b.WriteString("> ⚠️ 多篇对比文件组合时，须论证**组合动机/技术启示**\n\n")
		// Append IPC-specific inventiveness hints.
		if hint := ipc.GetInventivenessHints(ipcSection); hint != "" {
			b.WriteString(hint)
			b.WriteString("\n")
		}

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
	report.WriteString(FormatStrategySection(grounds))

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
// Evidence Integration Nodes
// =============================================================================

// judgeEvidenceNode performs triple-attribute review on gathered evidence.
func judgeEvidenceNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	out := copyInvBaseState(state)

	evidenceRaw, ok := state[InvStateEvidence]
	if !ok {
		graph.MarkDegraded(out, InvStateEvidenceJudgments, []evidence.EvidenceJudgment{},
			graph.DegradationNotImplemented, "证据检索未完成，跳过证据判断")
		return out, nil
	}

	spans, ok := evidenceRaw.([]agentcore_evidence.EvidenceSpan)
	if !ok || len(spans) == 0 {
		out[InvStateEvidenceJudgments] = []evidence.EvidenceJudgment{}
		return out, nil
	}

	engine := evidence.NewEngine(nil)
	judgments := make([]evidence.EvidenceJudgment, 0, len(spans))
	for _, span := range spans {
		j, err := engine.Judge(span)
		if err != nil {
			// 单条证据判断失败即跳过，不中断整批评审（fail-safe）。
			continue
		}
		judgments = append(judgments, *j)
	}
	out[InvStateEvidenceJudgments] = judgments
	return out, nil
}

// filterEvidenceNode filters evidence with overall_score < 0.5.
func filterEvidenceNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	out := copyInvBaseState(state)

	judgmentsRaw, ok := state[InvStateEvidenceJudgments]
	if !ok {
		graph.MarkDegraded(out, InvStateValidEvidence, []agentcore_evidence.EvidenceSpan{},
			graph.DegradationNotImplemented, "证据判断尚未完成，跳过证据过滤")
		return out, nil
	}

	judgments, ok := judgmentsRaw.([]evidence.EvidenceJudgment)
	if !ok {
		out[InvStateValidEvidence] = []agentcore_evidence.EvidenceSpan{}
		return out, nil
	}

	evidenceRaw := state[InvStateEvidence]
	spans, _ := evidenceRaw.([]agentcore_evidence.EvidenceSpan)

	judgmentBySpanID := make(map[string]evidence.EvidenceJudgment)
	for _, j := range judgments {
		judgmentBySpanID[j.SpanID] = j
	}

	var valid []agentcore_evidence.EvidenceSpan
	for _, span := range spans {
		if j, ok := judgmentBySpanID[span.ID]; ok && j.OverallScore >= 0.5 {
			valid = append(valid, span)
		}
	}
	out[InvStateValidEvidence] = valid
	return out, nil
}

// detectConflictNode detects conflicts among valid evidence.
func detectConflictNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	out := copyInvBaseState(state)

	validRaw, ok := state[InvStateValidEvidence]
	if !ok {
		out[InvStateConflicts] = []agentcore_evidence.Conflict{}
		return out, nil
	}

	spans, ok := validRaw.([]agentcore_evidence.EvidenceSpan)
	if !ok || len(spans) == 0 {
		out[InvStateConflicts] = []agentcore_evidence.Conflict{}
		return out, nil
	}

	cb := agentcore_evidence.NewClaimBinding()
	for _, span := range spans {
		cb.RegisterSpan(span)
	}

	detector := agentcore_evidence.NewConflictDetector(cb)
	conflicts := detector.Detect()
	out[InvStateConflicts] = conflicts
	return out, nil
}

// copyInvBaseState copies invalidation base state fields forward.
func copyInvBaseState(state graph.PregelState) graph.PregelState {
	out := graph.PregelState{}
	for _, key := range []string{InvStateInput, InvStateClaims, InvStateGrounds, InvStateClaimTree,
		InvStateEvidence, InvStateEvidenceJudgments, InvStateValidEvidence, InvStateConflicts} {
		if v, ok := state[key]; ok {
			out[key] = v
		}
	}
	return out
}
