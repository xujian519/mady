// Package domains provides the YAML-to-executable bridge for patent
// workflow orchestrations.
//
// This file maps YAML orchestration definitions (domains/rules/data/orchestrations/)
// to executable agentcore.OrchestrationManifest instances. Each orchestration
// is pre-defined here as a compiled manifest so that the run_orchestration
// tool can dispatch them directly.
//
// For orchestrations with conditional branching (e.g., OA response: call
// analyze_enablement only when the rejection mentions 26.3), the conditions
// are evaluated at runtime using the output of the parse_office_action step.
//
// Adding a new orchestration:
//  1. Define the YAML file in domains/rules/data/orchestrations/.
//  2. Add a build<Name>Manifest() function below.
//  3. Register it in GetOrchestrationManifest().
package domains

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
)

// GetOrchestrationManifest returns the compiled OrchestrationManifest for
// the given case type. Returns nil if no orchestration is defined.
//
// Currently supported:
//   - "oa_response" — 审查意见答复
//   - "re_examination" — 驳回复审
//   - "invalidation" — 无效宣告
func GetOrchestrationManifest(caseType string) *agentcore.OrchestrationManifest {
	switch strings.ToLower(caseType) {
	case "oa_response", "oa-response", "审查意见答复", "oa答复":
		return buildOAResponseManifest()
	case "re_examination", "re-examination", "复审", "驳回复审":
		return buildReexaminationManifest()
	case "invalidation", "无效", "无效宣告":
		return buildInvalidationManifest()
	case "patent_drafting", "drafting", "撰写专利", "专利撰写":
		return buildPatentDraftingManifest()
	default:
		return nil
	}
}

// =============================================================================
// OA Response Orchestration
// =============================================================================

// buildOAResponseManifest creates the OA response orchestration.
//
// Steps:
//
//	① parse_office_action — 解析审查意见
//	② [条件] get_article_framework — 根据驳回类型获取法条框架
//	③ [条件] analyze_enablement — 涉及26.3时调用
//	④ [条件] analyze_inventiveness — 涉及创造性时调用
//	⑤ [条件] analyze_patent_novelty — 涉及新颖性时调用
//	⑥ validate_amendment — A33修改合规检查
//	⑦ draft_oa_response — 起草答复书
//	⑧ analyze_slop — 套话检查（可选）
func buildOAResponseManifest() *agentcore.OrchestrationManifest {
	return &agentcore.OrchestrationManifest{
		ID:          "oa_response",
		Name:        "审查意见答复",
		Description: "解析审查意见→分析驳回理由→修改权利要求/说明书→A33合规检查→起草答复书→套话检查",
		Steps: []agentcore.OrchestrationStep{
			{
				ToolName:    "parse_office_action",
				Description: "解析审查意见通知书",
				InputKey:    "_input",
			},
			{
				ToolName:    "get_article_framework",
				Description: "获取驳回法条判断框架",
				InputKey:    "_article_framework",
				Condition:   hasRejectionGrounds,
			},
			{
				ToolName:    "analyze_enablement",
				Description: "26.3 充分公开分析",
				InputKey:    "_enablement_input",
				Condition:   hasEnablementGround,
				Optional:    true,
			},
			{
				ToolName:    "analyze_inventiveness",
				Description: "创造性分析",
				InputKey:    "_inventiveness_input",
				Condition:   hasInventivenessGround,
				Optional:    true,
			},
			{
				ToolName:    "analyze_patent_novelty",
				Description: "新颖性分析",
				InputKey:    "_novelty_input",
				Condition:   hasNoveltyGround,
				Optional:    true,
			},
			{
				ToolName:    "validate_amendment",
				Description: "A33 修改合规检查",
				InputKey:    "_amendment_input",
			},
			{
				ToolName:    "draft_oa_response",
				Description: "起草审查意见答复书",
				InputKey:    "_oa_response_input",
			},
			{
				ToolName:    "analyze_slop",
				Description: "AI套话检查（可选）",
				InputKey:    "_slop_input",
				Optional:    true,
			},
		},
	}
}

// =============================================================================
// Patent Drafting Orchestration
// =============================================================================

// buildPatentDraftingManifest creates the patent drafting orchestration.
//
// Steps:
//
//	① search_knowledge — 检索相关法规和现有技术
//	② draft_claims — 撰写权利要求书（8节点Pregel图）
//	③ draft_specification — 撰写说明书（12节点Pregel图）
//	④ validate_specification — 校验说明书（16条规则）
func buildPatentDraftingManifest() *agentcore.OrchestrationManifest {
	return &agentcore.OrchestrationManifest{
		ID:          "patent_drafting",
		Name:        "完整专利撰写",
		Description: "检索法规→撰写权利要求书→撰写说明书→校验说明书",
		Steps: []agentcore.OrchestrationStep{
			{
				ToolName:    "search_knowledge",
				Description: "检索相关法规和现有技术",
				InputKey:    "_search_input",
				Optional:    true,
			},
			{
				ToolName:    "draft_claims",
				Description: "撰写权利要求书",
				InputKey:    "_claims_input",
			},
			{
				ToolName:    "draft_specification",
				Description: "撰写说明书",
				InputKey:    "_spec_input",
			},
			{
				ToolName:    "validate_specification",
				Description: "校验说明书",
				InputKey:    "_validate_spec_input",
				Optional:    true,
			},
		},
	}
}

// =============================================================================
// Re-examination Orchestration
// =============================================================================

// buildReexaminationManifest creates the re-examination orchestration.
//
// Steps:
//
//	① parse_office_action — 解析驳回决定
//	② [条件] analyze_enablement — 涉及26.3
//	③ [条件] analyze_inventiveness — 涉及创造性
//	④ [条件] validate_amendment — 涉及A33
//	⑤ draft_reexamination_request — 起草复审请求书
func buildReexaminationManifest() *agentcore.OrchestrationManifest {
	return &agentcore.OrchestrationManifest{
		ID:          "re_examination",
		Name:        "驳回复审",
		Description: "解析驳回决定→分析驳回理由→起草复审请求→修改合规检查",
		Steps: []agentcore.OrchestrationStep{
			{
				ToolName:    "parse_office_action",
				Description: "解析驳回决定",
				InputKey:    "_input",
			},
			{
				ToolName:    "analyze_enablement",
				Description: "26.3 充分公开分析",
				InputKey:    "_enablement_input",
				Condition:   hasEnablementGround,
				Optional:    true,
			},
			{
				ToolName:    "analyze_inventiveness",
				Description: "创造性分析",
				InputKey:    "_inventiveness_input",
				Condition:   hasInventivenessGround,
				Optional:    true,
			},
			{
				ToolName:    "validate_amendment",
				Description: "A33 修改合规检查",
				InputKey:    "_amendment_input",
			},
			{
				ToolName:    "draft_reexamination_request",
				Description: "起草复审请求书",
				InputKey:    "_reexam_input",
			},
		},
	}
}

// =============================================================================
// Invalidation Orchestration
// =============================================================================

// buildInvalidationManifest creates the invalidation analysis orchestration.
//
// Steps:
//
//	① search_knowledge — 检索相关法规
//	② analyze_patent_invalidation — 无效宣告分析（现有工具链）
func buildInvalidationManifest() *agentcore.OrchestrationManifest {
	return &agentcore.OrchestrationManifest{
		ID:          "invalidation",
		Name:        "专利无效宣告分析",
		Description: "检索法规→识别无效理由→逐项生成论证→规则校验",
		Steps: []agentcore.OrchestrationStep{
			{
				ToolName:    "search_knowledge",
				Description: "检索相关法规",
				InputKey:    "_search_input",
				Optional:    true,
			},
			{
				ToolName:    "analyze_patent_invalidation",
				Description: "无效宣告分析",
				InputKey:    "_invalidation_input",
			},
		},
	}
}

// =============================================================================
// Condition Functions
// =============================================================================
//
// These functions inspect the orchestration state at runtime to decide
// whether a conditional step should execute. They use StepOutputKey(toolName)
// to access step outputs instead of hardcoded magic strings.
//
// Convention: output from step "parse_office_action" is stored at
// state[StepOutputKey("parse_office_action")].

// hasRejectionGrounds returns true when the state contains a parsed OA
// result with at least one rejection ground.
func hasRejectionGrounds(state map[string]any) bool {
	s := agentcore.OrchestrationState(state)
	key := agentcore.StepOutputKey("parse_office_action")

	// Check for structured grounds array in the parsed output.
	if m := s.GetMap(key); m != nil {
		if grounds, ok := m["grounds"]; ok {
			if arr, ok := grounds.([]any); ok && len(arr) > 0 {
				return true
			}
		}
	}

	// Fallback: check raw text for rejection keywords.
	return s.HasRejectionKeywords(key)
}

// hasEnablementGround returns true when the OA mentions 26.3 / 充分公开.
func hasEnablementGround(state map[string]any) bool {
	return checkOAText(state, "26.3", "充分公开", "公开不充分",
		"enablement", "可实现性", "无法实现")
}

// hasInventivenessGround returns true when the OA mentions 创造性 / 22.3.
func hasInventivenessGround(state map[string]any) bool {
	return checkOAText(state, "创造性", "22.3", "A22.3",
		"obviousness", "非显而易见", "三步法")
}

// hasNoveltyGround returns true when the OA mentions 新颖性 / 22.2.
func hasNoveltyGround(state map[string]any) bool {
	return checkOAText(state, "新颖性", "22.2", "A22.2",
		"novelty", "现有技术")
}

// checkOAText checks whether any of the given keywords appear in the
// parsed OA output stored at StepOutputKey("parse_office_action").
func checkOAText(state map[string]any, keywords ...string) bool {
	key := agentcore.StepOutputKey("parse_office_action")
	raw, ok := state[key]
	if !ok {
		return false
	}

	switch v := raw.(type) {
	case string:
		return util.ContainsAny(v, keywords...)
	case map[string]any:
		for _, nested := range []string{"result", "summary", "text", "grounds"} {
			if s, ok := v[nested].(string); ok {
				if util.ContainsAny(s, keywords...) {
					return true
				}
			}
		}
	}
	return false
}
