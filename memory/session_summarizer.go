package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/prompt"
)

// SessionSummarizer 在会话结束时从 Session 层记忆中提取值得长期保存的关键事实。
// 借鉴 Letta 的 Sleep-time Agent 模式：在后台异步运行，不阻塞主流程。
type SessionSummarizer struct {
	provider agentcore.Provider
	model    string
}

// NewSessionSummarizer 创建一个会话汇总器。
func NewSessionSummarizer(provider agentcore.Provider, model string) *SessionSummarizer {
	if model == "" {
		model = "default"
	}
	return &SessionSummarizer{provider: provider, model: model}
}

// Summarize 从会话记忆中提取长期有价值的事实。
// 调用 LLM 过滤噪音（寒暄、一次性问答），提取用户偏好、决策、领域知识。
func (s *SessionSummarizer) Summarize(ctx context.Context, sessionMemories []MemoryEntry) ([]ExtractedFact, error) {
	if len(sessionMemories) == 0 {
		return nil, nil
	}

	conversationText := buildConversationText(sessionMemories)
	if conversationText == "" {
		return nil, nil
	}

	req := &agentcore.ProviderRequest{
		Model: s.model,
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: prompt.ResolveSystemPromptOr("prompt://memory-session-summary", sessionSummarySystemPromptFallback)},
			{Role: agentcore.RoleUser, Content: conversationText},
		},
		Temperature: 0.1,
		MaxTokens:   1024,
		ResponseFormat: &agentcore.ResponseFormat{
			Type: agentcore.ResponseFormatJSONObject,
		},
	}

	resp, err := s.provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("memory/summarizer: LLM call failed: %w", err)
	}

	return parseSummaryResponse(resp.Content), nil
}

// buildConversationText 将多条 Session 记忆合并为一段对话文本。
func buildConversationText(memories []MemoryEntry) string {
	var b strings.Builder
	for _, m := range memories {
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// parseSummaryResponse 解析 LLM 返回的汇总事实。
func parseSummaryResponse(content string) []ExtractedFact {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	content = stripMarkdownFences(content)

	var result struct {
		Facts []string `json:"facts"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil || len(result.Facts) == 0 {
		// Fallback：逐行解析
		return parseFactsFromLines(content)
	}

	var facts []ExtractedFact
	for _, f := range result.Facts {
		f = strings.TrimSpace(f)
		if f != "" && f != "无" {
			facts = append(facts, ExtractedFact{
				Content:    f,
				Layer:      LayerLongTerm,
				Importance: estimateImportance(f),
				Metadata:   map[string]any{"source": "session_summary"},
			})
		}
	}
	return facts
}

// parseFactsFromLines 从纯文本内容中逐行解析事实（LLM JSON 解析失败时的回退）。
func parseFactsFromLines(content string) []ExtractedFact {
	var facts []ExtractedFact
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "0123456789. -•·*_")
		line = strings.TrimSpace(line)
		if line != "" && line != "无" && len([]rune(line)) >= 4 {
			facts = append(facts, ExtractedFact{
				Content:    line,
				Layer:      LayerLongTerm,
				Importance: estimateImportance(line),
				Metadata:   map[string]any{"source": "session_summary"},
			})
		}
	}
	return facts
}

// ---------------------------------------------------------------------------
// Prompt
// ---------------------------------------------------------------------------

const sessionSummarySystemPromptFallback = `你是一个会话记忆汇总器。从完整会话记录中提取值得跨会话长期保存的关键信息。

提取重点：
1. 用户偏好（写作风格、展示格式、决策习惯）
2. 重要决策和结论
3. 领域知识和专业背景信息
4. 反复出现的主题和关注点

忽略（不要提取）：
- 寒暄和礼貌用语
- 一次性的事实查询
- 纯工具调用和操作指令

请返回 JSON 格式：{"facts": ["事实1", "事实2", ...]}
每条事实一句话，使用第三人称。无长期价值时返回 {"facts": []}`
