package triz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// ToolName 是 TRIZ 矛盾分析工具的注册名。
const ToolName = "triz_contradiction_analysis"

const toolDescription = `交底书侧的技术矛盾分析与参数完备性检查（发明人侧用途）。

从技术交底书识别「改善某工程参数的同时牺牲另一工程参数」的技术矛盾，落格到 TRIZ 39x39 经典矛盾矩阵取推荐发明原理，并列出交底书提到但未给出取值的工程参数（缺现状值／目标值／单位／测量口径）。
两处用途：① 方案生成——按推荐原理给出可替代的手段方向；② 交底书补强——把缺口清单交发明人补充。

调用方式（两步）：
1. 只传 text：返回抽取提示与 JSON Schema，按提示识别矛盾后得到 extraction JSON；
2. 再传 text + extraction：程序核验编号合法性（1-39）、证据能否在原文逐字定位，并完成矩阵落格。证据不通过的矛盾被丢弃并计入 dropped_for_evidence。

本工具不产出审查语义的技术问题表述，也不构成法律结论；创造性三步法第二步的「实际解决的技术问题」必须相对区别特征确定且不含解决手段，不得由本工具产物代填。`

// ToolResult 是工具输出：未带 extraction 时为抽取指引，带了则为核验产物。
type ToolResult struct {
	Status   string         `json:"status"` // needs_extraction | analyzed
	Analysis *Analysis      `json:"analysis,omitempty"`
	Report   string         `json:"report,omitempty"`
	Prompt   string         `json:"prompt,omitempty"`
	Schema   map[string]any `json:"schema,omitempty"`
}

type toolArgs struct {
	Text       string          `json:"text"`
	Focus      string          `json:"focus,omitempty"`
	Extraction json.RawMessage `json:"extraction,omitempty"`
}

// NewTool 创建 triz_contradiction_analysis 工具。
func NewTool() *agentcore.Tool {
	t := &trizTool{}
	return &agentcore.Tool{
		Name:        ToolName,
		Description: toolDescription,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "技术交底书或技术方案原文",
				},
				"focus": map[string]any{
					"type":        "string",
					"description": "可选关注方向（收窄识别范围，如「节拍」「良率」）",
				},
				"extraction": map[string]any{
					"type":        "object",
					"description": "按抽取提示得到的矛盾/缺口 JSON；省略时工具返回抽取提示与 Schema",
				},
			},
			"required":             []string{"text"},
			"additionalProperties": false,
		},
		Func: t.Run,
	}
}

type trizTool struct{}

func (t *trizTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var input toolArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if strings.TrimSpace(input.Text) == "" {
		return nil, fmt.Errorf("缺少必填字段 'text'（技术交底书原文）")
	}
	if len(input.Extraction) == 0 || string(input.Extraction) == "null" {
		return ToolResult{
			Status: "needs_extraction",
			Prompt: ExtractionPrompt(input.Text, input.Focus),
			Schema: ExtractionSchema(),
		}, nil
	}
	var extraction Extraction
	if err := json.Unmarshal(input.Extraction, &extraction); err != nil {
		return nil, fmt.Errorf("extraction 不是有效的抽取结果: %w", err)
	}
	analysis, err := BuildAnalysis(extraction, input.Text)
	if err != nil {
		return nil, err
	}
	return ToolResult{
		Status:   "analyzed",
		Analysis: &analysis,
		Report:   RenderAnalysis(analysis),
	}, nil
}

var matrixStatusLabels = map[MatrixStatus]string{
	MatrixRecommended: "矩阵有推荐原理",
	MatrixPhysical:    "物理矛盾（对角格，经典矩阵无条目）",
	MatrixGap:         "矩阵转录空缺（无推荐原理）",
}

var gapKindLabels = map[GapKind]string{
	GapCurrentValue: "缺现状值",
	GapTargetValue:  "缺目标值",
	GapUnit:         "缺单位",
	GapTestMethod:   "缺测量口径",
	GapOther:        "其它缺口",
}

// RenderAnalysis 把分析产物渲染为可读文本（供 Agent 转述，不替代结构化字段）。
func RenderAnalysis(analysis Analysis) string {
	lines := []string{"【TRIZ 技术矛盾分析（识别由模型完成，编号/落格/证据由程序核验）】"}
	if len(analysis.Contradictions) == 0 {
		lines = append(lines, "未发现通过核验的技术矛盾。")
	}
	for _, item := range analysis.Contradictions {
		lines = append(lines, fmt.Sprintf("- %s 改善「%s」↔ 恶化「%s」｜%s",
			item.ID, parameterText(item.Improving), parameterText(item.Worsening),
			statusLabel(item.MatrixStatus)))
		lines = append(lines, "  - 矛盾："+item.Statement)
		lines = append(lines, "  - 证据："+item.Evidence)
		if len(item.Principles) == 0 {
			lines = append(lines, "  - 推荐原理：无（矩阵无条目或转录空缺）")
		} else {
			names := make([]string, 0, len(item.Principles))
			for _, principle := range item.Principles {
				names = append(names, fmt.Sprintf("%d %s", principle.Number, principle.Name))
			}
			lines = append(lines, "  - 推荐原理："+strings.Join(names, "；"))
		}
		for _, direction := range item.SolutionDirections {
			lines = append(lines, "  - 候选方案方向："+direction)
		}
	}
	if len(analysis.ParameterGaps) > 0 {
		lines = append(lines, "", "参数完备性缺口（交底书补强清单）：")
		for _, gap := range analysis.ParameterGaps {
			kinds := make([]string, 0, len(gap.Missing))
			for _, kind := range gap.Missing {
				kinds = append(kinds, gapKindLabel(kind))
			}
			line := "- " + parameterText(gap.Parameter) + "：" + strings.Join(kinds, "、")
			if gap.Detail != "" {
				line += " —— " + gap.Detail
			}
			lines = append(lines, line)
		}
	}
	if len(analysis.Unmapped) > 0 {
		lines = append(lines, "", "未采纳项：")
		for _, item := range analysis.Unmapped {
			lines = append(lines, fmt.Sprintf("- %s —— %s", item.Statement, item.Reason))
		}
	}
	if analysis.DroppedForEvidence > 0 {
		lines = append(lines, "", fmt.Sprintf("因证据无法在原文定位而丢弃的矛盾：%d 条。", analysis.DroppedForEvidence))
	}
	return strings.Join(lines, "\n")
}

func parameterText(param Parameter) string {
	return fmt.Sprintf("%s（%d）", param.Name, param.Number)
}

func statusLabel(status MatrixStatus) string {
	if label, ok := matrixStatusLabels[status]; ok {
		return label
	}
	return status
}

func gapKindLabel(kind GapKind) string {
	if label, ok := gapKindLabels[kind]; ok {
		return label
	}
	return kind
}
