package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// AsTool 将 Worker Executor 包装为 agentcore.Tool，使其可注册到 Agent 的工具列表中。
//
// Tool 元数据从 Worker Definition 派生：
//   - Name:        worker.Definition.Name
//   - Description: worker.Definition.Description
//   - Parameters:  从 Inputs 自动构建 JSON Schema
//   - Func:        将 state 提取的参数传给 Executor 的 PregelNode 执行
//
// 这与 AsPregelNode 不同：AsTool 生成的 Tool 被 LLM 调用（参数由 LLM 提供），
// 而 AsPregelNode 生成的 PregelNode 在 Pregel 图中执行（参数从 PregelState 提取）。
func AsTool(exec *Executor) *agentcore.Tool {
	def := exec.def

	// 从 Inputs 构建参数 schema
	params := map[string]any{
		"type":       "object",
		"properties": buildParamProperties(def.Inputs),
		"required":   buildRequiredParams(def.Inputs),
	}

	tool := &agentcore.Tool{
		Name:        def.Name,
		Description: def.Description,
		Parameters:  params,
		ReadOnly:    false, // Worker 产生输出产物，默认可写
		Func:        buildToolFunc(exec),
	}
	return tool
}

// buildParamProperties 从 Worker Inputs 构建 JSON Schema properties。
//
// 当前所有参数类型硬编码为 "string"（LLM 调用时参数通过文本传递），
// 后续可根据 Input.ContentSchema 扩展为 array/object 类型。
func buildParamProperties(inputs []Input) map[string]any {
	if len(inputs) == 0 {
		return map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "分析任务描述",
			},
		}
	}
	props := make(map[string]any, len(inputs))
	for _, in := range inputs {
		key := stateKeyFromPath(in.Path)
		desc := in.Description
		if desc == "" {
			desc = fmt.Sprintf("输入参数 %s", key)
		}
		props[key] = map[string]any{
			"type":        "string",
			"description": desc,
		}
	}
	return props
}

// buildRequiredParams 返回必需的参数名列表。
func buildRequiredParams(inputs []Input) []string {
	if len(inputs) == 0 {
		return []string{"query"}
	}
	var required []string
	for _, in := range inputs {
		if !in.Optional {
			key := stateKeyFromPath(in.Path)
			required = append(required, key)
		}
	}
	return required
}

// buildToolFunc 构造 Tool.Func，将 Tool 调用的 JSON 参数转为 PregelState 后交给 Executor 执行。
func buildToolFunc(exec *Executor) agentcore.ToolFunc {
	node := exec.ToPregelNode()
	def := exec.def

	return func(ctx context.Context, args json.RawMessage) (any, error) {
		// 解析参数
		var raw map[string]any
		if err := json.Unmarshal(args, &raw); err != nil {
			return agentcore.NewFailureResult("参数解析失败",
				fmt.Sprintf("Worker %q 参数格式错误", def.Name)), nil
		}

		// 转为 PregelState
		state := make(graph.PregelState)
		for k, v := range raw {
			state[k] = v
		}

		// 执行
		result, err := node(ctx, state)
		if err != nil {
			return agentcore.NewFailureResult("执行失败",
				fmt.Sprintf("Worker %q 执行出错", def.Name)), nil
		}

		// 从 output key 提取结果
		var output strings.Builder
		if len(def.Outputs) > 0 {
			for _, out := range def.Outputs {
				key := stateKeyFromPath(out.Path)
				if v, ok := result[key]; ok {
					if output.Len() > 0 {
						output.WriteString("\n\n")
					}
					fmt.Fprintf(&output, "%v", v)
				}
			}
		} else {
			// 无输出契约时尝试常用 key
			for _, key := range []string{"output", "result", def.Name} {
				if v, ok := result[key]; ok {
					fmt.Fprintf(&output, "%v", v)
					break
				}
			}
		}

		// 检查是否有降解标记
		if graph.HasDegradation(result) {
			summary := graph.DegradationSummary(result)
			var notes []string
			for _, d := range summary {
				notes = append(notes, fmt.Sprintf("[%s] %s", d.Severity, d.Message))
			}
			if len(notes) > 0 {
				output.WriteString("\n\n⚠️ 执行警告:\n")
				output.WriteString(strings.Join(notes, "\n"))
			}
		}

		resultStr := strings.TrimSpace(output.String())
		if resultStr == "" {
			return agentcore.NewFailureResult("结果为空",
				fmt.Sprintf("Worker %q 执行完成但未产生输出", def.Name)), nil
		}

		return agentcore.NewHandoffResult(
			fmt.Sprintf("Worker %q 执行完成", def.Name),
			resultStr,
		), nil
	}
}
