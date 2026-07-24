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
	ReexamStateOralPrep     = "reexam_oral_prep"     // 口审准备内容
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
// Graph Options
// =============================================================================

// ReexamGraphOption optionally configures the reexamination graph's structure.
type ReexamGraphOption func(*reexamGraphConfig)

type reexamGraphConfig struct {
	oralHearing bool
}

// WithOralHearingPrep inserts an oral hearing preparation node between
// draft_request and rule_check in the reexamination graph.
// When enabled, the graph generates 口审准备 materials:
// technical comparison table, amendment non-extension argument,
// possible challenges preview, and statement outline.
func WithOralHearingPrep() ReexamGraphOption {
	return func(c *reexamGraphConfig) { c.oralHearing = true }
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

// reexamOralHearingNode generates oral hearing preparation materials,
// including a technical comparison table, amendment non-extension argument,
// possible examiner challenges preview, and statement outline.
func reexamOralHearingNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	grounds, _ := state[ReexamStateGrounds].([]ReexamGround)
	info, _ := state[ReexamStateDecisionInfo].(ReexamDecisionInfo)
	draft := state.GetString(ReexamStateDraft)
	_ = draft // available for future claim extraction expansion

	var b strings.Builder
	b.WriteString("# 口审准备材料\n\n")
	b.WriteString("> 以下内容为口审准备参考材料，建议根据实际案件情况进一步补充完善。\n\n")

	// 1. 技术方案对比表
	b.WriteString("## 一、技术方案对比表\n\n")
	b.WriteString(prepareTechComparisonTable(grounds, info))
	b.WriteString("\n\n---\n\n")

	// 2. 修改不超范围的书面论证
	b.WriteString("## 二、修改不超范围论证\n\n")
	b.WriteString(prepareAmendmentNonExtensionArgument(grounds))
	b.WriteString("\n\n---\n\n")

	// 3. 审查员可能的质疑点预演
	b.WriteString("## 三、审查员可能的质疑点预演\n\n")
	b.WriteString(preparePossibleChallenges(grounds))
	b.WriteString("\n\n---\n\n")

	// 4. 陈述要点提纲
	b.WriteString("## 四、口审陈述要点提纲\n\n")
	b.WriteString(prepareStatementOutline(grounds))

	return graph.PregelState{
		ReexamStateOralPrep: b.String(),
	}, nil
}

// prepareTechComparisonTable generates a Markdown table comparing
// claim technical features against cited documents from the rejection decision.
func prepareTechComparisonTable(grounds []ReexamGround, info ReexamDecisionInfo) string {
	var b strings.Builder

	b.WriteString("### 权利要求技术特征与对比文件对比表\n\n")
	b.WriteString("| 序号 | 权利要求技术特征 | 对比文件对应特征 | 是否公开 | 分析结论 |\n")
	b.WriteString("|------|------------------|-----------------|----------|----------|\n")
	b.WriteString("| 1 | （原权利要求特征1） | （对比文件公开的特征） | 公开/未公开 | 具备新颖性/需修改 |\n")
	b.WriteString("| 2 | （原权利要求特征2） | （对比文件公开的特征） | 公开/未公开 | 具备新颖性/需修改 |\n")
	b.WriteString("| 3 | （原权利要求特征3） | （对比文件公开的特征） | 公开/未公开 | 具备新颖性/需修改 |\n")

	b.WriteString("\n**说明**：上表中每一行对应一个技术特征。请根据权利要求的实际技术方案逐项填写具体内容。\n")

	// Add amendment comparison section if relevant
	for _, g := range grounds {
		if g.Type == ReexamGroundAmendment {
			b.WriteString("\n### 修改前后特征对比\n\n")
			b.WriteString("| 修改前 | 修改后 | 原申请文件出处 | 是否超范围 |\n")
			b.WriteString("|--------|--------|----------------|----------|\n")
			b.WriteString("| （修改前内容） | （修改后内容） | （原说明书/权利要求书位置） | 未超范围 |\n")
			b.WriteString("\n**说明**：如已提交修改替换页，请在此逐项列明修改前后的内容对比及相应的原申请文件支持依据。\n")
			break
		}
	}

	if info.CitedDocs != "" {
		b.WriteString("\n### 引用对比文件清单\n\n")
		b.WriteString(info.CitedDocs)
		b.WriteString("\n")
	}

	return b.String()
}

// prepareAmendmentNonExtensionArgument generates a written argument
// on whether claim amendments comply with Article 33 (no extension beyond
// original disclosure). If no amendment ground exists, returns a placeholder.
func prepareAmendmentNonExtensionArgument(grounds []ReexamGround) string {
	var b strings.Builder

	hasAmendment := false
	for _, g := range grounds {
		if g.Type == ReexamGroundAmendment {
			hasAmendment = true
			break
		}
	}

	if !hasAmendment {
		b.WriteString("（本案驳回理由不涉及修改超范围问题。如复审过程中提交了修改替换页，仍需准备修改不超范围的书面论证。）\n\n")
		b.WriteString("### 论证框架（如需）\n\n")
		b.WriteString("1. **修改内容识别**：逐项列明修改内容\n")
		b.WriteString("2. **原申请文件支持依据**：指出每处修改在原说明书和/或权利要求书中的记载位置\n")
		b.WriteString("3. **论证结论**：修改未超出原说明书和权利要求书记载的范围，符合专利法第33条的规定\n\n")
		return b.String()
	}

	b.WriteString("### 修改内容概述\n\n")
	b.WriteString("（请在此列明复审阶段提交的修改替换页中的具体修改内容）\n\n")

	b.WriteString("### 修改依据与出处\n\n")
	b.WriteString("| 修改项 | 修改后内容 | 原申请文件依据 | 依据位置 |\n")
	b.WriteString("|--------|------------|----------------|----------|\n")
	b.WriteString("| 修改项1 | （修改后文本） | （原说明书/权利要求书原文） | （段落/行号） |\n")
	b.WriteString("| 修改项2 | （修改后文本） | （原说明书/权利要求书原文） | （段落/行号） |\n")
	b.WriteString("| 修改项3 | （修改后文本） | （原说明书/权利要求书原文） | （段落/行号） |\n")

	b.WriteString("\n### 法律论证\n\n")
	b.WriteString("根据专利法第33条和《专利审查指南》第二部分第八章第5.2节的规定，对申请文件的修改不得超出原说明书和权利要求书记载的范围。\n\n")
	b.WriteString("**判断标准**：修改内容是否能够从原说明书和权利要求书直接且毫无疑义地确定。\n\n")
	b.WriteString("**论证要点**：\n")
	b.WriteString("1. 上述修改内容在原申请文件中有明确记载，属于**直接且毫无疑义地确定**的内容\n")
	b.WriteString("2. 修改仅限于**消除驳回决定所指出的缺陷**，符合专利法实施细则第60条第1款关于复审修改范围的限制\n")
	b.WriteString("3. 修改未引入新的技术内容，未扩大保护范围\n\n")

	b.WriteString("### 结论\n\n")
	b.WriteString("综上所述，本次复审修改符合专利法第33条和专利法实施细则第60条第1款的规定，未超出原说明书和权利要求书记载的范围。\n")

	return b.String()
}

// preparePossibleChallenges generates a preview of questions the examiner
// panel may raise during the oral hearing, organized by rejection ground.
func preparePossibleChallenges(grounds []ReexamGround) string {
	var b strings.Builder

	b.WriteString("基于驳回决定中的审查意见，以下为合议组在口审中可能提出的质疑点预演：\n\n")

	for i, g := range grounds {
		fmt.Fprintf(&b, "### 质疑 %d：%s（%s）\n\n", i+1, g.Description, g.Article)

		switch g.Type {
		case ReexamGroundNovelty:
			b.WriteString("- Q1: 区别技术特征是否已被对比文件**隐含公开**？\n")
			b.WriteString("- Q2: 该区别技术特征是否属于本领域的**惯用手段**或**公知常识**？\n")
			b.WriteString("- Q3: 如果将对比文件与其他公知文献结合，是否可以得出权利要求的技术方案？\n")
			b.WriteString("- Q4: 请求人是否提交了修改后的权利要求？修改是否足以克服新颖性缺陷？\n")
			b.WriteString("\n**准备方向**：逐项对比技术特征，准备充分的技术对比证据，强调未被公开的区别特征。\n\n")

		case ReexamGroundInventiveness:
			b.WriteString("- Q1: 区别技术特征**实际解决的技术问题**是否被正确认定？\n")
			b.WriteString("- Q2: 现有技术整体上是否存在将区别特征应用到最接近现有技术的**技术启示**？\n")
			b.WriteString("- Q3: 技术效果是否为**可预料**的技术效果？是否存在**预料不到的技术效果**？\n")
			b.WriteString("- Q4: 辅助审查因素（商业成功、长期需求、他人失败）在本案中是否成立？\n")
			b.WriteString("- Q5: 审查员是否考虑了**正确的三步法**分析框架？\n")
			b.WriteString("\n**准备方向**：重点准备三步法反驳，尤其是技术启示的论证，准备实验数据证明预料不到的技术效果。\n\n")

		case ReexamGroundDisclosure:
			b.WriteString("- Q1: 说明书记载的内容是否足以使本领域技术人员**能够实现**该发明？\n")
			b.WriteString("- Q2: 哪些技术手段属于本领域技术人员的**常规实验范围**？说明书是否需要进一步细化？\n")
			b.WriteString("- Q3: 如涉及参数或效果数据，这些数据是否足以证明技术方案的可实现性？\n")
			b.WriteString("- Q4: 说明书中是否遗漏了实现发明所必需的技术内容？\n")
			b.WriteString("\n**准备方向**：准备补充实验数据或理论论证，证明本领域技术人员按说明书教导能够实现该发明。\n\n")

		case ReexamGroundClarity:
			b.WriteString("- Q1: 权利要求中的特定术语在说明书中有无**明确定义**或示例？\n")
			b.WriteString("- Q2: 权利要求的保护范围在阅读说明书和附图后是否能够**合理确定**？\n")
			b.WriteString("- Q3: 从属权利要求之间的引用关系是否存在不清楚之处？\n")
			b.WriteString("- Q4: 权利要求是否得到了说明书的**支持**（即权利要求范围与说明书公开的内容相适应）？\n")
			b.WriteString("\n**准备方向**：逐一确认每个有争议术语在说明书中的定义或示例，必要时提交修改后的权利要求。\n\n")

		case ReexamGroundAmendment:
			b.WriteString("- Q1: 修改内容是否能从原说明书和权利要求书中**直接且毫无疑义地**确定？\n")
			b.WriteString("- Q2: 修改是否引入了原申请文件**未记载**的技术内容？\n")
			b.WriteString("- Q3: 修改是否属于复审程序中允许的修改类型（**仅限于消除驳回缺陷**）？\n")
			b.WriteString("- Q4: 修改是否导致了保护范围的**扩大**？\n")
			b.WriteString("\n**准备方向**：准备修改前后的文本逐项对比，标注每处修改在原申请文件中的出处。\n\n")

		case ReexamGroundSubject:
			b.WriteString("- Q1: 权利要求限定的技术方案是否属于对产品的**形状、构造或其结合**？\n")
			b.WriteString("- Q2: 是否存在不属于实用新型保护客体的**方法、功能或材料**特征？\n")
			b.WriteString("- Q3: 如果存在非客体特征，能否通过修改将其删除或限缩？\n")
			b.WriteString("\n**准备方向**：准备基于产品结构特征的论证，考虑删除或限缩非客体特征。\n\n")
		}
	}

	b.WriteString("### 应考策略\n\n")
	b.WriteString("- **书面意见准备**：将上述质疑点的书面应答提前准备完整，口审时可提交书面补充意见\n")
	b.WriteString("- **证据准备**：准备好所有对比文件的技术特征对照表及关键段落摘录\n")
	b.WriteString("- **修改方案备选**：准备至少一套备选修改方案，以应对口审中合议组提出的新观点\n")
	b.WriteString("- **合议组背景**：了解合议组的技术背景和审查风格，有针对性地准备\n")
	b.WriteString("- **记录要点**：口审中的重要问题和答复要点应当场记录，以便后续补充书面意见\n")

	return b.String()
}

// prepareStatementOutline generates a timeline-based outline for oral
// hearing陈述 (statement), organized into four phases.
func prepareStatementOutline(grounds []ReexamGround) string {
	var b strings.Builder

	b.WriteString("### 口审陈述时间线\n\n")

	b.WriteString("#### 第一阶段：开场（约3-5分钟）\n\n")
	b.WriteString("1. **请求人自我介绍及身份确认**\n")
	b.WriteString("   - 说明请求人、代理机构、代理师姓名及执业资格\n")
	b.WriteString("2. **确认收到驳回决定**及相关对比文件\n")
	b.WriteString("3. **简要说明复审请求的核心观点**（不超过1分钟）\n")
	b.WriteString("4. 如有修改，简要说明修改类型和范围\n\n")

	b.WriteString("#### 第二阶段：复审理由陈述（约10-15分钟）\n\n")

	for i, g := range grounds {
		action := "论证"
		switch g.Type {
		case ReexamGroundNovelty:
			action = "论证新颖性"
		case ReexamGroundInventiveness:
			action = "论证创造性"
		case ReexamGroundDisclosure:
			action = "论证充分公开"
		case ReexamGroundClarity:
			action = "论证清楚/支持"
		case ReexamGroundAmendment:
			action = "论证修改合规"
		case ReexamGroundSubject:
			action = "论证客体合规"
		}
		fmt.Fprintf(&b, "%d. **%s**——%s\n", i+1, g.Description, action)
		b.WriteString("   - 指出驳回决定中的事实认定错误或法律适用不当\n")
		b.WriteString("   - 提出复审理由的核心论据和对比文件分析\n")
		b.WriteString("   - 引用对比文件中的具体段落或附图（如适用）\n")
		b.WriteString("   - 总结本项理由为什么能够成立\n\n")
	}

	b.WriteString("#### 第三阶段：合议组提问与应答（约15-20分钟）\n\n")
	b.WriteString("1. **认真听取合议组问题**，确认理解后再回答\n")
	b.WriteString("2. 回答时围绕以下核心论点展开：\n")
	b.WriteString("   - 技术方案的区别特征和独特技术效果\n")
	b.WriteString("   - 现有技术整体上未给出技术启示（针对创造性）\n")
	b.WriteString("   - 修改内容的原申请文件出处（针对A33修改合规性）\n")
	b.WriteString("   - 说明书记载的内容足以使本领域技术人员能够实现（针对A26.3）\n")
	b.WriteString("3. **对于不确定的问题**：\n")
	b.WriteString("   - 不仓促回答，可申请记录在案，在指定期限内提交书面补充意见\n")
	b.WriteString("   - 对于合议组提出的新对比文件或新理由，可请求给予答辩期限\n\n")

	b.WriteString("#### 第四阶段：总结陈述（约5分钟）\n\n")
	b.WriteString("1. **重申复审请求的核心立场**：驳回决定的事实认定和/或法律适用存在错误\n")
	b.WriteString("2. **强调技术方案的贡献和进步**：本发明相对于现有技术做出了实质性的改进\n")
	b.WriteString("3. **表达合作意愿**：愿意配合合议组进一步提供补充材料和说明\n")
	b.WriteString("4. **明确请求**：请求合议组撤销驳回决定，或发回原审查部门继续审查\n\n")

	b.WriteString("### 注意事项\n\n")
	b.WriteString("- **态度**：陈述时应保持专业、客观、尊重的态度，避免对抗性语言\n")
	b.WriteString("- **新观点处理**：对于合议组在口审中提出的新观点，切勿当场仓促回答，可要求给予时间准备书面意见\n")
	b.WriteString("- **口审记录**：口审记录将作为复审决定的依据，重要观点务必要求记录在案\n")
	b.WriteString("- **文件准备**：带齐所有原始文件——驳回决定、对比文件、修改替换页、复审请求书\n")
	b.WriteString("- **时间管理**：合理分配各环节时间，优先保证核心论证点的陈述质量\n")

	return b.String()
}

// =============================================================================
// Graph Builder
// =============================================================================

// BuildReexaminationGraph constructs a Pregel graph for reexamination request drafting.
//
// Graph structure (without options):
//
//	parse_decision → classify_grounds → draft_request → rule_check → conclude → __end__
//
// Graph structure (with WithOralHearingPrep):
//
//	parse_decision → classify_grounds → draft_request → oral_hearing_prep → rule_check → conclude → __end__
func BuildReexaminationGraph(opts ...ReexamGraphOption) (*graph.CompiledPregelGraph, error) {
	cfg := &reexamGraphConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

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

	// Conditionally insert oral hearing preparation node.
	if cfg.oralHearing {
		if err := g.AddNode("oral_hearing_prep", reexamOralHearingNode); err != nil {
			return nil, err
		}
	}

	if err := g.AddNode("rule_check", reexamRuleCheckNode); err != nil {
		return nil, err
	}
	if err := g.AddNode("conclude", reexamConcludeNode); err != nil {
		return nil, err
	}

	// Build edges.
	edges := [][2]string{
		{"parse_decision", "classify_grounds"},
		{"classify_grounds", "draft_request"},
	}
	if cfg.oralHearing {
		edges = append(edges, [][2]string{
			{"draft_request", "oral_hearing_prep"},
			{"oral_hearing_prep", "rule_check"},
		}...)
	} else {
		edges = append(edges, [2]string{"draft_request", "rule_check"})
	}
	edges = append(edges, [][2]string{
		{"rule_check", "conclude"},
		{"conclude", graph.PregelEnd},
	}...)

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
