package inventiveness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// ProblemCheckToolName 是技术问题合规检查工具的注册名。
const ProblemCheckToolName = "check_technical_problem"

const problemCheckToolDescription = `检查创造性三步法第二步「实际解决的技术问题」的表述合规性（A22.3，审查指南第二部分第四章 3.2.1.1）。

- PROBLEM-EQUALS-FEATURE：问题规范化后与某项区别技术特征相同（阻断）；
- PROBLEM-SOLUTION-BINDING：问题包含解决手段，如「通过设置X」「利用X装置」（阻断）；
- PROBLEM-MULTI-CAUSAL：问题含多个因果连接词，疑似捆绑/复合问题（阻断）；
- PROBLEM-UNMEASURED：问题缺可测指标（提示级，不影响通过）。

输入可直接给 problem，也可给 text（评估文本或 diff JSON），由工具按「actual_technical_problem」字段或「实际解决的技术问题：」句提取；提取不到时不阻断，报告写明原因。`

// ProblemCheckResult 是工具输出；Skipped 表示未提取到技术问题。
type ProblemCheckResult struct {
	ProblemComplianceResult
	Skipped bool `json:"skipped,omitempty"`
}

type problemCheckArgs struct {
	Problem  string   `json:"problem"`
	Features []string `json:"features"`
	Text     string   `json:"text"`
}

// NewProblemCheckTool 创建 check_technical_problem 工具。
func NewProblemCheckTool() *agentcore.Tool {
	t := &problemCheckTool{}
	return &agentcore.Tool{
		Name:        ProblemCheckToolName,
		Description: problemCheckToolDescription,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"problem": map[string]any{
					"type":        "string",
					"description": "实际解决的技术问题；与 text 二选一",
				},
				"features": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "区别技术特征列表；缺省时跳过「问题是特征本身」检查",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "评估文本或 diff JSON；problem 为空时从中提取技术问题",
				},
			},
			"anyOf": []any{
				map[string]any{"required": []string{"problem"}},
				map[string]any{"required": []string{"text"}},
			},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type problemCheckTool struct{}

func (t *problemCheckTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var input problemCheckArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	problem := strings.TrimSpace(input.Problem)
	if problem == "" && strings.TrimSpace(input.Text) != "" {
		problem = strings.TrimSpace(ExtractTechnicalProblem(input.Text))
	}
	if problem == "" {
		return ProblemCheckResult{
			Skipped: true,
			ProblemComplianceResult: ProblemComplianceResult{
				Passed: true,
				Report: "实际解决的技术问题合规检查（A22.3 第二步）：未提取到实际解决的技术问题，无法校验（不阻断）",
			},
		}, nil
	}
	return ProblemCheckResult{ProblemComplianceResult: CheckProblemCompliance(problem, input.Features)}, nil
}
