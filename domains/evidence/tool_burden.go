package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

// BurdenToolName is the agent-visible name of the check_burden tool.
const BurdenToolName = "check_burden"

const burdenToolDesc = `查询特定场景下的举证责任分配规则。

支持场景：
- patent_invalidation: 专利无效宣告
- patent_infringement: 专利侵权
- novelty_challenge: 新颖性质疑
- inventiveness: 创造性评估
- disclosure: 充分公开
- priority: 优先权核实

返回举证责任方、适用证明标准、转移条件等信息。`

type burdenTool struct{}

func newBurdenTool() *agentcore.Tool {
	t := &burdenTool{}
	return &agentcore.Tool{
		Name:        BurdenToolName,
		Description: burdenToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenario": map[string]any{
					"type": "string",
					"enum": []string{"patent_invalidation", "patent_infringement", "novelty_challenge", "inventiveness", "disclosure", "priority"},
				},
			},
			"required":             []string{"scenario"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type burdenArgs struct {
	Scenario string `json:"scenario"`
}

func (t *burdenTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p burdenArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if p.Scenario == "" {
		return nil, fmt.Errorf("缺少必填字段 'scenario'")
	}

	result := DetermineBurden(BurdenScenario(p.Scenario), nil)

	return map[string]any{
		"holder":       result.BurdenHolder,
		"standard":     result.Standard,
		"has_shifted":  result.HasShifted,
		"shift_reason": result.ShiftReason,
		"reasoning":    result.Reasoning,
	}, nil
}
