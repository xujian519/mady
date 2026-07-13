package evaluate

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xujian519/mady/agentcore"
)

// Pre-compiled regexes for parseLLMJudgeScore hot path.
var (
	reScoreFraction = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*/\s*10`)
	reScorePercent  = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)
	reScoreNumber   = regexp.MustCompile(`\d+(?:\.\d+)?`)
)

// LLMJudge uses a language model to score a prediction against a reference
// answer based on a rubric. This is suitable for long-form, subjective tasks
// (such as patent drafting and legal reasoning) where token-overlap metrics
// are too brittle.
//
// The judge prompts the model to return a single numeric score in [0,1].
// Higher scores indicate closer alignment with the reference answer in terms
// of legal conclusions, reasoning, and completeness.
type LLMJudge struct {
	// Judge is the provider used to evaluate the prediction.
	Judge agentcore.Provider

	// Model is the model name passed to the judge provider.
	Model string

	// SystemPrompt is an optional system prompt that establishes the judge's
	// persona and rubric. When empty, DefaultLLMJudgePrompt is used.
	SystemPrompt string

	// Timeout caps each judge call. Zero defaults to 60 seconds.
	Timeout time.Duration

	// MaxTokens caps the judge response. Zero defaults to 128 tokens.
	MaxTokens int64

	// Temperature controls determinism. Zero means deterministic.
	Temperature float64
}

// DefaultLLMJudgePrompt is the default system prompt used by LLMJudge.
const DefaultLLMJudgePrompt = `你是一名资深的专利代理人和专利审查专家，负责评估 AI 对专利代理人考试实务题的作答质量。

请从以下三个维度对“预测答案”与“参考答案”进行比较评分，每项均为 0 到 1 之间的小数：
1. conclusion：核心法律结论、判断（如新颖性/创造性/单一性/保护客体等）是否与参考答案一致。
2. reasoning：是否包含必要的法律分析、对比文件引用、三步法推理或权利要求修改策略。
3. citation：是否引用正确的专利法、实施细则或审查指南条款。

请只输出一个 JSON 对象，不要输出任何解释或 markdown 代码块。格式如下：
{"conclusion": 0.8, "reasoning": 0.6, "citation": 0.7}`

func (LLMJudge) Name() string { return "llm_judge" }

// Compute returns a score in [0,1] by asking the configured judge model to
// compare prediction against reference. If the judge call fails or the response
// cannot be parsed, Compute returns 0.
func (j LLMJudge) Compute(prediction, reference string) float64 {
	if j.Judge == nil {
		return 0
	}

	systemPrompt := j.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = DefaultLLMJudgePrompt
	}

	userPrompt := fmt.Sprintf(`%s

参考答案：
%s

预测答案：
%s

最终评分 JSON：`,
		systemPrompt, truncateForJudge(reference), truncateForJudge(prediction))

	timeout := j.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req := &agentcore.ProviderRequest{
		Model: j.Model,
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: systemPrompt},
			{Role: agentcore.RoleUser, Content: userPrompt},
		},
		Temperature: j.Temperature,
		MaxTokens:   j.MaxTokens,
	}
	if req.MaxTokens <= 0 {
		req.MaxTokens = 128
	}

	resp, err := j.Judge.Complete(ctx, req)
	if err != nil {
		return 0
	}
	// Defensive nil check: a misbehaving provider may return (nil, nil).
	if resp == nil {
		return 0
	}

	score := parseLLMJudgeScore(resp.Content)
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

// llmRubricScores is the structured output expected from the judge.
type llmRubricScores struct {
	Conclusion float64 `json:"conclusion"`
	Reasoning  float64 `json:"reasoning"`
	Citation   float64 `json:"citation"`
}

// truncateForJudge keeps the judge prompt within reasonable limits while
// preserving the most important parts of long-form answers. Both head and tail
// are retained because legal conclusions are usually at the end.
func truncateForJudge(s string) string {
	const maxRunes = 6000
	n := runeLen(s)
	if n <= maxRunes {
		return s
	}
	head := s[:tailingRuneIndex(s, maxRunes/2)]
	tailStart := n - maxRunes/2
	tail := s[tailingRuneIndex(s, tailStart):]
	return head + "\n\n...[中间内容省略]...\n\n" + tail
}

func tailingRuneIndex(s string, tailStart int) int {
	var idx int
	var count int
	for count < tailStart {
		_, size := utf8.DecodeRuneInString(s[idx:])
		idx += size
		count++
	}
	return idx
}

// parseLLMJudgeScore extracts a score in [0,1] from the judge response. It
// prefers a structured JSON rubric (conclusion/reasoning/citation) and averages
// the three dimensions. If JSON parsing fails, it falls back to the last numeric
// value in the response, accepting integers, decimals, percentages, and fractions.
func parseLLMJudgeScore(content string) float64 {
	content = strings.TrimSpace(content)
	if content == "" {
		return -1
	}

	// Strip markdown code fences if the model wrapped the JSON.
	clean := content
	if strings.HasPrefix(clean, "```") {
		lines := strings.Split(clean, "\n")
		var trimmed []string
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				continue
			}
			trimmed = append(trimmed, line)
		}
		clean = strings.Join(trimmed, "\n")
		clean = strings.TrimSpace(clean)
	}

	// Try structured rubric first.
	var rubric llmRubricScores
	if err := json.Unmarshal([]byte(clean), &rubric); err == nil {
		// 验证至少一个 rubric 字段非零，防止 {"foo":0.9} 这类垃圾 JSON
		// 返回全零值的假阴性。
		if rubric.Conclusion != 0 || rubric.Reasoning != 0 || rubric.Citation != 0 {
			return clampScore((rubric.Conclusion + rubric.Reasoning + rubric.Citation) / 3)
		}
	}

	// Try parsing the entire content as a number.
	// Use `clean` (fence-stripped) so markdown-wrapped numbers are caught.
	if v, err := strconv.ParseFloat(clean, 64); err == nil {
		return clampScore(normalizeScore(v))
	}

	// Handle "x/10" form.
	if m := reScoreFraction.FindStringSubmatch(content); len(m) > 1 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return clampScore(v / 10)
		}
	}

	// Handle percentages.
	if m := reScorePercent.FindStringSubmatch(content); len(m) > 1 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return clampScore(v / 100)
		}
	}

	// Extract the last number appearing in the text.
	numbers := reScoreNumber.FindAllString(content, -1)
	if len(numbers) > 0 {
		if v, err := strconv.ParseFloat(numbers[len(numbers)-1], 64); err == nil {
			return clampScore(normalizeScore(v))
		}
	}

	return -1
}

func clampScore(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func normalizeScore(v float64) float64 {
	if v > 1 && v <= 10 {
		return v / 10
	}
	if v > 10 && v <= 100 {
		return v / 100
	}
	return v
}

// SemanticSimilarity uses an LLM to estimate semantic equivalence between a
// prediction and a reference. It is a thin wrapper around LLMJudge with a
// prompt focused on meaning rather than surface form.
type SemanticSimilarity struct {
	Judge agentcore.Provider
	Model string
}

func (SemanticSimilarity) Name() string { return "semantic_similarity" }

func (m SemanticSimilarity) Compute(prediction, reference string) float64 {
	judge := LLMJudge{
		Judge:     m.Judge,
		Model:     m.Model,
		Timeout:   60 * time.Second,
		MaxTokens: 64,
		SystemPrompt: `你是一名资深的专利代理人考试阅卷专家。请判断以下两个答案在语义上是否等价，即它们是否表达了相同的法律结论、技术判断和核心要点。忽略表达方式和篇幅差异。

请只输出 0 到 1 之间的小数，0 表示完全不等价，1 表示完全等价。不要输出任何解释。`,
	}
	return judge.Compute(prediction, reference)
}
