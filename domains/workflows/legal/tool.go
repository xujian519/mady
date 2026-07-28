package legal

import (
	"context"
	"encoding/json"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// NewLegalComparisonTool 创建 compare_legal_cases 工具，
// 封装法律案例比较 Pregel 图（含三段论推理引擎）。
// 支持 LegalGraphOption 注入判例检索器等可选依赖。
func NewLegalComparisonTool(opts ...LegalGraphOption) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "compare_legal_cases",
		Description: "进行法律案例比较分析：输入案件事实描述，检索适用法条，查找相似判例，生成法律分析报告（含三段论推理链）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"case_facts": map[string]any{
					"type":        "string",
					"description": "案件事实描述，包括当事人信息、争议焦点、案件经过",
				},
			},
			"required":             []string{"case_facts"},
			"additionalProperties": false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				CaseFacts string `json:"case_facts"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "案件事实文本格式错误"), nil
			}
			if p.CaseFacts == "" {
				return agentcore.NewFailureResult("输入为空", "案件事实不能为空"), nil
			}

			compiled, rawBB, err := BuildComparisonGraphWithReasoning(
				"case-auto", CaseInvalidation, opts...,
			)
			if err != nil {
				return agentcore.NewFailureResult("分析引擎初始化失败",
					"法律案例分析功能暂时不可用，请稍后重试。"), nil
			}
			if rawBB == nil {
				return agentcore.NewFailureResult("推理引擎初始化失败",
					"法律推理引擎未能正确初始化。"), nil
			}

			bb := WrapBlackboard(rawBB)
			if err := bb.AddFact(FactEntry{
				ID:          "case_facts",
				Content:     p.CaseFacts,
				Source:      "user_text",
				ExtractedAt: time.Now().Format(time.RFC3339),
				Confidence:  0.9,
			}); err != nil {
				return agentcore.NewFailureResult("事实录入失败", err.Error()), nil
			}

			state, err := compiled.Run(ctx, graph.PregelState{
				StateCaseFacts: p.CaseFacts,
			})
			if err != nil {
				return agentcore.NewFailureResult("分析执行失败",
					"法律案例分析出现错误。"), nil
			}

			output := state.GetString(StateConclusion)
			if output == "" {
				output = state.GetString(StateOutput)
			}
			if output == "" {
				return agentcore.NewFailureResult("结果为空", "分析完成但未能生成输出。"), nil
			}

			return agentcore.NewHandoffResult("法律案例分析完成", output), nil
		},
	}
}
