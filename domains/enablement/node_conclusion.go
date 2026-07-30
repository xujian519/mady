package enablement

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// conclusionSchemaDef 是 generateConclusion 的 JSON Schema。
func conclusionSchemaDef() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"is_sufficient": map[string]any{"type": "boolean"},
			"reasoning":     map[string]any{"type": "string"},
			"confidence": map[string]any{
				"type": "string",
				"enum": []string{"high", "medium", "low"},
			},
			"deficiencies": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"support_warnings": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"experiment_data": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"data_needed":     map[string]any{"type": "boolean"},
					"data_provided":   map[string]any{"type": "boolean"},
					"is_valid":        map[string]any{"type": "boolean"},
					"missing_factors": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"notes":           map[string]any{"type": "string"},
				},
			},
			"legal_note": map[string]any{"type": "string"},
		},
		"required": []string{"is_sufficient", "reasoning", "confidence", "deficiencies"},
	}
}

// generateConclusionNode 汇总三步骤结果，生成结构化最终结论。
func generateConclusionNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step1, _ := state[stateKeyStep1].(string)
		step2, _ := state[stateKeyStep2].(string)
		step3, _ := state[stateKeyStep3].(string)

		prompt := strings.Join([]string{
			"你是一名资深专利审查员。请基于专利法第26条第3款（充分公开）评估的",
			"三个步骤的产出，生成最终的结构化评估结论。",
			"",
			"结论应包含：",
			"1. 整体判断：该说明书是否满足 26.3 充分公开的要求（is_sufficient: true/false）",
			"2. 置信度：high（证据充分且结论明确）/ medium（有一定依据但存在不确定性）/",
			"   low（信息不足，难以形成确定判断）",
			"3. 具体缺陷列表（deficiencies）：如果 is_sufficient=false，列出所有具体的公开缺陷",
			"4. 26.4联动风险（support_warnings）：如果存在公开不充分，评估是否影响权利要求得到说明书支持。",
			"   根据授权确权规定第六条第2款：公开不充分必然导致不支持。",
			"   请列出哪些权利要求可能因公开不充分而得不到支持。",
			"5. 实验数据评估（experiment_data）：评估技术效果是否需要实验数据、是否提供、是否有效。",
			"6. 法律提示：本判断由 AI 辅助生成，不构成正式法律意见",
			"",
			"**区分注意**：",
			"- 公开不充分（26.3）针对的是**说明书**——能否实现",
			"- 不支持（26.4）针对的是**权利要求**——概括是否恰当",
			"- 公开不充分必然导致不支持，但不支持不必然意味着公开不充分",
			"- 实用性（22.4）关注的是技术方案是否违背自然规律，公开不充分关注的是记载是否完整",
			"",
			"请输出 JSON 格式：",
			`{"is_sufficient": bool, "reasoning": "综合推理过程",`,
			`"confidence": "high/medium/low",`,
			`"deficiencies": ["具体缺陷1", "具体缺陷2"],`,
			`"support_warnings": ["26.4联动风险提示"],`,
			`"experiment_data": {"data_needed": bool, "data_provided": bool, "is_valid": bool,`,
			`"missing_factors": ["缺失要素"], "notes": "说明"},`,
			`"legal_note": "本判断由 AI 辅助生成，不构成正式法律意见"}`,
		}, "\n")

		agent := newEnablementAgent(provider, "enablement-conclusion", prompt, conclusionSchemaDef())
		defer agent.Close()

		inputText := fmt.Sprintf(
			"第 1 步（完整性检查）:\n%s\n\n第 2 步（清楚性检查）:\n%s\n\n第 3 步（能够实现性检查）:\n%s",
			step1, step2, step3)

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("generate_conclusion: %w", err)
		}

		result := buildResult(step1, step2, step3, output)
		// 设置技术领域标签
		if domainStr, ok := state[stateKeyDomain].(string); ok && domainStr != "" {
			result.TechDomain = domainStr
		}
		state[stateKeyResult] = result
		return state, nil
	}
}

func parseConclusion(output string) parsedConclusion {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return parsedConclusion{
			Reasoning:  output,
			Confidence: "medium",
		}
	}

	var parsed parsedConclusion
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return parsedConclusion{
			Reasoning:  output,
			Confidence: "medium",
		}
	}

	switch parsed.Confidence {
	case "high", "medium", "low":
	default:
		parsed.Confidence = "medium"
	}

	return parsed
}

// buildResult 从四步 LLM 输出构建结构化的 EnablementResult。
func buildResult(step1, step2, step3, conclusion string) *EnablementResult {
	result := &EnablementResult{
		Assessed: true,
	}

	// 解析 completeness 结果
	result.Completeness = parseCompleteness(step1)

	// 解析 clarity 结果
	result.Clarity = parseClarity(step2)

	// 解析 enablement 结果
	result.Enablement = parseEnablementJudgment(step3)

	// 解析最终结论
	cc := parseConclusion(conclusion)
	result.Conclusion = cc.Reasoning
	result.IsSufficient = cc.IsSufficient
	result.Confidence = cc.Confidence
	result.Deficiencies = cc.Deficiencies
	result.SupportIssue = len(cc.SupportWarnings) > 0
	result.SupportWarnings = cc.SupportWarnings
	result.DataAssessment = cc.ExperimentData

	return result
}
