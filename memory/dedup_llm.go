package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// llmDedupDecider 通过 agentcore.Provider 调用 LLM 进行去重判定。
// 参考 Mem0 的 ADD/UPDATE/DELETE/NOOP 判定逻辑。
type llmDedupDecider struct {
	provider agentcore.Provider
	model    string
}

// NewLLMDedupDecider 创建一个基于 LLM 的去重判定器。
func NewLLMDedupDecider(provider agentcore.Provider, model string) *llmDedupDecider {
	if model == "" {
		model = "default"
	}
	return &llmDedupDecider{provider: provider, model: model}
}

// Decide 实现 DedupDecider 接口。
func (d *llmDedupDecider) Decide(ctx context.Context, newFact string, existing []ScoredMemory) (DedupAction, string, error) {
	if len(existing) == 0 {
		return DedupAdd, "无已有记忆，直接新增", nil
	}

	userPrompt := buildDedupPrompt(newFact, existing)

	req := &agentcore.ProviderRequest{
		Model: d.model,
		Messages: []agentcore.Message{
			{Role: agentcore.RoleSystem, Content: dedupSystemPrompt},
			{Role: agentcore.RoleUser, Content: userPrompt},
		},
		Temperature: 0.0, // 判定任务需要确定性
		MaxTokens:   256,
		ResponseFormat: agentcore.NewJSONSchemaResponseFormat(
			"dedup_decision",
			dedupDecisionSchema,
		),
	}

	resp, err := d.provider.Complete(ctx, req)
	if err != nil {
		return DedupAdd, "", fmt.Errorf("dedup llm call failed: %w", err)
	}

	return parseDedupDecision(resp.Content)
}

// ---------------------------------------------------------------------------
// Prompt 模板
// ---------------------------------------------------------------------------

const dedupSystemPrompt = `你是一个记忆去重判定器。你的任务是比较一条新事实与已有记忆，判断应采取什么操作。

判定规则：
- ADD: 新事实与已有记忆完全不同，应新增
- UPDATE: 新事实与已有记忆相关但有更新/补充，应更新已有记忆
- DELETE: 新事实表明已有记忆不再正确，应删除
- NOOP: 新事实与已有记忆完全相同，无需操作

请严格按 JSON Schema 返回结果。`

var dedupDecisionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"action": map[string]any{
			"type":        "string",
			"enum":        []string{"add", "update", "delete", "noop"},
			"description": "判定的操作类型",
		},
		"reason": map[string]any{
			"type":        "string",
			"description": "判定原因，用中文简要说明",
		},
	},
	"required": []string{"action", "reason"},
}

// buildDedupPrompt 构建去重判定的用户提示词。
func buildDedupPrompt(newFact string, existing []ScoredMemory) string {
	var b strings.Builder
	b.WriteString("新事实：\n")
	b.WriteString(newFact)
	b.WriteString("\n\n已有记忆：\n")

	for i, sm := range existing {
		fmt.Fprintf(&b, "[%d] (相似度: %.2f) %s\n", i+1, sm.Semantic, sm.Entry.Content)
	}

	return b.String()
}

// dedupDecisionResponse 是 LLM 返回的去重判定响应。
type dedupDecisionResponse struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// parseDedupDecision 解析 LLM 返回的去重判定。
func parseDedupDecision(content string) (DedupAction, string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return DedupAdd, "空响应，默认新增", nil
	}

	content = stripMarkdownFences(content)

	var resp dedupDecisionResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return DedupAdd, fmt.Sprintf("解析失败(%v)，默认新增", err), nil
	}

	action := DedupAction(strings.ToLower(resp.Action))
	switch action {
	case DedupAdd, DedupUpdate, DedupDelete, DedupNoop:
		return action, resp.Reason, nil
	default:
		return DedupAdd, fmt.Sprintf("未知动作 %q，默认新增", resp.Action), nil
	}
}
