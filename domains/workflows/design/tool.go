package design

import (
	"context"
	"encoding/json"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// NewDesignInvalidationTool 创建 analyze_design_invalidation 工具，
// 封装外观设计专利无效宣告分析 Pregel 图。
// 输入外观设计专利描述，识别无效理由（现有设计/抵触申请/在先权利冲突），
// 按"整体观察、综合判断"四步法生成近似判断分析骨架，
// 经规则引擎校验完整性后输出结构化报告。
//
// 支持通过 DesignGraphOption 注入检索器等可选依赖。
func NewDesignInvalidationTool(opts ...DesignGraphOption) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "analyze_design_invalidation",
		Description: "对外观设计专利进行无效宣告分析：输入外观设计描述（产品名称、设计特征、洛迦诺分类），识别无效理由（A23.1现有设计/A23.2抵触申请/A23.3在先权利冲突），按整体观察综合判断四步法生成近似判断骨架并经规则引擎校验完整性。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"design_description": map[string]any{
					"type":        "string",
					"description": "外观设计专利描述，包括产品名称、设计要点、产品类别（洛迦诺分类号）、设计特征要素等信息",
				},
			},
			"required":             []string{"design_description"},
			"additionalProperties": false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				DesignDescription string `json:"design_description"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "外观设计描述文本格式错误"), nil
			}
			if p.DesignDescription == "" {
				return agentcore.NewFailureResult("输入为空", "外观设计描述不能为空"), nil
			}

			compiled, err := BuildDesignInvalidationGraphWithOpts(opts...)
			if err != nil {
				return agentcore.NewFailureResult("分析引擎初始化失败",
					"外观设计专利无效宣告分析功能暂时不可用，请稍后重试。"), nil
			}

			state, err := compiled.Run(ctx, graph.PregelState{
				DesignStateInput: p.DesignDescription,
			})
			if err != nil {
				return agentcore.NewFailureResult("分析执行失败",
					"外观设计专利无效宣告分析出现错误。"), nil
			}

			output := state.GetString(DesignStateOutput)
			if output == "" {
				return agentcore.NewFailureResult("结果为空", "分析完成但未能生成输出。"), nil
			}

			return agentcore.NewHandoffResult("外观设计专利无效宣告分析完成", output), nil
		},
	}
}
