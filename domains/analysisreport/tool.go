package analysisreport

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// ToolName 是专利分析报告工具的注册名。
const ToolName = "patent_analysis_report"

const toolDescription = `把权利要求、摘要与各引擎结论汇总为统一、可审计的专利分析报告。

输出：
- 技术特征抽取（以权利要求为粒度：类型/重要性/置信度）与分布统计；
- 多维质量评分（clarity / completeness 由规则确定性推导；novelty / technical_strength 可由 model_scores 注入）；
- checker 失败明细的聚合判级（任一 error → blocked；warning ≥3 条 → needs_revision）；
- IPC 分类摘要与审查要点、创新性洞察与专家考虑因素。

IPC 未显式提供时，按 claims + abstract（可加 ipc_text）做确定性分类；未命中关键词时不写入 IPC 段。
本工具不调用模型、不下法律结论；模型补分仅作为调用方输入记录在报告中。`

// ToolResult 是工具输出。
type ToolResult struct {
	Report  PatentAnalysisReport `json:"report"`
	Summary string               `json:"summary"`
}

type toolArgs struct {
	Claims               []string                     `json:"claims"`
	Abstract             string                       `json:"abstract"`
	PatentID             string                       `json:"patent_id"`
	Title                string                       `json:"title"`
	CheckerFailures      json.RawMessage              `json:"checker_failures"`
	IPCText              string                       `json:"ipc_text"`
	Graph                *GraphConclusionSummary      `json:"graph_conclusions"`
	EvidenceReceiptCount *int                         `json:"evidence_receipt_count"`
	KG                   *KgAnalysisSummary           `json:"kg_analysis"`
	ModelScores          map[QualityDomain]ModelScore `json:"model_scores"`
	SearchStrategy       *SearchStrategy              `json:"search_strategy"`
}

// NewTool 创建 patent_analysis_report 工具。
func NewTool() *agentcore.Tool {
	t := &reportTool{}
	return &agentcore.Tool{
		Name:        ToolName,
		Description: toolDescription,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"claims": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "权利要求文本列表（按权项顺序，第 1 项为独立权利要求）",
				},
				"abstract":  map[string]any{"type": "string", "description": "说明书摘要"},
				"patent_id": map[string]any{"type": "string"},
				"title":     map[string]any{"type": "string"},
				"checker_failures": map[string]any{
					"type":        "array",
					"description": "checker 失败明细；提供即视为已运行 checker（空数组表示无失败）",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"rule_id":  map[string]any{"type": "string"},
							"message":  map[string]any{"type": "string"},
							"severity": map[string]any{"type": "string", "enum": []string{"error", "warning", "info"}},
						},
						"required": []string{"rule_id", "message", "severity"},
					},
				},
				"ipc_text": map[string]any{
					"type":        "string",
					"description": "参与 IPC 分类的补充文本（缺省用 claims + abstract）",
				},
				"graph_conclusions": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"novelty":       map[string]any{"type": "string"},
						"inventiveness": map[string]any{"type": "string"},
						"enablement":    map[string]any{"type": "string"},
						"citations_ok":  map[string]any{"type": "boolean"},
					},
				},
				"evidence_receipt_count": map[string]any{"type": "integer", "minimum": 0},
				"kg_analysis": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"matched_entities": map[string]any{"type": "integer"},
						"related_entities": map[string]any{"type": "integer"},
						"confidence":       map[string]any{"type": "number"},
					},
				},
				"model_scores": map[string]any{
					"type":        "object",
					"description": "模型补分，键为维度（novelty/technical_strength/clarity/completeness）",
					"additionalProperties": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"score":     map[string]any{"type": "number", "minimum": 0, "maximum": 100},
							"rationale": map[string]any{"type": "string"},
						},
						"required": []string{"score"},
					},
				},
				"search_strategy": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query":    map[string]any{"type": "string"},
						"ipc":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"keywords": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"hits":     map[string]any{"type": "integer", "minimum": 0},
					},
					"required": []string{"query"},
				},
			},
			"required":             []string{"claims"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type reportTool struct{}

func (t *reportTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var input toolArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if len(input.Claims) == 0 {
		return nil, fmt.Errorf("缺少必填字段 'claims'（权利要求文本列表）")
	}
	inputs := Inputs{
		PatentID:             input.PatentID,
		Title:                input.Title,
		Claims:               input.Claims,
		Abstract:             input.Abstract,
		IPCText:              input.IPCText,
		Graph:                input.Graph,
		EvidenceReceiptCount: input.EvidenceReceiptCount,
		KG:                   input.KG,
		ModelScores:          input.ModelScores,
		SearchStrategy:       input.SearchStrategy,
	}
	if input.CheckerFailures != nil {
		if err := json.Unmarshal(input.CheckerFailures, &inputs.CheckerFailures); err != nil {
			return nil, fmt.Errorf("checker_failures 无效: %w", err)
		}
		inputs.CheckerRan = true
	}
	report := BuildReport(inputs)
	return ToolResult{Report: report, Summary: RenderSummary(report)}, nil
}

// RenderSummary 渲染报告要点（供 Agent 转述；结构化字段才是交付依据）。
func RenderSummary(report PatentAnalysisReport) string {
	lines := []string{"【专利分析报告】"}
	if report.Title != "" {
		lines = append(lines, "标题："+report.Title)
	}
	if len(report.IPC) > 0 {
		top := report.IPC[0]
		lines = append(lines, fmt.Sprintf("IPC：%s %s（置信度 %.2f）", top.Section, top.DomainName, top.Confidence))
	}
	lines = append(lines, fmt.Sprintf("技术特征：%d 项（装置 %d / 方法 %d / 材料 %d / 系统 %d / 数据 %d / 其它 %d）",
		report.FeatureStatistics.Total,
		report.FeatureStatistics.ByType[FeatureDevice],
		report.FeatureStatistics.ByType[FeatureMethod],
		report.FeatureStatistics.ByType[FeatureMaterial],
		report.FeatureStatistics.ByType[FeatureSystem],
		report.FeatureStatistics.ByType[FeatureData],
		report.FeatureStatistics.ByType[FeatureOther]))
	for _, score := range report.Scores {
		basis := "规则"
		if score.Basis == BasisModel {
			basis = "模型"
		}
		lines = append(lines, fmt.Sprintf("- %s：%.0f 分（%s）—— %s", score.Domain, score.Score, basis, score.Rationale))
	}
	if report.CheckerRan {
		lines = append(lines, "checker 判级："+string(report.CheckerVerdict))
	}
	if len(report.InnovationInsights) > 0 {
		lines = append(lines, "", "创新性洞察：")
		for _, insight := range report.InnovationInsights {
			lines = append(lines, "- "+insight)
		}
	}
	if len(report.ExpertConsiderations) > 0 {
		lines = append(lines, "", "专家考虑因素：")
		for _, consideration := range report.ExpertConsiderations {
			lines = append(lines, "- "+consideration)
		}
	}
	return strings.Join(lines, "\n")
}
