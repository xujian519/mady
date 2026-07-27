package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

// DetermineStandardToolName is the agent-visible name of the determine_standard tool.
const DetermineStandardToolName = "determine_standard"

const determineStandardToolDesc = `根据举证场景确定应当适用的证明标准。

适用场景与对应标准：
- patent_infringement:    高度盖然性 (high_probability)
- priority:               高度盖然性 (high_probability)
- patent_invalidation:    优势证据 (preponderance)
- novelty_challenge:      优势证据 (preponderance)
- inventiveness:          优势证据 (preponderance)
- disclosure:             优势证据 (preponderance)

先调用此工具确定标准，再调用 assess_standard 进行具体评估。`

type determineStandardTool struct{}

func newDetermineStandardTool() *agentcore.Tool {
	t := &determineStandardTool{}
	return &agentcore.Tool{
		Name:        DetermineStandardToolName,
		Description: determineStandardToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scenario": map[string]any{
					"type": "string",
					"enum": []string{
						"patent_invalidation",
						"patent_infringement",
						"novelty_challenge",
						"inventiveness",
						"disclosure",
						"priority",
					},
					"description": "举证场景",
				},
			},
			"required":             []string{"scenario"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type determineStandardArgs struct {
	Scenario string `json:"scenario"`
}

func (t *determineStandardTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p determineStandardArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if p.Scenario == "" {
		return nil, fmt.Errorf("缺少必填字段 'scenario'")
	}

	standard := DetermineStandard(BurdenScenario(p.Scenario))

	return map[string]any{
		"scenario":             p.Scenario,
		"recommended_standard": string(standard),
		"description":          StandardDescription(standard),
	}, nil
}

// StandardDescription 返回证明标准的中文说明。
func StandardDescription(standard StandardOfProof) string {
	switch standard {
	case StandardBeyondReasonableDoubt:
		return "排除合理怀疑——证据确凿、充分，不存在合理怀疑空间"
	case StandardHighProbability:
		return "高度盖然性——证据显示待证事实极有可能发生"
	case StandardPreponderance:
		return "优势证据——支持性证据的证明力超过反对性证据"
	case StandardSubstantialEvidence:
		return "实质性证据——存在至少一份有效直接证据"
	case StandardPrimaFacie:
		return "初步证据——存在至少一份表面有效的证据"
	default:
		return "未知标准"
	}
}
