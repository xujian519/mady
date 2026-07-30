// Package patent — rule engine for patent analysis, deterministic track of the dual-track checker.
//
// This engine performs keyword/synonym-based checks against Chinese patent law
// requirements (novelty, inventiveness three-step method, disclosure sufficiency,
// claim analysis, infringement). It is the deterministic counterpart to the
// semantic LLM-judge track in checker.go.
//
// Design (ported from @nuo/legal-bus patent-checks.ts + keyword-utils.ts):
//
//	RuleEngine.Evaluate(rules, text) → []RuleCheckResult
//	Aggregate(results) → Verdict (pass / needs_revision / blocked)
//
// Verdict aggregation: a single Level-0 or Level-1 failure → blocked;
// three or more Level-2 failures → needs_revision; otherwise pass.
package patent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/xujian519/mady/domains/rules"
)

// Severity describes how serious a rule violation is for reporting purposes.
type Severity = rules.Severity

const (
	// SeverityCritical indicates a critical rule violation.
	SeverityCritical Severity = "critical"
	// SeverityMajor indicates a major rule violation.
	SeverityMajor Severity = "major"
	// SeverityMinor indicates a minor rule violation.
	SeverityMinor Severity = "minor"
)

// RuleLevel controls verdict aggregation severity. Level 0 is the strictest.
type RuleLevel int

const (
	// LevelMust indicates a blocking failure that hard-blocks the verdict.
	LevelMust RuleLevel = 0
	// LevelShould indicates an important failure that also blocks the verdict.
	LevelShould RuleLevel = 1
	// LevelQuality indicates a quality concern; 3+ failures needed to block.
	LevelQuality RuleLevel = 2
)

// Verdict is the aggregate pass/fail decision of a rule check.
type Verdict string

const (
	// VerdictPass indicates the check passed all rules.
	VerdictPass Verdict = "pass"
	// VerdictNeedsRevision indicates the check passed but with suggestions.
	VerdictNeedsRevision Verdict = "needs_revision"
	// VerdictBlocked indicates the check failed and blocked the action.
	VerdictBlocked Verdict = "blocked"
)

// CheckType identifies the concrete checking strategy a rule uses.
type CheckType string

const (
	// CheckNovelty indicates a novelty check (Article 22.2).
	CheckNovelty CheckType = "patent_novelty"
	// CheckInventiveness indicates an inventiveness check (Article 22.3).
	CheckInventiveness CheckType = "patent_inventiveness"
	// CheckInfringement indicates an infringement check.
	CheckInfringement CheckType = "patent_infringement"
	// CheckDisclosure indicates a disclosure sufficiency check (Article 26.3).
	CheckDisclosure CheckType = "patent_disclosure"
	// CheckClaimAnalysis indicates a claim analysis check.
	CheckClaimAnalysis CheckType = "patent_claim_analysis"
	// CheckDesignComparison indicates a design patent comparison check.
	CheckDesignComparison CheckType = "patent_design_comparison"
	// CheckPublicAccess indicates a public access/prior art check.
	CheckPublicAccess CheckType = "patent_public_access"
	// CheckAmendmentScope indicates an amendment scope check (Article 33).
	CheckAmendmentScope CheckType = "patent_amendment_scope"
	// CheckSubjectMatter indicates a subject matter eligibility check.
	CheckSubjectMatter CheckType = "patent_subject_matter"
)

// Domain string constants (plain string, for Domain field).
const (
	domainInventiveness = "patent_inventiveness"
	domainNovelty       = "patent_novelty"
	domainInfringement  = "patent_infringement"
	domainDisclosure    = "patent_disclosure"
	domainClaims        = "patent_claims"
	domainExamination   = "patent_examination"
	domainDesign        = "patent_design"
	domainInvalidation  = "patent_invalidation"
	domainAmendment     = "patent_amendment"
	domainReexamination = "patent_reexamination"
)

// Category string constants (for ReasoningPattern.Category).
const (
	categoryCreativity = "creativity"
	categoryNovelty    = "novelty"
	categoryClaims     = "claims"
	categoryOther      = "other"
)

// Claim analysis dimension names (used for rule routing by the drafting engine).
const (
	dimClarity     = "clarity"
	dimSupport     = "support"
	dimEssential   = "essential_features"
	dimConsistency = "consistency"
)

// Claim analysis dimension names (used for rule routing by the drafting engine).
// The actual dimension values come from the claimdrafting package constants.
// These local aliases are kept for documentation clarity in the rule files.

// Frequently used patent term constants (Chinese).
const (
	termDistinguishingFeatures = "区别技术特征"
	termClosestPriorArt        = "最接近对比文件"
	termCommonKnowledge        = "公知常识"
	termObvious                = "显而易见"
	termTechHint               = "技术启示"
	termPriorArtDoc            = "对比文件"
	termPriority               = "优先权"
	termProtectionScope        = "保护范围"
	termAmendmentExceed        = "修改超范围"
	termTechSolution           = "技术方案"
	termTechEffect             = "技术效果"
	termNaturalLaw             = "自然规律"
	termSciDiscovery           = "科学发现"
	termMentalActivity         = "智力活动规则"
	termDesignPatent           = "外观设计"
	termOverallVisual          = "整体视觉效果"
	termProductCategory        = "产品种类"
	termFunctionalLimit        = "功能性限定"
	termExpData                = "实验数据"
	termCanUse                 = "能够使用"
	termPersonSkilled          = "本领域技术人员"
	termUseLimit               = "用途限定"
	termInternetDisclosure     = "互联网公开"
	termDiffFeatures           = "区别特征"
	termFilingDate             = "filing date"
	termEnablement             = "enablement"
	// additional frequently used Chinese terms
	termNovelty               = "新颖性"
	termInventiveness         = "创造性"
	termPriorArt              = "现有技术"
	termPriorityDate          = "优先权日"
	termSufficientDisclosure  = "充分公开"
	termConventionalDesign    = "常规设计"
	termTechFeature           = "技术特征"
	termCombinationMotivation = "组合动机"
	termBeneficialEffect      = "有益效果"
	termCanMake               = "能够制造"
	termFilingDateLit         = "申请日"
	// additional constants for frequently repeated strings
	termNonObvious              = "非显而易见"
	termSufficientDisclosurePub = "公开充分"
	termCommonTechnicalMeans    = "惯用技术手段"
	termCombinationHint         = "结合启示"
	termEnable                  = "能够实现"
	termClaimsLocal             = "权利要求"
	termClosestPriorArtFull     = "最接近的现有技术"
)

// Rule ID constants.
const (
	ruleInventivenessThreeStep  = "INVENTIVENESS-THREE-STEP"
	ruleNoveltySingleComparison = "NOVELTY-SINGLE-COMPARISON"
)

// CheckRule is a single deterministic rule in the patent rule engine.
// The CheckType field determines which check parameters are consulted
// (RequiredElements, StepElements, RequiredAspects, Dimensions, etc.).
type CheckRule struct {
	ID          string
	Name        string
	Description string
	Level       RuleLevel
	Severity    Severity
	Message     string // failure message (Chinese, user-visible)
	CheckType   CheckType
	Domain      string // applicable domain filter ("" = all domains)

	// Check parameters — meaning depends on CheckType.
	RequiredElements []string   // CheckNovelty / CheckInfringement: all must match
	StepElements     [][]string // CheckInventiveness: 3 steps, any match per step
	RequiredAspects  []string   // CheckDisclosure: all must match
	Dimensions       []string   // CheckClaimAnalysis: dimensions to verify
	PathElements     [][]string // reasoning path step completeness (any CheckType)
	SingleComparison bool       // CheckNovelty: enforce single-comparison principle
	DependsOn        []string   // rule IDs that must also be checked first
	FixSuggestion    string
}

// RuleCheckResult is the outcome of evaluating one rule against a text.
type RuleCheckResult struct {
	RuleID        string
	RuleName      string
	Passed        bool
	Level         RuleLevel
	Severity      Severity
	Message       string
	FixSuggestion string
}

// RuleEngine holds a registered set of deterministic rules.
type RuleEngine struct {
	rules map[string]CheckRule
}

// NewRuleEngine creates an empty rule engine.
func NewRuleEngine() *RuleEngine {
	return &RuleEngine{rules: make(map[string]CheckRule)}
}

// RegisterRule adds (or replaces) a rule by its ID.
func (e *RuleEngine) RegisterRule(rule CheckRule) {
	e.rules[rule.ID] = rule
}

// RegisterRules adds multiple rules.
func (e *RuleEngine) RegisterRules(rules []CheckRule) {
	for _, r := range rules {
		e.rules[r.ID] = r
	}
}

// RemoveRule deletes a rule by ID.
func (e *RuleEngine) RemoveRule(id string) {
	delete(e.rules, id)
}

// GetRule returns a rule by ID.
func (e *RuleEngine) GetRule(id string) (CheckRule, bool) {
	r, ok := e.rules[id]
	return r, ok
}

// Rules returns all registered rules.
func (e *RuleEngine) Rules() []CheckRule {
	out := make([]CheckRule, 0, len(e.rules))
	for _, r := range e.rules {
		out = append(out, r)
	}
	return out
}

// Evaluate runs the given rules against text. Only rules whose Domain is empty
// or matches the domain argument are evaluated. Each rule is dispatched to its
// CheckType-specific checker.
func (e *RuleEngine) Evaluate(rules []CheckRule, text string, domain string) []RuleCheckResult {
	var results []RuleCheckResult
	for _, rule := range rules {
		if domain != "" && rule.Domain != "" && rule.Domain != domain {
			continue
		}
		passed, detail := evaluateRule(rule, text)
		if !passed {
			msg := rule.Message
			if detail != "" {
				msg = detail
			}
			results = append(results, RuleCheckResult{
				RuleID:        rule.ID,
				RuleName:      rule.Name,
				Passed:        false,
				Level:         rule.Level,
				Severity:      rule.Severity,
				Message:       msg,
				FixSuggestion: rule.FixSuggestion,
			})
		}
	}
	return results
}

// Aggregate computes the verdict from a set of rule results.
//
//   - Any Level-0 (Must) or Level-1 (Should) failure → blocked.
//   - 3+ Level-2 (Quality) failures → needs_revision.
//   - Otherwise → pass.
func Aggregate(results []RuleCheckResult) Verdict {
	level2Failures := 0
	for _, r := range results {
		if r.Passed {
			continue
		}
		if r.Level <= LevelShould {
			return VerdictBlocked
		}
		if r.Level == LevelQuality {
			level2Failures++
		}
	}
	if level2Failures >= 3 {
		return VerdictNeedsRevision
	}
	return VerdictPass
}

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
