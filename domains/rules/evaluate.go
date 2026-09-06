package rules

import (
	"fmt"
	"regexp"
	"strconv"
)

// CheckResult is the output of a single rule evaluation.
type CheckResult struct {
	Passed  bool     `json:"passed"`
	Score   float64  `json:"score"` // 0.0 - 1.0
	Details []string `json:"details,omitempty"`
}

// Evaluate runs the check logic against the given text and returns a result.
// Supports multiple check types:
//   - "presence": ensure pattern(s) are present in the text
//   - "absence":  ensure pattern(s) are absent from the text
//   - "numeric":  evaluate numeric range/comparison conditions
//   - "composition": evaluate multiple sub-checks with AND/OR logic
func (c *Check) Evaluate(text string) (*CheckResult, error) {
	switch c.Type {
	case "presence", "exist", "must_include":
		return c.evaluatePresence(text)
	case "absence", "must_not", "forbidden":
		return c.evaluateAbsence(text)
	case "numeric", "range", "numerical":
		return c.evaluateNumeric(text)
	case "composition", "compound":
		return c.evaluateComposition(text)
	default:
		// For complex types like "patent_novelty", return a default passing
		// result since these require LLM reasoning.
		return &CheckResult{
			Passed:  true,
			Score:   0.5,
			Details: []string{fmt.Sprintf("类型 %q 需要 LLM 判断，跳过自动检查", c.Type)},
		}, nil
	}
}

// evaluatePresence checks that all rules/conditions patterns are present.
func (c *Check) evaluatePresence(text string) (*CheckResult, error) {
	patterns := c.patterns()
	if len(patterns) == 0 {
		return &CheckResult{Passed: true, Score: 1.0}, nil
	}

	var details []string
	hits := 0
	for _, p := range patterns {
		re, err := regexp.Compile(`(?i)` + regexp.QuoteMeta(p))
		if err != nil {
			continue
		}
		if re.MatchString(text) {
			hits++
			details = append(details, fmt.Sprintf("✅ 匹配: %s", p))
		} else {
			details = append(details, fmt.Sprintf("❌ 缺失: %s", p))
		}
	}

	score := float64(hits) / float64(len(patterns))
	return &CheckResult{
		Passed:  score >= 0.8,
		Score:   score,
		Details: details,
	}, nil
}

// evaluateAbsence checks that all forbidden patterns are absent.
func (c *Check) evaluateAbsence(text string) (*CheckResult, error) {
	patterns := c.patterns()
	if len(patterns) == 0 {
		return &CheckResult{Passed: true, Score: 1.0}, nil
	}

	var details []string
	violations := 0
	for _, p := range patterns {
		re, err := regexp.Compile(`(?i)` + regexp.QuoteMeta(p))
		if err != nil {
			continue
		}
		if re.MatchString(text) {
			violations++
			details = append(details, fmt.Sprintf("❌ 发现禁止内容: %s", p))
		} else {
			details = append(details, fmt.Sprintf("✅ 未发现: %s", p))
		}
	}

	score := 1.0 - float64(violations)/float64(len(patterns))
	if score < 0 {
		score = 0
	}
	return &CheckResult{
		Passed:  violations == 0,
		Score:   score,
		Details: details,
	}, nil
}

// evaluateNumeric checks numeric conditions extracted from Extra.
func (c *Check) evaluateNumeric(text string) (*CheckResult, error) {
	var details []string

	rawRange, hasRange := c.Extra["range"]
	if !hasRange {
		return &CheckResult{Passed: true, Score: 1.0,
			Details: []string{"无数值范围定义，跳过"}}, nil
	}

	numVal := extractFirstNumber(text)
	if numVal == nil {
		return &CheckResult{Passed: false, Score: 0,
			Details: []string{"无法从文本中提取数值"}}, nil
	}

	switch v := rawRange.(type) {
	case []any:
		if len(v) >= 2 {
			mn, minOK := toFloat64(v[0])
			mx, maxOK := toFloat64(v[1])
			if minOK && maxOK {
				passed := *numVal >= mn && *numVal <= mx
				details = append(details, fmt.Sprintf("数值 %.2f 在范围 [%.2f, %.2f] 内: %v", *numVal, mn, mx, passed))
				score := 1.0
				if !passed {
					score = 0
				}
				return &CheckResult{Passed: passed, Score: score, Details: details}, nil
			}
		}
	case map[string]any:
		if minVal, hasMin := v["min"]; hasMin {
			mn, ok := toFloat64(minVal)
			if !ok {
				return &CheckResult{Passed: false, Score: 0, Details: []string{fmt.Sprintf("最小值 %v 解析失败", minVal)}}, nil
			}
			if *numVal < mn {
				return &CheckResult{Passed: false, Score: 0, Details: []string{fmt.Sprintf("数值 %.2f 小于最小值 %.2f", *numVal, mn)}}, nil
			}
		}
		if maxVal, hasMax := v["max"]; hasMax {
			mx, ok := toFloat64(maxVal)
			if !ok {
				return &CheckResult{Passed: false, Score: 0, Details: []string{fmt.Sprintf("最大值 %v 解析失败", maxVal)}}, nil
			}
			if *numVal > mx {
				return &CheckResult{Passed: false, Score: 0, Details: []string{fmt.Sprintf("数值 %.2f 大于最大值 %.2f", *numVal, mx)}}, nil
			}
		}
		details = append(details, fmt.Sprintf("数值 %.2f 在范围内", *numVal))
		return &CheckResult{Passed: true, Score: 1.0, Details: details}, nil
	}

	return &CheckResult{Passed: false, Score: 0, Details: []string{fmt.Sprintf("范围定义格式无法解析: %T %v", rawRange, rawRange)}}, nil
}

// evaluateComposition evaluates sub-checks with AND/OR logic.
func (c *Check) evaluateComposition(text string) (*CheckResult, error) {
	subChecks := c.patterns()
	if len(subChecks) == 0 {
		return &CheckResult{Passed: true, Score: 1.0}, nil
	}

	results := make([]*CheckResult, 0, len(subChecks))
	var allDetails []string
	subType := "presence"
	if t, ok := c.Extra["check_type"]; ok {
		if s, ok := t.(string); ok {
			subType = s
		}
	}
	for _, sc := range subChecks {
		subCheck := &Check{Type: subType, Rules: []string{sc}}
		r, err := subCheck.evaluatePresence(text)
		if err != nil {
			allDetails = append(allDetails, fmt.Sprintf("子检查 %q 执行失败: %v", sc, err))
			continue
		}
		results = append(results, r)
	}

	if len(results) == 0 {
		return &CheckResult{Passed: false, Score: 0, Details: allDetails}, nil
	}

	allPassed := true
	var totalScore float64
	for _, r := range results {
		if !r.Passed {
			allPassed = false
		}
		totalScore += r.Score
		allDetails = append(allDetails, r.Details...)
	}

	score := totalScore / float64(len(results))
	return &CheckResult{
		Passed:  allPassed && score >= 0.8,
		Score:   score,
		Details: allDetails,
	}, nil
}

// patterns returns the list of search patterns from Rules or Conditions.
func (c *Check) patterns() []string {
	if len(c.Rules) > 0 {
		return c.Rules
	}
	if len(c.Conditions) > 0 {
		return c.Conditions
	}
	if len(c.Requirements) > 0 {
		return c.Requirements
	}
	return nil
}

// extractFirstNumber extracts the first numeric value from a string.
func extractFirstNumber(text string) *float64 {
	re := regexp.MustCompile(`[-+]?\d+\.?\d*`)
	match := re.FindString(text)
	if match == "" {
		return nil
	}
	val, err := strconv.ParseFloat(match, 64)
	if err != nil {
		// 正则已保证 match 为数字，跑数解析失败仅当越界/极端格式；
		// 按"文本中无数值"降级返回 nil，交由调用方按缺值处理。
		return nil
	}
	return &val
}

// toFloat64 converts an interface{} value to float64 if possible.
func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// EvaluateRule evaluates a single rule against the given text.
func (e *Engine) EvaluateRule(ruleID string, text string) (*CheckResult, error) {
	rule := e.RuleByID(ruleID)
	if rule == nil {
		return nil, fmt.Errorf("rule %q not found", ruleID)
	}
	return rule.Check.Evaluate(text)
}

// EvaluateRules evaluates multiple rules against the given text.
// Only rules whose Type supports programmatic evaluation are processed.
func (e *Engine) EvaluateRules(ruleIDs []string, text string) map[string]*CheckResult {
	results := make(map[string]*CheckResult, len(ruleIDs))
	for _, id := range ruleIDs {
		result, err := e.EvaluateRule(id, text)
		if err != nil {
			results[id] = &CheckResult{
				Passed:  false,
				Score:   0,
				Details: []string{err.Error()},
			}
			continue
		}
		results[id] = result
	}
	return results
}

// EvaluateAllRules evaluates all loaded rules against the given text.
func (e *Engine) EvaluateAllRules(text string) map[string]*CheckResult {
	rules := e.AllRules()
	ids := make([]string, 0, len(rules))
	for _, r := range rules {
		ids = append(ids, r.RuleID)
	}
	return e.EvaluateRules(ids, text)
}
