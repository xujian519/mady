// Rule engine for patent analysis — deterministic track of the dual-track checker.
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
)

// Severity describes how serious a rule violation is for reporting purposes.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityMajor    Severity = "major"
	SeverityMinor    Severity = "minor"
)

// RuleLevel controls verdict aggregation severity. Level 0 is the strictest.
type RuleLevel int

const (
	LevelMust    RuleLevel = 0 // blocking — a failure hard-blocks the verdict
	LevelShould  RuleLevel = 1 // important — a failure also blocks
	LevelQuality RuleLevel = 2 // quality — 3+ failures needed to block
)

// Verdict is the aggregate pass/fail decision of a rule check.
type Verdict string

const (
	VerdictPass          Verdict = "pass"
	VerdictNeedsRevision Verdict = "needs_revision"
	VerdictBlocked       Verdict = "blocked"
)

// CheckType identifies the concrete checking strategy a rule uses.
type CheckType string

const (
	CheckNovelty          CheckType = "patent_novelty"
	CheckInventiveness    CheckType = "patent_inventiveness"
	CheckInfringement     CheckType = "patent_infringement"
	CheckDisclosure       CheckType = "patent_disclosure"
	CheckClaimAnalysis    CheckType = "patent_claim_analysis"
	CheckDesignComparison CheckType = "patent_design_comparison"
	CheckPublicAccess     CheckType = "patent_public_access"
	CheckAmendmentScope   CheckType = "patent_amendment_scope"
	CheckSubjectMatter    CheckType = "patent_subject_matter"
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
	"新颖性":  {"新创性", "未公开", "不属于现有技术", "未被披露"},
	"创造性":  {"非显而易见", "发明高度", "创造性步骤", "inventive step"},
	"对比文件": {"现有技术", "在先技术", "引用文件", "文献", "reference"},
	"权利要求": {"权项", "claims", "保护范围"},
	"说明书":  {"specification", "申请文件"},
	"充分公开": {"公开充分", "能够实现", "enablement"},
	"三步法":  {"最接近的现有技术", "区别技术特征", "技术启示"},
	"单独对比": {"单独对比原则", "一一对比"},
	"公知常识": {"惯用技术手段", "常规设计", "common knowledge", "well-known"},
	// Infringement domain terms.
	"全面覆盖": {"全部技术特征", "逐一比对", "全覆盖原则"},
	"等同":   {"等同替换", "等同侵权", "基本相同的手段", "基本相同的功能", "基本相同的效果"},
	"禁止反悔": {"审查过程禁反言", "prosecution history estoppel", "修改导致放弃"},
	"捐献规则": {"捐献原则", "dedicated to the public"},
	"技术特征": {"技术特征分解", "权项特征", "limitation"},
	// Invalidation domain terms.
	"无效宣告": {"无效请求", "宣告无效", "invalidation"},
	"组合动机": {"结合启示", "有动机结合", "技术结合启示", "技术启示"},
	"优先权日": {"优先权", "申请日", "filing date"},
	// Reexamination domain terms.
	"复审":   {"复审请求", "驳回复审", "reexamination"},
	"程序违法": {"程序错误", "违反法定程序"},
	"新证据":  {"补充证据", "新提交的证据"},
	// Design comparison terms (外观设计).
	"外观设计":   {"工业设计", "design", "industrial design", "外观"},
	"整体视觉效果": {"视觉效果", "整体外观", "整体视觉", "overall visual effect"},
	"产品种类":   {"产品类别", "产品类型", "相似种类", "同类产品"},
	// Public access terms (公开方式).
	"出版物公开": {"公开出版", "论文", "期刊", "杂志", "书籍"},
	"使用公开":  {"公开使用", "销售公开", "展出", "公开实施"},
	"互联网公开": {"网络公开", "在线公开", "网页公开", "网站公开"},
	"公开方式":  {"公开途径", "公开形式", "公开类型"},
	// Amendment scope terms (修改超范围).
	"修改超范围":   {"超出原范围", "增加新内容", "超范围修改", "amendment beyond scope", "超范围"},
	"直接且毫无疑义": {"直接毫无疑义", "直接确定", "原申请文件"},
	// Subject matter terms (保护客体).
	"技术方案":   {"技术方案本身", "technical solution"},
	"保护客体":   {"可专利主题", "patentable subject matter", "授权客体"},
	"智力活动规则": {"智力活动的规则", "数学方法", "商业规则", "mental activity", "抽象思想"},
	"疾病诊断方法": {"诊断方法", "治疗方法", "手术方法"},
	"科学发现":   {"科学发现", "自然规律", "自然法则", "natural law"},
	// Reasoning pattern terms (推理模式).
	"预料不到":     {"预料不到的技术效果", "出乎意料", "surprising", "unexpected"},
	"用途限定":     {"用途特征", "用途限定", "use limitation"},
	"实验数据":     {"实验数据", "实施例", "实验例", "药效数据"},
	"最接近的现有技术": {"最接近对比文件", "最接近的对比文件"},
	"抵触申请":     {"在先申请在后公开", "conflicting application"},
	"功能性限定":    {"功能限定", "功能性特征", "functional limitation"},
	"实用性":      {"工业实用性", "产业应用", "industrial applicability"},
	"积极效果":     {"有益效果", "positive effect", "技术效果"},
	"本领域技术人员":  {"所属领域技术人员", "person skilled in the art"},
	"能够实现":     {"可实施", "enablement", "能够制造", "能够使用"},
	"显而易见":     {"obvious", "显而易见性", "非显而易见"},
	"转让":       {"transfer", "assign", "assignment"},
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
	"clarity":            {"清楚", "清晰", "明确", "简要"},
	"support":            {"以说明书为依据", "支持", "记载", "记载于", "说明书支持"},
	"essential_features": {"必要技术特征", "必要特征", "必不可少"},
	"consistency":        {"一致", "对应", "协调", "不矛盾"},
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
	var rules []CheckRule
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
// ----------------------------------------------------------------------------

// NoveltyRules returns rules specific to patent novelty analysis (专利法第22条第2款).
// These rules strengthen the baseline novelty checks with feature-coverage and
// search-completeness verification.
func NoveltyRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "NOVELTY-SINGLE-COMPARISON",
			Name:             "新颖性单独对比原则",
			Description:      "新颖性分析必须采用单独对比原则，不得结合多份对比文件",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "新颖性分析未遵循单独对比原则",
			CheckType:        CheckNovelty,
			RequiredElements: []string{"新颖性", "对比文件"},
			SingleComparison: true,
			Domain:           "patent_novelty",
			FixSuggestion:    "对每项权利要求与一份对比文件进行单独对比，明确相同或实质相同的技术方案",
		},
		{
			ID:               "NOVELTY-FEATURE-COVERAGE",
			Name:             "新颖性特征覆盖分析",
			Description:      "新颖性分析应逐一比对权利要求的所有技术特征与对比文件",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "新颖性分析缺少技术特征的逐一比对",
			CheckType:        CheckNovelty,
			RequiredElements: []string{"技术特征"},
			Domain:           "patent_novelty",
			FixSuggestion:    "列出权利要求的全部技术特征，逐一标注对比文件是否公开",
		},
	}
}

// InventivenessRules returns rules specific to inventiveness analysis using the
// three-step method (专利法第22条第3款).
func InventivenessRules() []CheckRule {
	return []CheckRule{
		{
			ID:          "INVENTIVENESS-THREE-STEP",
			Name:        "创造性三步法",
			Description: "创造性分析须包含三步法：最接近现有技术→区别技术特征→技术启示",
			Level:       LevelMust,
			Severity:    SeverityCritical,
			Message:     "创造性分析缺少三步法",
			CheckType:   CheckInventiveness,
			StepElements: [][]string{
				{"最接近的现有技术", "最接近对比文件"},
				{"区别技术特征", "区别特征"},
				{"技术启示", "显而易见", "公知常识"},
			},
			Domain:        "patent_inventiveness",
			FixSuggestion: "明确最接近现有技术，提炼区别技术特征，论证是否存在技术启示",
		},
		{
			ID:               "INVENTIVENESS-TECHNICAL-PROBLEM",
			Name:             "实际解决技术问题",
			Description:      "创造性三步法第二步应明确发明实际解决的技术问题",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "创造性分析未明确实际解决的技术问题",
			CheckType:        CheckInventiveness,
			RequiredElements: []string{"区别技术特征"},
			Domain:           "patent_inventiveness",
			FixSuggestion:    "基于区别技术特征，确定发明相对于最接近现有技术实际解决的技术问题",
		},
	}
}

// InfringementRules returns rules for patent infringement analysis.
// Covers the full-coverage principle, equivalence doctrine, prosecution history
// estoppel (禁止反悔), and dedication rule (捐献规则).
func InfringementRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "INFRINGEMENT-FULL-COVERAGE",
			Name:             "侵权全面覆盖原则",
			Description:      "侵权分析应全面比对被控方案是否包含权利要求的全部技术特征",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "侵权分析缺少全面覆盖分析",
			CheckType:        CheckInfringement,
			RequiredElements: []string{"全面覆盖", "技术特征"},
			Domain:           "patent_infringement",
			FixSuggestion:    "分解权利要求为技术特征A/B/C，逐一判断被控方案是否包含",
		},
		{
			ID:               "INFRINGEMENT-EQUIVALENCE",
			Name:             "等同侵权判定",
			Description:      "侵权分析应评估区别特征是否构成等同替换（手段/功能/效果基本相同）",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "侵权分析缺少等同原则评估",
			CheckType:        CheckInfringement,
			RequiredElements: []string{"等同"},
			Domain:           "patent_infringement",
			FixSuggestion:    "对不构成字面侵权的特征，检查是否满足等同三要素：手段/功能/效果基本相同+无需创造性劳动",
		},
		{
			ID:               "INFRINGEMENT-ESTOPPEL",
			Name:             "禁止反悔原则检查",
			Description:      "侵权分析应考虑审查过程中的修改是否导致权利放弃（禁止反悔）",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "侵权分析未考虑禁止反悔原则的限制",
			CheckType:        CheckInfringement,
			RequiredElements: []string{"禁止反悔"},
			Domain:           "patent_infringement",
			FixSuggestion:    "审查专利审查过程中的修改和陈述，确认是否对等同范围构成限制",
		},
		{
			ID:               "INFRINGEMENT-DEDICATION",
			Name:             "捐献规则检查",
			Description:      "说明书中披露但权利要求未保护的技术方案视为捐献公众",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "侵权分析未检查捐献规则的适用性",
			CheckType:        CheckInfringement,
			RequiredElements: []string{"捐献规则"},
			Domain:           "patent_infringement",
			FixSuggestion:    "确认被控方案对应的技术特征是否在说明书中披露但未写入权利要求",
		},
	}
}

// InvalidationRules returns rules for patent invalidation analysis (无效宣告).
// Key constraints:
//   - Each invalidation ground MUST be argued independently per claim
//   - Multi-document combinations MUST justify combination motivation
//   - Prior-art publication dates MUST be verified against priority date
func InvalidationRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "INVALID-NOVELTY-SINGLE-COMPARISON",
			Name:             "无效新颖性单独对比",
			Description:      "无效宣告中的新颖性理由须采用单独对比原则",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "无效宣告中新颖性论证未遵循单独对比原则",
			CheckType:        CheckNovelty,
			RequiredElements: []string{"新颖性", "对比文件"},
			SingleComparison: true,
			Domain:           "patent_invalidation",
			FixSuggestion:    "对每项权利要求逐一与单份对比文件进行新颖性比对",
		},
		{
			ID:          "INVALID-COMBINATION-MOTIVATION",
			Name:        "无效组合动机论证",
			Description: "多篇对比文件组合攻击创造性时，须论证组合的技术启示/动机",
			Level:       LevelMust,
			Severity:    SeverityCritical,
			Message:     "无效宣告中多篇组合缺乏组合动机论证",
			CheckType:   CheckInventiveness,
			StepElements: [][]string{
				{"最接近的现有技术", "最接近对比文件"},
				{"区别技术特征", "区别特征"},
				{"组合动机", "技术启示", "结合启示"},
			},
			Domain:        "patent_invalidation",
			FixSuggestion: "论证本领域技术人员有动机将对比文件组合，说明组合的合理性",
		},
		{
			ID:               "INVALID-PRIORITY-DATE-CHECK",
			Name:             "对比文件公开日核实",
			Description:      "无效宣告中引用的对比文件公开日须早于涉案专利的优先权日",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未核实对比文件的公开日是否早于优先权日",
			CheckType:        CheckNovelty,
			RequiredElements: []string{"优先权日"},
			Domain:           "patent_invalidation",
			FixSuggestion:    "核实每份对比文件的公开日，标注是否早于涉案专利的优先权日/申请日",
		},
	}
}

// ReexaminationRules returns rules for patent reexamination request drafting
// (复审请求). Covers rejection-scope analysis, procedural legality, and new
// evidence relevance.
func ReexaminationRules() []CheckRule {
	return []CheckRule{
		{
			ID:            "REEXAM-GROUNDS-SCOPE",
			Name:          "复审理由范围审查",
			Description:   "复审理由应在驳回决定范围内，或提供新的证据/理由",
			Level:         LevelShould,
			Severity:      SeverityMajor,
			Message:       "复审理由分析不完整",
			CheckType:     CheckClaimAnalysis,
			Dimensions:    []string{"clarity", "consistency"},
			Domain:        "patent_reexamination",
			FixSuggestion: "逐条列出驳回理由，针对性回应或提交新证据克服",
		},
		{
			ID:               "REEXAM-NEW-EVIDENCE",
			Name:             "新证据关联性",
			Description:      "复审中提交的新证据应与克服驳回理由直接相关",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "未说明新证据与驳回理由的关联性",
			CheckType:        CheckNovelty,
			RequiredElements: []string{"新证据"},
			Domain:           "patent_reexamination",
			FixSuggestion:    "对于每份新证据，说明其如何克服驳回决定中指出的缺陷",
		},
	}
}

// DisclosureRules returns rules for disclosure sufficiency and claim analysis.
func DisclosureRules() []CheckRule {
	return []CheckRule{
		{
			ID:              "DISCLOSURE-SUFFICIENCY",
			Name:            "充分公开审查",
			Description:     "说明书应充分公开发明，使本领域技术人员能够实现",
			Level:           LevelShould,
			Severity:        SeverityMajor,
			Message:         "充分公开分析不完整",
			CheckType:       CheckDisclosure,
			RequiredAspects: []string{"充分公开", "能够实现"},
			Domain:          "patent_disclosure",
			FixSuggestion:   "确认说明书是否提供足够的技术细节使本领域技术人员能够实现该发明",
		},
		{
			ID:            "CLAIM-CLARITY-SUPPORT",
			Name:          "权利要求清楚性与支持",
			Description:   "权利要求应当清楚、得到说明书支持",
			Level:         LevelShould,
			Severity:      SeverityMajor,
			Message:       "权利要求分析缺少必要维度",
			CheckType:     CheckClaimAnalysis,
			Dimensions:    []string{"clarity", "support"},
			Domain:        "patent_claims",
			FixSuggestion: "检查权利要求是否清楚简明、是否得到说明书支持",
		},
		{
			ID:            "CLAIM-ESSENTIAL-FEATURES",
			Name:          "必要技术特征完整性",
			Description:   "独立权利要求应包含解决技术问题的必要技术特征",
			Level:         LevelQuality,
			Severity:      SeverityMinor,
			Message:       "权利要求可能缺少必要技术特征",
			CheckType:     CheckClaimAnalysis,
			Dimensions:    []string{"essential_features", "consistency"},
			Domain:        "patent_claims",
			FixSuggestion: "核对独立权利要求是否包含全部必要技术特征",
		},
	}
}

// ReasoningPatternRules returns check rules derived from the 18 standardized
// reasoning patterns. Each pattern encodes a canonical reasoning template from
// the patent re-examination knowledge base. Rules use PathElements for step-
// based verification and span creativity, novelty, claims, and other categories.
func ReasoningPatternRules() []CheckRule {
	patterns := AllPatterns()
	var rules []CheckRule
	for _, p := range patterns {
		rules = append(rules, p.CheckRules...)
	}
	return rules
}

// DesignRules returns rules for design patent comparison (外观设计对比).
// Covers overall visual effect comparison, product category determination,
// design feature identification, direct copy evaluation, and multi-design
// comparison framework.
func DesignRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "DESIGN-01",
			Name:             "外观设计整体视觉效果对比",
			Description:      "外观设计对比应以整体视觉效果为准，综合判断是否构成相同或近似",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "外观设计对比缺少整体视觉效果分析",
			CheckType:        CheckDesignComparison,
			RequiredElements: []string{"外观设计", "整体视觉效果"},
			Domain:           "patent_design",
			FixSuggestion:    "以整体视觉效果为准进行外观设计对比，判断是否构成相同或近似",
		},
		{
			ID:               "DESIGN-02",
			Name:             "外观设计产品种类认定",
			Description:      "外观设计对比应在相同或相近种类的产品之间进行",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未明确认定产品种类是否相同或相近",
			CheckType:        CheckDesignComparison,
			RequiredElements: []string{"产品种类"},
			Domain:           "patent_design",
			FixSuggestion:    "根据产品用途、功能、销售渠道等因素认定产品种类是否相同或相近",
		},
		{
			ID:               "DESIGN-03",
			Name:             "外观设计设计特征识别",
			Description:      "应识别外观设计的设计特征，区分创新设计部分与惯常设计",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "未充分识别外观设计的设计特征",
			CheckType:        CheckDesignComparison,
			RequiredElements: []string{"设计特征"},
			Domain:           "patent_design",
			FixSuggestion:    "识别外观设计中区别于现有设计的创新设计特征",
		},
		{
			ID:               "DESIGN-04",
			Name:             "外观设计直接模仿判断",
			Description:      "判断外观设计是否构成直接模仿或仅存在局部细微差异",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "未分析外观设计是否构成直接模仿",
			CheckType:        CheckDesignComparison,
			RequiredElements: []string{"直接模仿", "局部差异"},
			Domain:           "patent_design",
			FixSuggestion:    "判断局部差异是否对整体视觉效果产生显著影响",
		},
		{
			ID:               "DESIGN-05",
			Name:             "外观设计多设计对比框架",
			Description:      "涉及多项外观设计时，应逐项对比并明确各设计对象的对比结果",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "多设计对比未逐项分析",
			CheckType:        CheckDesignComparison,
			RequiredElements: []string{"逐项对比", "外观设计"},
			Domain:           "patent_design",
			FixSuggestion:    "逐项对比每项外观设计与对比设计的整体视觉效果",
		},
	}
}

// SubjectMatterRules returns rules for patent subject matter eligibility analysis
// under Article 2 of Chinese Patent Law (专利法第2条). Covers technical solution
// definition, technical problem, technical means, non-patentable subject matter
// exclusion, and technical effect evaluation.
func SubjectMatterRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "SUBJECT-01",
			Name:             "技术方案构成审查",
			Description:      "保护客体应是利用自然规律解决技术问题的技术方案",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "未充分论证要求保护的主题是否构成技术方案",
			CheckType:        CheckSubjectMatter,
			RequiredElements: []string{"技术方案", "自然规律"},
			Domain:           "patent_examination",
			FixSuggestion:    "论证该主题是否利用自然规律、解决技术问题、产生技术效果",
		},
		{
			ID:               "SUBJECT-02",
			Name:             "技术问题认定",
			Description:      "技术方案应解决明确的技术问题",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未明确技术方案解决的技术问题",
			CheckType:        CheckSubjectMatter,
			RequiredElements: []string{"技术问题"},
			Domain:           "patent_examination",
			FixSuggestion:    "明确技术方案所要解决的技术问题",
		},
		{
			ID:               "SUBJECT-03",
			Name:             "技术手段审查",
			Description:      "技术方案应采用技术手段实现技术问题的解决",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未充分分析技术方案所采用的技术手段",
			CheckType:        CheckSubjectMatter,
			RequiredElements: []string{"技术手段"},
			Domain:           "patent_examination",
			FixSuggestion:    "说明技术方案采用了哪些技术手段来解决技术问题",
		},
		{
			ID:               "SUBJECT-04",
			Name:             "非可专利客体排除",
			Description:      "排除科学发现、智力活动规则、疾病诊断治疗方法、原子核变换方法",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未逐一排除非可专利客体",
			CheckType:        CheckSubjectMatter,
			RequiredElements: []string{"科学发现", "智力活动规则"},
			Domain:           "patent_examination",
			FixSuggestion:    "逐项排除科学发现、智力活动规则、疾病诊断治疗方法、原子核变换方法",
		},
		{
			ID:               "SUBJECT-05",
			Name:             "技术效果分析",
			Description:      "技术方案应产生与解决的技术问题相对应的技术效果",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "未分析技术方案的技术效果",
			CheckType:        CheckSubjectMatter,
			RequiredElements: []string{"技术效果"},
			Domain:           "patent_examination",
			FixSuggestion:    "说明技术方案所产生的技术效果与解决的技术问题之间的对应关系",
		},
	}
}

// PublicAccessRules returns rules for determining how prior art was made
// available to the public (公开方式判断). Covers publication disclosure,
// public use disclosure, internet disclosure, publication date verification,
// and confidentiality obligation analysis.
func PublicAccessRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "PUBACC-01",
			Name:             "出版物公开认定",
			Description:      "判断现有技术是否通过出版物方式公开（论文/期刊/书籍）",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未充分认定是否构成出版物公开",
			CheckType:        CheckPublicAccess,
			RequiredElements: []string{"出版物公开"},
			Domain:           "patent_novelty",
			FixSuggestion:    "确认出版物是否在申请日前出版发行，公众能否获知",
		},
		{
			ID:               "PUBACC-02",
			Name:             "使用公开认定",
			Description:      "判断现有技术是否通过使用方式公开（销售/展出/公开实施）",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未充分认定是否构成使用公开",
			CheckType:        CheckPublicAccess,
			RequiredElements: []string{"使用公开"},
			Domain:           "patent_novelty",
			FixSuggestion:    "确认使用行为是否在申请日前使技术内容为公众所知",
		},
		{
			ID:               "PUBACC-03",
			Name:             "互联网公开认定",
			Description:      "判断现有技术是否通过互联网方式公开（网页/在线公开）",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未充分认定是否构成互联网公开",
			CheckType:        CheckPublicAccess,
			RequiredElements: []string{"互联网公开"},
			Domain:           "patent_novelty",
			FixSuggestion:    "确认网页公开日的确定方式及公众能否通过互联网获知",
		},
		{
			ID:               "PUBACC-04",
			Name:             "公开日核实",
			Description:      "核实现有技术的公开日是否早于申请日或优先权日",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "未核实现有技术的公开日是否早于申请日",
			CheckType:        CheckPublicAccess,
			RequiredElements: []string{"公开日", "申请日"},
			Domain:           "patent_novelty",
			FixSuggestion:    "核实每份现有技术的公开日是否早于涉案专利的申请日或有效优先权日",
		},
		{
			ID:               "PUBACC-05",
			Name:             "保密义务分析",
			Description:      "判断技术内容在公开日之前是否处于保密状态",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "未分析技术内容是否处于保密状态",
			CheckType:        CheckPublicAccess,
			RequiredElements: []string{"保密", "保密义务"},
			Domain:           "patent_novelty",
			FixSuggestion:    "确认技术内容在公开日之前是否存在明示或默示的保密义务",
		},
	}
}

// PriorityRules returns rules for priority date determination and analysis
// (优先权规则). Covers priority date determination, priority transfer review,
// priority claim validity, priority date comparison, and partial priority
// handling.
func PriorityRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "PRIORITY-01",
			Name:             "优先权日认定",
			Description:      "确认专利申请的优先权日及其法律效力",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "未准确认定优先权日",
			CheckType:        CheckNovelty,
			RequiredElements: []string{"优先权日", "优先权"},
			Domain:           "patent_novelty",
			FixSuggestion:    "确认优先权主张的依据和优先权日的准确日期",
		},
		{
			ID:               "PRIORITY-02",
			Name:             "优先权转让审查",
			Description:      "优先权人变更应在申请日前完成转让手续",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未审查优先权转让的程序合规性",
			CheckType:        CheckAmendmentScope,
			RequiredElements: []string{"优先权", "转让"},
			Domain:           "patent_amendment",
			FixSuggestion:    "确认优先权转让是否在申请日前完成，手续是否完整",
		},
		{
			ID:               "PRIORITY-03",
			Name:             "优先权主张有效性",
			Description:      "优先权主张应符合形式条件和实质条件",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未充分审查优先权主张的有效性",
			CheckType:        CheckNovelty,
			RequiredElements: []string{"优先权", "有效性"},
			Domain:           "patent_novelty",
			FixSuggestion:    "审查优先权主张是否符合形式条件和实质条件（在先申请是否相同主题）",
		},
		{
			ID:               "PRIORITY-04",
			Name:             "优先权日与申请日对比",
			Description:      "以有效的优先权日作为现有技术判断的时间基准",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "未以有效的优先权日作为现有技术判断的时间基准",
			CheckType:        CheckNovelty,
			RequiredElements: []string{"优先权日", "申请日", "现有技术"},
			Domain:           "patent_novelty",
			FixSuggestion:    "确认优先权有效后，以优先权日作为判断现有技术的时间基准",
		},
		{
			ID:               "PRIORITY-05",
			Name:             "部分优先权处理",
			Description:      "同一申请中包含多项优先权时，应区分不同优先权对应的事项",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "未分析部分优先权的适用性",
			CheckType:        CheckNovelty,
			RequiredElements: []string{"部分优先权", "多项优先权"},
			Domain:           "patent_novelty",
			FixSuggestion:    "逐项确定各技术方案对应的优先权日，区分部分优先权和多项优先权",
		},
	}
}

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
