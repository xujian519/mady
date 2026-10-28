package fees

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

// ToolName 是工具名。
const ToolName = "patent_fees"

// Config 是工具配置。
type Config struct {
	// Table 直接注入费用索引；nil 时加载随包索引。
	Table *Table
}

// toolArgs 是工具入参。
type toolArgs struct {
	PatentType         string   `json:"patent_type"`
	Triggers           []string `json:"triggers"`
	Claims             *int     `json:"claims"`
	SpecificationPages *int     `json:"specification_pages"`
	PriorityClaims     *int     `json:"priority_claims"`
	ExtensionMonths    *int     `json:"extension_months"`
	AnnuityYears       []int    `json:"annuity_years"`
	LateMonths         *int     `json:"late_months"`
	Reduction          *struct {
		Kind  string `json:"kind"`
		Filed bool   `json:"filed"`
	} `json:"reduction"`
}

// NewTool 构造 patent_fees 工具。
func NewTool(cfg Config) *agentcore.Tool {
	table := cfg.Table
	return &agentcore.Tool{
		Name:        ToolName,
		Description: "按官费标准为一件中国专利案件计价：列出应缴费种、计数基准、减缴与滞纳金。金额未转录或缺少计数输入时拒绝给出合计。",
		ReadOnly:    true,
		Parameters:  toolParameters(),
		Func: func(_ context.Context, raw json.RawMessage) (any, error) {
			return runQuote(raw, table)
		},
	}
}

// toolParameters 返回工具参数 schema。
func toolParameters() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{
			"patent_type", "triggers",
		},
		"properties": map[string]any{
			"patent_type": map[string]any{
				"type":        "string",
				"enum":        patentTypeStrings(),
				"description": "专利类型：invention（发明）/ utility-model（实用新型）/ design（外观设计）。",
			},
			"triggers": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string", "enum": triggerStrings()},
				"description": "要计算的收费阶段，如 filing、substantive-examination、annual-fee。",
			},
			"claims":              map[string]any{"type": "integer", "description": "权利要求总项数（申请阶段的附加费需要）。"},
			"specification_pages": map[string]any{"type": "integer", "description": "说明书总页数。"},
			"priority_claims":     map[string]any{"type": "integer", "description": "优先权要求项数。"},
			"extension_months":    map[string]any{"type": "integer", "description": "请求延长的月数。"},
			"annuity_years": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "integer"},
				"description": "要计算年费的专利年度；本工具不自行推算年度序号。",
			},
			"late_months": map[string]any{"type": "integer", "description": "年费超期月数。"},
			"reduction": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":  map[string]any{"type": "string", "description": "减缴类型：individual / enterprise。"},
					"filed": map[string]any{"type": "boolean", "description": "是否已办理费减备案。"},
				},
				"description": "费用减缴请求；不填表示按全额计。",
			},
		},
	}
}

// runQuote 执行一次计价。
func runQuote(raw json.RawMessage, table *Table) (any, error) {
	var args toolArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("patent_fees: 参数解析失败: %w", err)
	}
	if table == nil {
		loaded, err := BundledTable()
		if err != nil {
			return nil, fmt.Errorf("patent_fees: 加载费用索引失败: %w", err)
		}
		table = loaded
	}
	report, err := table.Compute(toQuery(args))
	if err != nil {
		return nil, err
	}
	return report, nil
}

// toQuery 把工具入参转成计价查询。
func toQuery(args toolArgs) Query {
	triggers := make([]Trigger, 0, len(args.Triggers))
	for _, raw := range args.Triggers {
		triggers = append(triggers, Trigger(raw))
	}
	query := Query{
		PatentType:         PatentType(args.PatentType),
		Triggers:           triggers,
		Claims:             args.Claims,
		SpecificationPages: args.SpecificationPages,
		PriorityClaims:     args.PriorityClaims,
		ExtensionMonths:    args.ExtensionMonths,
		AnnuityYears:       args.AnnuityYears,
		LateMonths:         args.LateMonths,
	}
	if args.Reduction != nil {
		query.Reduction = &ReductionRequest{Kind: args.Reduction.Kind, Filed: args.Reduction.Filed}
	}
	return query
}
