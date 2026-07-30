package enablement

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/pkg/util"
)

// =============================================================================
// 节点实现
// =============================================================================

// loadInputNode 从 PregelState 读取 EnablementInput 并验证有效性。
// 当 PFE 三元组为空或特征数为 0 时设置 Skipped=true，后续节点全部跳过。
func loadInputNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		raw, ok := state[stateKeyInput]
		if !ok {
			state[stateKeyResult] = &EnablementResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "未提供输入数据（enablement_input state key 为空）",
			}
			return state, nil
		}

		input, ok := raw.(*EnablementInput)
		if !ok || input == nil {
			state[stateKeyResult] = &EnablementResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "输入数据格式无效",
			}
			return state, nil
		}

		// 无特征数据时无法评估充分公开。
		if len(input.Features) == 0 && len(input.PFETriples) == 0 {
			state[stateKeyResult] = &EnablementResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "未提取到技术特征或 PFE 三元组，无法评估说明书充分公开",
			}
			return state, nil
		}

		// 存储已验证的输入，供下游节点使用。
		state[stateKeyInput] = input

		// 自动检测技术领域，供下游节点附加领域规则。
		domain := DetectDomain(input)
		state[stateKeyDomain] = string(domain)

		return state, nil
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

// newEnablementAgent 创建统一配置的 LLM Agent 节点。
// 所有评估节点共享 Temperature=0.2 和 MaxTurns=1，仅 name/prompt/schema 不同。
func newEnablementAgent(provider agentcore.Provider, name, prompt string, schema map[string]any) *agentcore.Agent {
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:        name,
			Model:       "default",
			Provider:    provider,
			Temperature: 0.2,
		},
		SystemPrompt: prompt,
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 1,
		},
	}
	if schema != nil {
		cfg.ResponseFormat = agentcore.NewJSONSchemaResponseFormat(name, schema)
	}
	return agentcore.New(cfg)
}

// extractInput 从 state 中安全读取已验证的 EnablementInput。
func extractInput(state graph.PregelState) *EnablementInput {
	raw, ok := state[stateKeyInput]
	if !ok {
		return nil
	}
	input, ok := raw.(*EnablementInput)
	if !ok {
		return nil
	}
	return input
}

// stateHasSkip 检查 loadInputNode 是否设置了跳过标志。
func stateHasSkip(state graph.PregelState) bool {
	raw, ok := state[stateKeyResult]
	if !ok {
		return false
	}
	r, ok := raw.(*EnablementResult)
	return ok && r != nil && r.Skipped
}

// buildCompletenessInput 构建 Step 1 的 LLM 输入文本。

// renderSimilarCases 将类案列表格式化为 Markdown 引用块。
// 若 cases 为空则跳过输出。
func renderSimilarCases(sb *strings.Builder, cases []string) {
	if len(cases) == 0 {
		return
	}
	sb.WriteString("## 类案参考\n")
	for i, c := range cases {
		fmt.Fprintf(sb, "- 案例%d: %s\n", i+1, c)
	}
	sb.WriteString("\n")
}

// parsedConclusion 是最终结论的解析结果。
type parsedConclusion struct {
	IsSufficient    bool                      `json:"is_sufficient"`
	Reasoning       string                    `json:"reasoning"`
	Confidence      string                    `json:"confidence"`
	Deficiencies    []string                  `json:"deficiencies"`
	SupportWarnings []string                  `json:"support_warnings"`
	ExperimentData  *ExperimentDataAssessment `json:"experiment_data"`
}

// extractJSON 从文本中提取第一个 JSON 对象。
func extractJSON(text string) string {
	return util.ExtractJSONSimple(text)
}

// truncateText 截断文本到指定长度（rune 安全）。
func truncateText(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
