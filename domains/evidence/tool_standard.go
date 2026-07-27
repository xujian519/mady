package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

// StandardToolName is the agent-visible name of the assess_standard tool.
const StandardToolName = "assess_standard"

// JSON Schema constants for tool_standard.go.
const (
	jsTypeObject           = "object"
	jsTypeString           = "string"
	jsTypeArray            = "array"
	jsFieldProperties      = "properties"
	jsFieldRequired        = "required"
	jsFieldStandard        = "standard"
	jsFieldAdditional      = "additionalProperties"
	jsFieldMet             = "met"
	jsFieldReasoning       = "reasoning"
	jsFieldType            = "type"
	jsFieldEnum            = "enum"
	jsFieldSupportingCount = "supporting_count"
	jsFieldItems           = "items"
)

const standardToolDesc = `评估已有证据是否达到指定证明标准。

证明标准等级：
- beyond_reasonable_doubt: 排除合理怀疑（>= 95% + 至少3条证据）
- high_probability: 高度盖然性（>= 80% + 至少2条证据）
- preponderance: 优势证据（> 50%）
- substantial_evidence: 实质性证据（至少1条有效直接证据）
- prima_facie: 初步证据（至少1条表面有效证据）`

type standardTool struct{}

func newStandardTool() *agentcore.Tool {
	t := &standardTool{}
	return &agentcore.Tool{
		Name:        StandardToolName,
		Description: standardToolDesc,
		Parameters: map[string]any{
			jsFieldType: jsTypeObject,
			jsFieldProperties: map[string]any{
				jsFieldStandard: map[string]any{
					jsFieldType: jsTypeString,
					jsFieldEnum: []string{"beyond_reasonable_doubt", "high_probability", "preponderance", "substantial_evidence", "prima_facie"},
				},
				jsFieldSupportingCount: map[string]any{jsFieldType: "integer"},
				"total_count":          map[string]any{jsFieldType: "integer"},
				"gaps":                 map[string]any{jsFieldType: jsTypeArray, jsFieldItems: map[string]any{jsFieldType: jsTypeString}},
			},
			jsFieldRequired:   []string{jsFieldStandard, jsFieldSupportingCount, "total_count"},
			jsFieldAdditional: false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type standardArgs struct {
	Standard        string   `json:"standard"`
	SupportingCount int      `json:"supporting_count"`
	TotalCount      int      `json:"total_count"`
	Gaps            []string `json:"gaps"`
}

func (t *standardTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p standardArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if p.Standard == "" {
		return nil, fmt.Errorf("缺少必填字段 'standard'")
	}

	result := AssessProofStandard(StandardOfProof(p.Standard), p.SupportingCount, p.TotalCount, p.Gaps)

	return map[string]any{
		jsFieldMet:             result.Met,
		jsFieldStandard:        result.Standard,
		"confidence":           result.Confidence,
		jsFieldSupportingCount: result.SupportingCount,
		"contradicting_count":  result.ContradictingCount,
		jsFieldReasoning:       result.Reasoning,
		"gaps":                 result.Gaps,
	}, nil
}
