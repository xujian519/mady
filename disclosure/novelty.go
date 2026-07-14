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

// noveltyNode 返回新颖性初判的 Pregel 节点。
// 基于提取结果和关键词，使用 LLM 逐特征分析新颖性。
// 替代原有的纯启发式 stub 实现。
func noveltyNode(provider agentcore.Provider) graph.PregelNode {
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:           "disclosure-novelty",
			Model:          "default",
			Provider:       provider,
			Temperature:    0.2,
			ResponseFormat: agentcore.NewJSONSchemaResponseFormat("novelty_assessment", noveltySchema()),
		},
		SystemPrompt: buildNoveltyPrompt(),
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:          1,
			ValidateArguments: true,
		},
	}

	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		input := buildNoveltyInput(state)
		if input == "" {
			// No features extracted — cannot assess.
			state[StateKeyNovelty] = &NoveltyResult{
				Assessed:   false,
				Conclusion: "未提取到技术特征，无法进行新颖性初判",
				Notes:      "请确认交底书内容完整性后重新分析。",
			}
			return state, nil
		}

		agent := agentcore.New(cfg)
		defer agent.Close()

		output, err := agent.Run(ctx, input)
		if err != nil {
			// Fallback to heuristic assessment on LLM failure.
			fallback := assessNoveltyFromState(state)
			fallback.Notes += "\n\n【注意】LLM 评估失败，使用启发式回退：" + err.Error()
			state[StateKeyNovelty] = fallback
			return state, nil
		}

		result := parseNoveltyOutput(output, state)
		state[StateKeyNovelty] = result
		return state, nil
	}
}

// buildNoveltyPrompt 构造新颖性分析的 SystemPrompt。
func buildNoveltyPrompt() string {
	return strings.Join([]string{
		"你是一名资深专利审查员，负责对技术交底书进行新颖性预评估。",
		"请基于以下技术特征和检索关键词，逐项分析其新颖性。",
		"",
		"评估维度：",
		"1. 每个技术特征是否在现有技术中已知",
		"2. 已知的相似技术对比",
		"3. 特征组合是否构成新的技术方案",
		"",
		"输出要求：",
		"- 使用 JSON 格式，严格按照 schema 输出",
		"- 每个技术特征都要有独立的评估",
		"- 标注置信度（high/medium/low）",
		"- 无证据推测时明确标注为「疑似」",
	}, "\n")
}

// buildNoveltyInput 从 PregelState 构建新颖性分析的输入。
func buildNoveltyInput(state graph.PregelState) string {
	var sb strings.Builder

	if raw, ok := state[StateKeyExtraction]; ok {
		if ext, ok := raw.(*ExtractionResult); ok && ext != nil {
			fmt.Fprintf(&sb, "技术特征数量：%d\n\n", len(ext.Features))
			sb.WriteString("## 技术特征列表\n\n")
			for _, f := range ext.Features {
				fmt.Fprintf(&sb, "- ID: %s\n", f.ID)
				fmt.Fprintf(&sb, "  描述: %s\n", f.Description)
				fmt.Fprintf(&sb, "  分类: %s\n", f.Category)
				fmt.Fprintf(&sb, "  功能: %s\n", f.Function)
				fmt.Fprintf(&sb, "  现有技术状态: %s\n", f.PriorArtStatus)
				fmt.Fprintf(&sb, "  重要度: %s\n\n", f.Importance)
			}

			if len(ext.Problems) > 0 {
				sb.WriteString("## 要解决的技术问题\n\n")
				for _, p := range ext.Problems {
					fmt.Fprintf(&sb, "- %s\n", p)
				}
				sb.WriteString("\n")
			}

			if len(ext.Effects) > 0 {
				sb.WriteString("## 技术效果\n\n")
				for _, e := range ext.Effects {
					fmt.Fprintf(&sb, "- %s\n", e)
				}
				sb.WriteString("\n")
			}
		}
	}

	if kw, ok := state[StateKeySearchKeywords]; ok {
		if kwList, ok := kw.([]string); ok && len(kwList) > 0 {
			fmt.Fprintf(&sb, "## 检索关键词\n\n%s\n\n", strings.Join(kwList, "、"))
		}
	}

	return sb.String()
}

// noveltySchema 返回新颖性分析的 JSON Schema。
var noveltySchemaCache map[string]any
var noveltySchemaOnce sync.Once

func noveltySchema() map[string]any {
	noveltySchemaOnce.Do(func() {
		noveltySchemaCache = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"conclusion": map[string]any{
					"type":        "string",
					"description": "整体新颖性判断结论",
				},
				"feature_assessments": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"feature_id":        map[string]any{"type": "string"},
							"novelty_status":    map[string]any{"type": "string", "enum": []string{"likely_novel", "possibly_known", "likely_known", "unclear"}},
							"confidence":        map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
							"reasoning":         map[string]any{"type": "string"},
							"similar_prior_art": map[string]any{"type": "string"},
						},
						"required": []string{"feature_id", "novelty_status", "confidence", "reasoning"},
					},
				},
				"overall_confidence": map[string]any{
					"type": "string",
					"enum": []string{"high", "medium", "low"},
				},
			},
			"required": []string{"conclusion", "feature_assessments", "overall_confidence"},
		}
	})
	return noveltySchemaCache
}

var noveltyStatusLabels = map[string]string{
	"likely_novel":   "可能具有新颖性",
	"possibly_known": "可能属于现有技术",
	"likely_known":   "很可能属于现有技术",
	"unclear":        "不确定",
}

type noveltyOutput struct {
	Conclusion         string              `json:"conclusion"`
	FeatureAssessments []featureAssessment `json:"feature_assessments"`
	OverallConfidence  string              `json:"overall_confidence"`
}

type featureAssessment struct {
	FeatureID       string `json:"feature_id"`
	NoveltyStatus   string `json:"novelty_status"`
	Confidence      string `json:"confidence"`
	Reasoning       string `json:"reasoning"`
	SimilarPriorArt string `json:"similar_prior_art,omitempty"`
}

// parseNoveltyOutput 解析 LLM 的 JSON 输出为 NoveltyResult。
func parseNoveltyOutput(output string, _ graph.PregelState) *NoveltyResult {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return &NoveltyResult{
			Assessed:   true,
			Conclusion: "LLM 输出解析失败",
			Notes:      "原始输出：\n" + output,
		}
	}

	var parsed noveltyOutput
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return &NoveltyResult{
			Assessed:   true,
			Conclusion: "LLM 输出解析失败",
			Notes:      fmt.Sprintf("JSON 解析错误: %v\n原始输出：\n%s", err, output),
		}
	}

	var b strings.Builder
	b.WriteString("## 新颖性分析（LLM 评估）\n\n")
	for _, fa := range parsed.FeatureAssessments {
		label := noveltyStatusLabels[fa.NoveltyStatus]
		if label == "" {
			label = fa.NoveltyStatus
		}
		fmt.Fprintf(&b, "- 特征 %s: **%s** (置信度: %s)\n", fa.FeatureID, label, fa.Confidence)
		b.WriteString("  " + fa.Reasoning + "\n")
		if fa.SimilarPriorArt != "" {
			b.WriteString("  相似现有技术: " + fa.SimilarPriorArt + "\n")
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\n**整体置信度**: %s\n", parsed.OverallConfidence)
	b.WriteString("\n**注意：** 本评估为 AI 辅助预分析，不构成正式新颖性判断。")

	return &NoveltyResult{
		Assessed:   true,
		Conclusion: parsed.Conclusion,
		Notes:      b.String(),
	}
}

// extractJSON 从文本中提取第一个 JSON 对象。
func extractJSON(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return ""
}
