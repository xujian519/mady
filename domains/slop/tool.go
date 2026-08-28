package slop

import (
	"context"
	"encoding/json"

	"github.com/xujian519/mady/agentcore"
)

// NewSlopGateTool 创建 slop_gate 工具：对草稿文本做确定性套话扫描，
// 输出 pass/needs_revision 判定与逐条修订建议。纯规则、零 LLM 调用，
// 可在工作流检查步骤或起草后直接调用。
func NewSlopGateTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name: "slop_gate",
		Description: "反套话闸门：对专利文书草稿（权利要求/说明书/答复意见/分析报告）做确定性套话扫描。" +
			"检查四类问题：无数据支撑的效果断言、无引证的结论式评述、空话模板、同一套话重复。" +
			"输出 pass/needs_revision 判定与逐条修订建议；判定为 needs_revision 时应修订后重新提交。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "待检查的草稿全文",
				},
			},
			"required": []string{"text"},
		},
		ReadOnly: true,
		Func:     handleSlopGate,
	}
}

func handleSlopGate(_ context.Context, args json.RawMessage) (any, error) {
	var input struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return agentcore.NewFailureResult("参数解析失败", "slop_gate 参数格式错误"), nil
	}
	if input.Text == "" {
		return agentcore.NewFailureResult("输入为空", "text 不能为空"), nil
	}

	report := Check(input.Text)
	output := map[string]any{
		"ok":          true,
		"verdict":     report.Verdict,
		"score":       report.Score,
		"finding_num": len(report.Findings),
		"report":      report,
	}
	if report.Verdict == VerdictNeedsRevision {
		output["revision_hint"] = "存在需修订的套话或无支撑断言，请按 findings 的建议修订后重新提交"
	}
	data, err := json.Marshal(output)
	if err != nil {
		return agentcore.NewFailureResult("序列化失败", err.Error()), nil
	}
	return string(data), nil
}
