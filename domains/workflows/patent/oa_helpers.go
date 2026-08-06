package patent

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
)

// extractExaminerArguments extracts the examiner's reasoning sentences from
// the OA text by splitting on common argument markers.
func extractExaminerArguments(text string) []string {
	markers := []string{"审查员认为", termPriorArtDoc, "本领域技术人员", "因此", "所以", "综上"}
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

// rejectionTypesFromState extracts the ordered rejection type list from state,
// falling back to the single primary type for backward compatibility.
func rejectionTypesFromState(state graph.PregelState) []OaRejectionType {
	if types, ok := state[OAStateRejectionTypes].([]OaRejectionType); ok && len(types) > 0 {
		return types
	}
	if t := state.GetString(OAStateRejectionType); t != "" {
		return []OaRejectionType{OaRejectionType(t)}
	}
	return nil
}

// determineResponseStrategies returns one response strategy per rejection type,
// preserving the order of types.
func determineResponseStrategies(types []OaRejectionType) []string {
	out := make([]string, len(types))
	for i, t := range types {
		out[i] = determineResponseStrategy(string(t))
	}
	return out
}

// selectOATemplates returns one doc template name per rejection type,
// preserving the order of types.
func selectOATemplates(types []OaRejectionType, strategies []string) []string {
	out := make([]string, len(types))
	for i, t := range types {
		strategy := ""
		if i < len(strategies) {
			strategy = strategies[i]
		}
		out[i] = selectOATemplate(string(t), strategy)
	}
	return out
}

// determineResponseStrategy picks the response strategy based on rejection type.
func determineResponseStrategy(rejectionType string) string {
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

// rejectionTypeLabel renders a rejection type as a Chinese label for tables/headings.
func rejectionTypeLabel(t OaRejectionType) string {
	switch t {
	case OaNovelty:
		return "新颖性"
	case OaInventiveness:
		return "创造性"
	case OaClarity:
		return "不清楚"
	case OaSupport:
		return "不支持"
	case OaDisclosure:
		return "公开不充分"
	case OaScope:
		return "修改超范围"
	case OaFormal:
		return "形式问题"
	default:
		return string(t)
	}
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

// summarizeStrategies renders a compact "类型→策略" summary for the strategy header,
// e.g. "新颖性→争辩、创造性→争辩、不清楚→修改".
func summarizeStrategies(types []OaRejectionType, strategies []string) string {
	var parts []string
	for i, rt := range types {
		strategy := ""
		if i < len(strategies) {
			strategy = strategies[i]
		}
		parts = append(parts, fmt.Sprintf("%s→%s", rejectionTypeLabel(rt), strategyLabel(strategy)))
	}
	if len(parts) == 0 {
		return "综合答复"
	}
	return strings.Join(parts, "、")
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

// cnNumeral renders 1-based section numbers as Chinese numerals (一、二、三…).
func cnNumeral(n int) string {
	digits := []string{"一", "二", "三", "四", "五", "六", "七", "八", "九", "十"}
	if n >= 1 && n <= len(digits) {
		return digits[n-1]
	}
	return fmt.Sprintf("%d", n)
}

// draftResponseBodies renders one argument section per rejection type,
// numbered sequentially, then closes with a single conclusion.
func draftResponseBodies(types []OaRejectionType) string {
	var b strings.Builder
	for i, rt := range types {
		b.WriteString(draftResponseBody(rt, i+1))
		b.WriteString("\n")
	}
	b.WriteString("### 结论\n\n")
	b.WriteString("综上所述，修改后的权利要求克服了审查意见指出的缺陷，请求审查员在修改文本的基础上继续审查。\n")
	return b.String()
}

// draftResponseBody generates the core argument section for a single rejection type.
// sectionNo is 1-based and rendered as a Chinese numeral in the heading.
func draftResponseBody(rejectionType OaRejectionType, sectionNo int) string {
	var b strings.Builder
	numeral := cnNumeral(sectionNo)

	switch rejectionType {
	case OaNovelty:
		fmt.Fprintf(&b, "### %s、关于新颖性（专利法第22条第2款）\n\n", numeral)
		b.WriteString("审查员认为本申请相对于对比文件不具备新颖性。申请人认为该审查意见不能成立。\n\n")
		b.WriteString("#### 区别特征分析\n\n")
		b.WriteString("经逐项比对，对比文件至少未公开以下技术特征：\n\n")
		b.WriteString("- [特征1]：[分析说明]\n")
		b.WriteString("- [特征2]：[分析说明]\n\n")
		b.WriteString("#### 单独对比原则\n\n")
		b.WriteString("根据审查指南第二部分第三章的规定，新颖性判断应遵循单独对比原则。")
		b.WriteString("对比文件未公开权利要求1的全部技术特征，因此权利要求1具备新颖性。\n")

	case OaInventiveness:
		fmt.Fprintf(&b, "### %s、关于创造性（专利法第22条第3款）\n\n", numeral)
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
		fmt.Fprintf(&b, "### %s、关于权利要求不清楚（专利法第26条第4款）\n\n", numeral)
		b.WriteString("针对审查员指出的不清楚之处，申请人已作出相应修改：\n\n")
		b.WriteString("- [修改项1]：[说明]\n")
		b.WriteString("- [修改项2]：[说明]\n\n")
		b.WriteString("修改后的权利要求清楚地限定了保护范围。\n")

	case OaSupport:
		fmt.Fprintf(&b, "### %s、关于权利要求得不到说明书支持（专利法第26条第4款）\n\n", numeral)
		b.WriteString("针对审查员的意见，申请人已将原权利要求的[上位概念]限缩为说明书[具体段落]明确记载的[具体实施方式]。\n\n")
		b.WriteString("修改后的权利要求得到说明书的充分支持。\n")

	default:
		fmt.Fprintf(&b, "### %s、关于审查意见的答复\n\n", numeral)
		b.WriteString("针对审查意见通知书中指出的问题，申请人逐条答复如下：\n\n")
		b.WriteString("[逐条答辩内容]\n")
	}

	return b.String()
}

// formatClaimList renders claim numbers as a compact list ("1, 2, 3"), or "—"
// when the list is empty.
func formatClaimList(claims []int) string {
	if len(claims) == 0 {
		return "—"
	}
	parts := make([]string, len(claims))
	for i, c := range claims {
		parts[i] = fmt.Sprintf("%d", c)
	}
	return strings.Join(parts, ", ")
}
