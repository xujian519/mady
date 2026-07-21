package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xujian519/mady/agentcore"
)

func nowUnixNano() int64 { return time.Now().UnixNano() }

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
	Query          string          `json:"query"`           // 分析任务描述
	CaseType       string          `json:"case_type"`       // 案件类型，可选
	TechField      string          `json:"tech_field"`      // 技术领域，可选
	CheckpointID   string          `json:"checkpoint_id"`   // 恢复用的 checkpoint ID（确认规则后续跑时传入）
	ConfirmedRules json.RawMessage `json:"confirmed_rules"` // 人工确认的规则集 JSON（恢复时传入）
}

// AsWorkflowTool wraps a FiveStepRunner as an agentcore.Tool without checkpoint
// support (backward-compatible: no confirmation gate). Equivalent to
// AsWorkflowToolWithCheckpoint(runner, nil).
func AsWorkflowTool(runner *FiveStepRunner) *agentcore.Tool {
	return AsWorkflowToolWithCheckpoint(runner, nil)
}

// AsWorkflowToolWithCheckpoint wraps a FiveStepRunner as an agentcore.Tool with
// confirmation-gate checkpoint support. When store is non-nil and the runner is
// configured with RequireRuleConfirmation, the tool:
//   - On normal run: if Stage ② interrupts, saves a checkpoint and returns the
//     checkpoint_id + retrieved rules for human confirmation.
//   - On resume (CheckpointID provided): restores from checkpoint, applies the
//     confirmed rule set, and continues Stage ③④⑤.
//
// When store is nil the tool behaves identically to the legacy version (no
// checkpoint, no resume) — preserving backward compatibility.
func AsWorkflowToolWithCheckpoint(runner *FiveStepRunner, store CheckpointStore) *agentcore.Tool {
	params := map[string]any{
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
	}
	// Extend schema with checkpoint fields when store is configured.
	if store != nil {
		props := params["properties"].(map[string]any)
		props["checkpoint_id"] = map[string]any{
			"type":        "string",
			"description": "恢复规则确认中断的工作流。传入上次中断返回的 checkpoint_id + confirmed_rules 续跑 Stage③④⑤",
		}
		props["confirmed_rules"] = map[string]any{
			"type":        "object",
			"description": "人工确认的规则集（ConfirmedRuleSet JSON）。恢复时必传",
		}
	}

	return &agentcore.Tool{
		Name:        WorkflowToolName,
		Description: WorkflowToolDescription,
		Parameters:  params,
		ReadOnly:    true,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input WorkflowToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("workflow tool: parse args: %w", err)
			}

			// --- Resume path: checkpoint_id provided → restore + confirm + continue ---
			if input.CheckpointID != "" {
				return resumeWorkflow(ctx, store, runner, input)
			}

			// --- Normal path: run from Stage ① ---
			query := input.Query
			if input.TechField != "" {
				query = fmt.Sprintf("【技术领域】%s\n【分析任务】%s", input.TechField, input.Query)
			}

			result, err := runner.Run(ctx, query)
			if err != nil {
				// If the runner interrupted at the confirmation gate, save a
				// checkpoint so the user can resume after confirming rules.
				if store != nil && agentcore.IsInterrupt(err) {
					return handleConfirmationInterrupt(ctx, store, runner, err, query)
				}
				return "", fmt.Errorf("workflow tool: execution failed: %w", err)
			}
			return result, nil
		},
	}
}

// handleConfirmationInterrupt saves a checkpoint at the interruption point and
// returns a message telling the caller how to resume. The retrieved rules are
// included so the human can review/edit them before confirming.
func handleConfirmationInterrupt(ctx context.Context, store CheckpointStore, runner *FiveStepRunner, interruptErr error, query string) (any, error) {
	data := agentcore.InterruptData(interruptErr)
	caseID, _ := data["case_id"].(string)
	if caseID == "" {
		caseID = runner.bb.CaseID
	}
	cpID := fmt.Sprintf("wf-%s-%d", caseID, nowUnixNano())
	if err := runner.SaveCheckpoint(ctx, store, cpID); err != nil {
		return "", fmt.Errorf("workflow tool: save checkpoint: %w", err)
	}

	rules := runner.bb.RuleConstraints()
	return fmt.Sprintf("⏸️ 规则检索完成，等待人工确认（%d 条规则）。\n\n"+
		"checkpoint_id: %s\n\n"+
		"检索到的规则：\n%s\n\n"+
		"请确认后，用 checkpoint_id=%s + confirmed_rules 重新调用本工具续跑。",
		len(rules), cpID, formatRulesForConfirmation(rules), cpID), nil
}

// resumeWorkflow restores a runner from checkpoint, applies the confirmed rule
// set, and continues execution from Stage ③.
func resumeWorkflow(ctx context.Context, store CheckpointStore, template *FiveStepRunner, input WorkflowToolInput) (any, error) {
	// Reuse the template's config (planner/compiler/checker) for the restored runner.
	cfg := FiveStepRunnerConfig{
		Planner:      template.planner,
		NodeBuilder:  nil, // compiler already built; node builder not needed for resume
		Retriever:    template.retriever,
		Checker:      template.checker,
		Manifest:     template.manifest,
		RuleManifest: template.ruleManifest,
		Collectors:   template.stage1Collectors,
	}
	restored, err := ResumeFromCheckpoint(ctx, store, input.CheckpointID, cfg)
	if err != nil {
		return "", fmt.Errorf("workflow tool: resume: %w", err)
	}

	// Apply the human-confirmed rule set (if provided).
	if len(input.ConfirmedRules) > 0 {
		var rs ConfirmedRuleSet
		if err := json.Unmarshal(input.ConfirmedRules, &rs); err != nil {
			return "", fmt.Errorf("workflow tool: parse confirmed_rules: %w", err)
		}
		restored.bb.SetConfirmedRules(rs)
	}

	// Continue from Stage ③ (Plan/Execute/Check).
	result, err := restored.runFrom(ctx, input.Query, 3)
	if err != nil {
		return "", fmt.Errorf("workflow tool: resume execution: %w", err)
	}
	return result, nil
}

// formatRulesForConfirmation renders the retrieved rules as a readable list for
// the human confirmation prompt.
func formatRulesForConfirmation(rules []RuleConstraint) string {
	if len(rules) == 0 {
		return "（无规则检索到）"
	}
	var b []string
	for _, r := range rules {
		b = append(b, fmt.Sprintf("- [%s] %s (%s): %s", r.ArticleID, r.ArticleName, r.Requirement, r.Description))
	}
	return joinLines(b)
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

// NewWorkflowRunner creates a pre-configured FiveStepRunner with
// default templates and optional retriever/checker.
//
// llm is optional — when non-nil, the checker uses it for Level 2 (logical
// consistency) and Level 3 (evidentiary sufficiency) validation. Without it,
// only Level 1 (reference existence) checking is performed.
//
// Manifest 优先级：YAML 文件（~/.mady/workflows/）> 内置默认值。
// 当 globalWorkflowStore 中有匹配 caseType 的 YAML manifest 时优先使用；
// 否则回退到 DefaultManifests() 中对应的内置配置。
func NewWorkflowRunner(caseID string, caseType CaseType, techField string, retriever *MultiSourceRetriever, llm LlmClient) *FiveStepRunner {
	planner := NewPlanner(llm)
	for _, v := range DefaultManifests() {
		v := v
		plan := v.Stage4.ToPlan(v.CaseType)
		planner.RegisterTemplate(v.CaseType, PlanIntentChain, *plan)
	}

	// 优先从 YAML 加载的全局 store 中查找匹配的 manifest。
	// 如果 store 为空（启动时未加载任何 YAML）或无匹配 caseType，回退到内置默认值。
	var manifest *WorkflowManifest
	if gs := GlobalWorkflowStore(); gs != nil {
		if m, ok := gs.GetByCaseType(caseType); ok {
			manifest = m
		}
	}
	// 回退：遍历内置默认值查找匹配的 manifest。
	if manifest == nil {
		for _, m := range DefaultManifests() {
			if m.CaseType == caseType {
				m := m
				manifest = m
				break
			}
		}
	}

	return NewFiveStepRunner(FiveStepRunnerConfig{
		Planner:     planner,
		Checker:     NewEnhancedSyllogismChecker(llm, 2),
		Retriever:   retriever,
		Manifest:    manifest,
		CaseID:      caseID,
		CaseType:    caseType,
		TechField:   techField,
		NodeBuilder: NewLLMNodeBuilder(llm), // real LLM analysis (not noop)
	})
}
