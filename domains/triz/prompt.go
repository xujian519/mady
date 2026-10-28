package triz

import (
	"fmt"
	"strings"
)

// 抽取提示与 JSON 形状：识别由调用方（Agent）完成，工具只做确定性核验。
// 提示里给出 39 个参数的编号与名称，避免模型自造编号后再被逐条丢弃。

// ExtractionPrompt 构建交底书 TRIZ 矛盾抽取提示。
func ExtractionPrompt(sourceText, focus string) string {
	params := make([]string, 0, MatrixMax)
	for _, param := range engineeringParams {
		params = append(params, fmt.Sprintf("%d %s", param.Number, param.Name))
	}
	lines := []string{
		"你是专利技术交底书分析师。请从下面的交底书中识别**技术矛盾**（改善某一工程参数的同时牺牲了另一工程参数），并指出参数完备性缺口。",
		"要求：",
		"- 只识别交底书里真实写到的取舍关系，不得臆断；交底书没说的一律不写。",
		"- evidence 必须是交底书原文的连续片段，逐字摘抄，供程序核验；核验不通过的矛盾会被丢弃。",
		"- improving 与 worsening 只能取下列 39 个经典工程参数的编号；无法映射的表述放进 unmapped，不要硬凑编号。",
		"- parameter_gaps 只列交底书**提到但没给取值**的参数（缺现状值 / 缺目标值 / 缺单位 / 缺测量口径）。",
		"- solution_directions 写候选方案方向（可替换的手段），不写结论，不写法条依据。",
	}
	if focus != "" {
		lines = append(lines, "关注方向："+focus)
	}
	lines = append(lines,
		"",
		"【39 个经典工程参数】",
		strings.Join(params, "；"),
		"",
		"【技术交底书】",
		sourceText,
		"",
		"请严格输出 JSON：{ contradictions: [{ improving, worsening, statement, evidence, solution_directions }], parameter_gaps: [{ parameter, missing, detail }], unmapped: [{ statement, reason }] }。",
		"识别完成后，把该 JSON 作为 extraction 参数交给 triz_contradiction_analysis（text 仍传交底书原文），由程序做核验与落格。",
	)
	return strings.Join(lines, "\n")
}

// ExtractionSchema 返回抽取结果的提示级 JSON Schema（供 Agent 生成 extraction 时参照）。
func ExtractionSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"contradictions": map[string]any{
				"type":        "array",
				"description": "交底书中真实存在的技术矛盾，逐条给出原文证据",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"improving": map[string]any{"type": "integer", "description": "改善的工程参数编号，1-39"},
						"worsening": map[string]any{"type": "integer", "description": "随之恶化的工程参数编号，1-39"},
						"statement": map[string]any{"type": "string", "description": "矛盾表述：改善什么、同时牺牲什么"},
						"evidence":  map[string]any{"type": "string", "description": "交底书原文片段，须逐字摘抄"},
						"solution_directions": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "候选方案方向（1-3 条，指向可替换的手段）",
						},
					},
					"required": []string{"improving", "worsening", "statement", "evidence"},
				},
			},
			"parameter_gaps": map[string]any{
				"type":        "array",
				"description": "交底书提到但未给出取值的工程参数",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"parameter": map[string]any{"type": "integer", "description": "工程参数编号，1-39"},
						"missing": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "string",
								"enum": GapKinds,
							},
						},
						"detail": map[string]any{"type": "string", "description": "缺口说明"},
					},
					"required": []string{"parameter", "missing"},
				},
			},
			"unmapped": map[string]any{
				"type":        "array",
				"description": "像技术问题但无法映射到 1-39 工程参数的表述",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"statement": map[string]any{"type": "string"},
						"reason":    map[string]any{"type": "string"},
					},
					"required": []string{"statement"},
				},
			},
		},
		"required": []string{"contradictions"},
	}
}
