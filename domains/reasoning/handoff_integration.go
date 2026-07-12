package reasoning

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

// =============================================================================
// Handoff Integration — FiveStepRunner as an agentcore.Tool
// =============================================================================
//
// The FiveStepRunner implements agentcore.Step. To integrate with the
// agent's LLM loop, we wrap it as a Tool that the patent/legal domain
// agent can invoke when deep reasoning is required.
//
// Usage in domain config:
//
//	cfg.Tools = append(cfg.Tools, reasoning.AsWorkflowTool(runner))
//

// WorkflowToolName is the tool name used to invoke the five-step workflow.
const WorkflowToolName = "run_five_step_workflow"

// WorkflowToolDescription describes what the tool does for the LLM.
const WorkflowToolDescription = `执行五步工作法进行深度专业分析：
① 收集事实 → ② 检索规则 → ③ 制定计划 → ④ 执行推理 → ⑤ 校验结论
适用于专利新颖性/创造性判断、法律条文适用分析等需要结构化推理的场景。
输入为自然语言描述的分析任务。`

// WorkflowToolInput is the JSON schema for the tool's input.
type WorkflowToolInput struct {
	Query     string `json:"query"`      // 分析任务描述
	CaseType  string `json:"case_type"`  // 案件类型，可选
	TechField string `json:"tech_field"` // 技术领域，可选
}

// AsWorkflowTool wraps a FiveStepRunner as an agentcore.Tool.
func AsWorkflowTool(runner *FiveStepRunner) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        WorkflowToolName,
		Description: WorkflowToolDescription,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "分析任务的自然语言描述",
				},
				"case_type": map[string]any{
					"type":        "string",
					"description": "案件类型：novelty_search / patentability / invalidation / fto / validity",
				},
				"tech_field": map[string]any{
					"type":        "string",
					"description": "技术领域，如 '人工智能'、'通信'",
				},
			},
			"required": []string{"query"},
		},
		ReadOnly: true, // workflow analysis has no side effects
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input WorkflowToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("workflow tool: parse args: %w", err)
			}

			// Build the query for the runner.
			query := input.Query
			if input.TechField != "" {
				query = fmt.Sprintf("【技术领域】%s\n【分析任务】%s", input.TechField, input.Query)
			}

			result, err := runner.Run(ctx, query)
			if err != nil {
				return "", fmt.Errorf("workflow tool: execution failed: %w", err)
			}
			return result, nil
		},
	}
}

// NewWorkflowRunner creates a pre-configured FiveStepRunner with
// default templates and optional retriever/checker.
//
// llm is optional — when non-nil, the checker uses it for Level 2 (logical
// consistency) and Level 3 (evidentiary sufficiency) validation. Without it,
// only Level 1 (reference existence) checking is performed.
func NewWorkflowRunner(caseID string, caseType CaseType, techField string, retriever *MultiSourceRetriever, llm LlmClient) *FiveStepRunner {
	planner := NewPlanner(nil)
	for _, v := range DefaultManifests() {
		v := v
		plan := v.Stage4.ToPlan(v.CaseType)
		planner.RegisterTemplate(v.CaseType, PlanIntentChain, *plan)
	}

	// Find matching manifest.
	var manifest *WorkflowManifest
	for _, m := range DefaultManifests() {
		if m.CaseType == caseType {
			m := m
			manifest = m
			break
		}
	}

	return NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:   planner,
		Checker:   NewEnhancedSyllogismChecker(llm, 2),
		Retriever: retriever,
		Manifest:  manifest,
		CaseID:    caseID,
		CaseType:  caseType,
		TechField: techField,
	})
}
