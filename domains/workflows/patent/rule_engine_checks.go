package patent

import (
	"fmt"
	"regexp"
	"strings"
)

// evaluateRule dispatches to the type-specific checker and returns
// (passed, detailMessage). After type-specific checking it validates
// PathElements (reasoning step completeness) if the rule defines them.
func evaluateRule(rule CheckRule, text string) (bool, string) {
	var passed bool
	var detail string

	switch rule.CheckType {
	case CheckNovelty:
		passed, detail = checkNovelty(text, rule)
	case CheckInventiveness:
		passed, detail = checkInventiveness(text, rule)
	case CheckInfringement:
		passed, detail = checkInfringement(text, rule)
	case CheckDisclosure:
		passed, detail = checkDisclosure(text, rule)
	case CheckClaimAnalysis:
		passed, detail = checkClaimAnalysis(text, rule)
	case CheckDesignComparison:
		passed, detail = checkDesignComparison(text, rule)
	case CheckPublicAccess:
		passed, detail = checkPublicAccess(text, rule)
	case CheckAmendmentScope:
		passed, detail = checkAmendmentScope(text, rule)
	case CheckSubjectMatter:
		passed, detail = checkSubjectMatter(text, rule)
	default:
		passed = true
	}

	if !passed {
		return false, detail
	}

	// Post-processing: validate reasoning path step completeness.
	if len(rule.PathElements) > 0 {
		pathOk, pathDetail := checkReasoningPath(text, rule)
		if !pathOk {
			return false, pathDetail
		}
	}
	return true, ""
}

func checkNovelty(text string, rule CheckRule) (bool, string) {
	if !matchKeywordsAll(text, rule.RequiredElements) {
		return false, "新颖性分析缺少必要要素（如单独对比、现有技术认定）"
	}
	if rule.SingleComparison {
		for _, bp := range singleComparisonBanPhrases {
			if strings.Contains(text, bp) {
				return false, "新颖性分析违反单独对比原则：不应将多份对比文件结合"
			}
		}
	}
	return true, ""
}

func checkInventiveness(text string, rule CheckRule) (bool, string) {
	if len(rule.StepElements) < 3 {
		return true, ""
	}
	for i := 0; i < 3; i++ {
		if !matchKeywordsAny(text, rule.StepElements[i]) {
			return false, "创造性分析缺少三步法必要步骤（最接近现有技术→区别技术特征→技术启示）"
		}
	}
	return true, ""
}

func checkInfringement(text string, rule CheckRule) (bool, string) {
	if !matchKeywordsAll(text, rule.RequiredElements) {
		return false, "侵权分析缺少必要对比要素（如全面覆盖、技术特征比对）"
	}
	return true, ""
}

func checkDisclosure(text string, rule CheckRule) (bool, string) {
	if !matchKeywordsAll(text, rule.RequiredAspects) {
		return false, "充分公开分析缺少必要审查维度（如能够实现、技术效果）"
	}
	return true, ""
}

func checkClaimAnalysis(text string, rule CheckRule) (bool, string) {
	for _, dim := range rule.Dimensions {
		patterns, ok := claimDimensionPatterns[dim]
		if !ok {
			continue
		}
		if !matchKeywordsAny(text, patterns) {
			return false, "权利要求分析缺少必要维度（清楚性/说明书支持/必要技术特征/一致性）"
		}
	}
	return true, ""
}

func checkDesignComparison(text string, rule CheckRule) (bool, string) {
	if !matchKeywordsAll(text, rule.RequiredElements) {
		return false, "外观设计对比分析缺少必要要素（如整体视觉效果、产品种类认定）"
	}
	return true, ""
}

func checkPublicAccess(text string, rule CheckRule) (bool, string) {
	if !matchKeywordsAll(text, rule.RequiredElements) {
		return false, "公开方式判断缺少必要要素（如公开方式认定、公开日核实）"
	}
	return true, ""
}

func checkAmendmentScope(text string, rule CheckRule) (bool, string) {
	if !matchKeywordsAll(text, rule.RequiredElements) {
		return false, "修改超范围分析缺少必要要素（如原申请文件范围、直接且毫无疑义的确定）"
	}
	return true, ""
}

func checkSubjectMatter(text string, rule CheckRule) (bool, string) {
	if !matchKeywordsAll(text, rule.RequiredElements) {
		return false, "保护客体分析缺少必要要素（如技术方案认定、排除客体分析）"
	}
	return true, ""
}

// checkReasoningPath validates that all reasoning path steps are present in text.
// Each PathElements[i] is a set of keywords for step i — at least one keyword from
// each step must be affirmatively matched for the path to be complete.
func checkReasoningPath(text string, rule CheckRule) (bool, string) {
	for i, step := range rule.PathElements {
		if !matchKeywordsAny(text, step) {
			return false, fmt.Sprintf("推理路径步骤%d不完整，缺少关键词：%s", i+1, strings.Join(step, "/"))
		}
	}
	return true, ""
}

// ----------------------------------------------------------------------------
// Keyword matching utilities (ported from keyword-utils.ts)
// ----------------------------------------------------------------------------

// synonymMap expands a keyword to its synonyms for more robust matching.
var synonymMap = map[string][]string{
	termNovelty:              {"新创性", "未公开", "不属于现有技术", "未被披露"},
	"创造性":                    {termNonObvious, "发明高度", "创造性步骤", "inventive step"},
	termPriorArtDoc:          {"现有技术", "在先技术", "引用文件", "文献", "reference"},
	termClaimsLocal:          {"权项", categoryClaims, "保护范围"},
	"说明书":                    {"specification", "申请文件"},
	termSufficientDisclosure: {termSufficientDisclosurePub, termEnable, "enablement"},
	"三步法":                    {termClosestPriorArtFull, termDistinguishingFeatures, termTechHint},
	"单独对比":                   {"单独对比原则", "一一对比"},
	termCommonKnowledge:      {termCommonTechnicalMeans, termConventionalDesign, "common knowledge", "well-known"},
	// Infringement domain terms.
	"全面覆盖":          {"全部技术特征", "逐一比对", "全覆盖原则"},
	"等同":            {"等同替换", "等同侵权", "基本相同的手段", "基本相同的功能", "基本相同的效果"},
	"禁止反悔":          {"审查过程禁反言", "prosecution history estoppel", "修改导致放弃"},
	"捐献规则":          {"捐献原则", "dedicated to the public"},
	termTechFeature: {"技术特征分解", "权项特征", "limitation"},
	// Invalidation domain terms.
	"无效宣告":                    {"无效请求", "宣告无效", "invalidation"},
	termCombinationMotivation: {termCombinationHint, "有动机结合", "技术结合启示", termTechHint},
	termPriorityDate:          {termPriority, termFilingDateLit, "filing date"},
	// Reexamination domain terms.
	"复审":   {"复审请求", "驳回复审", "reexamination"},
	"程序违法": {"程序错误", "违反法定程序"},
	"新证据":  {"补充证据", "新提交的证据"},
	// Design comparison terms (外观设计).
	termDesignPatent: {"工业设计", "design", "industrial design", "外观"},
	"整体视觉效果":         {"视觉效果", "整体外观", "整体视觉", "overall visual effect"},
	"产品种类":           {"产品类别", "产品类型", "相似种类", "同类产品"},
	// Public access terms (公开方式).
	"出版物公开": {"公开出版", "论文", "期刊", "杂志", "书籍"},
	"使用公开":  {"公开使用", "销售公开", "展出", "公开实施"},
	"互联网公开": {"网络公开", "在线公开", "网页公开", "网站公开"},
	"公开方式":  {"公开途径", "公开形式", "公开类型"},
	// Amendment scope terms (修改超范围).
	termAmendmentExceed: {"超出原范围", "增加新内容", "超范围修改", "amendment beyond scope", "超范围"},
	"直接且毫无疑义":           {"直接毫无疑义", "直接确定", "原申请文件"},
	// Subject matter terms (保护客体).
	"技术方案":           {"技术方案本身", "technical solution"},
	"保护客体":           {"可专利主题", "patentable subject matter", "授权客体"},
	"智力活动规则":         {"智力活动的规则", "数学方法", "商业规则", "mental activity", "抽象思想"},
	"疾病诊断方法":         {"诊断方法", "治疗方法", "手术方法"},
	termSciDiscovery: {"科学发现", "自然规律", "自然法则", "natural law"},
	// Reasoning pattern terms (推理模式).
	"预料不到":                  {"预料不到的技术效果", "出乎意料", "surprising", "unexpected"},
	"用途限定":                  {"用途特征", "用途限定", "use limitation"},
	"实验数据":                  {"实验数据", "实施例", "实验例", "药效数据"},
	termClosestPriorArtFull: {termClosestPriorArt, "最接近的对比文件"},
	"抵触申请":                  {"在先申请在后公开", "conflicting application"},
	"功能性限定":                 {"功能限定", "功能性特征", "functional limitation"},
	"实用性":                   {"工业实用性", "产业应用", "industrial applicability"},
	"积极效果":                  {termBeneficialEffect, "positive effect", "技术效果"},
	"本领域技术人员":               {"所属领域技术人员", "person skilled in the art"},
	termEnable:              {"可实施", "enablement", termCanMake, termCanUse},
	termObvious:             {"obvious", "显而易见性", termNonObvious},
	"转让":                    {"transfer", "assign", "assignment"},
}

// negationPatterns detect negated mentions within a context window.
var negationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`不具有`),
	regexp.MustCompile(`不构成`),
	regexp.MustCompile(`无法证明`),
	regexp.MustCompile(`缺少`),
	regexp.MustCompile(`未发现`),
	regexp.MustCompile(`没有公开`),
	regexp.MustCompile(`不满足`),
	regexp.MustCompile(`不符合`),
	regexp.MustCompile(`难以看出`),
	regexp.MustCompile(`不能证明`),
}

// singleComparisonBanPhrases are forbidden when SingleComparison is enforced.
var singleComparisonBanPhrases = []string{
	"多份对比文件结合", "多篇文献相结合", "对比文件1-3",
	"对比文件1、2和3", "结合对比文件1-",
}

// claimDimensionPatterns maps a claim-analysis dimension to its keyword set.
var claimDimensionPatterns = map[string][]string{
	dimClarity:           {"清楚", "清晰", "明确", "简要"},
	dimSupport:           {"以说明书为依据", "支持", "记载", "记载于", "说明书支持"},
	"essential_features": {"必要技术特征", "必要特征", "必不可少"},
	dimConsistency:       {"一致", "对应", "协调", "不矛盾"},
}

// matchKeyword checks whether a keyword (or any of its synonyms) is
// affirmatively mentioned in text (not negated). The context window is the
// 60 characters preceding the match.
func matchKeyword(text, keyword string) bool {
	candidates := []string{keyword}
	if syns, ok := synonymMap[keyword]; ok {
		candidates = append(candidates, syns...)
	}
	lower := strings.ToLower(text)
	for _, c := range candidates {
		idx := strings.Index(lower, strings.ToLower(c))
		if idx == -1 {
			continue
		}
		start := idx - 60
		if start < 0 {
			start = 0
		}
		before := text[start:idx]
		if !hasNegation(before) {
			return true
		}
	}
	return false
}

func hasNegation(before string) bool {
	for _, p := range negationPatterns {
		if p.MatchString(before) {
			return true
		}
	}
	return false
}

// matchKeywordsAll returns true only if every keyword is affirmatively matched.
func matchKeywordsAll(text string, keywords []string) bool {
	for _, k := range keywords {
		if !matchKeyword(text, k) {
			return false
		}
	}
	return true
}

// matchKeywordsAny returns true if at least one keyword is affirmatively matched.
func matchKeywordsAny(text string, keywords []string) bool {
	for _, k := range keywords {
		if matchKeyword(text, k) {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// Default rules — core Chinese patent law checks
// ----------------------------------------------------------------------------

// DefaultPatentRules returns a baseline rule set covering the most common
// patent examination checks under Chinese patent law (专利法第22条/第26条).
// It aggregates rules from all scenario-specific rule sets for backward
// compatibility. For targeted analysis, use the specific rule set functions
// (NoveltyRules, InfringementRules, etc.) instead.
func DefaultPatentRules() []CheckRule {
	rules := make([]CheckRule, 0, 11)
	rules = append(rules, NoveltyRules()...)
	rules = append(rules, InventivenessRules()...)
	rules = append(rules, DisclosureRules()...)
	rules = append(rules, InfringementRules()...)
	rules = append(rules, InvalidationRules()...)
	rules = append(rules, ReexaminationRules()...)
	rules = append(rules, ReasoningPatternRules()...)
	rules = append(rules, DesignRules()...)
	rules = append(rules, SubjectMatterRules()...)
	rules = append(rules, PublicAccessRules()...)
	rules = append(rules, PriorityRules()...)
	return rules
}

// ----------------------------------------------------------------------------
// Scenario-specific rule sets

// FormatRuleResults renders a slice of rule results as a Markdown report section.
func FormatRuleResults(results []RuleCheckResult, verdict Verdict) string {
	var b strings.Builder
	b.WriteString("## 规则引擎检查\n\n")
	b.WriteString("检查结论: ")
	switch verdict {
	case VerdictPass:
		b.WriteString("✅ 通过")
	case VerdictNeedsRevision:
		b.WriteString("⚠️ 需修改")
	case VerdictBlocked:
		b.WriteString("⛔ 阻断")
	}
	b.WriteString("\n\n")

	if len(results) == 0 {
		b.WriteString("所有规则检查均通过。\n")
		return b.String()
	}

	b.WriteString("| 规则 | 级别 | 严重度 | 问题 | 修改建议 |\n")
	b.WriteString("|------|------|--------|------|----------|\n")
	for _, r := range results {
		b.WriteString("| ")
		b.WriteString(r.RuleName)
		b.WriteString(" | ")
		b.WriteString(levelLabel(r.Level))
		b.WriteString(" | ")
		b.WriteString(string(r.Severity))
		b.WriteString(" | ")
		b.WriteString(r.Message)
		b.WriteString(" | ")
		b.WriteString(r.FixSuggestion)
		b.WriteString(" |\n")
	}
	return b.String()
}

func levelLabel(l RuleLevel) string {
	switch l {
	case LevelMust:
		return "必须"
	case LevelShould:
		return "应当"
	case LevelQuality:
		return "质量"
	default:
		return "未知"
	}
}
