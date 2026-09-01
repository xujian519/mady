package patent

import (
	"context"
	"encoding/json"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// JSON Schema field constants used in tool parameter schemas.
const (
	schemaType                 = "type"
	schemaString               = "string"
	schemaObject               = "object"
	schemaProperties           = "properties"
	schemaDescription          = "description"
	schemaRequired             = "required"
	schemaAdditionalProperties = "additionalProperties"
)

// NewOAResponseTool creates the draft_oa_response tool that wraps the OA
// response Pregel graph for use by the Patent Agent.
//
// The tool takes an OA notification text as input, runs it through the
// Pregel pipeline (parse → classify → analyze → draft → [llm_enhance] → approve),
// and returns a structured response skeleton.
// Supports optional LLM enhancement via OAGraphOption.
func NewOAResponseTool(opts ...OAGraphOption) *agentcore.Tool {
	return &agentcore.Tool{
		Name: "draft_oa_response",
		Description: `起草审查意见答复书：输入审查意见通知书文本，自动解析驳回类型和引用对比文件，` +
			`分析受影响的权利要求，制定答复策略，生成结构化答复书骨架。
输出包含：权利要求修改对照表、答复策略建议、对比文件分析、法律依据引用。
如有 LLM 增强配置，将在确定性骨架基础上自动生成实质性论证段落。`,
		Parameters: map[string]any{
			schemaType: schemaObject,
			schemaProperties: map[string]any{
				"oa_text": map[string]any{
					schemaType:        schemaString,
					schemaDescription: "审查意见通知书全文文本（支持中文）",
				},
				"claim_text": map[string]any{
					schemaType:        schemaString,
					schemaDescription: "当前权利要求书文本（可选，用于更精确的权利要求分析）",
				},
			},
			schemaRequired:             []string{"oa_text"},
			schemaAdditionalProperties: false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				OAText    string `json:"oa_text"`
				ClaimText string `json:"claim_text"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				// 参数错误转为结构化失败响应返回调用方。
				return agentcore.NewFailureResult("参数解析失败", "OA 通知书文本格式错误"), nil
			}
			if p.OAText == "" {
				return agentcore.NewFailureResult("输入为空", "审查意见通知书文本不能为空"), nil
			}

			compiled, err := BuildOAResponseGraphWithOpts(opts...)
			if err != nil {
				// 图构建失败显式降级响应，不向调用方暴露内部错误。
				return agentcore.NewFailureResult("答复引擎初始化失败",
					"OA 答复功能暂时不可用，请稍后重试。"), nil
			}

			initialState := graph.PregelState{
				OAStateInput: p.OAText,
			}
			if p.ClaimText != "" {
				initialState["claim_text"] = p.ClaimText
			}

			state, err := compiled.Run(ctx, initialState)
			if err != nil {
				// 图执行失败显式降级响应。
				return agentcore.NewFailureResult("答复生成失败",
					"OA 答复生成过程出现错误。"), nil
			}

			output := state.GetString(OAStateOutput)
			if output == "" {
				return agentcore.NewFailureResult("结果为空", "分析完成但未能生成输出。"), nil
			}

			return agentcore.NewHandoffResult("OA 答复书骨架已生成", output), nil
		},
	}
}
