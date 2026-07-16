package reasoning

import (
	"fmt"
	"strings"
)

// ManifestToSystemPrompt generates a structured system prompt from a
// WorkflowManifest's Stage 4 steps. Unlike the generic five-step prompt
// ("① 收集事实 ② 检索规则 ..."), this prompt embeds the manifest's specific,
// domain-tailored step descriptions — providing the Agent with precise
// analytical guidance without the overhead of external tool-call orchestration.
//
// Empirical finding (2026-07-16): for LLM Agents, structured prompt guidance
// outperforms external step-by-step orchestration (PlanStep → Pregel → 5 LLM
// calls). The manifest's step descriptions are more valuable as prompt context
// than as execution instructions.
func ManifestToSystemPrompt(m *WorkflowManifest) string {
	if m == nil || len(m.Stage4.Steps) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "你是一名资深的专利代理人。请按照以下%d步分析流程完成本次任务：\n\n", len(m.Stage4.Steps))

	for _, step := range m.Stage4.Steps {
		strategyHint := strategyDescription(step.Strategy)
		fmt.Fprintf(&sb, "第%d步（%s）：%s\n", step.Order, strategyHint, step.Description)
	}

	sb.WriteString("\n分析要求：\n")
	sb.WriteString("- 按上述步骤顺序逐步分析，每步产出明确的中间结论\n")
	sb.WriteString("- 后续步骤须基于前序步骤的分析结果\n")
	sb.WriteString("- 最终给出综合结论，引用具体法条\n")
	sb.WriteString("- 不得编造对比文件、法条或技术特征\n")

	return sb.String()
}

// strategyDescription maps a StrategyType to a human-readable hint for the prompt.
func strategyDescription(s StrategyType) string {
	switch s {
	case StrategyChain:
		return "链式分析"
	case StrategyReact:
		return "检索推理"
	case StrategyMultiHypothesis:
		return "多假设论证"
	default:
		return string(s)
	}
}
