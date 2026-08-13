package piagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sky-valley/pi/agent"
	"github.com/sky-valley/pi/ai"

	"github.com/xujian519/mady/agentcore"
)

// ToolPolicy 是子会话工具执行前的安全门控策略（阶段 1 由调用方注入）。
//
// 返回值约定：返回 nil 表示放行；返回错误表示拒绝执行（错误原文作为
// 工具结果回传模型，不暴露内部堆栈）。
type ToolPolicy func(ctx context.Context, toolName string, args json.RawMessage) error

// BridgeConfig 配置工具桥接行为。
type BridgeConfig struct {
	// ReadOnly 为 true 时，桥接层拒绝写类工具（预设级只读强制，AC-2）。
	ReadOnly bool
	// Policy 是额外的执行前门控（permission Allow/Ask/Deny 由调用方接入）。
	// 可为 nil。返回错误时工具不执行。
	Policy ToolPolicy
}

// ToAgentTool 将 Mady agentcore.Tool 转换为 pi agent.AgentTool。
//
// 转换后的 Execute 包装 Mady 工具实现，并强制执行安全不变量：
//  1. ReadOnly 预设：写类工具在**执行前**拒绝（不产生任何副作用）
//  2. Policy 门控：permission/域裁剪由调用方注入（可为 nil）
//  3. WorkingDir 沙箱由底层 Mady 工具自身执行（resolvePathSandboxed），
//     本层不重复实现
//
// 已知边界（阶段 1 接受）：工具上的 Before/After 生命周期钩子不随桥接
// 传递 —— 内置工具当前未使用该钩子，若后续启用审计类钩子需在此补齐。
//
// 参数 schema 转换失败（如 $ref）时返回错误，由调用方跳过该工具并 WARN
// （03-design.md §1.4 降级策略）。
func ToAgentTool(t *agentcore.Tool, cfg BridgeConfig) (agent.AgentTool, error) {
	if t == nil {
		return agent.AgentTool{}, fmt.Errorf("nil tool")
	}
	schema, err := schemaFromMap(t.Parameters)
	if err != nil {
		return agent.AgentTool{}, fmt.Errorf("tool %s: schema conversion: %w", t.Name, err)
	}

	at := agent.AgentTool{
		Name:        t.Name,
		Description: t.Description,
		Parameters:  schema,
		Label:       t.Name,
	}
	at.Execute = func(ctx context.Context, toolCallID string, params map[string]any, onUpdate agent.ToolUpdateFunc) (agent.AgentToolResult, error) {
		if cfg.ReadOnly && IsWriteTool(t.Name) {
			return agent.AgentToolResult{}, fmt.Errorf("只读子会话不允许调用写类工具 %s", t.Name)
		}
		if cfg.Policy != nil {
			if err := cfg.Policy(ctx, t.Name, mustJSON(params)); err != nil {
				return agent.AgentToolResult{}, err
			}
		}
		res, err := t.Func(ctx, mustJSON(params))
		if err != nil {
			return agent.AgentToolResult{}, err
		}
		content, err := contentFromResult(res)
		if err != nil {
			return agent.AgentToolResult{}, fmt.Errorf("tool %s result: %w", t.Name, err)
		}
		return agent.AgentToolResult{Content: content}, nil
	}
	return at, nil
}

// ToAgentTools 批量转换；转换失败的工具被跳过并计入 skipped（03-design §1.4）。
func ToAgentTools(tools []*agentcore.Tool, cfg BridgeConfig) ([]agent.AgentTool, []string) {
	out := make([]agent.AgentTool, 0, len(tools))
	var skipped []string
	for _, t := range tools {
		at, err := ToAgentTool(t, cfg)
		if err != nil {
			skipped = append(skipped, t.Name)
			continue
		}
		out = append(out, at)
	}
	return out, skipped
}

// contentFromResult 将 Mady 工具返回值转换为 pi 内容块列表。
// string → 纯文本；其他可 JSON 序列化类型 → 紧凑 JSON 文本。
func contentFromResult(res any) (ai.ContentList, error) {
	switch v := res.(type) {
	case string:
		return ai.ContentList{ai.TextContent{Text: v}}, nil
	case []byte:
		return ai.ContentList{ai.TextContent{Text: string(v)}}, nil
	case nil:
		return ai.ContentList{ai.TextContent{Text: ""}}, nil
	default:
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal result: %w", err)
		}
		return ai.ContentList{ai.TextContent{Text: string(b)}}, nil
	}
}

// mustJSON 将参数 map 序列化为 JSON。序列化失败时回退为空对象 ——
// 底层 Mady 工具对非法参数返回参数错误，安全不变量不受影响。
func mustJSON(params map[string]any) json.RawMessage {
	if len(params) == 0 {
		return json.RawMessage("{}")
	}
	b, err := json.Marshal(params)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}
