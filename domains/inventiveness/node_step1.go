package inventiveness

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// step1ClosestPriorArtNode 三步法第 1 步：确定最接近的现有技术。
func step1ClosestPriorArtNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		input := extractInput(state)
		if input == nil {
			return state, nil // skipped already
		}

		prompt := "你是一名资深专利审查员。请执行三步法创造性评估的第 1 步：\n\n"
		prompt += personSkilledDefinition() + "\n\n"
		prompt += "从以下现有技术证据中，确定与目标技术方案最接近的一篇对比文件。\n"
		prompt += "最接近的现有技术是指与目标方案技术领域相同、要解决的技术问题最接近、"
		prompt += "或技术特征最多的现有技术文献。\n"
		prompt += "**选取优先级规则**（按权重从高到低考虑）：\n"
		prompt += "1. 技术领域相同优先于仅相似领域\n"
		prompt += "2. 技术领域相同的前提下，选择公开技术特征最多的文献\n"
		prompt += "3. 技术特征数量相当时，选择要解决的技术问题最接近的文献\n"
		prompt += "4. 技术问题也相当时，选择技术效果最接近的文献\n"
		prompt += "5. 跨领域选取：仅当同一/相似领域内无适格文献时，才考虑跨领域选取，并说明跨领域理由\n"
		prompt += "注意：选取后应简要说明所选文献在各优先级维度上的符合情况。\n\n"
		if typeGuidance := inventionTypeGuidance(input.InventionType); typeGuidance != "" {
			prompt += typeGuidance + "\n\n"
		}
		if domainGuidance := techDomainGuidance(input.TechDomain); domainGuidance != "" {
			prompt += domainGuidance + "\n\n"
		}
		prompt += "请列出选定文献的标题和理由。\n\n"
		prompt += "**现有技术信息质量提示**：\n"
		prompt += "- 确定最接近现有技术时，应基于该文献的整体公开内容，而非仅关注局部文字或最佳实施例\n"
		prompt += "- 注意识别文献中是否存在明显笔误（前后文字不一致、与整体教导相矛盾）\n"
		prompt += "- 负面描述（如「不宜」「不适合」）不必然否定该技术手段的可实施性，应结合上下文整体把握\n"

		inputText := buildInputText(input)
		agent := newInventivenessAgent(provider, "inventiveness-step1", prompt, step1Schema())
		defer agent.Close()

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step1: %w", err)
		}

		state[stateKeyStep1] = output
		return state, nil
	}
}

// =============================================================================
// JSON Schema 定义（对标 enablement 的 Schema 函数）
// =============================================================================

// step1Schema 三步法第 1 步的 JSON Schema。
func step1Schema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"closest_prior_art": map[string]any{jsFieldType: jsTypeString},
			"selection_reason":  map[string]any{jsFieldType: jsTypeString},
		},
		jsFieldRequired: []string{"closest_prior_art", "selection_reason"},
	}
}

// =============================================================================
// 解析函数（对标 enablement 的 parse 函数）
// =============================================================================

// parseStep1 从 LLM 输出解析 Step1Result。
func parseStep1(output string) Step1Result {
	r := Step1Result{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.SelectionReason = output
		return r
	}

	var parsed struct {
		ClosestPriorArt string `json:"closest_prior_art"`
		SelectionReason string `json:"selection_reason"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.SelectionReason = output // LLM 返回非 JSON：降级为原始文本作为判断依据
		return r
	}

	r.ClosestPriorArt = parsed.ClosestPriorArt
	r.SelectionReason = parsed.SelectionReason
	return r
}
