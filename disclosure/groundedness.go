package disclosure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Groundedness Filter — 提取特征原文依据过滤
// =============================================================================
//
// 在 merge_extractions → check_consistency 之间插入，评估提取的特征是否在
// 原始交底书中有坚实依据。使用批处理 LLM 调用（一次调用评估所有特征），
// 将 groundedness 分数写入各特征的 Confidence 字段供下游使用。
// LLM 调用失败时 fail-open（不阻塞管线），设置 Skipped 标记。

// groundednessFilterNode 返回 groundedness 过滤的 Pregel 节点。
func groundednessFilterNode(provider agentcore.Provider) graph.PregelNode {
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:        "disclosure-groundedness",
			Model:       "default",
			Provider:    provider,
			Temperature: 0.1, // 低温度减少评分波动
		},
		SystemPrompt: buildGroundednessPrompt(),
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:          1,
			ValidateArguments: true,
		},
	}
	if supportsJSONSchemaResponseFormat() {
		cfg.ResponseFormat = agentcore.NewJSONSchemaResponseFormat("groundedness_assessment", groundednessSchema())
	}

	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		ext, ok := GetExtraction(state)
		if !ok || len(ext.Features) == 0 {
			// 无特征可过滤，跳过
			state[StateKeyGroundedness] = &GroundednessResult{Skipped: true}
			return state, nil
		}

		input := buildGroundednessInput(state, ext)
		if input == "" {
			state[StateKeyGroundedness] = &GroundednessResult{Skipped: true}
			return state, nil
		}

		agent := agentcore.New(cfg)
		defer agent.Close()

		output, err := agent.Run(ctx, input)
		if err != nil {
			// fail-open: LLM 调用失败不阻塞管线
			state[StateKeyGroundedness] = &GroundednessResult{
				Skipped:  true,
				Feedback: "LLM 调用失败，跳过 groundedness 过滤: " + err.Error(),
			}
			return state, nil
		}

		result := parseGroundednessOutput(output, ext)
		// 将 groundedness 分数写入各特征的 Confidence 字段
		for i := range ext.Features {
			if score, ok := result.Scores[ext.Features[i].ID]; ok {
				ext.Features[i].Confidence = score
			}
		}
		state[StateKeyExtraction] = ext
		state[StateKeyGroundedness] = result
		return state, nil
	}
}

// buildGroundednessPrompt 构造 Groundedness 评估的 SystemPrompt。
func buildGroundednessPrompt() string {
	return strings.Join([]string{
		"你是一名资深专利审查员，负责验证从技术交底书中提取的技术特征是否" +
			"在原文中有坚实依据。",
		"",
		"任务：",
		"对于列出的每个技术特征，判断其描述是否直接基于交底书原文内容。",
		"",
		"评分标准：",
		"- 0.8-1.0：特征描述在原文中有明确依据，用词高度对应",
		"- 0.6-0.8：特征描述在原文中有一定依据，但存在少量推断或概括",
		"- 0.3-0.6：特征描述部分有依据，但存在明显推断或组合",
		"- 0.0-0.3：特征描述在原文中找不到依据（可能是 LLM 脑补）",
		"",
		"注意：",
		"- 特征提取允许合理概括和归并，只要核心内容来自原文即可",
		"- 只有完全脱离原文的「幻觉」内容才打低分",
		"- 如果不确定，给出中等分数并提供理由",
	}, "\n")
}

// groundednessSchema 返回 groundedness 评估的 JSON Schema。
var groundednessSchemaCache map[string]any
var groundednessSchemaOnce sync.Once

func groundednessSchema() map[string]any {
	groundednessSchemaOnce.Do(func() {
		groundednessSchemaCache = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"assessments": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"feature_id": map[string]any{"type": "string"},
							"score":      map[string]any{"type": "number", "description": "0-1 groundedness 分数"},
							"reasoning":  map[string]any{"type": "string", "description": "评分理由"},
							"source_snippet": map[string]any{
								"type":        "string",
								"description": "原文中最相关的依据片段（如找到的话）",
							},
						},
						"required":             []string{"feature_id", "score", "reasoning"},
						"additionalProperties": false,
					},
				},
				"overall_note": map[string]any{
					"type":        "string",
					"description": "整体评估说明",
				},
			},
			"required":             []string{"assessments"},
			"additionalProperties": false,
		}
	})
	return groundednessSchemaCache
}

// buildGroundednessInput 构建 groundedness 评估的 LLM 输入。
// 格式为：原始交底书章节在前，提取的特征列表在后。
func buildGroundednessInput(state graph.PregelState, ext *ExtractionResult) string {
	var sb strings.Builder

	// 添加原始交底书内容（优先使用 section 内容）
	if doc, ok := state[StateKeyDoc].(*DisclosureDoc); ok && doc != nil {
		// 提取关键章节：技术方案、具体实施方式、有益效果
		for _, section := range []DocSection{SecProblem, SecSolution, SecEffect, SecEmbodiments} {
			if content, ok := doc.Sections[section]; ok && content != "" {
				fmt.Fprintf(&sb, "=== %s ===\n", section)
				sb.WriteString(truncate(content, 3000))
				sb.WriteString("\n\n")
			}
		}
	}

	// 添加提取的特征列表
	sb.WriteString("=== 提取的技术特征 ===\n\n")
	for _, f := range ext.Features {
		fmt.Fprintf(&sb, "- 特征 %s:\n", f.ID)
		fmt.Fprintf(&sb, "  描述: %s\n", f.Description)
		if f.Function != "" {
			fmt.Fprintf(&sb, "  功能: %s\n", f.Function)
		}
		fmt.Fprintf(&sb, "  分类: %s\n\n", f.Category)
	}

	sb.WriteString("\n请逐条判断每个特征是否在原文中有依据，按 JSON schema 输出分数。")
	return sb.String()
}

// groundednessOutput 是 LLM 响应的内部解析结构。
type groundednessOutput struct {
	Assessments []groundednessItem `json:"assessments"`
	OverallNote string             `json:"overall_note,omitempty"`
}

type groundednessItem struct {
	FeatureID     string  `json:"feature_id"`
	Score         float64 `json:"score"`
	Reasoning     string  `json:"reasoning"`
	SourceSnippet string  `json:"source_snippet,omitempty"`
}

// parseGroundednessOutput 解析 LLM 的 JSON 输出为 GroundednessResult。
// LLM 响应中缺失的特征默认分数为 0.3（不确定）。
func parseGroundednessOutput(output string, ext *ExtractionResult) *GroundednessResult {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		// 无法解析时所有特征赋默认值
		scores := make(map[string]float64, len(ext.Features))
		for _, f := range ext.Features {
			scores[f.ID] = 0.3
		}
		return &GroundednessResult{
			Scores:   scores,
			LowCount: len(ext.Features),
			Feedback: "LLM 输出解析失败，所有特征默认 groundedness = 0.3（不确定）",
		}
	}

	var parsed groundednessOutput
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		scores := make(map[string]float64, len(ext.Features))
		for _, f := range ext.Features {
			scores[f.ID] = 0.3
		}
		return &GroundednessResult{
			Scores:   scores,
			LowCount: len(ext.Features),
			Feedback: fmt.Sprintf("JSON 解析错误: %v，全部默认 0.3", err),
		}
	}

	// 构建 feature_id → score 映射
	scores := make(map[string]float64, len(ext.Features))
	lowCount := 0
	for _, item := range parsed.Assessments {
		score := item.Score
		if score < 0 {
			score = 0
		} else if score > 1 {
			score = 1
		}
		scores[item.FeatureID] = score
		if score < 0.6 {
			lowCount++
		}
	}

	// 补全 LLM 未返回的特征（默认 0.3）
	for _, f := range ext.Features {
		if _, exists := scores[f.ID]; !exists {
			scores[f.ID] = 0.3
			lowCount++
		}
	}

	return &GroundednessResult{
		Scores:   scores,
		LowCount: lowCount,
	}
}
