package inventiveness

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// generateConclusionNode 汇总所有步骤的产出，生成最终创造性评估结论。
// 结论逻辑：IsInventive = Step3.NonObvious AND Step4.HasSignificantProgress
func generateConclusionNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step1, ok := state[stateKeyStep1].(string)
		if !ok {
			step1 = ""
		}
		step2, ok := state[stateKeyStep2].(string)
		if !ok {
			step2 = ""
		}
		step3, ok := state[stateKeyStep3].(string)
		if !ok {
			step3 = ""
		}
		step4, ok := state[stateKeyStep4].(string)
		if !ok {
			step4 = ""
		}

		prompt := "你是一名资深专利审查员。请基于创造性评估各步骤的产出，生成最终的结构化评估结论。\n\n"
		prompt += personSkilledDefinition() + "\n\n"
		prompt += "**判断逻辑**：创造性 = 突出的实质性特点（Step3 非显而易见） AND 显著的进步（Step4 有益效果）\n\n"
		prompt += "结论应包含：\n"
		prompt += "1. 整体判断：该技术方案是否具备创造性\n"
		prompt += "2. 是否具有显著的进步（基于 Step4 结论）\n"
		prompt += "3. 置信度：high/medium/low\n"
		prompt += "4. 辅助判断因素结构化分析（如有相关信息则逐项检查，无则跳过）\n\n"
		prompt += "**辅助判断因素分析**：\n\n"
		prompt += "1. 预料不到的技术效果（如有实验数据）\n"
		prompt += "   - 发明实际取得的技术效果是否超越了申请日前技术人员的预期？\n"
		prompt += "   - 效果的「质变」（新性能）或「量变」（远超预期）是否无法从现有技术预测？\n"
		prompt += "   - 实验数据来源：原始申请文件已有数据 vs 申请日后补充数据？\n"
		prompt += "   - 补充实验数据证明的效果是否可从原始申请文件公开内容中得到？\n"
		prompt += "   - 对比试验是否具有代表性？（避免以个别最优实施例代表权利要求整体保护范围）\n"
		prompt += "   **特别注意**：预料不到的技术效果是创造性的充分条件而非必要条件。\n"
		prompt += "   不能以「不具有预料不到的技术效果」为由得出不具备创造性的结论。\n\n"
		prompt += "2. 商业上的成功（如有相关证据）\n"
		prompt += "   - 是否真正取得商业成功？（市场份额、销售量等客观证据）\n"
		prompt += "   - 商业成功是否直接由发明的区别技术特征导致？（排除广告宣传、销售策略等非技术因素）\n"
		prompt += "   - 专利权人是否提交了商业成功与技术特征之间的关联证据？\n\n"
		prompt += "3. 克服了技术偏见（如有相关证据）\n"
		prompt += "   - 偏见是否具有普遍性？（如被教科书、技术手册等权威资料肯定的认识）\n"
		prompt += "   - 该认识是否偏离客观事实？（而非仅是效果不理想的「取舍」）\n"
		prompt += "   - 发明是否采用了因偏见而被行业舍弃的技术方案并取得成功？\n\n"
		prompt += "4. 长期未满足的技术需求\n"
		prompt += "   - 现有技术是否确实长期缺乏有效解决方案？\n"
		prompt += "   - 现有技术中是否已存在解决该问题的有效手段？（有则不构成「长期未满足」）\n\n"
		prompt += confidenceCalibration() + "\n\n"

		// HITL 反馈回流：同案卷若已有历史驳回/修正反馈，注入提示词让结论节点吸取修正。
		if caseID, _ := state[StateKeyCaseID].(string); caseID != "" {
			if fb := FeedbackPrompt(caseID); fb != "" {
				prompt += "\n\n" + fb
			}
		}

		prompt += "请输出 JSON 格式：\n"
		prompt += "- conclusion: 整体结论\n"
		prompt += "- is_inventive: true/false\n"
		prompt += "- has_significant_progress: true/false\n"
		prompt += "- confidence: high/medium/low\n"
		prompt += "- aux_factors: 辅助考虑因素（可选）"

		agent := newInventivenessAgent(provider, "inventiveness-conclusion", prompt, conclusionSchema())
		defer agent.Close()

		experimentData, _ := state[stateKeyExperiment].(string)

		inputText := fmt.Sprintf("第 1 步（最接近现有技术）:\n%s\n\n第 2 步（区别特征与技术问题）:\n%s\n\n第 3 步（技术启示判断）:\n%s\n\n第 4 步（显著的进步）:\n%s",
			step1, step2, step3, step4)
		if experimentData != "" {
			inputText += "\n\n" + experimentData
		}

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("conclusion: %w", err)
		}

		result := buildResult(step1, step2, step3, step4, output)

		state[StateKeyResult] = result
		return state, nil
	}
}

// conclusionSchema 最终结论的 JSON Schema（扩展版：含 has_significant_progress）。
func conclusionSchema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"conclusion":               map[string]any{jsFieldType: jsTypeString},
			"is_inventive":             map[string]any{jsFieldType: jsTypeBoolean},
			jsFieldSignificantProgress: map[string]any{jsFieldType: jsTypeBoolean},
			jsFieldConfidence: map[string]any{
				jsFieldType: jsTypeString,
				jsFieldEnum: []string{jsValHigh, jsValMedium, jsValLow},
			},
			"aux_factors": map[string]any{
				jsFieldType:  jsTypeArray,
				jsFieldItems: map[string]any{jsFieldType: jsTypeString},
			},
		},
		jsFieldRequired: []string{"conclusion", "is_inventive", jsFieldSignificantProgress, jsFieldConfidence},
	}
}

// parseConclusion 从 LLM 输出解析最终结论。
func parseConclusion(output string) parsedConclusion {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return parsedConclusion{
			Conclusion: output,
			Confidence: jsValMedium,
		}
	}

	var parsed parsedConclusion
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return parsedConclusion{
			Conclusion: output,
			Confidence: jsValMedium,
		}
	}

	parsed.parseOK = true

	switch parsed.Confidence {
	case jsValHigh, jsValMedium, jsValLow:
	default:
		parsed.Confidence = jsValMedium
	}

	return parsed
}

// buildResult 从各步 LLM 输出构建完整的 InventivenessResult。
// 结论逻辑：IsInventive = Step3.NonObvious AND Step4.HasSignificantProgress
func buildResult(step1, step2, step3, step4, conclusion string) *InventivenessResult {
	result := &InventivenessResult{
		Assessed: true,
	}

	// Parse individual step results.
	result.Step1 = parseStep1(step1)
	result.Step2 = parseStep2(step2)
	result.Step3 = parseStep3(step3)
	result.Step4 = parseStep4(step4)

	// Build backward-compatible ThreeStep summary.
	result.ThreeStep = ThreeStepResult{
		ClosestPriorArt:        result.Step1.ClosestPriorArt,
		DistinguishingFeatures: result.Step2.DistinguishingFeatures,
		ActualTechProblem:      result.Step2.ActualTechProblem,
		TechnicalSuggestion:    result.Step3.TechnicalSuggestion,
		SuggestionRationale:    result.Step3.Rationale,
	}

	// Parse final conclusion.
	cc := parseConclusion(conclusion)
	result.Conclusion = cc.Conclusion
	result.Confidence = cc.Confidence
	result.AuxFactors = cc.AuxFactors

	// 核心结论逻辑：IsInventive = NonObvious AND HasSignificantProgress
	// 优先使用 LLM conclusion 节点的综合判断（parseOK=true），
	// 仅在 LLM 输出解析失败时使用 Step3 + Step4 的计算结果作为兜底。
	if cc.parseOK {
		// LLM 综合判断优先——无论 is_inventive=true 还是 false 都信任
		result.IsInventive = cc.IsInventive
	} else {
		// LLM 输出解析失败（非 JSON 或 JSON 格式错误），兜底使用计算逻辑
		result.IsInventive = !result.Step3.TechnicalSuggestion && result.Step4.HasSignificantProgress
	}

	return result
}
