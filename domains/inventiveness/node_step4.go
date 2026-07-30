package inventiveness

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// step4SignificantProgressNode 显著的进步判断：评估发明是否具有有益技术效果。
// 创造性 = 突出的实质性特点（Step3：非显而易见） AND 显著的进步（Step4：有益效果）。
func step4SignificantProgressNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step3Output, ok := state[stateKeyStep3].(string)
		if !ok {
			step3Output = ""
		}

		prompt := "你是一名资深专利审查员。请执行创造性评估的「显著的进步」判断：\n\n"
		prompt += personSkilledDefinition() + "\n\n"
		prompt += "根据专利法第22条第3款，创造性包含两个独立要件：\n"
		prompt += "(1) 突出的实质性特点（非显而易见性，已在第3步判断）；\n"
		prompt += "(2) 显著的进步（有益技术效果，本步骤判断）。\n\n"
		prompt += "**显著的进步四种类型**：\n"
		prompt += "1. 效果改善型：与现有技术相比具有更好的技术效果（质量改善、产量提高、节约能源等）\n"
		prompt += "2. 异途同归型：提供技术构思不同的技术方案，效果基本达到现有技术水平\n"
		prompt += "3. 趋势引领型：代表某种新技术发展趋势\n"
		prompt += "4. 利弊权衡型：某些方面有负面效果，但其他方面具有明显积极的技术效果\n\n"
		prompt += "**重要提示**：\n"
		prompt += "- 即使三步法第3步认定非显而易见，如果发明没有任何有益技术效果，仍不具备创造性\n"
		prompt += "- 「显著的进步」门槛较低：只要具有有益的技术效果（哪怕不是「预料不到」的），通常满足此要件\n"
		prompt += "- 在大多数情况下，非显而易见的发明通常也具有某种有益效果\n\n"
		prompt += "请输出 JSON 格式：\n"
		prompt += "- has_significant_progress: true/false\n"
		prompt += "- progress_type: effect_improve/different_path/trend_leading/tradeoff\n"
		prompt += "- rationale: 判断理由"

		agent := newInventivenessAgent(provider, "inventiveness-step4", prompt, step4Schema())
		defer agent.Close()

		inputText := "第 3 步（技术启示判断）结论：\n" + step3Output

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step4: %w", err)
		}

		state[stateKeyStep4] = output
		return state, nil
	}
}

// step4Schema 显著的进步判断的 JSON Schema。
func step4Schema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			jsFieldSignificantProgress: map[string]any{jsFieldType: jsTypeBoolean},
			"progress_type": map[string]any{
				jsFieldType: jsTypeString,
				jsFieldEnum: []string{"effect_improve", "different_path", "trend_leading", "tradeoff"},
			},
			jsFieldRationale: map[string]any{jsFieldType: jsTypeString},
		},
		jsFieldRequired: []string{jsFieldSignificantProgress, jsFieldRationale},
	}
}

// parseStep4 从 LLM 输出解析 Step4Result。
func parseStep4(output string) Step4Result {
	r := Step4Result{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.Rationale = output
		return r
	}

	var parsed struct {
		HasSignificantProgress bool   `json:"has_significant_progress"`
		ProgressType           string `json:"progress_type"`
		Rationale              string `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.Rationale = output
		return r
	}

	r.HasSignificantProgress = parsed.HasSignificantProgress
	r.ProgressType = parsed.ProgressType
	r.Rationale = parsed.Rationale
	return r
}
