// Package patent provides Pregel-based OA (Office Action) response workflow.
//
// The OA response workflow automates the patent agent's highest-frequency task:
// analyzing a Chinese patent office action notification, classifying rejection
// grounds, analyzing affected claims, and drafting a structured response.
//
// Graph structure:
//
//	parse_oa → classify_rejection → analyze_claims → draft_response → approval_gate → __end__
//
// Each node is deterministic (no LLM calls) — the graph produces a structured
// response skeleton that the patent agent reviews and finalizes in the TUI.
package patent

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
)

// State keys used by the OA response workflow.
const (
	OAStateInput            = "oa_input"             // original OA notification text
	OAStateParsed           = "oa_parsed"            // *ParsedOfficeAction
	OAStateRejectionType    = "oa_rejection_type"    // string: OaRejectionType value
	OAStateCitations        = "oa_citations"         // []CitedReference
	OAStateAffectedClaims   = "oa_affected_claims"   // []int
	OAStateResponseStrategy = "oa_response_strategy" // string: argument / amendment / combined
	OAStateClaimAmendments  = "oa_claim_amendments"  // string: claim amendment markup
	OAStateResponseDraft    = "oa_response_draft"    // string: final response draft
	OAStateTemplateUsed     = "oa_template_used"     // string: which doc template was used
	OAStateOutput           = "oa_output"            // string: final output text
)

// =============================================================================
// Pregel Nodes
// =============================================================================

// parseOANode parses the OA notification text using the deterministic rules.OAParser.
// It extracts rejection type, cited references, and affected claim numbers.
func parseOANode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	input := state.GetString(OAStateInput)
	if input == "" {
		return nil, fmt.Errorf("oa_response: OA notification text is empty")
	}

	parsed := ParseOA(input)

	// Also extract examiner arguments by splitting on common sentence patterns.
	examinerArgs := extractExaminerArguments(input)
	parsed.ExaminerArguments = examinerArgs

	return graph.PregelState{
		OAStateInput:          input,
		OAStateParsed:         &parsed,
		OAStateRejectionType:  parsed.RejectionType,
		OAStateCitations:      parsed.Citations,
		OAStateAffectedClaims: parsed.AffectedClaims,
	}, nil
}

// extractExaminerArguments extracts the examiner's reasoning sentences from
// the OA text by splitting on common argument markers.
func extractExaminerArguments(text string) []string {
	markers := []string{"审查员认为", "对比文件", "本领域技术人员", "因此", "所以", "综上"}
	var args []string
	lower := strings.ToLower(text)

	for _, marker := range markers {
		idx := strings.Index(lower, strings.ToLower(marker))
		if idx >= 0 {
			end := min(idx+200, len(text))
			snippet := strings.TrimSpace(text[idx:end])
			// Cut at sentence boundary
			for _, delim := range []string{"。", "；"} {
				if i := strings.Index(snippet, delim); i > 0 {
					snippet = snippet[:i+len(delim)]
					break
				}
			}
			if len(snippet) > 10 {
				args = append(args, snippet)
			}
		}
	}
	args = args[:min(len(args), 5)]
	return args
}

// classifyRejectionNode determines the response strategy based on rejection type.
//
// Strategy selection:
//   - novelty (A22.2) → argument strategy (争辩)
//   - inventiveness (A22.3) → argument strategy with three-step method (三步法)
//   - clarity/support (A26.4) → amendment strategy (修改)
//   - disclosure (A26.3) → argument + evidence strategy
//   - scope (A33) → amendment strategy (删除/限缩)
//   - formal → simple amendment
//   - multiple types → combined strategy (争辩+修改)
func classifyRejectionNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	rejectionType := state.GetString(OAStateRejectionType)
	parsed, ok := state[OAStateParsed].(*ParsedOfficeAction)
	if !ok {
		return nil, fmt.Errorf("oa_response: invalid or missing parsed OA state")
	}

	strategy := determineResponseStrategy(rejectionType, parsed)
	templateName := selectOATemplate(rejectionType, strategy)

	return graph.PregelState{
		OAStateRejectionType:    rejectionType,
		OAStateParsed:           parsed,
		OAStateCitations:        state[OAStateCitations],
		OAStateAffectedClaims:   state[OAStateAffectedClaims],
		OAStateInput:            state[OAStateInput],
		OAStateResponseStrategy: strategy,
		OAStateTemplateUsed:     templateName,
	}, nil
}

// determineResponseStrategy picks the response strategy based on rejection type.
func determineResponseStrategy(rejectionType string, parsed *ParsedOfficeAction) string {
	switch OaRejectionType(rejectionType) {
	case OaNovelty, OaInventiveness:
		return "argument" // 主要通过争辩
	case OaClarity, OaSupport, OaScope:
		return "amendment" // 主要通过修改权利要求
	case OaDisclosure:
		return "argument" // 需要论述公开充分
	case OaFormal:
		return "amendment" // 形式修改
	default:
		return "combined" // 争辩+修改组合
	}
}

// selectOATemplate maps rejection type to the appropriate doc template name.
func selectOATemplate(rejectionType string, strategy string) string {
	switch OaRejectionType(rejectionType) {
	case OaNovelty:
		return "novelty-defense"
	case OaInventiveness:
		return "inventiveness-defense"
	case OaClarity, OaSupport:
		return "clarity-amendment"
	default:
		if strategy == "argument" {
			return "novelty-defense"
		}
		return "clarity-amendment"
	}
}

// analyzeClaimsNode performs claim-level analysis and generates amendment suggestions.
// It identifies which claims need modification and drafts amendment markup.
func analyzeClaimsNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	parsed, ok := state[OAStateParsed].(*ParsedOfficeAction)
	if !ok || parsed == nil {
		return nil, fmt.Errorf("oa_response: invalid or missing parsed OA state")
	}
	strategy := state.GetString(OAStateResponseStrategy)
	rejectionType := state.GetString(OAStateRejectionType)

	var amendments strings.Builder
	amendments.WriteString("## 权利要求修改对照表\n\n")
	amendments.WriteString("| 权利要求 | 修改类型 | 修改前 | 修改后 | 修改依据 |\n")
	amendments.WriteString("|-----------|----------|--------|--------|----------|\n")

	if strategy == "amendment" || strategy == "combined" {
		for i, claimNum := range parsed.AffectedClaims {
			if i >= 5 {
				break // limit to 5 claims in the table
			}
			amendType := claimAmendmentType(OaRejectionType(rejectionType), claimNum)
			fmt.Fprintf(&amendments, "| 权利要求 %d | %s | [原内容] | [建议修改] | %s |\n",
				claimNum, amendType, amendmentBasis(OaRejectionType(rejectionType)))
		}
	} else {
		amendments.WriteString("| — | 无需修改 | — | — | 基于以下争辩理由，本权利要求无需修改 |\n")
	}

	amendments.WriteString("\n")

	// Add strategy-specific guidance.
	amendments.WriteString("## 答复策略建议\n\n")
	switch OaRejectionType(rejectionType) {
	case OaNovelty:
		amendments.WriteString("- **策略**：单独对比原则争辩\n")
		amendments.WriteString("- **要点**：论证对比文件未公开至少一项技术特征\n")
		amendments.WriteString("- **风险**：低（新颖性争辩成功率相对较高）\n")
	case OaInventiveness:
		amendments.WriteString("- **策略**：三步法争辩\n")
		amendments.WriteString("- **要点**：确定区别特征 → 确定实际解决的技术问题 → 论证非显而易见\n")
		amendments.WriteString("- **关键**：重点论述'不存在技术启示'\n")
	case OaClarity:
		amendments.WriteString("- **策略**：澄清修改\n")
		amendments.WriteString("- **要点**：明确限定用语含义、删除模糊表述、补充连接关系\n")
	case OaSupport:
		amendments.WriteString("- **策略**：修改权利要求使其得到说明书支持\n")
		amendments.WriteString("- **要点**：将上位概念限缩为说明书明确记载的具体实施方式\n")
	case OaDisclosure:
		amendments.WriteString("- **策略**：论述公开充分\n")
		amendments.WriteString("- **要点**：说明本领域技术人员根据说明书能够实现发明\n")
	default:
		amendments.WriteString("- **策略**：综合答复\n")
		amendments.WriteString("- **要点**：逐条回应审查意见的驳回理由\n")
	}

	// Add citation analysis.
	if len(parsed.Citations) > 0 {
		amendments.WriteString("\n## 引用对比文件分析\n\n")
		for _, cit := range parsed.Citations {
			fmt.Fprintf(&amendments, "- **%s** (相关性: %s)\n",
				cit.DocumentNumber, relevancyLabel(cit.Relevancy))
			if len(cit.ClaimsAffected) > 0 {
				fmt.Fprintf(&amendments, "  - 影响权利要求: %v\n", cit.ClaimsAffected)
			}
		}
	}

	return graph.PregelState{
		OAStateClaimAmendments:  amendments.String(),
		OAStateParsed:           parsed,
		OAStateRejectionType:    rejectionType,
		OAStateCitations:        state[OAStateCitations],
		OAStateAffectedClaims:   state[OAStateAffectedClaims],
		OAStateResponseStrategy: strategy,
		OAStateInput:            state[OAStateInput],
		OAStateTemplateUsed:     state[OAStateTemplateUsed],
	}, nil
}

// claimAmendmentType returns the amendment action for a specific claim.
func claimAmendmentType(rejectionType OaRejectionType, claimNum int) string {
	switch rejectionType {
	case OaClarity:
		if claimNum == 1 {
			return "澄清限定"
		}
		return "从属引用调整"
	case OaSupport:
		return "限缩"
	case OaScope:
		if claimNum == 1 {
			return "限缩/删除"
		}
		return "删除"
	default:
		return "调整"
	}
}

// amendmentBasis returns the legal basis description for the amendment.
func amendmentBasis(rejectionType OaRejectionType) string {
	switch rejectionType {
	case OaClarity:
		return "专利法第26条第4款（清楚）"
	case OaSupport:
		return "专利法第26条第4款（支持）"
	case OaScope:
		return "专利法第33条（修改不超范围）"
	case OaNovelty, OaInventiveness:
		return "区别技术特征（非修改，争辩）"
	default:
		return "审查指南相关规定"
	}
}

// relevancyLabel converts ST.36 relevancy codes to Chinese labels.
func relevancyLabel(code string) string {
	switch code {
	case "X":
		return "X（单独影响新颖性/创造性）"
	case "Y":
		return "Y（结合影响创造性）"
	case "A":
		return "A（背景技术）"
	case "E":
		return "E（抵触申请）"
	default:
		return code
	}
}

// draftResponseNode assembles the final response draft by rendering the
// appropriate doc template with structured analysis data.
func draftResponseNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	parsed, ok := state[OAStateParsed].(*ParsedOfficeAction)
	if !ok {
		return nil, fmt.Errorf("oa_response: invalid or missing parsed OA state in draft phase")
	}
	rejectionType := state.GetString(OAStateRejectionType)
	strategy := state.GetString(OAStateResponseStrategy)
	templateName := state.GetString(OAStateTemplateUsed)
	amendments := state.GetString(OAStateClaimAmendments)

	var response strings.Builder
	response.WriteString("# 审查意见答复书\n\n")

	// Header section.
	response.WriteString("## 审查意见概述\n\n")
	if parsed != nil {
		response.WriteString(FormatOaSummary(*parsed))
		response.WriteString("\n\n")

		if len(parsed.ExaminerArguments) > 0 {
			response.WriteString("### 审查员主要论点\n\n")
			for _, arg := range parsed.ExaminerArguments {
				fmt.Fprintf(&response, "- %s\n", arg)
			}
			response.WriteString("\n")
		}
	}

	// Strategy section.
	fmt.Fprintf(&response, "### 答复策略: %s (模板: %s)\n\n", strategyLabel(strategy), templateName)

	// Claim analysis section.
	response.WriteString(amendments)
	response.WriteString("\n")

	// Template-specific drafting guidance.
	response.WriteString("## 意见陈述\n\n")
	response.WriteString(draftResponseBody(OaRejectionType(rejectionType), parsed))
	response.WriteString("\n")

	// Disclaimer.
	response.WriteString("---\n\n")
	response.WriteString("> ⚠️ **人工审核提醒**\n")
	response.WriteString("> \n")
	response.WriteString("> 本答复书由 Mady AI 辅助生成骨架，以下内容必须由专利代理人逐项核实后定稿：\n")
	response.WriteString("> 1. 区别技术特征的认定是否准确\n")
	response.WriteString("> 2. 对比文件的分析是否完整（审查员可能引用未提取的段落）\n")
	response.WriteString("> 3. 法律依据的引用是否正确\n")
	response.WriteString("> 4. 修改后的权利要求是否获得说明书支持且不超出原范围\n")
	response.WriteString("> \n")
	response.WriteString("> 本分析由 AI 辅助生成，不构成正式法律意见。\n")

	final := response.String()
	return graph.PregelState{
		OAStateResponseDraft:   final,
		OAStateOutput:          final,
		OAStateParsed:          parsed,
		OAStateRejectionType:   rejectionType,
		OAStateTemplateUsed:    templateName,
		OAStateClaimAmendments: amendments,
		OAStateInput:           state[OAStateInput],
	}, nil
}

// draftResponseBody generates the core argument section based on rejection type.
func draftResponseBody(rejectionType OaRejectionType, parsed *ParsedOfficeAction) string {
	var b strings.Builder

	switch rejectionType {
	case OaNovelty:
		b.WriteString("### 一、关于新颖性（专利法第22条第2款）\n\n")
		b.WriteString("审查员认为本申请相对于对比文件不具备新颖性。申请人认为该审查意见不能成立。\n\n")
		b.WriteString("#### 区别特征分析\n\n")
		b.WriteString("经逐项比对，对比文件至少未公开以下技术特征：\n\n")
		b.WriteString("- [特征1]：[分析说明]\n")
		b.WriteString("- [特征2]：[分析说明]\n\n")
		b.WriteString("#### 单独对比原则\n\n")
		b.WriteString("根据审查指南第二部分第三章的规定，新颖性判断应遵循单独对比原则。")
		b.WriteString("对比文件未公开权利要求1的全部技术特征，因此权利要求1具备新颖性。\n")

	case OaInventiveness:
		b.WriteString("### 一、关于创造性（专利法第22条第3款）\n\n")
		b.WriteString("#### 第一步：最接近的现有技术\n\n")
		b.WriteString("[认可/不认可]对比文件1作为最接近的现有技术。\n\n")
		b.WriteString("#### 第二步：区别特征及实际解决的技术问题\n\n")
		b.WriteString("权利要求1与对比文件1的区别在于：[区别特征描述]\n\n")
		b.WriteString("基于该区别特征，本发明实际解决的技术问题是：[技术问题]\n\n")
		b.WriteString("#### 第三步：非显而易见性\n\n")
		b.WriteString("对比文件2未给出将上述区别特征与对比文件1结合以解决所述技术问题的技术启示。理由：\n\n")
		b.WriteString("1. [技术启示分析1]\n")
		b.WriteString("2. [技术启示分析2]\n\n")
		b.WriteString("因此，权利要求1的技术方案对于本领域技术人员而言并非显而易见。\n")

	case OaClarity:
		b.WriteString("### 一、关于权利要求不清楚（专利法第26条第4款）\n\n")
		b.WriteString("针对审查员指出的不清楚之处，申请人已作出相应修改：\n\n")
		b.WriteString("- [修改项1]：[说明]\n")
		b.WriteString("- [修改项2]：[说明]\n\n")
		b.WriteString("修改后的权利要求清楚地限定了保护范围。\n")

	case OaSupport:
		b.WriteString("### 一、关于权利要求得不到说明书支持（专利法第26条第4款）\n\n")
		b.WriteString("针对审查员的意见，申请人已将原权利要求的[上位概念]限缩为说明书[具体段落]明确记载的[具体实施方式]。\n\n")
		b.WriteString("修改后的权利要求得到说明书的充分支持。\n")

	default:
		b.WriteString("### 关于审查意见的答复\n\n")
		b.WriteString("针对审查意见通知书中指出的问题，申请人逐条答复如下：\n\n")
		b.WriteString("[逐条答辩内容]\n")
	}

	b.WriteString("\n### 结论\n\n")
	b.WriteString("综上所述，修改后的权利要求克服了审查意见指出的缺陷，请求审查员在修改文本的基础上继续审查。\n")

	return b.String()
}

// strategyLabel converts strategy code to Chinese label.
func strategyLabel(strategy string) string {
	switch strategy {
	case "argument":
		return "争辩"
	case "amendment":
		return "修改"
	case "combined":
		return "争辩+修改"
	default:
		return strategy
	}
}

// approvalGateNode marks the response as needing human review.
// This node implements the same pattern as disclosure's review_gate —
// it sets a gate flag and halts the pipeline for manual approval.
func approvalGateNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	draft := state.GetString(OAStateResponseDraft)
	if draft == "" {
		return nil, fmt.Errorf("oa_response: no response draft to review")
	}

	// Mark as ready for human review.
	return graph.PregelState{
		OAStateOutput:          draft,
		OAStateResponseDraft:   draft,
		OAStateInput:           state[OAStateInput],
		OAStateParsed:          state[OAStateParsed],
		OAStateRejectionType:   state[OAStateRejectionType],
		OAStateTemplateUsed:    state[OAStateTemplateUsed],
		OAStateClaimAmendments: state[OAStateClaimAmendments],
	}, nil
}

// =============================================================================
// Graph Builder
// =============================================================================

// BuildOAResponseGraph constructs a Pregel graph for OA response drafting.
//
// Graph structure:
//
//	parse_oa → classify_rejection → analyze_claims → draft_response → approval_gate → __end__
//
// All nodes are deterministic (no LLM calls). The graph produces a structured
// response skeleton with claim amendment tables, strategy guidance, citation
// analysis, and a template-based response body.
func BuildOAResponseGraph() (*graph.CompiledPregelGraph, error) {
	g := graph.NewPregelGraph()

	if err := g.AddNode("parse_oa", parseOANode); err != nil {
		return nil, err
	}
	if err := g.AddNode("classify_rejection", classifyRejectionNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("analyze_claims", analyzeClaimsNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("draft_response", draftResponseNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("approval_gate", approvalGateNode); err != nil {
		return nil, err
	}

	// Linear flow: parse → classify → analyze → draft → approve → end.
	for _, edge := range [][2]string{
		{"parse_oa", "classify_rejection"},
		{"classify_rejection", "analyze_claims"},
		{"analyze_claims", "draft_response"},
		{"draft_response", "approval_gate"},
		{"approval_gate", graph.PregelEnd},
	} {
		if err := g.AddEdge(edge[0], edge[1]); err != nil {
			return nil, err
		}
	}

	return g.Compile("parse_oa", 10)
}
