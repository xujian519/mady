package domains

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xujian519/mady/agentcore"
)

const deadlineExtName = "deadline_calculator"

// DeadlineCalculatorExtension 将专利期限计算工具注入 Agent。
type DeadlineCalculatorExtension struct{}

var (
	_ agentcore.Extension    = (*DeadlineCalculatorExtension)(nil)
	_ agentcore.ToolProvider = (*DeadlineCalculatorExtension)(nil)
)

// NewDeadlineCalculatorExtension 创建期限计算扩展。
func NewDeadlineCalculatorExtension() *DeadlineCalculatorExtension {
	return &DeadlineCalculatorExtension{}
}

func (e *DeadlineCalculatorExtension) Name() string                                     { return deadlineExtName }
func (e *DeadlineCalculatorExtension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }
func (e *DeadlineCalculatorExtension) Dispose() error                                   { return nil }

func (e *DeadlineCalculatorExtension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		deadlineTool(),
	}
}

//nolint:gosec // 工具名称和描述不是硬编码凭据
const deadlineToolName = "calculate_patent_deadlines"

//nolint:gosec // 工具描述不是硬编码凭据
const deadlineToolDesc = `计算中国专利生命周期中的所有法定期限。

支持发明、实用新型、外观设计三类专利，根据申请日（或优先权日）计算：
- 优先权期限（发明/实用新型 12 个月，外观设计 6 个月）
- 实质审查请求期限（发明 3 年）
- 分案申请期限（发明/实用新型 2 年）
- 审查意见答复期限
- 办理登记手续期限
- 年费缴纳提醒

输出为 Markdown 表格格式或 JSON 结构。`

type deadlineArgs struct {
	FilingDate   string `json:"filing_date"`
	CaseType     string `json:"case_type"`
	OutputFormat string `json:"output_format,omitempty"`
}

// mapCaseType 将工具输入的中英文专利类型映射为 CalculatePatentDeadlines 接受的格式。
func mapCaseType(raw string) string {
	switch raw {
	case "invention", "发明":
		return "发明"
	case "utility_model", "实用新型":
		return "实用新型"
	case "design", "外观设计", "外观":
		return "外观设计"
	default:
		return raw
	}
}

func deadlineTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        deadlineToolName,
		Description: deadlineToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filing_date": map[string]any{
					"type":        "string",
					"format":      "date",
					"description": "申请日（YYYY-MM-DD）",
				},
				"case_type": map[string]any{
					"type":        "string",
					"description": "专利类型",
					"enum":        []string{"invention", "utility_model", "design", "发明", "实用新型", "外观设计"},
				},
				"output_format": map[string]any{
					"type":        "string",
					"enum":        []string{"markdown", "json"},
					"description": "输出格式，默认 markdown",
				},
			},
			"required":             []string{"filing_date", "case_type"},
			"additionalProperties": false,
		},
		Func: func(_ context.Context, raw json.RawMessage) (any, error) {
			var p deadlineArgs
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, fmt.Errorf("参数无效: %w", err)
			}
			filingDate, err := time.Parse("2006-01-02", p.FilingDate)
			if err != nil {
				return nil, fmt.Errorf("日期格式无效（需要 YYYY-MM-DD）: %w", err)
			}
			deadlines := CalculatePatentDeadlines(filingDate, mapCaseType(p.CaseType))
			if p.OutputFormat == "json" {
				summary := SummarizeDeadlines(deadlines)
				return SerializeDeadlineSummary(summary), nil
			}
			return FormatDeadlineReport(deadlines), nil
		},
		ReadOnly: true,
	}
}
