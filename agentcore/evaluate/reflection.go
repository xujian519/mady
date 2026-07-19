package evaluate

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// Reflection evaluates the quality of an agent's self-reflection by measuring
// how much the reflected output improves over the initial output relative to
// the reference answer.
//
// The metric works in two passes:
//  1. Score the initial answer against the reference (initialScore)
//  2. Score the reflected (final) answer against the reference (reflectedScore)
//  3. Return a composite: improvement * reflectedQuality
//
// Improvement: how much the reflection closed the gap to the reference.
// ReflectedQuality: the absolute quality of the final answer.
//
// The final score = (improvement + reflectedQuality) / 2.
type Reflection struct {
	// Judge is the LLM provider used to score both initial and reflected
	// answers. If nil, the metric falls back to F1-based scoring.
	Judge agentcore.Provider

	// Model is the model name passed to the judge provider.
	Model string

	// SystemPrompt is an optional system prompt for the judge rubric.
	// When empty, uses DefaultLLMJudgePrompt (the same as LLMJudge).
	SystemPrompt string

	// Timeout caps each judge call. Zero defaults to 60 seconds.
	Timeout time.Duration
}

func (m Reflection) Name() string { return "reflection" }

// Compute evaluates reflection quality.
// prediction: the reflected (final) answer.
// reference: the expected correct answer.
//
// The initial answer is embedded in the prediction using a delimiter:
//
//	[INITIAL]
//	<initial answer>
//	[REFLECTED]
//	<reflected answer>
//
// If the delimiter is not found, the prediction is treated as the final answer
// and an ideal initial score of 0 is assumed (full improvement).
func (m Reflection) Compute(prediction, reference string) float64 {
	initial, reflected := splitReflection(prediction)
	if reflected == "" {
		reflected = prediction
	}

	initialScore := m.score(initial, reference)
	reflectedScore := m.score(reflected, reference)

	// Improvement: how much the gap was closed. If initial is already good,
	// improvement is 1 (nothing to improve). If reflected is worse, improvement
	// is 0.
	improvement := 0.0
	if initialScore >= reflectedScore {
		improvement = 1.0 // either perfect initial or reflection didn't help
	} else {
		gap := 1.0 - initialScore
		if gap > 0 {
			improvement = (reflectedScore - initialScore) / gap
		} else {
			improvement = 1.0
		}
	}
	if improvement > 1 {
		improvement = 1
	}

	return (improvement + reflectedScore) / 2.0
}

// score evaluates a single answer against the reference. Uses LLM judge if
// available, otherwise falls back to F1Score.
func (m Reflection) score(answer, reference string) float64 {
	if answer == "" {
		return 0
	}
	if reference == "" {
		return 1
	}

	if m.Judge == nil {
		return F1Score{}.Compute(answer, reference)
	}

	judge := LLMJudge{
		Judge:        m.Judge,
		Model:        m.Model,
		SystemPrompt: m.SystemPrompt,
		Timeout:      m.Timeout,
		Temperature:  0.01,
		MaxTokens:    64,
		Samples:      1,
	}
	return judge.computeOnce(answer, reference)
}

// splitReflection splits a prediction into initial and reflected parts.
func splitReflection(prediction string) (initial, reflected string) {
	// Try various delimiter formats.
	delimiters := []string{
		"[INITIAL]",
		"[initial]",
		"## Initial Answer",
		"**Initial Answer**",
		"[REFLECTED]",
		"[reflected]",
		"## Reflected Answer",
		"## Final Answer",
		"**Reflected Answer**",
	}

	// Find INITIAL delimiter.
	initIdx := -1
	for _, d := range delimiters[:4] { // first 4 are initial markers
		if idx := strings.Index(prediction, d); idx >= 0 {
			initIdx = idx
			break
		}
	}

	if initIdx < 0 {
		return "", prediction // no delimiter, entire string is reflected
	}

	// Content after INITIAL marker until REFLECTED marker.
	rest := prediction[initIdx:]
	refIdx := -1
	for _, d := range delimiters[4:] { // last 4 are reflected markers
		if idx := strings.Index(rest, d); idx >= 0 {
			refIdx = idx
			break
		}
	}

	if refIdx < 0 {
		// Only INITIAL marker found — treat as initial = everything after marker.
		initial = strings.TrimSpace(rest[len("[INITIAL]"):])
		return initial, ""
	}

	initial = strings.TrimSpace(rest[len("[INITIAL]"):refIdx])
	reflected = strings.TrimSpace(rest[refIdx+len("[REFLECTED]"):])
	return initial, reflected
}

// ============================================================================
// RubricJudge - multi-criteria LLM-based evaluation with custom rubrics
// ============================================================================

// RubricCriterion is a single evaluation dimension with a name, description,
// and weight.
type RubricCriterion struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Weight      float64 `json:"weight"`
}

// DefaultRubricCriteria returns sensible defaults for Mady evaluation.
// These mirror the dimensions used in DefaultLLMJudgePrompt with equal weight.
func DefaultRubricCriteria() []RubricCriterion {
	return []RubricCriterion{
		{Name: "conclusion", Description: "核心法律结论、判断是否与参考答案一致", Weight: 1},
		{Name: "reasoning", Description: "是否包含必要的法律分析、对比文件引用、三步法推理", Weight: 1},
		{Name: "citation", Description: "是否引用正确的专利法、实施细则或审查指南条款", Weight: 1},
	}
}

// RubricJudge uses a language model to score a prediction against a reference
// using a customizable set of criteria. This generalizes LLMJudge (which has
// a hardcoded rubric) to arbitrary evaluation domains.
//
// The judge prompts the model to return a JSON object with a score for each
// criterion. The final score is the weighted average of all criterion scores.
type RubricJudge struct {
	// Judge is the provider used to evaluate the prediction.
	Judge agentcore.Provider

	// Model is the model name passed to the judge provider.
	Model string

	// Criteria defines the evaluation dimensions. When nil, DefaultRubricCriteria
	// is used.
	Criteria []RubricCriterion

	// SystemPrompt is an optional system prompt override. When empty, one is
	// generated from the Criteria.
	SystemPrompt string

	// Timeout caps each judge call. Zero defaults to 60 seconds.
	Timeout time.Duration

	// MaxTokens caps the judge response. Zero defaults to 256 tokens.
	MaxTokens int64

	// Temperature controls determinism. Zero defaults to 0.01.
	Temperature float64
}

func (m RubricJudge) Name() string { return "rubric_judge" }

// Compute scores a prediction against a reference using the configured rubric.
func (m RubricJudge) Compute(prediction, reference string) float64 {
	if m.Judge == nil {
		return 0
	}

	criteria := m.Criteria
	if len(criteria) == 0 {
		criteria = DefaultRubricCriteria()
	}

	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	temp := m.Temperature
	if temp == 0 {
		temp = 0.01
	}

	maxTokens := m.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 256
	}

	// Build the rubric description from criteria.
	rubricLines := make([]string, len(criteria))
	totalWeight := 0.0
	for i, c := range criteria {
		rubricLines[i] = fmt.Sprintf("%d. %s (权重 %.1f): %s", i+1, c.Name, c.Weight, c.Description)
		totalWeight += c.Weight
	}
	rubricText := strings.Join(rubricLines, "\n")

	// Build criterion keys for the output JSON.
	criterionKeys := make([]string, len(criteria))
	for i, c := range criteria {
		criterionKeys[i] = c.Name
	}
	jsonTemplate := buildRubricJSONTemplate(criterionKeys)

	systemPrompt := m.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = fmt.Sprintf(`你是一名资深的评估专家。请根据以下评分标准对"预测答案"进行评分。

评分标准：
%s

请对每项标准输出一个 0 到 1 之间的小数，并输出一个 overall 综合评分（所有标准的加权平均）。
只输出一个 JSON 对象，不要输出任何解释或 markdown 代码块。
输出格式：%s`, rubricText, jsonTemplate)
	}

	userPrompt := fmt.Sprintf(`参考答案：
%s

预测答案：
%s

请评分：`,
		truncateForJudge(reference), truncateForJudge(prediction))

	req := &agentcore.ProviderRequest{
		Model: m.Model,
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: systemPrompt},
			{Role: agentcore.RoleUser, Content: userPrompt},
		},
		Temperature: temp,
		MaxTokens:   maxTokens,
	}

	resp, err := m.Judge.Complete(ctx, req)
	if err != nil || resp == nil {
		return 0
	}

	return m.parseRubricScore(resp.Content, criteria, totalWeight)
}

// parseRubricScore extracts dimension scores from the judge's JSON response
// and returns the weighted average.
func (m RubricJudge) parseRubricScore(content string, criteria []RubricCriterion, totalWeight float64) float64 {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0
	}

	// Strip markdown code fences.
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		var trimmed []string
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				continue
			}
			trimmed = append(trimmed, line)
		}
		content = strings.Join(trimmed, "\n")
		content = strings.TrimSpace(content)
	}

	// Parse the JSON response.
	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// Fallback: try to extract any numeric score.
		return extractNumericScore(content)
	}

	// Compute weighted average from criteria keys.
	var weightedSum float64
	effectiveWeight := totalWeight
	for _, c := range criteria {
		if v, ok := result[c.Name]; ok {
			score := toFloat64(v)
			weightedSum += score * c.Weight
		} else {
			// Missing criterion: subtract its weight from total.
			effectiveWeight -= c.Weight
		}
	}

	// Also check for "overall" key.
	if overall, ok := result["overall"]; ok && effectiveWeight == 0 {
		return clampScore(toFloat64(overall))
	}

	if effectiveWeight <= 0 {
		return 0
	}

	score := weightedSum / effectiveWeight
	return clampScore(score)
}

// buildRubricJSONTemplate creates a JSON template showing the expected output
// format for the given criterion keys.
func buildRubricJSONTemplate(keys []string) string {
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = fmt.Sprintf("%q: 0.8", k)
	}
	return "{" + strings.Join(pairs, ", ") + `, "overall": 0.8}`
}

// extractNumericScore extracts a single numeric value from text as fallback.
func extractNumericScore(content string) float64 {
	// Try fraction format first (e.g., "8/10").
	if strings.Contains(content, "/") {
		// Find the last fraction pattern.
		reFrac := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*/\s*(\d+(?:\.\d+)?)`)
		if matches := reFrac.FindStringSubmatch(content); len(matches) >= 3 {
			num, err1 := strconv.ParseFloat(matches[1], 64)
			den, err2 := strconv.ParseFloat(matches[2], 64)
			if err1 == nil && err2 == nil && den != 0 {
				return clampScore(num / den)
			}
		}
	}

	reNumeric := regexp.MustCompile(`(\d+(?:\.\d+)?)`)
	matches := reNumeric.FindAllString(content, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		if v, err := strconv.ParseFloat(matches[i], 64); err == nil {
			score := normalizeScore(v)
			if score >= 0 && score <= 1 {
				return score
			}
		}
	}
	return 0
}

// toFloat64 converts an any to float64.
func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	case string:
		f, err := parseFloatFlexible(val)
		if err == nil {
			return normalizeScore(f)
		}
	}
	return 0
}

// parseFloatFlexible parses a string as float64, handling various formats.
func parseFloatFlexible(s string) (float64, error) {
	s = strings.TrimSpace(s)
	// Handle "x/10" fractions.
	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)
		if len(parts) == 2 {
			num, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			den, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if err1 == nil && err2 == nil && den != 0 {
				return num / den, nil
			}
		}
	}
	return strconv.ParseFloat(s, 64)
}
