package design

import (
	"strings"

	"github.com/xujian519/mady/workflows/patent"
)

// DesignRuleEngine wraps a patent.RuleEngine with design-specific evaluation logic.
// It reuses the common CheckRule structure and RuleEngine for rule management
// but defines its own CheckType dispatch for design comparison rules.
type DesignRuleEngine struct {
	engine *patent.RuleEngine
}

// NewDesignRuleEngine creates a new rule engine pre-loaded with design-specific rules.
func NewDesignRuleEngine() *DesignRuleEngine {
	e := patent.NewRuleEngine()
	e.RegisterRules(DesignRules())
	return &DesignRuleEngine{engine: e}
}

// Rules returns all registered design rules.
func (dre *DesignRuleEngine) Rules() []patent.CheckRule {
	return dre.engine.Rules()
}

// GetRule returns a rule by ID.
func (dre *DesignRuleEngine) GetRule(id string) (patent.CheckRule, bool) {
	return dre.engine.GetRule(id)
}

// RegisterRule adds a design rule.
func (dre *DesignRuleEngine) RegisterRule(rule patent.CheckRule) {
	dre.engine.RegisterRule(rule)
}

// RegisterRules adds multiple design rules.
func (dre *DesignRuleEngine) RegisterRules(rules []patent.CheckRule) {
	dre.engine.RegisterRules(rules)
}

// Evaluate runs design-specific rules against the given text.
// Only rules with CheckType == DesignCheckType are evaluated;
// the domain filter is applied when domain is non-empty.
func (dre *DesignRuleEngine) Evaluate(text, domain string) []patent.RuleCheckResult {
	rules := dre.engine.Rules()
	var results []patent.RuleCheckResult
	for _, rule := range rules {
		if rule.CheckType != DesignCheckType {
			continue
		}
		if domain != "" && rule.Domain != "" && rule.Domain != domain {
			continue
		}
		passed, detail := evaluateDesignRule(text, rule)
		if !passed {
			msg := rule.Message
			if detail != "" {
				msg = detail
			}
			results = append(results, patent.RuleCheckResult{
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

// EvaluateAll is a convenience method that evaluates all design rules without domain filter.
func (dre *DesignRuleEngine) EvaluateAll(text string) []patent.RuleCheckResult {
	return dre.Evaluate(text, "")
}

// Aggregate delegates to the patent package's Aggregate function.
func Aggregate(results []patent.RuleCheckResult) patent.Verdict {
	return patent.Aggregate(results)
}

// FormatRuleResults delegates to the patent package's FormatRuleResults function.
func FormatRuleResults(results []patent.RuleCheckResult, verdict patent.Verdict) string {
	return patent.FormatRuleResults(results, verdict)
}

// evaluateDesignRule dispatches to the type-specific checker for design rules.
func evaluateDesignRule(text string, rule patent.CheckRule) (bool, string) {
	// All design rules use CheckType == DesignCheckType.
	// RequiredElements must ALL be present in the text.
	if len(rule.RequiredElements) > 0 {
		if !matchKeywordsAll(text, rule.RequiredElements) {
			return false, ""
		}
	}
	return true, ""
}

// =============================================================================
// Design-specific rules — 5 reasoning patterns for design patent analysis
// =============================================================================

// DesignRules returns the default set of design patent analysis rules.
// These cover the five core reasoning patterns under Chinese design patent law:
//  1. 整体视觉效果对比四步法
//  2. 设计空间认定
//  3. 惯常设计排除
//  4. GUI 特殊规则
//  5. 组合与转用
func DesignRules() []patent.CheckRule {
	return []patent.CheckRule{
		{
			ID:               "DESIGN-OVERALL-COMPARISON",
			Name:             "外观设计整体视觉效果对比四步法",
			Description:      "外观设计近似判断须遵循四步法：确定设计特征→对比整体视觉效果→判断是否构成近似→结论",
			Level:            patent.LevelMust,
			Severity:         patent.SeverityCritical,
			Message:          "外观设计对比分析缺少整体视觉效果四步法",
			CheckType:        DesignCheckType,
			RequiredElements: []string{"确定设计特征", "整体视觉效果", "判断近似", "整体观察"},
			Domain:           "design_comparison",
			FixSuggestion:    "按四步法展开分析：(1)确定涉案专利的设计特征；(2)以一般消费者视角对比整体视觉效果；(3)判断是否构成近似；(4)给出结论并说明理由",
		},
		{
			ID:               "DESIGN-SPACE-DETERMINATION",
			Name:             "外观设计设计空间认定",
			Description:      "设计空间大小影响近似判断的尺度：设计空间大则判断尺度宽松，设计空间小则判断尺度严格",
			Level:            patent.LevelShould,
			Severity:         patent.SeverityMajor,
			Message:          "外观设计对比分析未考虑设计空间因素",
			CheckType:        DesignCheckType,
			RequiredElements: []string{"设计空间"},
			Domain:           "design_comparison",
			FixSuggestion:    "分析该产品类别设计自由度的大小，论证设计空间对近似判断尺度的影响。设计空间大时，细微差异即可能不构成近似；设计空间小时，一般消费者对差异更敏感",
		},
		{
			ID:               "DESIGN-COMMON-EXCLUSION",
			Name:             "惯常设计排除",
			Description:      "惯常设计（通用/常见设计）在近似判断中不予考虑，应将惯常设计从对比中排除后判断整体视觉效果",
			Level:            patent.LevelShould,
			Severity:         patent.SeverityMajor,
			Message:          "外观设计对比分析未排除惯常设计",
			CheckType:        DesignCheckType,
			RequiredElements: []string{"惯常设计"},
			Domain:           "design_comparison",
			FixSuggestion:    "识别并排除该产品类别中的惯常设计（即通用/常见设计），仅在非惯常设计部分进行整体视觉效果对比",
		},
		{
			ID:               "DESIGN-GUI-SPECIAL",
			Name:             "图形用户界面（GUI）外观设计特殊规则",
			Description:      "GUI 外观设计具有特殊的审查规则：须考虑交互方式、动态变化、与产品的关联等要素",
			Level:            patent.LevelShould,
			Severity:         patent.SeverityMajor,
			Message:          "GUI 外观设计对比分析未涉及 GUI 特殊审查规则",
			CheckType:        DesignCheckType,
			RequiredElements: []string{"图形用户界面", "GUI"},
			Domain:           "design_comparison",
			FixSuggestion:    "对于 GUI 外观设计，重点分析：(1)界面布局和交互方式；(2)动态变化过程；(3)界面与产品的关联关系；(4)GUI 惯常设计特征",
		},
		{
			ID:               "DESIGN-COMBINATION-TRANSFORMATION",
			Name:             "外观设计组合与转用",
			Description:      "外观设计的组合创新与转用判断：将现有设计特征组合或转用于其他产品类别时，应评估是否构成明显区别",
			Level:            patent.LevelQuality,
			Severity:         patent.SeverityMinor,
			Message:          "外观设计组合与转用分析不完整",
			CheckType:        DesignCheckType,
			RequiredElements: []string{"组合", "转用"},
			Domain:           "design_comparison",
			FixSuggestion:    "评估是否存在设计特征的组合创新或跨产品类别的转用，分析组合/转用后的整体视觉效果是否产生明显区别",
		},
	}
}

// =============================================================================
// Keyword matching utilities (ported from patent/rule_engine.go pattern)
// =============================================================================

// designSynonymMap expands design keywords to their synonyms for robust matching.
var designSynonymMap = map[string][]string{
	"整体视觉效果": {"整体观察", "综合判断", "整体视觉", "视觉印象", "整体印象"},
	"设计特征":   {"设计要素", "设计要点", "设计元素", "设计特点"},
	"判断近似":   {"近似判断", "是否近似", "近似性", "整体近似", "构成近似"},
	"惯常设计":   {"通用设计", "常见设计", "惯用设计", "常规设计", "惯常性设计"},
	"设计空间":   {"设计自由度", "创作空间", "设计余地", "设计自由"},
	"图形用户界面": {"GUI", "用户界面", "界面设计", "人机交互"},
	"组合":     {"组合设计", "要素组合", "设计组合", "组合创新", "特征组合"},
	"转用":     {"转用设计", "设计转用", "转用创新", "跨越转用"},
}

// matchKeyword checks whether a keyword (or any of its synonyms) is present in text.
func matchKeyword(text, keyword string) bool {
	candidates := []string{keyword}
	if syns, ok := designSynonymMap[keyword]; ok {
		candidates = append(candidates, syns...)
	}
	lower := strings.ToLower(text)
	for _, c := range candidates {
		if strings.Contains(lower, strings.ToLower(c)) {
			return true
		}
	}
	return false
}

// matchKeywordsAll returns true only if every keyword is matched.
func matchKeywordsAll(text string, keywords []string) bool {
	for _, k := range keywords {
		if !matchKeyword(text, k) {
			return false
		}
	}
	return true
}
