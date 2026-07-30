package inventiveness

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/pkg/util"
)

// =============================================================================
// State keys
// =============================================================================

const (
	// StateKeyInput 是 PregelState 中存储 InventivenessInput 的 key。
	StateKeyInput = "inventiveness_input"
	// StateKeyResult 是 PregelState 中存储 InventivenessResult 的 key。
	StateKeyResult     = "inventiveness_result"
	stateKeyExperiment = "evaluate_experimental_data"
	stateKeyStep1      = "step1_closest_prior_art"
	stateKeyStep2      = "step2_distinguishing_features"
	stateKeyStep3      = "step3_technical_suggestion"
	stateKeyStep4      = "step4_significant_progress"
)

// JSON Schema constants.
const (
	jsTypeObject               = "object"
	jsTypeString               = "string"
	jsTypeArray                = "array"
	jsTypeBoolean              = "boolean"
	jsFieldProperties          = "properties"
	jsFieldRequired            = "required"
	jsFieldItems               = "items"
	jsFieldEnum                = "enum"
	jsFieldRationale           = "rationale"
	jsFieldConfidence          = "confidence"
	jsValHigh                  = "high"
	jsValLow                   = "low"
	jsValMedium                = "medium"
	jsFieldType                = "type"
	jsFieldSignificantProgress = "has_significant_progress"
)

// =============================================================================
// 节点实现
// =============================================================================

// loadInputNode 从 PregelState 读取 InventivenessInput。
// 当 EvidenceCoverage == "none" 时跳过，设置 Skipped=true。
func loadInputNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		raw, ok := state[StateKeyInput]
		if !ok {
			state[StateKeyResult] = &InventivenessResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "未提供输入数据（inventiveness_input state key 为空）",
			}
			return state, nil
		}

		input, ok := raw.(*InventivenessInput)
		if !ok || input == nil {
			state[StateKeyResult] = &InventivenessResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "输入数据格式无效",
			}
			return state, nil
		}

		if input.EvidenceCoverage == "none" {
			state[StateKeyResult] = &InventivenessResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "无检索证据，无法进行三步法创造性评估",
			}
			return state, nil
		}

		// Store validated input for downstream nodes.
		state[StateKeyInput] = input
		return state, nil
	}
}

// =============================================================================
// Agent 工厂函数
// =============================================================================

// newInventivenessAgent 创建统一配置的 Agent 节点。
// 所有三步法 LLM 节点共享 Temperature=0.2 和 MaxTurns=1，仅 name/prompt/schema 不同。
func newInventivenessAgent(provider agentcore.Provider, name, prompt string, schema map[string]any) *agentcore.Agent {
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

// =============================================================================
// 辅助函数
// =============================================================================

// extractInput 从 state 中安全读取已验证的 InventivenessInput。
// loadInputNode 已确保类型正确且 EvidenceCoverage != "none"，此处仅做防御性断言。
func extractInput(state graph.PregelState) *InventivenessInput {
	raw, ok := state[StateKeyInput]
	if !ok {
		return nil
	}
	input, ok := raw.(*InventivenessInput)
	if !ok {
		return nil
	}
	return input
}

// stateHasSkip 检查 loadInputNode 是否设置了跳过标志。
func stateHasSkip(state graph.PregelState) bool {
	raw, ok := state[StateKeyResult]
	if !ok {
		return false
	}
	r, ok := raw.(*InventivenessResult)
	return ok && r != nil && r.Skipped
}

// buildInputText 将结构化输入格式化为 LLM 友好的 Markdown 文本。
func buildInputText(input *InventivenessInput) string {
	var sb strings.Builder
	if input == nil {
		return ""
	}

	if len(input.Features) > 0 {
		sb.WriteString("## 技术特征\n")
		for _, f := range input.Features {
			fmt.Fprintf(&sb, "- [%s] %s (%s)\n", f.Category, f.Description, f.Importance)
		}
		sb.WriteString("\n")
	}

	if len(input.PriorArtChunks) > 0 {
		sb.WriteString("## 现有技术证据\n")
		for i, c := range input.PriorArtChunks {
			fmt.Fprintf(&sb, "[%d] %s\n    %s\n\n", i+1, c.Title, c.Snippet)
		}
	}

	if len(input.PFETriples) > 0 {
		sb.WriteString("## PFE 三元组（问题→特征→效果因果链）\n")
		for _, t := range input.PFETriples {
			fmt.Fprintf(&sb, "- [%s] 问题: %s → 效果: %s\n", t.ID, t.Problem, t.Effect)
		}
		sb.WriteString("\n")
	}

	if input.NoveltyConclusion != "" {
		fmt.Fprintf(&sb, "## 新颖性初判结论\n%s\n\n", input.NoveltyConclusion)
	}

	return sb.String()
}

func extractJSON(text string) string {
	return util.ExtractJSONSimple(text)
}
