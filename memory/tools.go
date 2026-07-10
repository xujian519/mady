package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// NewMemoryTools 创建记忆相关的工具列表。
// 借鉴 Letta 的自我编辑记忆模式：Agent 通过 Tool Calling 主动管理记忆。
func NewMemoryTools(manager *Manager, scope MemoryScope) []*agentcore.Tool {
	if manager == nil {
		return nil
	}

	return []*agentcore.Tool{
		{
			Name:        "remember",
			Description: "记住一条重要信息，以便将来参考。适用于需要长期保存的用户偏好、决策、事实等。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{
						"type":        "string",
						"description": "要记住的内容，用完整的自然语言描述",
					},
					"importance": map[string]any{
						"type":        "number",
						"description": "重要性 (0.0~1.0)，1.0 = 极其重要",
						"default":     0.5,
					},
					"layer": map[string]any{
						"type":        "string",
						"description": "存储层级: 'long_term' (长期记忆, 跨会话) 或 'user' (用户偏好)",
						"default":     "long_term",
					},
				},
				"required": []string{"content"},
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				return handleRemember(ctx, manager, scope, args)
			},
		},
		{
			Name:        "recall",
			Description: "搜索历史记忆。当需要回忆之前讨论过的内容、用户偏好或已经做出的决策时使用。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "搜索关键词或问题",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "最大返回结果数",
						"default":     5,
					},
				},
				"required": []string{"query"},
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				return handleRecall(ctx, manager, scope, args)
			},
		},
		{
			Name:        "forget",
			Description: "删除一条已保存的记忆。当记忆不再正确或用户要求删除时使用。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"memory_id": map[string]any{
						"type":        "string",
						"description": "要删除的记忆 ID",
					},
				},
				"required": []string{"memory_id"},
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				return handleForget(ctx, manager, args)
			},
		},
	}
}

// --- Tool Handlers ---

type rememberArgs struct {
	Content    string  `json:"content"`
	Importance float64 `json:"importance"`
	Layer      string  `json:"layer"`
}

func handleRemember(ctx context.Context, manager *Manager, scope MemoryScope, args json.RawMessage) (any, error) {
	var p rememberArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	if p.Content == "" {
		return "请提供要记住的内容", nil
	}
	var layer MemoryLayer
	switch p.Layer {
	case "user":
		layer = LayerUser
	case "session":
		layer = LayerSession
	default:
		layer = LayerLongTerm
	}
	if p.Importance <= 0 {
		p.Importance = 0.5
	}

	id, err := manager.Remember(ctx, p.Content, scope, layer, map[string]any{
		"source":     "tool",
		"importance": p.Importance,
		"saved_at":   time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Sprintf("保存失败: %v", err), nil
	}

	layerLabel := map[MemoryLayer]string{
		LayerUser:     "用户偏好",
		LayerSession:  "会话上下文",
		LayerLongTerm: "长期记忆",
	}[layer]

	return fmt.Sprintf("已保存到 %s (ID: %s, 重要性: %.1f)", layerLabel, id, p.Importance), nil
}

type recallArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func handleRecall(ctx context.Context, manager *Manager, scope MemoryScope, args json.RawMessage) (any, error) {
	var p recallArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	if p.Query == "" {
		return "请提供搜索关键词", nil
	}
	if p.Limit <= 0 {
		p.Limit = 5
	}

	filter := MemoryFilter{
		UserID: scope.UserID,
		TopK:   p.Limit,
	}
	results, err := manager.Search(ctx, p.Query, filter)
	if err != nil {
		return fmt.Sprintf("搜索失败: %v", err), nil
	}

	if len(results) == 0 {
		return "未找到相关记忆", nil
	}

	var output strings.Builder
	for i, sr := range results {
		layerLabel := string(sr.Entry.Layer)
		fmt.Fprintf(&output, "\n[%d] (相关度: %.2f, 类型: %s) %s",
			i+1, sr.Composite, layerLabel, sr.Entry.Content)
	}
	fmt.Fprintf(&output, "\n\n共找到 %d 条相关记忆", len(results))

	return output.String(), nil
}

type forgetArgs struct {
	MemoryID string `json:"memory_id"`
}

func handleForget(ctx context.Context, manager *Manager, args json.RawMessage) (any, error) {
	var p forgetArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err), nil
	}
	if p.MemoryID == "" {
		return "请提供要删除的记忆 ID", nil
	}

	if err := manager.Forget(ctx, p.MemoryID); err != nil {
		return fmt.Sprintf("删除失败: %v", err), nil
	}

	return fmt.Sprintf("记忆 %s 已删除", p.MemoryID), nil
}
