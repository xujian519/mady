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

// NewDebateTool 创建 examiner_debate 工具，
// 封装审查员-代理人模拟辩论 Pregel 图（3 轮审查意见 + 答复往复）。
// 输入权利要求文本和技术交底书，输出模拟辩论记录和答复策略建议。
func NewDebateTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "examiner_debate",
		Description: "模拟审查员与专利代理人之间的审查意见辩论：输入权利要求文本，生成3轮审查意见通知书和代理人答复，输出辩论记录和答复策略建议。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"claims": map[string]any{
					"type":        "string",
					"description": "权利要求文本，将被审查员逐项审查",
				},
				"disclosure": map[string]any{
					"type":        "string",
					"description": "技术交底书内容（可选），用于验证修改是否超范围和支持性审查",
				},
			},
			"required":             []string{"claims"},
			"additionalProperties": false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Claims     string `json:"claims"`
				Disclosure string `json:"disclosure"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "权利要求文本格式错误"), nil
			}
			if p.Claims == "" {
				return agentcore.NewFailureResult("输入为空", "权利要求文本不能为空"), nil
			}

			compiled, err := BuildDebateGraph()
			if err != nil {
				return agentcore.NewFailureResult("辩论引擎初始化失败",
					"审查员辩论模拟功能暂时不可用，请稍后重试。"), nil
			}

			state, err := compiled.Run(ctx, graph.PregelState{
				StateDebateClaims:     p.Claims,
				StateDebateDisclosure: p.Disclosure,
			})
			if err != nil {
				return agentcore.NewFailureResult("辩论执行失败",
					"模拟辩论出现错误。"), nil
			}

			output := state.GetString(StateDebateOutput)
			if output == "" {
				return agentcore.NewFailureResult("结果为空", "辩论完成但未能生成输出。"), nil
			}

			return agentcore.NewHandoffResult("审查意见辩论模拟完成", output), nil
		},
	}
}

// NewInvalidationTool 创建 analyze_patent_invalidation 工具，
// 封装专利无效宣告分析 Pregel 图。
// 支持通过 InvGraphOption 注入检索器等可选依赖。
func NewInvalidationTool(opts ...InvGraphOption) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "analyze_patent_invalidation",
		Description: "对目标专利进行无效宣告分析：输入专利权利要求文本，识别无效理由（新颖性/创造性/充分公开/权利要求清楚/修改超范围），逐项生成无效论证骨架并经规则引擎校验完整性。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patent_claims": map[string]any{
					"type":        "string",
					"description": "目标专利的权利要求文本及请求人提出的无效理由",
				},
			},
			"required":             []string{"patent_claims"},
			"additionalProperties": false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Claims string `json:"patent_claims"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "权利要求文本格式错误"), nil
			}
			if p.Claims == "" {
				return agentcore.NewFailureResult("输入为空", "权利要求文本不能为空"), nil
			}

			compiled, err := BuildInvalidationGraphWithOpts(opts...)
			if err != nil {
				return agentcore.NewFailureResult("分析引擎初始化失败",
					"专利无效宣告分析功能暂时不可用，请稍后重试。"), nil
			}

			state, err := compiled.Run(ctx, graph.PregelState{
				InvStateInput: p.Claims,
			})
			if err != nil {
				return agentcore.NewFailureResult("分析执行失败",
					"专利无效宣告分析出现错误。"), nil
			}

			output := state.GetString(InvStateOutput)
			if output == "" {
				return agentcore.NewFailureResult("结果为空", "分析完成但未能生成输出。"), nil
			}

			return agentcore.NewHandoffResult("专利无效宣告分析完成", output), nil
		},
	}
}

// NewReexaminationTool 创建 draft_reexamination_request 工具，
// 封装专利驳回复审请求书起草 Pregel 图。
// 支持通过 ReexamGraphOption 启用口审准备等可选功能。
func NewReexaminationTool(opts ...ReexamGraphOption) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "draft_reexamination_request",
		Description: "根据驳回决定书起草复审请求书：解析驳回决定要素（文号/日期/驳回理由/对比文件），逐条生成复审论证骨架并经规则引擎校验完整性。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"rejection_decision": map[string]any{
					"type":        "string",
					"description": "驳回决定书全文或核心段落",
				},
			},
			"required":             []string{"rejection_decision"},
			"additionalProperties": false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Decision string `json:"rejection_decision"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "驳回决定书文本格式错误"), nil
			}
			if p.Decision == "" {
				return agentcore.NewFailureResult("输入为空", "驳回决定书文本不能为空"), nil
			}

			compiled, err := BuildReexaminationGraph(opts...)
			if err != nil {
				return agentcore.NewFailureResult("引擎初始化失败",
					"复审请求书起草功能暂时不可用，请稍后重试。"), nil
			}

			state, err := compiled.Run(ctx, graph.PregelState{
				ReexamStateInput: p.Decision,
			})
			if err != nil {
				return agentcore.NewFailureResult("起草失败",
					"复审请求书起草出现错误。"), nil
			}

			output := state.GetString(ReexamStateOutput)
			if output == "" {
				return agentcore.NewFailureResult("结果为空", "起草完成但未能生成输出。"), nil
			}

			return agentcore.NewHandoffResult("复审请求书起草完成", output), nil
		},
	}
}
