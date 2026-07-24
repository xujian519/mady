// Package domains provides the run_orchestration tool — the executable
// bridge between YAML orchestrations and the Agent runtime.
//
// The run_orchestration tool allows the LLM to execute a complete
// orchestrated workflow with a single tool call, rather than manually
// interpreting YAML orchestration text and chaining individual tools.
//
// Usage by LLM:
//
//	run_orchestration({
//	  "case_type": "oa_response",
//	  "oa_text": "审查意见通知书全文...",
//	  "claims_text": "当前权利要求书...",
//	  "spec_text": "当前说明书..."
//	})
//
// The tool internally:
//  1. Looks up the OrchestrationManifest for the case type.
//  2. Prepares state inputs from the user-provided context.
//  3. Runs the OrchestrationExecutor, which calls domain tools in order.
//  4. Returns a structured result with step outputs and a Markdown summary.
package domains

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// =============================================================================
// Tool Registration
// =============================================================================

// RunOrchestrationToolName is the tool name used to invoke an orchestration.
const RunOrchestrationToolName = "run_orchestration"

// RunOrchestrationToolDesc describes the tool for the LLM.
const RunOrchestrationToolDesc = `执行专利事务编排工作流。

当用户要求以下任务时，优先调用此工具（而非逐个调用子工具）：
- 答复审查意见 → case_type="oa_response"
- 驳回复审 → case_type="re_examination"
- 无效宣告分析 → case_type="invalidation"
- 撰写专利全文 → case_type="patent_drafting"

此工具内部自动串联：法规检索→驳回分析→条件分支→文档起草→合规检查。
输入为自然语言描述的任务上下文和相关文档文本。

如无匹配的编排，应回退到 run_five_step_workflow（通用五步工作法）。`

// NewOrchestrationTool creates the run_orchestration tool bound to the
// given Agent. The agent is captured in a closure, so no global state
// is needed. This eliminates the lifecycle race between tool execution
// and agent rebuild that a global reference would create.
func NewOrchestrationTool(a *agentcore.Agent) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        RunOrchestrationToolName,
		Description: RunOrchestrationToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"case_type": map[string]any{
					"type":        "string",
					"description": "事务类型：oa_response（审查意见答复）/ re_examination（复审）/ invalidation（无效宣告）/ patent_drafting（专利撰写）",
				},
				"oa_text": map[string]any{
					"type":        "string",
					"description": "审查意见通知书/驳回决定全文（OA/复审时必填）",
				},
				"claims_text": map[string]any{
					"type":        "string",
					"description": "当前权利要求书文本（修改前）",
				},
				"spec_text": map[string]any{
					"type":        "string",
					"description": "当前说明书文本（修改前）",
				},
				"disclosure_text": map[string]any{
					"type":        "string",
					"description": "技术交底书内容（撰写专利时必填）",
				},
				"tech_domain": map[string]any{
					"type":        "string",
					"description": "技术领域：mechanical / electrical / chemical / software / general",
				},
			},
			"required": []string{"case_type"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return handleRunOrchestration(ctx, args, a)
		},
	}
}

// handleRunOrchestration implements the run_orchestration tool logic.
// The agent is passed in from the closure captured by NewOrchestrationTool.
func handleRunOrchestration(ctx context.Context, args json.RawMessage, agent *agentcore.Agent) (any, error) {
	var params struct {
		CaseType       string `json:"case_type"`
		OAText         string `json:"oa_text"`
		ClaimsText     string `json:"claims_text"`
		SpecText       string `json:"spec_text"`
		DisclosureText string `json:"disclosure_text"`
		TechDomain     string `json:"tech_domain"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return agentcore.NewFailureResult("参数解析失败", "run_orchestration 参数格式错误"), nil
	}

	manifest := GetOrchestrationManifest(params.CaseType)
	if manifest == nil {
		return agentcore.NewFailureResult("未知的事务类型",
			fmt.Sprintf("不支持的事务类型 %q。支持：oa_response / re_examination / invalidation / patent_drafting。请改用 run_five_step_workflow。", params.CaseType)), nil
	}

	// Prepare orchestration state from user-provided context.
	state := buildOrchestrationState(params, manifest)

	executor := agentcore.NewOrchestrationExecutor(agent)
	result, err := executor.Run(ctx, manifest, state)
	if err != nil {
		return agentcore.NewFailureResult("编排执行失败",
			fmt.Sprintf("编排 %q 执行过程中断：%v", manifest.ID, err)), nil
	}

	// Build a comprehensive result for the LLM.
	return formatOrchestrationResult(result, manifest), nil
}

// orchParams is the input struct for the run_orchestration tool.
type orchParams struct {
	CaseType       string `json:"case_type"`
	OAText         string `json:"oa_text"`
	ClaimsText     string `json:"claims_text"`
	SpecText       string `json:"spec_text"`
	DisclosureText string `json:"disclosure_text"`
	TechDomain     string `json:"tech_domain"`
}

// stateMapperFunc maps orchestration params to state entries for a single tool.
// Returns nil when the tool has no params to map for the given orchestration.
type stateMapperFunc func(params orchParams) map[string]any

// stateMappers dispatches state-building to per-tool mapper functions.
// Adding a new tool to an orchestration only requires adding its mapper here.
var stateMappers = map[string]stateMapperFunc{
	"parse_office_action": func(p orchParams) map[string]any {
		if p.OAText == "" {
			return nil
		}
		return map[string]any{"_input": map[string]string{"text": p.OAText}}
	},
	"get_article_framework": func(p orchParams) map[string]any {
		if p.OAText == "" {
			return nil
		}
		return map[string]any{"_article_framework": map[string]string{"article_id": detectArticleID(p.OAText)}}
	},
	"validate_amendment": func(p orchParams) map[string]any {
		return map[string]any{"_amendment_input": map[string]any{
			"modification_type":  "passive",
			"office_action_text": p.OAText,
			"original_claims":    p.ClaimsText,
			"original_spec":      p.SpecText,
		}}
	},
	"draft_oa_response": func(p orchParams) map[string]any {
		return map[string]any{"_oa_response_input": map[string]string{
			"oa_text": p.OAText, "claim_text": p.ClaimsText,
		}}
	},
	"draft_claims": func(p orchParams) map[string]any {
		if p.DisclosureText == "" {
			return nil
		}
		return map[string]any{"_claims_input": map[string]string{
			"title": extractTitle(p.DisclosureText), "description": p.DisclosureText,
		}}
	},
	"draft_specification": func(p orchParams) map[string]any {
		if p.DisclosureText == "" {
			return nil
		}
		return map[string]any{"_spec_input": map[string]string{
			"disclosure": p.DisclosureText, "tech_domain": p.TechDomain,
		}}
	},
	"draft_reexamination_request": func(p orchParams) map[string]any {
		return map[string]any{"_reexam_input": map[string]string{
			"oa_text": p.OAText, "claim_text": p.ClaimsText,
		}}
	},
	"search_knowledge": func(p orchParams) map[string]any {
		var query string
		switch {
		case p.DisclosureText != "":
			query = truncateText(p.DisclosureText, 500)
		case p.OAText != "":
			query = extractRejectionSummary(p.OAText)
		default:
			return nil
		}
		return map[string]any{"_search_input": map[string]string{"query": query}}
	},
	"analyze_slop": func(p orchParams) map[string]any {
		return map[string]any{"_slop_input": map[string]string{"text": p.OAText}}
	},
	"analyze_patent_invalidation": func(p orchParams) map[string]any {
		if p.ClaimsText == "" {
			return nil
		}
		return map[string]any{"_invalidation_input": map[string]string{"claims_text": p.ClaimsText}}
	},
	"analyze_disclosure": func(p orchParams) map[string]any {
		if p.DisclosureText == "" {
			return nil
		}
		return map[string]any{"_disclosure_input": map[string]string{"text": p.DisclosureText}}
	},
	"validate_specification": func(p orchParams) map[string]any {
		return map[string]any{"_validate_spec_input": map[string]string{"specification": p.DisclosureText}}
	},
}

// buildOrchestrationState maps user-provided parameters to the input keys
// expected by each orchestration step. Uses the stateMappers dispatch table
// so that adding a new tool only requires adding a mapper entry.
func buildOrchestrationState(params orchParams, manifest *agentcore.OrchestrationManifest) agentcore.OrchestrationState {
	state := agentcore.OrchestrationState{}

	// Common inputs.
	if params.OAText != "" {
		state["_input"] = map[string]string{"text": params.OAText}
	}
	if params.ClaimsText != "" {
		state["_claims_text"] = params.ClaimsText
	}
	if params.SpecText != "" {
		state["_spec_text"] = params.SpecText
	}
	if params.DisclosureText != "" {
		state["_disclosure_text"] = params.DisclosureText
	}
	if params.TechDomain != "" {
		state["_tech_domain"] = params.TechDomain
	}

	// Dispatch step-specific mappings by tool name.
	for _, step := range manifest.Steps {
		if mapper, ok := stateMappers[step.ToolName]; ok {
			if sub := mapper(params); sub != nil {
				for k, v := range sub {
					state[k] = v
				}
			}
		}
	}

	return state
}

// detectArticleID returns the primary article ID inferred from the OA text.
func detectArticleID(oaText string) string {
	lower := strings.ToLower(oaText)
	switch {
	case strings.Contains(lower, "26.3") || strings.Contains(lower, "充分公开") || strings.Contains(lower, "公开不充分"):
		return "A26.3"
	case strings.Contains(lower, "22.3") || strings.Contains(lower, "创造性"):
		return "A22.3"
	case strings.Contains(lower, "22.2") || strings.Contains(lower, "新颖性"):
		return "A22.2"
	case strings.Contains(lower, "26.4") || strings.Contains(lower, "不清楚") || strings.Contains(lower, "不支持"):
		return "A26.4"
	case strings.Contains(lower, "33条") || strings.Contains(lower, "a33") || strings.Contains(lower, "修改超范围"):
		return "A33"
	default:
		return "A22.3" // default: creative step (most common rejection)
	}
}

func extractTitle(text string) string {
	// Simple heuristic: first non-empty line, truncated.
	lines := strings.SplitN(strings.TrimSpace(text), "\n", 2)
	title := strings.TrimSpace(lines[0])
	if len([]rune(title)) > 50 {
		title = string([]rune(title)[:50]) + "…"
	}
	if title == "" {
		title = "未命名发明"
	}
	return title
}

func extractRejectionSummary(oaText string) string {
	return "审查意见驳回理由分析 " + truncateText(oaText, 200)
}

func truncateText(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}

// formatOrchestrationResult builds a HandoffResult from an orchestration run.
func formatOrchestrationResult(result *agentcore.OrchestrationResult, manifest *agentcore.OrchestrationManifest) agentcore.HandoffResult {
	if result.Success {
		return agentcore.NewHandoffResult(
			fmt.Sprintf("%s — 已完成（%d/%d 步）", manifest.Name, result.StepsCompleted, len(manifest.Steps)),
			result.Summary,
		)
	}

	var errMsgs []string
	for step, err := range result.StepErrors {
		errMsgs = append(errMsgs, fmt.Sprintf("- %s: %s", step, err))
	}
	return agentcore.NewFailureResult(
		fmt.Sprintf("%s — 未完成", manifest.Name),
		fmt.Sprintf("%s\n\n错误：\n%s", result.Summary, strings.Join(errMsgs, "\n")),
	)
}
