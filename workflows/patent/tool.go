package patent

import (
	"context"
	"encoding/json"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// NewPatentNoveltyTool 创建 analyze_patent_novelty 工具，
// 封装专利新颖性分析 Pregel 图（含规则引擎检查）。
// 支持通过 GraphOption 注入检索器等可选依赖。
func NewPatentNoveltyTool(opts ...GraphOption) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "analyze_patent_novelty",
		Description: "对发明进行新颖性和创造性分析：输入发明描述，提取技术特征，与现有技术对比，生成结构化分析报告（含规则引擎校验）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"invention_description": map[string]any{
					"type":        "string",
					"description": "发明内容描述，包括技术领域、要解决的技术问题、技术方案和有益效果",
				},
			},
			"required":             []string{"invention_description"},
			"additionalProperties": false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Description string `json:"invention_description"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "发明描述文本格式错误"), nil
			}
			if p.Description == "" {
				return agentcore.NewFailureResult("输入为空", "发明描述不能为空"), nil
			}

			compiled, err := BuildNoveltyGraphWithRulesWithOpts(opts...)
			if err != nil {
				return agentcore.NewFailureResult("分析引擎初始化失败",
					"专利新颖性分析功能暂时不可用，请稍后重试。"), nil
			}

			state, err := compiled.Run(ctx, graph.PregelState{
				StateInput: p.Description,
			})
			if err != nil {
				return agentcore.NewFailureResult("分析执行失败",
					"专利新颖性分析出现错误。"), nil
			}

			output := state.GetString(StateOutput)
			if output == "" {
				return agentcore.NewFailureResult("结果为空", "分析完成但未能生成输出。"), nil
			}

			return agentcore.NewHandoffResult("专利新颖性分析完成", output), nil
		},
	}
}
