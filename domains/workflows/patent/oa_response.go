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
//
// Optional nodes (oa_enhance.go): rule_retrieval (law-article lookup per
// rejection type) and llm_enhance (argument polishing). Helpers and rendering
// live in oa_helpers.go.
//
// Nodes return only the state keys they produce: the Pregel engine merges
// each node's output into the shared state (graph/state_schema.go), so
// pass-through of unrelated keys is neither needed nor done.
package patent

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// State keys used by the OA response workflow.
const (
	OAStateInput            = "oa_input"             // original OA notification text
	OAStateParsed           = "oa_parsed"            // *ParsedOfficeAction
	OAStateRejectionType    = "oa_rejection_type"    // string: primary OaRejectionType value
	OAStateRejectionTypes   = "oa_rejection_types"   // []OaRejectionType: all detected categories (text order)
	OAStateCitations        = "oa_citations"         // []CitedReference
	OAStateAffectedClaims   = "oa_affected_claims"   // []int
	OAStateResponseStrategy = "oa_response_strategy" // string: primary strategy (argument / amendment / combined)
	OAStateStrategies       = "oa_strategies"        // []string: one strategy per rejection type
	OAStateTemplates        = "oa_templates"         // []string: one doc template per rejection type
	OAStateClaimAmendments  = "oa_claim_amendments"  // string: claim amendment markup
	OAStateResponseDraft    = "oa_response_draft"    // string: final response draft
	OAStateTemplateUsed     = "oa_template_used"     // string: primary doc template used
	OAStateOutput           = "oa_output"            // string: final output text
	OAStateLLMEnhanced      = "oa_llm_enhanced"      // bool: whether LLM enhancement was applied
	OAStateApplicableRules  = "oa_applicable_rules"  // string: dynamically retrieved law articles (Markdown)
)

// =============================================================================
// Pregel Nodes
// =============================================================================

// parseOANode parses the OA notification text using the deterministic rules.OAParser.
// It extracts rejection types, cited references, and affected claim numbers.
func parseOANode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	input := state.GetString(OAStateInput)
	if input == "" {
		return nil, fmt.Errorf("oa_response: OA notification text is empty")
	}

	parsed := ParseOA(input)

	// Also extract examiner arguments by splitting on common sentence patterns.
	parsed.ExaminerArguments = extractExaminerArguments(input)

	return graph.PregelState{
		OAStateInput:          input,
		OAStateParsed:         &parsed,
		OAStateRejectionType:  parsed.RejectionType,
		OAStateRejectionTypes: parsed.RejectionTypes,
		OAStateCitations:      parsed.Citations,
		OAStateAffectedClaims: parsed.AffectedClaims,
	}, nil
}

// classifyRejectionNode determines the response strategy for every detected
// rejection type. An OA citing several grounds (e.g. novelty + inventiveness +
// clarity) yields one strategy and one doc template per ground; the first
// ground in text order remains the primary strategy/template for backward
// compatibility.
//
// Strategy selection per type:
//   - novelty (A22.2) → argument strategy (争辩)
//   - inventiveness (A22.3) → argument strategy with three-step method (三步法)
//   - clarity/support (A26.4) → amendment strategy (修改)
//   - disclosure (A26.3) → argument + evidence strategy
//   - scope (A33) → amendment strategy (删除/限缩)
//   - formal → simple amendment
func classifyRejectionNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	rejectionTypes := rejectionTypesFromState(state)
	if len(rejectionTypes) == 0 {
		return nil, fmt.Errorf("oa_response: no rejection type detected")
	}
	if _, ok := state[OAStateParsed].(*ParsedOfficeAction); !ok {
		return nil, fmt.Errorf("oa_response: invalid or missing parsed OA state")
	}

	strategies := determineResponseStrategies(rejectionTypes)
	templates := selectOATemplates(rejectionTypes, strategies)

	primaryStrategy, primaryTemplate := "", ""
	if len(strategies) > 0 {
		primaryStrategy = strategies[0]
	}
	if len(templates) > 0 {
		primaryTemplate = templates[0]
	}

	return graph.PregelState{
		OAStateRejectionTypes:   rejectionTypes,
		OAStateResponseStrategy: primaryStrategy,
		OAStateStrategies:       strategies,
		OAStateTemplateUsed:     primaryTemplate,
		OAStateTemplates:        templates,
	}, nil
}

// analyzeClaimsNode performs claim-level analysis and generates amendment
// suggestions for every detected rejection type. The amendment table lists one
// row per amendment-strategy rejection type; argument-only grounds contribute
// no rows.
func analyzeClaimsNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	parsed, ok := state[OAStateParsed].(*ParsedOfficeAction)
	if !ok || parsed == nil {
		return nil, fmt.Errorf("oa_response: invalid or missing parsed OA state")
	}
	rejectionTypes := rejectionTypesFromState(state)
	if len(rejectionTypes) == 0 {
		return nil, fmt.Errorf("oa_response: no rejection type detected")
	}
	strategies := determineResponseStrategies(rejectionTypes)

	var amendments strings.Builder
	amendments.WriteString("## 权利要求修改对照表\n\n")
	amendments.WriteString("| 驳回类型 | 修改类型 | 修改前 | 修改后 | 修改依据 | 涉及权利要求 |\n")
	amendments.WriteString("|----------|----------|--------|--------|----------|---------------|\n")

	// Amendment rows are generated per rejection type; argument-only grounds
	// (novelty/inventiveness) contribute no rows unless combined with amendments.
	hasAmendment := false
	for i, rt := range rejectionTypes {
		strategy := "argument"
		if i < len(strategies) {
			strategy = strategies[i]
		}
		if strategy != "amendment" && strategy != "combined" {
			continue
		}
		hasAmendment = true
		// 以受影响权利要求中最靠前的为准，判定独立/从属权利要求的修改动作。
		claimNum := 1
		if len(parsed.AffectedClaims) > 0 {
			claimNum = parsed.AffectedClaims[0]
		}
		fmt.Fprintf(&amendments, "| %s | %s | [原内容] | [建议修改] | %s | %s |\n",
			rejectionTypeLabel(rt), claimAmendmentType(rt, claimNum), amendmentBasis(rt),
			formatClaimList(parsed.AffectedClaims))
	}
	if !hasAmendment {
		amendments.WriteString("| — | 无需修改 | — | — | 基于以下争辩理由，本权利要求无需修改 | — |\n")
	}
	amendments.WriteString("\n> 注：涉及权利要求按通知书整体提取，各驳回理由对应的具体权利要求请以通知书原文为准。\n\n")

	// Strategy-specific guidance, one block per rejection type.
	amendments.WriteString("## 答复策略建议\n\n")
	for i, rt := range rejectionTypes {
		strategy := ""
		if i < len(strategies) {
			strategy = strategies[i]
		}
		fmt.Fprintf(&amendments, "### %s（策略：%s）\n\n", rejectionTypeLabel(rt), strategyLabel(strategy))
		switch rt {
		case OaNovelty:
			amendments.WriteString("- **要点**：论证对比文件未公开至少一项技术特征（单独对比原则）\n")
			amendments.WriteString("- **风险**：低（新颖性争辩成功率相对较高）\n")
		case OaInventiveness:
			amendments.WriteString("- **要点**：确定区别特征 → 确定实际解决的技术问题 → 论证非显而易见（三步法）\n")
			amendments.WriteString("- **关键**：重点论述'不存在技术启示'\n")
		case OaClarity:
			amendments.WriteString("- **要点**：明确限定用语含义、删除模糊表述、补充连接关系\n")
		case OaSupport:
			amendments.WriteString("- **要点**：将上位概念限缩为说明书明确记载的具体实施方式\n")
		case OaDisclosure:
			amendments.WriteString("- **要点**：说明本领域技术人员根据说明书能够实现发明\n")
		case OaScope:
			amendments.WriteString("- **要点**：删除/限缩超出原说明书和权利要求书记载范围的内容\n")
		case OaFormal:
			amendments.WriteString("- **要点**：按审查指南要求修正形式缺陷\n")
		default:
			amendments.WriteString("- **要点**：逐条回应审查意见的驳回理由\n")
		}
		amendments.WriteString("\n")
	}

	// Add citation analysis.
	if len(parsed.Citations) > 0 {
		amendments.WriteString("## 引用对比文件分析\n\n")
		for _, cit := range parsed.Citations {
			fmt.Fprintf(&amendments, "- **%s** (相关性: %s)\n",
				cit.DocumentNumber, relevancyLabel(cit.Relevancy))
			if len(cit.ClaimsAffected) > 0 {
				fmt.Fprintf(&amendments, "  - 影响权利要求: %v\n", cit.ClaimsAffected)
			}
		}
	}

	return graph.PregelState{
		OAStateClaimAmendments: amendments.String(),
	}, nil
}

// draftResponseNode assembles the final response draft by rendering the
// appropriate doc template with structured analysis data. Mixed-ground OAs get
// one argument section per rejection type, numbered sequentially.
func draftResponseNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	parsed, ok := state[OAStateParsed].(*ParsedOfficeAction)
	if !ok {
		return nil, fmt.Errorf("oa_response: invalid or missing parsed OA state in draft phase")
	}
	rejectionTypes := rejectionTypesFromState(state)
	if len(rejectionTypes) == 0 {
		return nil, fmt.Errorf("oa_response: no rejection type detected in draft phase")
	}
	strategies := determineResponseStrategies(rejectionTypes)
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

	// Strategy section — one strategy per detected rejection type.
	response.WriteString("### 答复策略: ")
	response.WriteString(summarizeStrategies(rejectionTypes, strategies))
	if templateName != "" {
		fmt.Fprintf(&response, " (模板: %s)", templateName)
	}
	response.WriteString("\n\n")

	// Dynamic law articles (if retrieved).
	if applicableRules := state.GetString(OAStateApplicableRules); applicableRules != "" {
		response.WriteString(applicableRules)
		response.WriteString("\n\n")
	}

	// Claim analysis section.
	response.WriteString(amendments)
	response.WriteString("\n")

	// Template-specific drafting guidance.
	response.WriteString("## 意见陈述\n\n")
	response.WriteString(draftResponseBodies(rejectionTypes))
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
		OAStateResponseDraft: final,
		OAStateOutput:        final,
	}, nil
}

// approvalGateNode marks the response as needing human review.
// This node implements the same pattern as disclosure's review_gate —
// it confirms the draft and publishes it as the final output.
func approvalGateNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	draft := state.GetString(OAStateResponseDraft)
	if draft == "" {
		return nil, fmt.Errorf("oa_response: no response draft to review")
	}

	// Mark as ready for human review.
	return graph.PregelState{
		OAStateOutput: draft,
	}, nil
}

// =============================================================================
// Graph Builder
// =============================================================================

// OAGraphOption 可选地配置 OA 答复图的依赖（如 LLM 增强节点、法条检索器）。
type OAGraphOption func(*oaGraphConfig)

type oaGraphConfig struct {
	provider      agentcore.Provider
	ruleRetriever OARuleRetriever
}

// WithOAProvider 注入 LLM Provider，启用 draft_response 之后的 LLM 增强节点。
// 注入后管线变为：draft_response → llm_enhance → approval_gate
// 未注入时 llm_enhance 为 no-op，保留纯确定性输出（向后兼容）。
func WithOAProvider(p agentcore.Provider) OAGraphOption {
	return func(c *oaGraphConfig) { c.provider = p }
}

// WithOARuleRetriever 注入法条检索器，在 classify_rejection 之后插入
// rule_retrieval 节点，根据驳回类型动态检索适用法条和审查指南段落。
// 未注入时使用硬编码模板法条（向后兼容）。
func WithOARuleRetriever(r OARuleRetriever) OAGraphOption {
	return func(c *oaGraphConfig) { c.ruleRetriever = r }
}

// BuildOAResponseGraph constructs a Pregel graph for OA response drafting
// (无 LLM 增强，全确定性节点）。
//
// Graph structure:
//
//	parse_oa → classify_rejection → analyze_claims → draft_response → approval_gate → __end__
func BuildOAResponseGraph() (*graph.CompiledPregelGraph, error) {
	return BuildOAResponseGraphWithOpts()
}

// BuildOAResponseGraphWithOpts 构造 OA 答复 Pregel 图，支持可选的法条检索器和 LLM 增强节点注入。
//
// 无注入时管线：
//
//	parse_oa → classify_rejection → analyze_claims → draft_response → approval_gate → __end__
//
// 有 ruleRetriever 时管线（在 classify 之后插入 rule_retrieval）：
//
//	parse_oa → classify_rejection → rule_retrieval → analyze_claims → draft_response → approval_gate → __end__
//
// 有 provider 时（在 draft 之后插入 llm_enhance）：
//
//	… → draft_response → llm_enhance → approval_gate → __end__
func BuildOAResponseGraphWithOpts(opts ...OAGraphOption) (*graph.CompiledPregelGraph, error) {
	cfg := &oaGraphConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	g := graph.NewPregelGraph()

	if err := g.AddNode("parse_oa", parseOANode); err != nil {
		return nil, err
	}
	if err := g.AddNode("classify_rejection", classifyRejectionNode); err != nil {
		return nil, err
	}

	// Conditionally insert rule_retrieval node.
	hasRetriever := cfg.ruleRetriever != nil
	if hasRetriever {
		if err := g.AddNode("rule_retrieval", newRuleRetrievalNode(cfg.ruleRetriever)); err != nil {
			return nil, err
		}
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

	// 插入 LLM 增强节点（如有 provider 注入）。
	if cfg.provider != nil {
		if err := g.AddNode("llm_enhance", newOAEnhanceNode(cfg.provider)); err != nil {
			return nil, err
		}
	}

	// Build edges — rule_retrieval is inserted between classify_rejection and analyze_claims.
	edges := [][2]string{
		{"parse_oa", "classify_rejection"},
	}
	if hasRetriever {
		edges = append(edges,
			[2]string{"classify_rejection", "rule_retrieval"},
			[2]string{"rule_retrieval", "analyze_claims"},
		)
	} else {
		edges = append(edges, [2]string{"classify_rejection", "analyze_claims"})
	}
	edges = append(edges, [2]string{"analyze_claims", "draft_response"})
	if cfg.provider != nil {
		edges = append(edges, [][2]string{
			{"draft_response", "llm_enhance"},
			{"llm_enhance", "approval_gate"},
		}...)
	} else {
		edges = append(edges, [2]string{"draft_response", "approval_gate"})
	}
	edges = append(edges, [2]string{"approval_gate", graph.PregelEnd})

	for _, edge := range edges {
		if err := g.AddEdge(edge[0], edge[1]); err != nil {
			return nil, err
		}
	}

	return g.Compile("parse_oa", 10)
}
