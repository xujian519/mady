package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/xujian519/mady/agentcore"
)

// EvalHook 实现 RAGAS 风格的检索/生成质量评估。
// 在 AfterModelCall 中收集 Question、Context、Answer，计算质量指标。
//
// 评估维度：
//   - Faithfulness: 答案是否忠实于检索上下文（防止幻觉）
//   - AnswerRelevancy: 答案是否针对问题（防止答非所问）
//   - ContextPrecision: 检索结果中是否有噪声（防止上下文污染）
//
// Phase 3 实现启发式评分；Phase 4+ 将接入 LLM 评分器。
type EvalHook struct {
	agentcore.BaseLifecycleHook

	cfg EvalConfig
}

// EvalConfig 控制 EvalHook 的行为。
type EvalConfig struct {
	// Enabled 是否启用评估。
	Enabled bool `json:"enabled"`

	// LogResults 将评估结果记录到事件总线。
	LogResults bool `json:"log_results"`

	// MinFaithfulness 答案忠实度阈值（低于此值时发出警告）。
	MinFaithfulness float64 `json:"min_faithfulness"`
}

// DefaultEvalConfig 返回默认配置。
func DefaultEvalConfig() EvalConfig {
	return EvalConfig{
		Enabled:         false, // Phase 3 默认关闭
		LogResults:      true,
		MinFaithfulness: 0.7,
	}
}

// NewEvalHook 创建评估钩子。
func NewEvalHook(cfg EvalConfig) *EvalHook {
	return &EvalHook{cfg: cfg}
}

// EvalResult 单次评估的结果。
type EvalResult struct {
	Turn             int64    `json:"turn"`
	Question         string   `json:"question"`
	Answer           string   `json:"answer"`
	ContextSnippets  int      `json:"context_snippets"`
	Faithfulness     float64  `json:"faithfulness"`
	AnswerRelevancy  float64  `json:"answer_relevancy"`
	ContextPrecision float64  `json:"context_precision"`
	Duration         string   `json:"duration"`
	Warnings         []string `json:"warnings,omitempty"`
}

// AfterModelCall 在每次模型调用后评估质量。
func (h *EvalHook) AfterModelCall(_ context.Context, arc *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) {
	if !h.cfg.Enabled || mcc == nil || mcc.Request == nil || mcc.Response == nil {
		return
	}

	question := agentcore.LastUserMessage(arc.Messages)
	answer := mcc.Response.Content
	if question == "" || answer == "" {
		return
	}

	contextSnippets := countContextSnippets(mcc.Request)

	start := time.Now()

	result := EvalResult{
		Turn:            arc.Turn,
		Question:        truncate(question, 200),
		Answer:          truncate(answer, 200),
		ContextSnippets: contextSnippets,
	}

	// 启发式评估（无 LLM）
	result.Faithfulness = scoreFaithfulness(answer, mcc.Request)
	result.AnswerRelevancy = scoreAnswerRelevancy(answer, question)
	result.ContextPrecision = scoreContextPrecision(mcc.Request)

	result.Duration = time.Since(start).Round(time.Microsecond).String()

	// 检查警告
	if result.FaithlessnessWarning() != "" {
		result.Warnings = append(result.Warnings, result.FaithlessnessWarning())
	}

	if h.cfg.LogResults {
		// 记录到事件总线（可被 GUI 或日志消费）。
		if arc.Agent != nil {
			arc.Agent.EmitEvent(evalResultEvent{at: time.Now(), result: result})
		}
	}
}

// evalResultEvent wraps an EvalResult as an agentcore.Event.
type evalResultEvent struct {
	at     time.Time
	result EvalResult
}

func (e evalResultEvent) EventKind() agentcore.EventType { return agentcore.EventType("eval_result") }
func (e evalResultEvent) EventTime() time.Time           { return e.at }

// FaithlessnessWarning 返回忠实度警告（如果有）。
func (r *EvalResult) FaithlessnessWarning() string {
	if r.Faithfulness < 0.5 {
		return fmt.Sprintf("低忠实度 (%.2f): 答案可能脱离检索上下文", r.Faithfulness)
	}
	return ""
}

// ---------------------------------------------------------------------------
// 启发式评分（Phase 3 轻量实现，不依赖 LLM）
// ---------------------------------------------------------------------------

// scoreFaithfulness 估算答案对上下文的忠实度。
// 基于答案中的专有名词/数字是否出现在检索上下文中。
func scoreFaithfulness(answer string, req *agentcore.ProviderRequest) float64 {
	if answer == "" || req == nil {
		return 0
	}

	// 提取上下文中所有文本
	contextText := extractContextText(req.Messages)
	if contextText == "" {
		return 0.8 // 无上下文时默认较高（没有可违反的材料）
	}

	answerLower := strings.ToLower(answer)
	contextLower := strings.ToLower(contextText)

	// 提取答案中的关键实体（简单版本：长度 > 2 的词）
	answerWords := tokenizeEval(answerLower)
	if len(answerWords) == 0 {
		return 0.8
	}

	hits := 0
	total := 0
	for _, w := range answerWords {
		if len(w) <= 2 {
			continue
		}
		total++
		if strings.Contains(contextLower, w) {
			hits++
		}
	}

	if total == 0 {
		return 0.8
	}

	ratio := float64(hits) / float64(total)
	// 映射到 0.3~1.0 范围
	return 0.3 + ratio*0.7
}

// scoreAnswerRelevancy 估算答案与问题的相关度。
func scoreAnswerRelevancy(answer, question string) float64 {
	if answer == "" || question == "" {
		return 0
	}

	qWords := tokenizeEval(strings.ToLower(question))
	if len(qWords) == 0 {
		return 0.8
	}

	aLower := strings.ToLower(answer)
	hits := 0
	for _, w := range qWords {
		if len(w) <= 1 {
			continue
		}
		if strings.Contains(aLower, w) {
			hits++
		}
	}

	ratio := float64(hits) / float64(len(qWords))
	return 0.4 + ratio*0.6
}

// scoreContextPrecision 估算上下文中的噪声比例。
func scoreContextPrecision(req *agentcore.ProviderRequest) float64 {
	if req == nil {
		return 1.0
	}

	// 找到最后一条系统消息（假设是注入的检索结果）
	snippets := countContextSnippets(req)
	if snippets == 0 {
		return 1.0 // 无检索上下文 = 满分
	}

	// 简单评估：如果上下文块有明确标记则假设为高质量
	contextText := extractContextText(req.Messages)
	if strings.Contains(contextText, "参考片段") || strings.Contains(contextText, "检索") {
		return 0.85
	}

	return 0.7
}

// countContextSnippets 统计上下文中的参考片段数。
func countContextSnippets(req *agentcore.ProviderRequest) int {
	if req == nil {
		return 0
	}
	contextText := extractContextText(req.Messages)
	if contextText == "" {
		return 0
	}
	count := strings.Count(contextText, "---")
	if count > 0 {
		return count / 2 // 每段有两个 --- 标记（开头和结尾）
	}
	return 1
}

// extractContextText 从请求消息中提取注入的上下文文本。
func extractContextText(msgs []agentcore.Message) string {
	for _, m := range msgs {
		if m.Role == agentcore.RoleSystem && len(m.Content) > 200 {
			if strings.Contains(m.Content, "---") ||
				strings.Contains(m.Content, "参考") ||
				strings.Contains(m.Content, "检索") {
				return m.Content
			}
		}
	}
	return ""
}

// tokenizeEval 简单的分词（空格 + 标点）。
func tokenizeEval(text string) []string {
	var tokens []string
	var buf strings.Builder
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || unicode.Is(unicode.Han, r) {
			buf.WriteRune(r)
		} else if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		tokens = append(tokens, buf.String())
	}
	return tokens
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
