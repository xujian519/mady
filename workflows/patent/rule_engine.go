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
	CheckNovelty       CheckType = "patent_novelty"
	CheckInventiveness CheckType = "patent_inventiveness"
	CheckInfringement  CheckType = "patent_infringement"
	CheckDisclosure    CheckType = "patent_disclosure"
	CheckClaimAnalysis CheckType = "patent_claim_analysis"
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
	RequiredElements  []string   // CheckNovelty / CheckInfringement: all must match
	StepElements      [][]string // CheckInventiveness: 3 steps, any match per step
	RequiredAspects   []string   // CheckDisclosure: all must match
	Dimensions        []string   // CheckClaimAnalysis: dimensions to verify
	SingleComparison  bool       // CheckNovelty: enforce single-comparison principle
	FixSuggestion     string
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
// (passed, detailMessage).
func evaluateRule(rule CheckRule, text string) (bool, string) {
	switch rule.CheckType {
	case CheckNovelty:
		return checkNovelty(text, rule)
	case CheckInventiveness:
		return checkInventiveness(text, rule)
	case CheckInfringement:
		return checkInfringement(text, rule)
	case CheckDisclosure:
		return checkDisclosure(text, rule)
	case CheckClaimAnalysis:
		return checkClaimAnalysis(text, rule)
	default:
		return true, ""
	}
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

// ----------------------------------------------------------------------------
// Keyword matching utilities (ported from keyword-utils.ts)
// ----------------------------------------------------------------------------

// synonymMap expands a keyword to its synonyms for more robust matching.
var synonymMap = map[string][]string{
	"新颖性":   {"新创性", "未公开", "不属于现有技术", "未被披露"},
	"创造性":   {"非显而易见", "发明高度", "创造性步骤", "inventive step"},
	"对比文件":  {"现有技术", "在先技术", "引用文件", "文献", "reference"},
	"权利要求":  {"权项", "claims", "保护范围"},
	"说明书":   {"specification", "申请文件"},
	"充分公开":  {"公开充分", "能够实现", "enablement"},
	"三步法":   {"最接近的现有技术", "区别技术特征", "技术启示"},
	"单独对比":  {"单独对比原则", "一一对比"},
	"公知常识":  {"惯用技术手段", "常规设计", "common knowledge"},
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
func DefaultPatentRules() []CheckRule {
	return []CheckRule{
		{
			ID:          "NOVELTY-SINGLE-COMPARISON",
			Name:        "新颖性单独对比原则",
			Description: "新颖性分析必须采用单独对比原则，不得结合多份对比文件",
			Level:       LevelMust,
			Severity:    SeverityCritical,
			Message:     "新颖性分析未遵循单独对比原则",
			CheckType:   CheckNovelty,
			RequiredElements: []string{"新颖性", "对比文件"},
			SingleComparison: true,
			FixSuggestion:    "对每项权利要求与一份对比文件进行单独对比，明确相同或实质相同的技术方案",
		},
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
			FixSuggestion: "明确最接近现有技术，提炼区别技术特征，论证是否存在技术启示",
		},
		{
			ID:          "DISCLOSURE-SUFFICIENCY",
			Name:        "充分公开审查",
			Description: "说明书应充分公开发明，使本领域技术人员能够实现",
			Level:       LevelShould,
			Severity:    SeverityMajor,
			Message:     "充分公开分析不完整",
			CheckType:   CheckDisclosure,
			RequiredAspects: []string{"充分公开", "能够实现"},
			Domain:          "patent_disclosure",
			FixSuggestion:   "确认说明书是否提供足够的技术细节使本领域技术人员能够实现该发明",
		},
		{
			ID:          "CLAIM-CLARITY-SUPPORT",
			Name:        "权利要求清楚性与支持",
			Description: "权利要求应当清楚、得到说明书支持",
			Level:       LevelShould,
			Severity:    SeverityMajor,
			Message:     "权利要求分析缺少必要维度",
			CheckType:   CheckClaimAnalysis,
			Dimensions:  []string{"clarity", "support"},
			Domain:      "patent_claims",
			FixSuggestion: "检查权利要求是否清楚简明、是否得到说明书支持",
		},
		{
			ID:          "CLAIM-ESSENTIAL-FEATURES",
			Name:        "必要技术特征完整性",
			Description: "独立权利要求应包含解决技术问题的必要技术特征",
			Level:       LevelQuality,
			Severity:    SeverityMinor,
			Message:     "权利要求可能缺少必要技术特征",
			CheckType:   CheckClaimAnalysis,
			Dimensions:  []string{"essential_features", "consistency"},
			Domain:      "patent_claims",
			FixSuggestion: "核对独立权利要求是否包含全部必要技术特征",
		},
		{
			ID:              "INFRINGEMENT-FULL-COVERAGE",
			Name:            "侵权全面覆盖",
			Description:     "侵权分析应进行全面技术特征比对",
			Level:           LevelQuality,
			Severity:        SeverityMinor,
			Message:         "侵权分析缺少全面覆盖分析",
			CheckType:       CheckInfringement,
			RequiredElements: []string{"权利要求", "技术特征"},
			Domain:           "patent_infringement",
			FixSuggestion:    "逐一比对被控技术方案与权利要求的全部技术特征",
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
