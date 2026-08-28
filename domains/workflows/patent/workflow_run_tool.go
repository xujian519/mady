package patent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/provenance"
)

// NewPatentWorkflowRunTool 创建 patent_workflow_run 工具：按声明式工作流 manifest
// 路由到既有 Go 入口，输出可执行步骤计划并逐步骤写溯源日志（provenance）。
//
// 本工具只做声明式路由——把 case 类型对应的既有入口（EntryPoint）展开为可执行计划并留痕，
// 真正执行仍由既有图/工具完成，绝不在此重写既有实现。与 run_orchestration 并存：
// 前者显式单流程带溯源，后者 LLM 自组织编排。
func NewPatentWorkflowRunTool(prov *provenance.ProvenanceLogger) *agentcore.Tool {
	return &agentcore.Tool{
		Name: "patent_workflow_run",
		Description: "按声明式专利工作流 manifest（patentability-opinion/search-report/invalidation/oa-response/" +
			"re-examination/patent-drafting/novelty-analysis/infringement-analysis）把既有分析入口展开为可执行计划，" +
			"并写溯源日志。配合既有 run_orchestration 使用；本工具显式单流程带溯源。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workflow_id": map[string]any{"type": "string", "description": "工作流 ID（见 patent 工作流 manifests 目录）"},
			},
			"required": []string{"workflow_id"},
		},
		ReadOnly: true,
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			var p struct {
				WorkflowID string `json:"workflow_id"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return agentcore.NewFailureResult("参数解析失败", "patent_workflow_run 参数格式错误"), nil
			}
			if p.WorkflowID == "" {
				return agentcore.NewFailureResult("缺少参数", "workflow_id 不能为空"), nil
			}

			ms, err := LoadPatentWorkflowManifests()
			if err != nil {
				return agentcore.NewFailureResult("加载失败", err.Error()), nil
			}
			m := FindWorkflowManifest(ms, p.WorkflowID)
			if m == nil {
				return agentcore.NewFailureResult("未知工作流", fmt.Sprintf("workflow_id=%q 不存在（可选: %s）", p.WorkflowID, joinWorkflowIDs(ms))), nil
			}
			if err := ValidateManifestSchema(m); err != nil {
				return agentcore.NewFailureResult("manifest 无效", err.Error()), nil
			}

			steps := make([]map[string]string, 0, len(m.Steps))
			for _, s := range m.Steps {
				label, _ := ResolveWorkflowEntryPoint(s.EntryPoint)
				step := map[string]string{
					"id":          s.ID,
					"step_type":   s.StepType,
					"entry_point": s.EntryPoint,
					"entry_label": label,
				}
				detail := s.ID + " → " + label
				// 条件回退声明：随计划下发，执行侧据此在产出命中时回退重做。
				if s.Retry != nil {
					step["retry_when_output_matches"] = s.Retry.WhenOutputMatches
					step["retry_rewind_to"] = s.Retry.RewindTo
					step["retry_max_retries"] = strconv.Itoa(s.Retry.MaxRetries)
					detail += fmt.Sprintf(" ↻ 命中 /%s/ 时回退 %s（最多 %d 次）",
						s.Retry.WhenOutputMatches, s.Retry.RewindTo, s.Retry.MaxRetries)
				}
				steps = append(steps, step)
				_ = prov.Log(provenance.ProvenanceEvent{
					Kind:       provenance.KindWorkflowStep,
					Tool:       "patent_workflow_run",
					WorkflowID: m.ID,
					Details:    detail,
				})
			}

			out := map[string]any{
				"ok":          true,
				"workflow_id": m.ID,
				"name":        m.Name,
				"case_type":   m.CaseType,
				"steps":       steps,
			}
			data, err := json.Marshal(out)
			if err != nil {
				return agentcore.NewFailureResult("序列化失败", err.Error()), nil
			}
			return string(data), nil
		},
	}
}

func joinWorkflowIDs(ms []PatentWorkflowManifest) string {
	ids := make([]string, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	return strings.Join(ids, ", ")
}
