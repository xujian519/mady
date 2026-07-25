package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

const StandardToolName = "assess_standard"

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
			"type": "object",
			"properties": map[string]any{
				"standard": map[string]any{
					"type": "string",
					"enum": []string{"beyond_reasonable_doubt", "high_probability", "preponderance", "substantial_evidence", "prima_facie"},
				},
				"supporting_count": map[string]any{"type": "integer"},
				"total_count":      map[string]any{"type": "integer"},
				"gaps":             map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required":             []string{"standard", "supporting_count", "total_count"},
			"additionalProperties": false,
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
		"met":                 result.Met,
		"standard":            result.Standard,
		"confidence":          result.Confidence,
		"supporting_count":    result.SupportingCount,
		"contradicting_count": result.ContradictingCount,
		"reasoning":           result.Reasoning,
		"gaps":                result.Gaps,
	}, nil
}
