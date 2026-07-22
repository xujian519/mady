// Package patent provides a Pregel-based patent reexamination request workflow.
//
// The reexamination workflow processes a rejection decision (驳回决定) and
// generates a structured reexamination request (复审请求书) skeleton under
// Chinese patent law Article 41 (专利法第41条).
//
// Graph structure:
//
//	parse_decision → classify_grounds → draft_request → rule_check → conclude → __end__
//
// Key legal differences from OA response:
//   - Triggered by a formal rejection decision (驳回决定), not an office action
//   - Filed with the Reexamination and Invalidation Board (复审和无效审理部门)
//   - 3-month deadline from receipt of rejection decision
//   - Arguments may go beyond the original rejection grounds with new evidence
package patent

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
)

// State keys used by the reexamination workflow.
const (
	ReexamStateInput        = "reexam_input"         // original rejection decision text
	ReexamStateDecisionInfo = "reexam_decision_info" // parsed decision metadata
	ReexamStateGrounds      = "reexam_grounds"       // []ReexamGround: classified rejection grounds
	ReexamStatePatentType   = "reexam_patent_type"   // "invention" or "utility_model"
	ReexamStateDraft        = "reexam_draft"         // draft request text
	ReexamStateRuleCheck    = "reexam_rule_check"    // rule engine report
	ReexamStateRuleVerdict  = "reexam_rule_verdict"  // aggregate verdict
	ReexamStateConclusion   = "reexam_conclusion"    // final conclusion
	ReexamStateOutput       = "reexam_output"        // final output text
)

// ReexamGroundType identifies the type of rejection ground in the decision.
type ReexamGroundType string

const (
	ReexamGroundNovelty       ReexamGroundType = "novelty"
	ReexamGroundInventiveness ReexamGroundType = "inventiveness"
	ReexamGroundDisclosure    ReexamGroundType = "disclosure"
	ReexamGroundClarity       ReexamGroundType = "clarity"
	ReexamGroundAmendment     ReexamGroundType = "amendment"
	ReexamGroundSubject       ReexamGroundType = "subject" // 实用新型客体 (Art. 2.3)
)

// ReexamDecisionInfo holds metadata extracted from the rejection decision.
type ReexamDecisionInfo struct {
	DecisionNumber string // 驳回决定文号
	DecisionDate   string // 驳回决定日期
	ApplicantName  string // 申请人
	PatentTitle    string // 发明/实用新型名称
	ApplicationNo  string // 申请号
	CitedDocs      string // 引用的对比文件
}

// ReexamGround represents one rejection ground identified in the decision.
type ReexamGround struct {
	Type        ReexamGroundType
	Article     string // legal article reference
	Description string // human-readable description
}

// =============================================================================
// Pregel Nodes
// =============================================================================

// reexamParseDecisionNode parses the rejection decision text, extracting
// metadata (decision number, date, applicant, application number, cited docs)
// and identifying rejection grounds.
func reexamParseDecisionNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	input := state.GetString(ReexamStateInput)
	if input == "" {
		return nil, fmt.Errorf("reexamination: rejection decision input is empty")
	}

	info := extractDecisionInfo(input)
	grounds := identifyReexaminationGrounds(input)
	patentType := detectPatentType(input, grounds)

	return graph.PregelState{
		ReexamStateInput:        input,
		ReexamStateDecisionInfo: info,
		ReexamStateGrounds:      grounds,
		ReexamStatePatentType:   patentType,
	}, nil
}

// reexamClassifyGroundsNode refines the rejection grounds classification,
// adding argument strategies for each ground type.
func reexamClassifyGroundsNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	grounds, _ := state[ReexamStateGrounds].([]ReexamGround)
	patentType := state.GetString(ReexamStatePatentType)

	// For utility models, filter out inventiveness (not examined in UM).
	if patentType == "utility_model" {
		var filtered []ReexamGround
		for _, g := range grounds {
			if g.Type != ReexamGroundInventiveness {
				filtered = append(filtered, g)
			}
		}
		if len(filtered) > 0 {
			grounds = filtered
		}
	}

	// Ensure at least one ground.
	if len(grounds) == 0 {
		grounds = []ReexamGround{{
			Type:        ReexamGroundNovelty,
			Article:     "专利法第22条第2款",
			Description: "新颖性缺陷（默认分析维度）",
		}}
	}

	return graph.PregelState{
		ReexamStateGrounds: grounds,
	}, nil
}

// reexamDraftNode generates the reexamination request skeleton, with
// per-ground argumentation framework and required legal sections.
func reexamDraftNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	info, _ := state[ReexamStateDecisionInfo].(ReexamDecisionInfo)
	grounds, _ := state[ReexamStateGrounds].([]ReexamGround)
	patentType := state.GetString(ReexamStatePatentType)

	var b strings.Builder
	b.WriteString("# 复审请求书\n\n")

	// Basic information section.
	b.WriteString("## 请求人信息\n\n")
	b.WriteString("（请填充以下信息）\n\n")
	if info.ApplicantName != "" {
		fmt.Fprintf(&b, "- 申请人：%s\n", info.ApplicantName)
	} else {
		b.WriteString("- 申请人：\n")
	}
	b.WriteString("- 代理机构：\n")
	b.WriteString("- 代理人：\n\n")

	b.WriteString("## 涉案专利信息\n\n")
	if info.PatentTitle != "" {
		fmt.Fprintf(&b, "- 名称：%s\n", info.PatentTitle)
	} else {
		b.WriteString("- 名称：\n")
	}
	if info.ApplicationNo != "" {
		fmt.Fprintf(&b, "- 申请号：%s\n", info.ApplicationNo)
	} else {
		b.WriteString("- 申请号：\n")
	}
	if patentType != "" {
		fmt.Fprintf(&b, "- 类型：%s\n", patentTypeLabel(patentType))
	}
	b.WriteString("\n")

	// Rejection decision reference.
	b.WriteString("## 驳回决定\n\n")
	if info.DecisionNumber != "" {
		fmt.Fprintf(&b, "- 文号：%s\n", info.DecisionNumber)
	}
	if info.DecisionDate != "" {
		fmt.Fprintf(&b, "- 日期：%s\n", info.DecisionDate)
	}
	if info.CitedDocs != "" {
		fmt.Fprintf(&b, "- 引用对比文件：%s\n", info.CitedDocs)
	}
	b.WriteString("\n")

	// Legal basis.
	b.WriteString("## 法律依据\n\n")
	b.WriteString("根据《专利法》第41条，请求人对上述驳回决定不服，特向专利复审委员会提出复审请求。\n\n")

	// Per-ground arguments.
	b.WriteString("## 复审理由\n\n")
	for i, g := range grounds {
		fmt.Fprintf(&b, "### 理由%d：%s\n\n", i+1, g.Description)
		fmt.Fprintf(&b, "**驳回依据**：%s\n\n", g.Article)
		writeReexamArgument(&b, g)
		b.WriteString("\n")
	}

	// Modification statement.
	b.WriteString("## 修改说明\n\n")
	b.WriteString("（如有修改替换页，请在此说明修改内容及理由）\n\n")

	// Evidence.
	b.WriteString("## 证据清单\n\n")
	b.WriteString("（如有新证据，请列明证据编号、名称及证明目的）\n\n")

	return graph.PregelState{
		ReexamStateDraft: b.String(),
	}, nil
}

// reexamRuleCheckNode runs the reexamination rule engine.
func reexamRuleCheckNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	draft := state.GetString(ReexamStateDraft)
	grounds, _ := state[ReexamStateGrounds].([]ReexamGround)

	// Compose check text: draft + ground descriptions.
	var checkText strings.Builder
	checkText.WriteString(draft)
	for _, g := range grounds {
		checkText.WriteString("\n")
		checkText.WriteString(g.Description)
		checkText.WriteString(" ")
		checkText.WriteString(g.Article)
	}

	engine := NewRuleEngine()
	engine.RegisterRules(ReexaminationRules())
	results := engine.Evaluate(engine.Rules(), checkText.String(), "patent_reexamination")
	verdict := Aggregate(results)
	report := FormatRuleResults(results, verdict)

	return graph.PregelState{
		ReexamStateRuleCheck:   report,
		ReexamStateRuleVerdict: string(verdict),
	}, nil
}

// reexamConcludeNode generates the final reexamination request report.
func reexamConcludeNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	draft := state.GetString(ReexamStateDraft)
	ruleCheck := state.GetString(ReexamStateRuleCheck)
	ruleVerdict := state.GetString(ReexamStateRuleVerdict)

	var report strings.Builder

	if ruleVerdict == string(VerdictBlocked) {
		report.WriteString("> ⛔ **规则引擎检查未通过**：复审请求书存在严重缺陷，请逐项修正后再提交。\n\n")
	}

	report.WriteString(draft)
	report.WriteString("\n---\n\n")
	report.WriteString(ruleCheck)

	// Deadline reminder.
	report.WriteString("\n## 期限提醒\n\n")
	report.WriteString("> ⚠️ **复审请求期限**：收到驳回决定之日起 **3 个月内** 提出复审请求。\n")
	report.WriteString("> 如需延长，可在期限届满前申请延长 **1 个月**（需缴纳延长期限费）。\n\n")

	// Disclaimer.
	report.WriteString("---\n\n")
	report.WriteString("> ⚠️ **人工审核提醒**\n")
	report.WriteString("> \n")
	report.WriteString("> 本复审请求书由 AI 辅助生成骨架，以下内容必须由专利代理师/律师逐项核实后定稿：\n")
	report.WriteString("> 1. 驳回决定文号、日期等信息是否准确\n")
	report.WriteString("> 2. 每项复审理由的论证是否充分且有针对性\n")
	report.WriteString("> 3. 修改替换页是否符合专利法第33条（修改不超范围）\n")
	report.WriteString("> 4. 新证据的证明目的与关联性是否明确\n")
	report.WriteString("> \n")
	report.WriteString("> 本请求书不构成正式法律意见。\n")

	final := report.String()
	return graph.PregelState{
		ReexamStateConclusion: final,
		ReexamStateOutput:     final,
	}, nil
}

// =============================================================================
// Graph Builder
// =============================================================================

// BuildReexaminationGraph constructs a Pregel graph for reexamination request drafting.
//
// Graph structure:
//
//	parse_decision → classify_grounds → draft_request → rule_check → conclude → __end__
func BuildReexaminationGraph() (*graph.CompiledPregelGraph, error) {
	g := graph.NewPregelGraph()

	if err := g.AddNode("parse_decision", reexamParseDecisionNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("classify_grounds", reexamClassifyGroundsNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("draft_request", reexamDraftNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("rule_check", reexamRuleCheckNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("conclude", reexamConcludeNode); err != nil {
		return nil, err
	}

	edges := [][2]string{
		{"parse_decision", "classify_grounds"},
		{"classify_grounds", "draft_request"},
		{"draft_request", "rule_check"},
		{"rule_check", "conclude"},
		{"conclude", graph.PregelEnd},
	}
	for _, edge := range edges {
		if err := g.AddEdge(edge[0], edge[1]); err != nil {
			return nil, err
		}
	}

	return g.Compile("parse_decision", 15)
}

// =============================================================================
// Helpers
// =============================================================================

// extractDecisionInfo parses metadata from the rejection decision text.
func extractDecisionInfo(text string) ReexamDecisionInfo {
	info := ReexamDecisionInfo{}
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Decision number: 驳回决定编号/文号
		if info.DecisionNumber == "" {
			if v := extractAfterPrefix(line, "驳回决定编号", "决定编号", "文号"); v != "" {
				info.DecisionNumber = v
			}
		}
		// Decision date
		if info.DecisionDate == "" {
			if v := extractAfterPrefix(line, "决定日期", "驳回日期", "日"); v != "" {
				info.DecisionDate = v
			}
		}
		// Applicant
		if info.ApplicantName == "" {
			if v := extractAfterPrefix(line, "申请人"); v != "" {
				info.ApplicantName = v
			}
		}
		// Patent title
		if info.PatentTitle == "" {
			if v := extractAfterPrefix(line, "发明名称", "实用新型名称", "名称"); v != "" {
				info.PatentTitle = v
			}
		}
		// Application number
		if info.ApplicationNo == "" {
			if v := extractAfterPrefix(line, "申请号"); v != "" {
				info.ApplicationNo = v
			}
		}
	}

	// Extract cited documents from full text (may span multiple lines).
	if idx := strings.Index(text, "对比文件"); idx >= 0 {
		snippet := truncate(text[idx:], 200)
		// Collapse newlines to keep Markdown rendering clean.
		snippet = strings.ReplaceAll(snippet, "\n", " ")
		info.CitedDocs = snippet
	}

	return info
}

// extractAfterPrefix checks if line starts with any of the prefixes,
// and returns the value after the prefix and separator.
func extractAfterPrefix(line string, prefixes ...string) string {
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			rest := strings.TrimPrefix(line, p)
			rest = strings.TrimLeft(rest, " ：:\t")
			rest = strings.TrimRight(rest, " \t")
			if rest != "" && len(rest) > 1 {
				return rest
			}
		}
	}
	return ""
}

// reexaminationGroundRules is the pattern table for reexamination ground
// identification. Includes utility-model-specific subject-matter ground.
var reexaminationGroundRules = []groundPattern{
	{TypeKey: string(ReexamGroundNovelty), Article: "专利法第22条第2款",
		Desc:     "新颖性缺陷",
		Patterns: []string{"22条第2款", "22.2", "新颖性", "不具备新颖"}},
	{TypeKey: string(ReexamGroundInventiveness), Article: "专利法第22条第3款",
		Desc:     "创造性缺陷",
		Patterns: []string{"22条第3款", "22.3", "创造性", "不具备创造", "显而易见"}},
	{TypeKey: string(ReexamGroundDisclosure), Article: "专利法第26条第3款",
		Desc:     "公开不充分",
		Patterns: []string{"26条第3款", "26.3", "公开充分", "充分公开", "能够实现"}},
	{TypeKey: string(ReexamGroundClarity), Article: "专利法第26条第4款",
		Desc:     "权利要求不清楚/不支持",
		Patterns: []string{"26条第4款", "26.4", "清楚", "不支持"}},
	{TypeKey: string(ReexamGroundAmendment), Article: "专利法第33条",
		Desc:     "修改超范围",
		Patterns: []string{"第33条", "A33", "修改超范围", "超出原"}},
	{TypeKey: string(ReexamGroundSubject), Article: "专利法第2条第3款",
		Desc:     "实用新型客体缺陷",
		Patterns: []string{"第2条第3款", "2.3", "客体", "不属于实用新型"}},
}

// identifyReexaminationGrounds scans the text for rejection grounds.
func identifyReexaminationGrounds(text string) []ReexamGround {
	matched := scanGrounds(text, reexaminationGroundRules)
	var grounds []ReexamGround
	for _, r := range matched {
		grounds = append(grounds, ReexamGround{
			Type:        ReexamGroundType(r.TypeKey),
			Article:     r.Article,
			Description: r.Desc,
		})
	}

	if len(grounds) == 0 {
		grounds = append(grounds, ReexamGround{
			Type:        ReexamGroundNovelty,
			Article:     "专利法第22条第2款",
			Description: "新颖性缺陷（默认分析维度）",
		})
	}

	return grounds
}

// detectPatentType determines if this is an invention or utility model from text.
func detectPatentType(text string, grounds []ReexamGround) string {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "实用新型") {
		return "utility_model"
	}
	if strings.Contains(lower, "发明") {
		return "invention"
	}
	// If subject matter ground present, likely utility model.
	for _, g := range grounds {
		if g.Type == ReexamGroundSubject {
			return "utility_model"
		}
	}
	return "invention"
}

// patentTypeLabel returns a human-readable label for the patent type.
func patentTypeLabel(t string) string {
	switch t {
	case "utility_model":
		return "实用新型"
	case "invention":
		return "发明"
	default:
		return t
	}
}

// writeReexamArgument writes the argumentation framework for a single ground.
func writeReexamArgument(b *strings.Builder, g ReexamGround) {
	switch g.Type {
	case ReexamGroundNovelty:
		b.WriteString("**复审策略**：\n")
		b.WriteString("- 采用**单独对比原则**，论证对比文件未公开权利要求的全部技术特征\n")
		b.WriteString("- 指出区别技术特征及其技术效果\n")
		b.WriteString("- 如有必要，提交修改后的权利要求以明确区别\n\n")

	case ReexamGroundInventiveness:
		b.WriteString("**复审策略（三步法反驳）**：\n")
		b.WriteString("1. 论证最接近现有技术的选择不当\n")
		b.WriteString("2. 指出区别技术特征和实际解决的技术问题\n")
		b.WriteString("3. 论证现有技术不存在**技术启示/组合动机**\n\n")

	case ReexamGroundDisclosure:
		b.WriteString("**复审策略**：\n")
		b.WriteString("- 论证说明书记载的内容足以使本领域技术人员**能够实现**\n")
		b.WriteString("- 如有必要，补充实验数据证明可实现性\n\n")

	case ReexamGroundClarity:
		b.WriteString("**复审策略**：\n")
		b.WriteString("- 论证权利要求术语的含义对本领域技术人员是**清楚的**\n")
		b.WriteString("- 提交修改后的权利要求以消除不清楚之处\n\n")

	case ReexamGroundAmendment:
		b.WriteString("**复审策略**：\n")
		b.WriteString("- 论证修改内容能够从原说明书和权利要求书中**直接且毫无疑义地**确定\n")
		b.WriteString("- 如修改确有不当，提交符合第33条的替代修改方案\n\n")

	case ReexamGroundSubject:
		b.WriteString("**复审策略（实用新型客体抗辩）**：\n")
		b.WriteString("- 论证权利要求限定的技术方案属于**产品的形状、构造或其结合**\n")
		b.WriteString("- 对于涉及软件/方法步骤的权利要求，修改为仅保留产品结构特征\n\n")
	}
}
