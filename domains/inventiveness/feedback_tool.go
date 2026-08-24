package inventiveness

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

// NewInventivenessFeedbackTool 创建记录用户对创造性评估结论的 HITL 反馈工具。
// 反馈落盘到 $MADY_HOME/cases/<caseID>/inventiveness-feedback.jsonl，供后续
// 同案卷 evaluate_inventiveness 的结论节点（generateConclusionNode）加载并
// 注入提示词。该工具可写，故非 ReadOnly——与只读的 evaluate_inventiveness 分离。
func NewInventivenessFeedbackTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "record_inventiveness_feedback",
		Description: "记录用户对创造性（A22.3）评估结论的 HITL 反馈：驳回（rejection）或修正（modification），落盘到案卷目录，供后续同案卷评估的结论节点注入历史反馈以吸取修正。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"case_id": map[string]any{"type": "string", "description": "案卷 ID（必须是纯标识符，不含路径分隔符）"},
				"action":  map[string]any{"type": "string", "enum": []string{"rejection", "modification"}, "description": "反馈类型：rejection 驳回 / modification 修正"},
				"reason":  map[string]any{"type": "string", "description": "反馈原因（可选，供后续结论节点吸取）"},
			},
			"required": []string{"case_id", "action"},
		},
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			var p struct {
				CaseID string `json:"case_id"`
				Action string `json:"action"`
				Reason string `json:"reason"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "反馈参数格式错误"), nil
			}
			if p.CaseID == "" {
				return agentcore.NewFailureResult("缺少 case_id", "case_id 不能为空"), nil
			}
			action := FeedbackAction(p.Action)
			if action != ActionRejection && action != ActionModification {
				return agentcore.NewFailureResult("非法 action", "action 仅支持 rejection / modification"), nil
			}
			dir := CaseFeedbackDir(p.CaseID)
			if dir == "" {
				return agentcore.NewFailureResult("非法 case_id", "case_id 仅允许不含路径分隔符的标识符（或 MadyHome 未配置）"), nil
			}
			if err := AppendInventivenessFeedback(dir, FeedbackEntry{
				CaseID: p.CaseID,
				Action: action,
				Reason: p.Reason,
			}); err != nil {
				return agentcore.NewFailureResult("保存失败", err.Error()), nil
			}
			label := "驳回"
			if action == ActionModification {
				label = "修正"
			}
			return fmt.Sprintf("已记录用户%s反馈（案卷 %s）", label, p.CaseID), nil
		},
	}
}
