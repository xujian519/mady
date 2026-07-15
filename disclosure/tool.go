package disclosure

import (
	"context"
	"encoding/json"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// NewDisclosureTool 创建 analyze_disclosure 工具，Patent Agent 可用其触发交底书分析。
// 工具接收交底书文本，运行完整的 10 节点 Pregel 分析流水线，返回结构化分析报告。
func NewDisclosureTool(provider agentcore.Provider) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "analyze_disclosure",
		Description: "分析技术交底书：提取技术问题、特征、效果，进行一致性校验，生成结构化分析报告。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "技术交底书的完整文本内容",
				},
			},
			"required":             []string{"text"},
			"additionalProperties": false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "交底书文本格式错误"), nil
			}
			if p.Text == "" {
				return agentcore.NewFailureResult("输入为空", "交底书文本不能为空"), nil
			}

			compiled, err := BuildDisclosureAnalysisGraph(provider)
			if err != nil {
				return agentcore.NewFailureResult("分析引擎初始化失败",
					"技术交底书分析功能暂时不可用，请稍后重试。"), nil
			}

			state, err := compiled.Run(ctx, graph.PregelState{
				"input": p.Text,
			})
			if err != nil {
				return agentcore.NewFailureResult("分析执行失败",
					"分析过程中出现错误，请检查交底书内容是否完整。"), nil
			}

			report := ExtractReportFromState(state)
			if report == nil {
				return agentcore.NewFailureResult("结果提取失败",
					"分析完成但未能生成结构化报告。"), nil
			}

			reportJSON, err := json.Marshal(report)
			if err != nil {
				return agentcore.NewFailureResult("结果序列化失败",
					"分析完成但报告无法序列化。"), nil
			}
			return agentcore.NewHandoffResult("技术交底书分析完成", string(reportJSON)), nil
		},
	}
}
