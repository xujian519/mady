package guardrails

import (
	"fmt"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// 宪法规则 Rule 实现
// ---------------------------------------------------------------------------

// keywordBlocklistRule 禁止特定关键词出现。
type keywordBlocklistRule struct {
	name     string
	severity Severity
	action   Action
	keywords []string
}

func (r *keywordBlocklistRule) Name() string { return r.name }

func (r *keywordBlocklistRule) Check(content string, _ map[string]any) RuleResult {
	for _, kw := range r.keywords {
		if strings.Contains(content, kw) {
			return RuleResult{
				Passed:   false,
				Severity: r.severity,
				Action:   r.action,
				Message:  fmt.Sprintf("包含禁止关键词: %q", kw),
			}
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// patternAnalysisRule 用预编译正则模式检查内容。
type patternAnalysisRule struct {
	name     string
	severity Severity
	action   Action
	compiled []*regexp.Regexp
}

func (r *patternAnalysisRule) Name() string { return r.name }

func (r *patternAnalysisRule) Check(content string, _ map[string]any) RuleResult {
	for _, re := range r.compiled {
		if re.MatchString(content) {
			return RuleResult{
				Passed:   false,
				Severity: r.severity,
				Action:   r.action,
				Message:  fmt.Sprintf("模式匹配: %s", re.String()),
			}
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// categoryDetectionRule 检测内容是否包含特定类别。
type categoryDetectionRule struct {
	name       string
	severity   Severity
	action     Action
	categories []string
}

func (r *categoryDetectionRule) Name() string { return r.name }

func (r *categoryDetectionRule) Check(content string, _ map[string]any) RuleResult {
	// 类别检测：扫描内容中是否包含类别关键词。
	found := make([]string, 0)
	for _, cat := range r.categories {
		if strings.Contains(strings.ToLower(content), strings.ToLower(cat)) {
			found = append(found, cat)
		}
	}
	if len(found) > 0 {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  fmt.Sprintf("检测到类别: %s", strings.Join(found, ", ")),
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// sectionStructureRule 检查必要章节的完整性。
type sectionStructureRule struct {
	name     string
	severity Severity
	action   Action
	titles   []string
	minCount int
}

func (r *sectionStructureRule) Name() string { return r.name }

func (r *sectionStructureRule) Check(content string, _ map[string]any) RuleResult {
	contentLower := strings.ToLower(content)
	var found int
	var missing []string
	for _, title := range r.titles {
		if strings.Contains(contentLower, strings.ToLower(title)) {
			found++
		} else {
			missing = append(missing, title)
		}
	}
	if found < r.minCount {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  fmt.Sprintf("缺少必要章节，已含 %d/%d 个，缺失: %s", found, r.minCount, strings.Join(missing, ", ")),
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// specificationRule 检查说明书的必要字段。
type specificationRule struct {
	name     string
	severity Severity
	action   Action
	required []string
}

func (r *specificationRule) Name() string { return r.name }

func (r *specificationRule) Check(content string, _ map[string]any) RuleResult {
	contentLower := strings.ToLower(content)
	var missing []string
	for _, field := range r.required {
		if !strings.Contains(contentLower, strings.ToLower(field)) {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  fmt.Sprintf("缺少必要字段: %s", strings.Join(missing, ", ")),
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// structuralAnalysisRule 基于结构特征分析内容。
type structuralAnalysisRule struct {
	name     string
	severity Severity
	action   Action
	minCount int
	maxCount int
}

func (r *structuralAnalysisRule) Name() string { return r.name }

func (r *structuralAnalysisRule) Check(content string, _ map[string]any) RuleResult {
	// 计算段落数作为结构复杂度指标。
	paras := strings.Split(strings.TrimSpace(content), "\n\n")
	paraCount := len(paras)

	if r.minCount > 0 && paraCount < r.minCount {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  fmt.Sprintf("段落数 %d 低于最低要求 %d", paraCount, r.minCount),
		}
	}
	if r.maxCount > 0 && paraCount > r.maxCount {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  fmt.Sprintf("段落数 %d 超过上限 %d", paraCount, r.maxCount),
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// scopeComparisonRule 检查范围比较的合理性。
type scopeComparisonRule struct {
	name      string
	severity  Severity
	action    Action
	stopWords []string
}

func (r *scopeComparisonRule) Name() string { return r.name }

func (r *scopeComparisonRule) Check(content string, _ map[string]any) RuleResult {
	// 检查是否包含范围比较词汇但无具体范围描述。
	hasComparison := strings.Contains(content, "范围") ||
		strings.Contains(content, "超出") ||
		strings.Contains(content, "涵盖") ||
		strings.Contains(content, "落入")
	hasStop := false
	for _, sw := range r.stopWords {
		if strings.Contains(content, sw) {
			hasStop = true
			break
		}
	}
	if hasComparison && !hasStop {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  "涉及范围比较但缺少具体范围描述",
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}

// claimClarityRule 检查权利要求的清晰度。
type claimClarityRule struct {
	name       string
	severity   Severity
	action     Action
	fieldRules map[string]string
}

func (r *claimClarityRule) Name() string { return r.name }

func (r *claimClarityRule) Check(content string, _ map[string]any) RuleResult {
	var violations []string
	for field, expected := range r.fieldRules {
		if strings.Contains(content, field) {
			if !strings.Contains(content, expected) {
				violations = append(violations, fmt.Sprintf("%q 附近缺少 %q", field, expected))
			}
		}
	}
	if len(violations) > 0 {
		return RuleResult{
			Passed:   false,
			Severity: r.severity,
			Action:   r.action,
			Message:  strings.Join(violations, "; "),
		}
	}
	return RuleResult{Passed: true, RuleName: r.name}
}
