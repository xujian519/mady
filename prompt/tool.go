package prompt

import (
	"context"
	"encoding/json"

	"github.com/xujian519/mady/agentcore"
)

// PromptSummary is a lightweight view of a prompt template for tool output.
type PromptSummary struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Domain      string   `json:"domain"`
	Category    string   `json:"category"`
	Version     string   `json:"version"`
	Triggers    []string `json:"triggers,omitempty"`
}

// listResult wraps prompt summaries for JSON tool output.
type listResult struct {
	Templates []PromptSummary `json:"templates"`
	Count     int             `json:"count"`
}

// NewListPromptsTool creates the list_prompts agent tool.
// It exposes the prompt template catalog so the agent can browse available
// templates by category, domain, or keyword.
func NewListPromptsTool(store *PromptStore) *agentcore.Tool {
	return &agentcore.Tool{
		Name:     "list_prompts",
		ReadOnly: true,
		Description: "列出可用的提示词模板，支持按 category（search/analysis/drafting/oa/disclosure/quality/legal/memory/evaluate/guardian/workflow）" +
			"、domain（patent/legal/memory/evaluate/guardian）和关键词模糊搜索。返回模板名、标题、描述、触发词。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"category": map[string]any{
					"type":        "string",
					"description": "可选，按类别筛选",
				},
				"domain": map[string]any{
					"type":        "string",
					"description": "可选，按领域筛选",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "可选，按模板名/标题/描述模糊搜索",
				},
			},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Category string `json:"category"`
				Domain   string `json:"domain"`
				Query    string `json:"query"`
			}
			if len(args) > 0 {
				if err := json.Unmarshal(args, &p); err != nil {
					return agentcore.NewFailureResult("参数错误", "无法解析 list_prompts 参数"), nil
				}
			}

			templates := store.List(ListOptions{
				Category: p.Category,
				Domain:   p.Domain,
				Query:    p.Query,
			})

			if len(templates) == 0 {
				return agentcore.NewHandoffResult(
					"list_prompts",
					"没有匹配的提示词模板。",
				), nil
			}

			summaries := make([]PromptSummary, len(templates))
			for i, t := range templates {
				summaries[i] = PromptSummary{
					Name:        t.Name,
					Title:       t.Title,
					Description: t.Description,
					Domain:      t.Domain,
					Category:    t.Category,
					Version:     t.Version,
					Triggers:    t.Triggers,
				}
			}

			result := listResult{
				Templates: summaries,
				Count:     len(summaries),
			}
			data, err := json.Marshal(result)
			if err != nil {
				return agentcore.NewFailureResult("序列化失败", err.Error()), nil
			}
			return agentcore.NewHandoffResult("list_prompts", string(data)), nil
		},
	}
}
