package enablement

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// claritySchema 是 step2 的 JSON Schema。
func claritySchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"is_clear": map[string]any{"type": "boolean"},
			"ambiguous_terms": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"coined_terms": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"obvious_errors": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"orphan_features": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"orphan_effects": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"notes": map[string]any{"type": "string"},
		},
		"required": []string{"is_clear", "ambiguous_terms", "coined_terms", "obvious_errors", "orphan_features", "orphan_effects"},
	}
}

func parseClarity(output string) ClarityResult {
	r := ClarityResult{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.Notes = output
		return r
	}

	var parsed struct {
		IsClear        bool     `json:"is_clear"`
		AmbiguousTerms []string `json:"ambiguous_terms"`
		CoinedTerms    []string `json:"coined_terms"`
		ObviousErrors  []string `json:"obvious_errors"`
		OrphanFeatures []string `json:"orphan_features"`
		OrphanEffects  []string `json:"orphan_effects"`
		Notes          string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.Notes = output
		return r
	}

	r.IsClear = parsed.IsClear
	r.AmbiguousTerms = parsed.AmbiguousTerms
	r.CoinedTerms = parsed.CoinedTerms
	r.ObviousErrors = parsed.ObviousErrors
	r.OrphanFeatures = parsed.OrphanFeatures
	r.OrphanEffects = parsed.OrphanEffects
	r.Notes = parsed.Notes
	return r
}

// step2ClarityNode 三步法第 2 步：检查说明书清楚性。
// 判断技术术语是否含义明确、无歧义；PFE 三元组中是否存在孤立的特征或效果。
func step2ClarityNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step1Output, _ := state[stateKeyStep1].(string)
		input := extractInput(state)

		prompt := strings.Join([]string{
			"你是一名资深专利审查员。请执行专利法第26条第3款（充分公开）评估的第 2 步：",
			"**检查说明书清楚性**。",
			"",
			"根据专利法第26条第3款和审查指南第二部分第二章第 2.1.1 节，「清楚」包括：",
			"- 主题明确：技术问题、技术方案和有益效果相互适应，不得矛盾",
			"- 表述准确：使用所属技术领域的技术术语，不得含糊不清或模棱两可",
			"",
			"**技术用语的三种常见问题**（审查指南 §2.1.1）：",
			"1. **歧义术语**：领域内存在多种理解且未界定 → 不符合26.3",
			"   （例如：「藤子暗消」中药异名指代两种不同药材）",
			"2. **自造词**：非领域常规术语，且说明书未给出明确定义或说明 → 不符合26.3",
			"   （例如：「气相指痕光谱」非领域常规术语）",
			"3. **明显错误**：技术人员能识别的错误。只有能确定**唯一**正确理解时才不影响充分公开；",
			"   存在多种合理解释时，不构成明显错误，不符合26.3",
			"   （例如：滤网位置笔误，附图与文字不一致，能确定唯一正确理解→不影响；",
			"   多种位置均可能→不构成明显错误）",
			"",
			"另外请检查：",
			"- 是否存在没有对应技术效果的特征（孤立特征）",
			"- 是否存在没有对应技术特征的效果（孤立效果）",
			"",
			"请输出 JSON 格式：",
			`{"is_clear": bool, "ambiguous_terms": ["歧义术语"],`,
			`"coined_terms": ["自造词"], "obvious_errors": ["明显错误描述"],`,
			`"orphan_features": ["孤立特征描述"], "orphan_effects": ["孤立效果描述"],`,
			`"notes": "详细说明"}`,
		}, "\n")

		agent := newEnablementAgent(provider, "enablement-step2", prompt, claritySchema())
		defer agent.Close()

		inputText := buildPFEInput(input)

		// 追加领域特殊检查指令
		domainStr, _ := state[stateKeyDomain].(string)
		if supplement := DomainStep2Supplement(TechDomain(domainStr)); supplement != "" {
			inputText += "\n" + supplement
		}
		if step1Output != "" {
			inputText = "第 1 步（完整性检查）结论：\n" + step1Output + "\n\n" + inputText
		}

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step2_clarity: %w", err)
		}

		state[stateKeyStep2] = output
		return state, nil
	}
}
