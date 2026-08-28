package enablement

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// step1CompletenessNode 三步法第 1 步：检查说明书结构完整性。
// 对照 5 项必要章节（技术领域/背景技术/发明内容/附图说明/具体实施方式），
// 识别缺失章节并给出完整性评分。
func step1CompletenessNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		prompt := strings.Join([]string{
			"你是一名资深专利审查员。请执行专利法第26条第3款（充分公开）评估的第 1 步：",
			"**检查说明书结构完整性**。",
			"",
			"根据审查指南第二部分第二章第 2.1.2 节，「完整」的说明书应包含三个层次：",
			"1. 帮助理解发明**不可缺少**的内容（技术领域、背景技术）",
			"2. 确定新颖性、创造性、实用性**所需**的内容（发明内容的核心技术方案）",
			"3. **实现**发明所需的内容（具体实施方式的详细描述）",
			"",
			"**必要章节**（缺失任一项即为结构不完整）：",
			"1. 技术领域",
			"2. 背景技术",
			"3. 发明内容（要解决的技术问题、技术方案、有益效果）",
			"4. 附图说明（如有附图）",
			"5. 具体实施方式（至少一个实施例）",
			"",
			"**内容质量检查**（不仅检查章节存在，还需评估内容充分性）：",
			"- 发明内容中是否明确记载了**要解决的技术问题**、**技术方案**和**有益效果**",
			"- 具体实施方式是否提供了足以实施的详细描述（非仅泛泛描述）",
			"- 背景技术中引证文件（如有）是否给出明确指引（出处和内容）",
			"",
			"**「完整 ≠ 面面俱到」原则**：",
			"- 所属领域技术人员基于常识能知晓的公知内容可以省略：",
			"  · 公知的常规载体信息可省略",
			"  · 公知的电子元件内部结构可省略",
			"  · 所属领域熟知的工艺流程可省略",
			"- 不可省略的是：发明核心创新点、关键技术参数、实施步骤",
			"",
			"请基于提供的说明书章节内容，判断每一项是否存在**及其内容质量**。",
			"如果某项章节缺失或内容过于简略（如仅有一句话），请标记为缺失。",
			"",
			"",
			"请输出 JSON 格式：",
			`{"has_tech_field": bool, "has_background": bool, "has_content": bool,`,
			`"has_drawings": bool, "has_embodiment": bool,`,
			`"missing_sections": ["缺失章节名称"], "score": 0.0-1.0,`,
			`"notes": "详细说明，包括内容质量问题"}`,
		}, "\n")

		agent := newEnablementAgent(provider, "enablement-step1", prompt, completenessSchema())
		defer agent.Close()

		output, err := agent.Run(ctx, buildCompletenessInput(input))
		if err != nil {
			return state, fmt.Errorf("step1_completeness: %w", err)
		}

		state[stateKeyStep1] = output
		return state, nil
	}
}

func buildCompletenessInput(input *EnablementInput) string {
	var sb strings.Builder
	sb.WriteString("## 说明书章节内容\n\n")

	if len(input.DocSections) > 0 {
		for name, content := range input.DocSections {
			fmt.Fprintf(&sb, "### %s\n%s\n\n", name, truncateText(content, 1000))
		}
	} else {
		sb.WriteString("（未提供章节切分结果，请基于技术特征和 PFE 三元组进行判断）\n\n")
	}

	fmt.Fprintf(&sb, "## 是否有附图\n%v\n\n", input.HasDrawings)

	if len(input.Features) > 0 {
		sb.WriteString("## 技术特征\n")
		for _, f := range input.Features {
			fmt.Fprintf(&sb, "- [%s] %s (重要度: %s)\n", f.Category, f.Description, f.Importance)
		}
		sb.WriteString("\n")
	}
	renderSimilarCases(&sb, input.SimilarCases)

	return sb.String()
}

// completenessSchema 是 step1 的 JSON Schema。
func completenessSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"has_tech_field": map[string]any{"type": "boolean"},
			"has_background": map[string]any{"type": "boolean"},
			"has_content":    map[string]any{"type": "boolean"},
			"has_drawings":   map[string]any{"type": "boolean"},
			"has_embodiment": map[string]any{"type": "boolean"},
			"missing_sections": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"score": map[string]any{"type": "number"},
			"notes": map[string]any{"type": "string"},
		},
		"required": []string{"has_tech_field", "has_background", "has_content", "has_embodiment", "missing_sections", "score"},
	}
}

func parseCompleteness(output string) CompletenessResult {
	r := CompletenessResult{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.Notes = output
		return r
	}

	var parsed struct {
		HasTechField    bool     `json:"has_tech_field"`
		HasBackground   bool     `json:"has_background"`
		HasContent      bool     `json:"has_content"`
		HasDrawings     bool     `json:"has_drawings"`
		HasEmbodiment   bool     `json:"has_embodiment"`
		MissingSections []string `json:"missing_sections"`
		Score           float64  `json:"score"`
		Notes           string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.Notes = output // LLM 返回非 JSON：降级为原始文本作为判断依据
		return r
	}

	r.MissingSections = parsed.MissingSections
	r.Score = parsed.Score
	r.Notes = parsed.Notes
	return r
}
