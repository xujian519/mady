package adversary

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// 工具名。
const (
	// CheckToolName 是对立审查员（计划评估）工具名。
	CheckToolName = "adversary_check"
	// ReviewToolName 是换位审查（最终产物评估）工具名。
	ReviewToolName = "adversary_review"
)

const checkToolDescription = `从对立审查员视角评估 OA 答复计划的薄弱环节（信息隔离：只读通知书、权利要求与答复计划）。

确定性部分：独立识别争议法条（驳回类型→法条）、检查答复计划是否覆盖全部驳回理由、每条策略的权项与通知书涉及权项是否对齐、与基线比对标记一致/分歧/基线未覆盖。
模型部分（可选）：把 Agent 按 prompt 得到的 JSON 作为 model_output 传入——模型争议点按法条合并（置信度高者优先），其摘要用作报告摘要；模型输出包含禁止术语时整段丢弃并回退确定性结果。

不传 model_output 时返回 prompt（含审查意见、权利要求、答复计划），供 Agent 调用后回传。本工具不下法律结论。`

const reviewToolDescription = `换位审查：从对立审查员视角评估**最终意见陈述书**的论证强度。

确定性检查：通知书驳回理由是否逐项回应（关键词）、是否引用对比文件内容、是否引用权利要求编号；据此给出论证强度（strong/weak/insufficient）、遗漏理由、证据缺口与整体置信度。
与 adversary_check 的分工：adversary_check 评估「答复计划」，本工具评估「写好的意见陈述书」。不替代人工审阅。`

// CheckResult 是计划评估结果。
type CheckResult struct {
	Report    Report           `json:"report"`
	Rendered  string           `json:"rendered"`
	Prompt    string           `json:"prompt,omitempty"`
	ModelUsed bool             `json:"model_used"`
	Isolation *IsolationResult `json:"isolation,omitempty"`
	Note      string           `json:"note,omitempty"`
}

// ReviewResult 是换位审查结果。
type ReviewResult struct {
	Report   ReviewReport `json:"report"`
	Rendered string       `json:"rendered"`
}

type officeActionArg struct {
	RejectionTypes    []string `json:"rejection_types"`
	AffectedClaims    []int    `json:"affected_claims"`
	ExaminerArguments []string `json:"examiner_arguments,omitempty"`
}

func (a officeActionArg) toOfficeAction() *OfficeAction {
	return &OfficeAction{
		RejectionTypes:    a.RejectionTypes,
		AffectedClaims:    a.AffectedClaims,
		ExaminerArguments: a.ExaminerArguments,
	}
}

var officeActionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"rejection_types": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "驳回类型（novelty/inventiveness/clarity/support/scope/disclosure/formal 等）",
		},
		"affected_claims": map[string]any{
			"type": "array", "items": map[string]any{"type": "integer", "minimum": 1},
		},
		"examiner_arguments": map[string]any{
			"type": "array", "items": map[string]any{"type": "string"},
		},
	},
}

type checkArgs struct {
	OfficeAction        *officeActionArg `json:"office_action"`
	ClaimsText          string           `json:"claims_text"`
	ResponsePlan        []PlanEntry      `json:"response_plan"`
	ResponsePlanSummary string           `json:"response_plan_summary"`
	ModelOutput         string           `json:"model_output"`
	StateKeys           []string         `json:"state_keys"`
}

// NewCheckTool 创建 adversary_check 工具。
func NewCheckTool() *agentcore.Tool {
	t := &checkTool{}
	return &agentcore.Tool{
		Name:        CheckToolName,
		Description: checkToolDescription,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"office_action":         officeActionSchema,
				"claims_text":           map[string]any{"type": "string", "description": "权利要求书正文"},
				"response_plan":         responsePlanSchema(),
				"response_plan_summary": map[string]any{"type": "string"},
				"model_output": map[string]any{
					"type":        "string",
					"description": "按 prompt 得到的模型 JSON；省略时返回 prompt",
				},
				"state_keys": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "可选：调用方持有的上游状态键，用于信息隔离检查",
				},
			},
			"required":             []string{"office_action"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

func responsePlanSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "答复计划条目（每项策略一行）",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ground":   map[string]any{"type": "string"},
				"strategy": map[string]any{"type": "string"},
				"claims":   map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
				"sections": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"ground", "strategy"},
		},
	}
}

type checkTool struct{}

func (t *checkTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var input checkArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if input.OfficeAction == nil {
		return nil, fmt.Errorf("缺少必填字段 'office_action'")
	}
	officeAction := input.OfficeAction.toOfficeAction()
	// 空对象与「未提供通知书」等价：否则会跳过正文扫描的未验证分支，
	// 静默产出低质量结论。
	if len(officeAction.RejectionTypes) == 0 && len(officeAction.AffectedClaims) == 0 &&
		len(officeAction.ExaminerArguments) == 0 {
		officeAction = nil
	}

	result := CheckResult{}
	if len(input.StateKeys) > 0 {
		isolation := CheckInputIsolation(input.StateKeys)
		result.Isolation = &isolation
		if !isolation.Clean {
			return nil, fmt.Errorf("信息隔离检查未通过：输入包含己方中间推理键 %s",
				strings.Join(isolation.LeakedKeys, "、"))
		}
	}

	located := LocateDisputesIndependent(officeAction, input.ClaimsText)
	assessed := AssessEvidenceIndependent(officeAction, input.ResponsePlan)
	heuristic := MergeHeuristicDisputes(located, assessed)

	prompt := BuildPrompt(officeAction, input.ClaimsText, input.ResponsePlan, input.ResponsePlanSummary)
	if strings.TrimSpace(input.ModelOutput) == "" {
		report, note := degradedResult(heuristic, input.ResponsePlan, "")
		result.Report, result.Rendered, result.Prompt, result.Note = report, RenderReport(report), prompt, note
		return result, nil
	}

	parsed, ok := ParseModelOutput(input.ModelOutput)
	if !ok {
		report, note := degradedResult(heuristic, input.ResponsePlan, "模型输出不是可解析的 JSON，已回退到确定性结果。")
		result.Report, result.Rendered, result.Note = report, RenderReport(report), note
		return result, nil
	}
	parts := []string{parsed.OverallAssessment}
	for _, dispute := range parsed.Disputes {
		parts = append(parts, dispute.Point, dispute.DivergenceReason)
	}
	if leak := ScanOutputForLeaks(strings.Join(parts, "\n")); !leak.Clean {
		report, note := degradedResult(heuristic, input.ResponsePlan,
			fmt.Sprintf("模型输出包含禁止术语（%s），已丢弃并回退到确定性结果。", strings.Join(leak.LeakedKeys, "、")))
		result.Report, result.Rendered, result.Note = report, RenderReport(report), note
		return result, nil
	}

	merged := MergeWithModel(heuristic, parsed.Disputes)
	marked := DetectDivergence(merged, input.ResponsePlan)
	summary := parsed.OverallAssessment
	if strings.TrimSpace(summary) == "" {
		summary = DegradedSummary(marked)
	}
	report := BuildReport(marked, summary)
	result.Report, result.Rendered, result.ModelUsed = report, RenderReport(report), true
	return result, nil
}

func degradedResult(disputes []Dispute, plan []PlanEntry, note string) (Report, string) {
	marked := DetectDivergence(disputes, plan)
	summary := DegradedSummary(marked)
	if note != "" {
		summary += "\n\n" + note
	}
	report := BuildReport(marked, summary)
	return report, summary
}

type reviewArgs struct {
	FinalDraft         string           `json:"final_draft"`
	ReferenceDocuments string           `json:"reference_documents"`
	Claims             string           `json:"claims"`
	OfficeAction       *officeActionArg `json:"office_action"`
}

// NewReviewTool 创建 adversary_review 工具。
func NewReviewTool() *agentcore.Tool {
	t := &reviewTool{}
	return &agentcore.Tool{
		Name:        ReviewToolName,
		Description: reviewToolDescription,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"final_draft":         map[string]any{"type": "string", "description": "最终意见陈述书全文"},
				"reference_documents": map[string]any{"type": "string", "description": "对比文件原文或引用信息"},
				"claims":              map[string]any{"type": "string", "description": "权利要求书正文"},
				"office_action":       officeActionSchema,
			},
			"required":             []string{"final_draft"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type reviewTool struct{}

func (t *reviewTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var input reviewArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if strings.TrimSpace(input.FinalDraft) == "" {
		return nil, fmt.Errorf("缺少必填字段 'final_draft'")
	}
	reviewInput := ReviewInput{
		FinalDraft:         input.FinalDraft,
		ReferenceDocuments: input.ReferenceDocuments,
		Claims:             input.Claims,
	}
	if input.OfficeAction != nil {
		reviewInput.OfficeAction = input.OfficeAction.toOfficeAction()
	}
	report := ReviewFinalDraft(reviewInput)
	return ReviewResult{Report: report, Rendered: RenderReview(report)}, nil
}

// RenderReview 渲染换位审查报告为可读文本。
func RenderReview(report ReviewReport) string {
	lines := []string{
		"换位审查报告：" + report.Summary,
	}
	if len(report.Disputes) > 0 {
		lines = append(lines, "", "争议点：")
		for _, dispute := range report.Disputes {
			claims := "未指定"
			if len(dispute.Claims) > 0 {
				claims = joinInts(dispute.Claims)
			}
			lines = append(lines, fmt.Sprintf("- %s（权项 %s）：%s", dispute.Statute, claims, dispute.Point))
		}
	}
	lines = append(lines, "", "限制说明：确定性骨架按关键词与编号覆盖判断，未做语义论证评估；结论须人工复核。")
	return strings.Join(lines, "\n")
}
