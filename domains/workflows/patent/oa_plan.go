package patent

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 答复计划的确定性拼装：逐驳回理由的必答段落 + 逐权项的修改对照 + 无法从通知书
// 确定的缺口。全部来自闭集对照表，不调用模型、不生成论证内容。
//
// 与旧实现的差异（旧实现见 oa_helpers.go 的 claimAmendmentType / amendmentBasis）：
//   - 修改对照逐权项出行。旧实现只取「受影响权项中最靠前的一个编号」判定修改动作，
//     再在「涉及权利要求」列列出全部权项，于是从属权利要求的动作从不生效，且列出的
//     权项与判定的动作不对应。
//   - 修改对象不明时报缺口而不兜底。旧实现在未抽出权项编号时默认按权利要求 1 出表，
//     等于把「不知道改哪条」当成了「改权利要求 1」。
//   - 无驳回理由时产出缺口说明，而不是空骨架。

// OAClaimKind 是参与修改对照的权项类型。
type OAClaimKind string

// Claim kind constants.
const (
	OAClaimIndependent OAClaimKind = "independent"
	OAClaimDependent   OAClaimKind = "dependent"
)

// 答复策略取值。
const (
	StrategyArgument  = "argument"
	StrategyAmendment = "amendment"
	StrategyCombined  = "combined"
)

// 修改动作取值。
const (
	AmendmentClarify         = "clarify"
	AmendmentNarrow          = "narrow"
	AmendmentDelete          = "delete"
	AmendmentAdjustReference = "adjust-reference"
	AmendmentNone            = "none"
)

// 缺口类型。
const (
	PlanGapNoGround               = "no-ground-detected"
	PlanGapAmendmentWithoutClaims = "amendment-ground-without-claims"
	PlanGapIndependentAbsent      = "independent-claim-absent"
)

// OAPlanSection 是单条驳回理由的答复段落骨架。
type OAPlanSection struct {
	Ground   OaRejectionType
	Strategy string
	Sections []string
}

// OAPlanAmendmentRow 是一条修改对照行。
type OAPlanAmendmentRow struct {
	Claim  int
	Kind   OAClaimKind
	Ground OaRejectionType
	Action string
	Basis  string
}

// OAPlanGap 是一处无法从通知书确定下来的缺口。
type OAPlanGap struct {
	Kind   string
	Ground OaRejectionType
	Detail string
}

// OAResponsePlan 是答复计划。
type OAResponsePlan struct {
	StrategySummary string
	Sections        []OAPlanSection
	AmendmentRows   []OAPlanAmendmentRow
	ClaimsToAmend   []int
	ClaimsMentioned []int
	Gaps            []OAPlanGap
}

// 驳回类型 → 答复策略。新颖性/创造性/公开不充分以争辩为主；不清楚、不支持、
// 修改超范围、形式缺陷以修改为主；未识别到的类型按「争辩+修改」并标缺口。
var oaRejectionStrategy = map[OaRejectionType]string{
	OaNovelty:       StrategyArgument,
	OaInventiveness: StrategyArgument,
	OaDisclosure:    StrategyArgument,
	OaClarity:       StrategyAmendment,
	OaSupport:       StrategyAmendment,
	OaScope:         StrategyAmendment,
	OaFormal:        StrategyAmendment,
}

// 驳回类型 → 修改动作（按独立/从属分列）。争辩型理由不产生修改。
var oaRejectionAmendmentActions = map[OaRejectionType]map[OAClaimKind]string{
	OaNovelty:       {OAClaimIndependent: AmendmentNone, OAClaimDependent: AmendmentNone},
	OaInventiveness: {OAClaimIndependent: AmendmentNone, OAClaimDependent: AmendmentNone},
	OaDisclosure:    {OAClaimIndependent: AmendmentNone, OAClaimDependent: AmendmentNone},
	OaClarity:       {OAClaimIndependent: AmendmentClarify, OAClaimDependent: AmendmentAdjustReference},
	OaSupport:       {OAClaimIndependent: AmendmentNarrow, OAClaimDependent: AmendmentNarrow},
	OaScope:         {OAClaimIndependent: AmendmentNarrow, OAClaimDependent: AmendmentDelete},
	OaFormal:        {OAClaimIndependent: AmendmentClarify, OAClaimDependent: AmendmentAdjustReference},
}

// 驳回类型 → 修改依据（或争辩型理由的答复依据）。
var oaRejectionBasis = map[OaRejectionType]string{
	OaNovelty:       "专利法第22条第2款（新颖性，以争辩为主）",
	OaInventiveness: "专利法第22条第3款（创造性，以争辩为主）",
	OaDisclosure:    "专利法第26条第3款（充分公开）",
	OaClarity:       "专利法第26条第4款（清楚）",
	OaSupport:       "专利法第26条第4款（支持）",
	OaScope:         "专利法第33条（修改不超出原说明书和权利要求书记载的范围）",
	OaFormal:        "《专利审查指南》关于申请文件格式与撰写形式的规定",
}

// 未识别类型（含 other）的兜底文案。
const (
	unknownGroundBasis    = "未识别到具体驳回条款，依据须人工判读"
	unknownGroundSection  = "逐条回应审查意见（驳回条款须人工判读）"
	unknownGroundStrategy = StrategyCombined
)

// 驳回类型 → 意见陈述必答段落。
var oaRejectionSections = map[OaRejectionType][]string{
	OaNovelty:       {"区别技术特征的认定", "单独对比（单篇文件是否公开全部技术特征）"},
	OaInventiveness: {"最接近的现有技术", "区别技术特征与实际解决的技术问题", "技术启示（非显而易见）"},
	OaDisclosure:    {"说明书公开的内容", "本领域技术人员能否实现"},
	OaClarity:       {"修改内容与理由", "修改后用语含义唯一确定"},
	OaSupport:       {"修改后权利要求的说明书依据", "上位概括与所公开技术效果的对应"},
	OaScope:         {"修改内容的原申请文件出处", "未引入新技术内容、未扩大保护范围"},
	OaFormal:        {"缺陷逐项修正说明"},
}

// amendmentActionLabel 返回修改动作的中文标签。
func amendmentActionLabel(action string) string {
	switch action {
	case AmendmentClarify:
		return "澄清限定"
	case AmendmentNarrow:
		return "限缩"
	case AmendmentDelete:
		return "删除"
	case AmendmentAdjustReference:
		return "从属引用调整"
	case AmendmentNone:
		return "无需修改"
	default:
		return action
	}
}

func oaClaimKindLabel(kind OAClaimKind) string {
	if kind == OAClaimIndependent {
		return "独立"
	}
	return "从属"
}

func strategyFor(ground OaRejectionType) string {
	if strategy, ok := oaRejectionStrategy[ground]; ok {
		return strategy
	}
	return unknownGroundStrategy
}

func isAmendmentStrategy(strategy string) bool {
	return strategy == StrategyAmendment || strategy == StrategyCombined
}

func amendmentActionFor(ground OaRejectionType, kind OAClaimKind) string {
	actions, ok := oaRejectionAmendmentActions[ground]
	if !ok {
		return AmendmentNone
	}
	return actions[kind]
}

func basisFor(ground OaRejectionType) string {
	if basis, ok := oaRejectionBasis[ground]; ok {
		return basis
	}
	return unknownGroundBasis
}

// buildOAResponsePlan 由通知书解析结果拼装答复计划。
// independentClaims 为独立权利要求编号；未给出的权项按从属处理并报缺口。
func buildOAResponsePlan(notice *ParsedOfficeAction, independentClaims []int) OAResponsePlan {
	plan := OAResponsePlan{
		ClaimsMentioned: append([]int(nil), notice.AffectedClaims...),
	}
	for _, ground := range notice.RejectionTypes {
		strategy := strategyFor(ground)
		sections, ok := oaRejectionSections[ground]
		if !ok {
			sections = []string{unknownGroundSection}
		}
		plan.Sections = append(plan.Sections, OAPlanSection{
			Ground: ground, Strategy: strategy, Sections: sections,
		})
		if !isAmendmentStrategy(strategy) {
			continue
		}
		if len(notice.AffectedClaims) == 0 {
			plan.Gaps = append(plan.Gaps, OAPlanGap{
				Kind: PlanGapAmendmentWithoutClaims, Ground: ground,
				Detail: fmt.Sprintf("%s须修改权利要求，但通知书未抽出权项编号，修改对象须人工确认",
					rejectionTypeLabel(ground)),
			})
			continue
		}
		for _, claim := range notice.AffectedClaims {
			kind := OAClaimDependent
			if containsInt(independentClaims, claim) {
				kind = OAClaimIndependent
			}
			plan.AmendmentRows = append(plan.AmendmentRows, OAPlanAmendmentRow{
				Claim: claim, Kind: kind, Ground: ground,
				Action: amendmentActionFor(ground, kind), Basis: basisFor(ground),
			})
		}
	}
	if len(notice.RejectionTypes) == 0 {
		plan.Gaps = append(plan.Gaps, OAPlanGap{
			Kind: PlanGapNoGround,
			Detail: fmt.Sprintf("通知书未识别到具体驳回条款（rejectionType=%s），答复依据须人工判读",
				notice.RejectionType),
		})
	}
	if len(plan.AmendmentRows) > 0 && !hasIndependentRow(plan.AmendmentRows) {
		claims := make([]string, 0, len(plan.AmendmentRows))
		for _, row := range plan.AmendmentRows {
			claims = append(claims, fmt.Sprintf("%d", row.Claim))
		}
		plan.Gaps = append(plan.Gaps, OAPlanGap{
			Kind: PlanGapIndependentAbsent,
			Detail: fmt.Sprintf("修改类驳回涉及的权项（%s）中没有独立权利要求被点出，"+
				"修改动作按从属权利要求取值，请确认独立权利要求是否也需要修改",
				strings.Join(claims, "、")),
		})
	}
	plan.ClaimsToAmend = sortedUniqueClaims(plan.AmendmentRows)
	plan.StrategySummary = summarizePlanStrategies(plan.Sections)
	return plan
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasIndependentRow(rows []OAPlanAmendmentRow) bool {
	for _, row := range rows {
		if row.Kind == OAClaimIndependent {
			return true
		}
	}
	return false
}

func sortedUniqueClaims(rows []OAPlanAmendmentRow) []int {
	seen := make(map[int]bool, len(rows))
	claims := make([]int, 0, len(rows))
	for _, row := range rows {
		if seen[row.Claim] {
			continue
		}
		seen[row.Claim] = true
		claims = append(claims, row.Claim)
	}
	sort.Ints(claims)
	return claims
}

func summarizePlanStrategies(sections []OAPlanSection) string {
	if len(sections) == 0 {
		return "综合答复"
	}
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		parts = append(parts, fmt.Sprintf("%s→%s",
			rejectionTypeLabel(section.Ground), strategyLabel(section.Strategy)))
	}
	return strings.Join(parts, "、")
}

// 从属权利要求的引用措辞：「根据权利要求 1 所述」「如权利要求 1 或 2 所述」一类。
var dependentClaimReferenceRE = regexp.MustCompile(
	`(?:根据|如|按照|依据|同)\s*(?:上述|前述|以上)?\s*权利要求\s*\d+|权利要求\s*\d+(?:\s*[-–~至或、,，和]\s*\d+)*\s*(?:中任一项)?\s*所述`)

var claimHeadingRE = regexp.MustCompile(`(?:^|\n)\s*(\d+)\s*[.．、]\s*`)

// detectIndependentClaims 从权利要求书正文保守检测独立权利要求编号。
//
// 逐条按行首编号（`1. …`）切分；条内出现「根据权利要求……」一类引用措辞的按从属
// 处理。检测只作缺省填充：正文缺失或切分不出条目时返回空，由
// PlanGapIndependentAbsent 提示人工确认。
func detectIndependentClaims(claimsText string) []int {
	type entry struct {
		claim        int
		headingStart int
		bodyStart    int
	}
	matches := claimHeadingRE.FindAllStringSubmatchIndex(claimsText, -1)
	entries := make([]entry, 0, len(matches))
	for _, match := range matches {
		claim, err := strconv.Atoi(claimsText[match[2]:match[3]])
		if err != nil || claim < 1 {
			continue
		}
		entries = append(entries, entry{claim: claim, headingStart: match[0], bodyStart: match[1]})
	}
	if len(entries) == 0 {
		return nil
	}
	independent := make([]int, 0, len(entries))
	for i, current := range entries {
		end := len(claimsText)
		if i+1 < len(entries) {
			end = entries[i+1].headingStart
		}
		if current.bodyStart > end {
			continue
		}
		body := claimsText[current.bodyStart:end]
		if !dependentClaimReferenceRE.MatchString(body) {
			independent = append(independent, current.claim)
		}
	}
	return sortedUniqueInts(independent)
}

func sortedUniqueInts(values []int) []int {
	seen := make(map[int]bool, len(values))
	out := make([]int, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}
