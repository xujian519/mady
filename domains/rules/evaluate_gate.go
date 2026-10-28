package rules

import (
	"fmt"
	"strings"
)

// 本文件实现门禁专用的确定性检查类型：keyword_blocklist 与 structural_analysis。
//
// 它们来自 deepseek-harness patent-rule 的规则资产格式。与 Evaluate 中其余类型不同，
// 这两类不需要 LLM：命中即在文本层面可判定，因此可以放在 AfterToolExecution 热路径。

// evaluateKeywordBlocklist 判定文本中是否出现禁用的完整表述。
//
// 支持两个排除条件（由规则资产声明）：
//   - negationContext：命中点前文含否定词（不/未/无/非/避免/防止/并非）时不报；
//   - quoteImmune：命中点位于成对引号内时不报（引述审查意见或法条，不是本模型的主张）。
func (c *Check) evaluateKeywordBlocklist(text string) (*CheckResult, error) {
	keywords := extraStrings(c.Extra, "keywords")
	if len(keywords) == 0 {
		return &CheckResult{Passed: true, Score: 1.0}, nil
	}
	negation := extraBool(c.Extra, "negationContext")
	quoteImmune := extraBool(c.Extra, "quoteImmune")

	var hits []string
	for _, keyword := range keywords {
		if keyword == "" {
			continue
		}
		if hit, at := findEffectiveHit(text, keyword, negation, quoteImmune); hit {
			hits = append(hits, fmt.Sprintf("%s（位置 %d）", keyword, at))
		}
	}
	if len(hits) == 0 {
		return &CheckResult{Passed: true, Score: 1.0}, nil
	}
	return &CheckResult{
		Passed:  false,
		Score:   0,
		Details: []string{"命中禁用表述：" + strings.Join(hits, "、")},
	}, nil
}

// findEffectiveHit 判断关键词是否有一处不受排除条件约束的命中。
func findEffectiveHit(text, keyword string, negation, quoteImmune bool) (bool, int) {
	offset := 0
	for {
		idx := strings.Index(text[offset:], keyword)
		if idx < 0 {
			return false, 0
		}
		position := offset + idx
		if (!negation || !hasNegationBefore(text, position)) && (!quoteImmune || !insideQuotes(text, position)) {
			return true, position
		}
		offset = position + len(keyword)
		if offset >= len(text) {
			return false, 0
		}
	}
}

// hasNegationBefore 判断命中点前 12 个字符内是否出现否定词。
func hasNegationBefore(text string, position int) bool {
	start := max(0, position-12)
	window := text[start:position]
	for _, word := range []string{"不", "未", "无", "非", "避免", "防止", "并非", "不会", "不能"} {
		if strings.Contains(window, word) {
			return true
		}
	}
	return false
}

// insideQuotes 判断位置是否落在成对引号内。
//
// 只有「命中点之前有开引号、之后有对应闭引号」才算引文；未闭合的引号不豁免——
// 宁可误报，也不静默放行红线措辞。
func insideQuotes(text string, position int) bool {
	if position > len(text) {
		return false
	}
	before, after := text[:position], text[position:]
	pairs := []struct{ open, close string }{
		{"“", "”"},
		{"「", "」"},
		{"『", "』"},
	}
	for _, pair := range pairs {
		if strings.Count(before, pair.open) > strings.Count(before, pair.close) &&
			strings.Contains(after, pair.close) {
			return true
		}
	}
	// 双引号成对出现：命中点前为奇数个、之后还有至少一个。
	return strings.Count(before, `"`)%2 == 1 && strings.Contains(after, `"`)
}

// evaluateStructuralAnalysis 判定文本是否覆盖规则要求的全部结构要素。
//
// requiresAll 的每一项是一组同义模式，命中任一即视为该要素存在；
// 命中比例低于 minConfidence（默认 1.0）即不通过。
func (c *Check) evaluateStructuralAnalysis(text string) (*CheckResult, error) {
	elements := extraMappings(c.Extra, "requiresAll")
	if len(elements) == 0 {
		return &CheckResult{Passed: true, Score: 1.0}, nil
	}
	minConfidence := extraFloat(c.Extra, "minConfidence", 1.0)

	details := make([]string, 0, len(elements))
	hits := 0
	for _, element := range elements {
		name := stringValue(element["element"])
		if name == "" {
			name = "未命名要素"
		}
		patterns := toStrings(element["patterns"])
		matched := false
		for _, pattern := range patterns {
			if pattern != "" && strings.Contains(text, pattern) {
				matched = true
				break
			}
		}
		if matched {
			hits++
			details = append(details, "✅ 存在: "+name)
		} else {
			details = append(details, "❌ 缺少: "+name)
		}
	}
	confidence := float64(hits) / float64(len(elements))
	return &CheckResult{
		Passed:  confidence >= minConfidence,
		Score:   confidence,
		Details: details,
	}, nil
}

// extraStrings 读取 Extra 中的字符串数组。
func extraStrings(extra map[string]any, key string) []string {
	if extra == nil {
		return nil
	}
	return toStrings(extra[key])
}

// toStrings 把 YAML 解出的数组转成字符串数组。
func toStrings(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if strings, ok := value.([]string); ok {
			return strings
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text := stringValue(item); text != "" {
			out = append(out, text)
		}
	}
	return out
}

// stringValue 把标量转成字符串。
func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

// extraBool 读取 Extra 中的布尔值。
func extraBool(extra map[string]any, key string) bool {
	if extra == nil {
		return false
	}
	flag, _ := extra[key].(bool)
	return flag
}

// extraFloat 读取 Extra 中的数值，缺省返回 fallback。
func extraFloat(extra map[string]any, key string, fallback float64) float64 {
	if extra == nil {
		return fallback
	}
	switch typed := extra[key].(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	default:
		return fallback
	}
}

// extraMappings 读取 Extra 中的映射数组。
func extraMappings(extra map[string]any, key string) []map[string]any {
	if extra == nil {
		return nil
	}
	items, ok := extra[key].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if mapping, ok := item.(map[string]any); ok {
			out = append(out, mapping)
		}
	}
	return out
}
