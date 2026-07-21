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

// NewSpecificationTool 创建 specification_drafter 工具，
// 封装专利说明书撰写 Pregel 图（技术领域→背景技术→发明内容→附图说明→具体实施方式）。
// 输入技术交底书和权利要求文本，输出完整的说明书文档（Markdown 格式）。
func NewSpecificationTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "specification_drafter",
		Description: "根据技术交底书和权利要求撰写完整的专利说明书，包含技术领域、背景技术、发明内容、附图说明和具体实施方式五个标准章节。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"disclosure": map[string]any{
					"type":        "string",
					"description": "技术交底书内容，描述发明的技术问题、技术方案和有益效果",
				},
				"claims": map[string]any{
					"type":        "string",
					"description": "权利要求文本（可选），用于填充发明内容部分的技术方案描述",
				},
			},
			"required":             []string{"disclosure"},
			"additionalProperties": false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Disclosure string `json:"disclosure"`
				Claims     string `json:"claims"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "技术交底书文本格式错误"), nil
			}
			if p.Disclosure == "" {
				return agentcore.NewFailureResult("输入为空", "技术交底书内容不能为空"), nil
			}

			compiled, err := BuildSpecificationGraph()
			if err != nil {
				return agentcore.NewFailureResult("撰写引擎初始化失败",
					"说明书撰写功能暂时不可用，请稍后重试。"), nil
			}

			state, err := compiled.Run(ctx, graph.PregelState{
				StateSpecDisclosure: p.Disclosure,
				StateSpecClaims:     p.Claims,
			})
			if err != nil {
				return agentcore.NewFailureResult("撰写执行失败",
					"说明书撰写出现错误。"), nil
			}

			output := state.GetString(StateSpecOutput)
			if output == "" {
				return agentcore.NewFailureResult("结果为空", "撰写完成但未能生成输出。"), nil
			}

			return agentcore.NewHandoffResult("说明书撰写完成", output), nil
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
