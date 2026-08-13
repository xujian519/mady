package piagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// NewSpawnAgentTool 创建 spawn_agent 工具。
//
// registered 是父 Agent 已注册的工具快照（BuildTools 中除 spawn_agent 外的
// 全部工具），子会话白名单从其中解析（03-design.md §2.2）。
func NewSpawnAgentTool(cfg SpawnConfig, registered []*agentcore.Tool) *agentcore.Tool {
	paramTypes := map[string]any{
		"subagent_type": map[string]any{
			"type":        "string",
			"enum":        PresetNames(),
			"description": "子会话预设：explore 只读探查 / verify 只读核验 / plan 只读规划 / general-purpose 通用",
		},
		"directive": map[string]any{
			"type":        "string",
			"description": "定向任务指令（子会话的系统提示词主体，须包含明确范围与输出要求）",
		},
		"tools": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "可选，追加到预设白名单的 Mady 工具名",
		},
		"exclude_tools": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "可选，从生效白名单中剔除的工具名",
		},
		"model": map[string]any{
			"type":        "string",
			"description": "可选，子会话模型规格（pi 格式，如 deepseek/deepseek-chat；支持 :thinking 后缀）",
		},
		"thinking": map[string]any{
			"type":        "string",
			"enum":        []any{"off", "minimal", "low", "medium", "high", "xhigh"},
			"description": "可选，子会话推理档位（默认 medium，按模型能力自动收敛）",
		},
		"max_tokens": map[string]any{
			"type":        "integer",
			"minimum":     float64(256),
			"description": "可选，子会话单轮输出上限",
		},
	}

	presetDesc := make([]string, 0, len(Presets))
	for _, p := range Presets {
		presetDesc = append(presetDesc, fmt.Sprintf("%s：%s", p.Name, p.Description))
	}

	return &agentcore.Tool{
		Name: "spawn_agent",
		Description: "派发一个受限的辅助智能体（子会话）完成定向探查/核验/规划任务，返回结构化报告。" +
			"子会话拥有独立上下文，不会污染父会话；工具白名单由预设决定，只读预设拒绝写类工具。" +
			"预设：" + strings.Join(presetDesc, "；") +
			"。禁止嵌套派发（子会话内 spawn_agent 不可用）。",
		Parameters: map[string]any{
			"type":       "object",
			"properties": paramTypes,
			"required":   []any{"subagent_type", "directive"},
		},
		ReadOnly: true,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var params SpawnParams
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if params.SubagentType == "" || params.Directive == "" {
				return nil, fmt.Errorf("subagent_type 与 directive 为必填参数")
			}
			outcome := RunSpawn(ctx, cfg, registered, params)
			text, err := outcome.Marshal()
			if err != nil {
				return nil, err
			}
			if !outcome.Success {
				return text, nil
			}
			return text, nil
		},
	}
}
