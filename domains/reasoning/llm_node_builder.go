package reasoning

import (
	"context"
	"fmt"
	"strings"
)

// LLMNodeBuilder is a NodeBuilder implementation that actually calls an LLM to
// perform each plan step's analysis. Unlike noopNodeBuilder (which only echoes
// step descriptions), this builder sends the step's description plus the
// relevant facts/rules from the blackboard to the LLM, producing real analytical
// output that accumulates across steps.
//
// This is what makes the five-step workflow tool produce substance rather than
// empty metadata. Without it, formatResult only outputs step names and JSON
// state — no actual patent analysis.
type LLMNodeBuilder struct {
	llm LlmClient
}

// NewLLMNodeBuilder creates a NodeBuilder that uses the given LlmClient for
// real analysis. If llm is nil, falls back to noop behavior.
func NewLLMNodeBuilder(llm LlmClient) *LLMNodeBuilder {
	return &LLMNodeBuilder{llm: llm}
}

// BuildChainNode creates a node that asks the LLM to perform a single analysis
// step. The prompt includes the step description, all known facts from the
// blackboard, and any rules retrieved in Stage ②. The output is stored in
// state["step_{order}_analysis"] and accumulated in state["analysis_history"].
func (b *LLMNodeBuilder) BuildChainNode(step PlanStep, bb *FactBlackboard) PregelNode {
	nodeID := fmt.Sprintf("step_%d_chain", step.Order)
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		if b.llm == nil {
			state[nodeID+"_output"] = step.Description + " — 完成"
			return state, nil
		}

		prompt := b.buildStepPrompt(step, bb, state)
		result, err := b.llm.Chat(ctx, []LlmMessage{
			{Role: "system", Content: "你是一位资深专利代理人。请根据以下案件信息，完成指定的分析步骤。输出简洁专业的分析，不超过500字。"},
			{Role: "user", Content: prompt},
		})
		if err != nil {
			state[nodeID+"_output"] = fmt.Sprintf("%s（分析失败: %v）", step.Description, err)
			return state, nil // non-fatal: continue with remaining steps
		}

		state[nodeID+"_output"] = result
		// Accumulate analysis on the blackboard (accessible to later steps
		// and to formatResult for the final output).
		prevAny, _ := bb.StageOutput("analysis")
		prev, _ := prevAny.(string)
		bb.SetStageOutput("analysis", prev+fmt.Sprintf("\n\n### 步骤%d：%s\n%s", step.Order, step.Description, result))
		return state, nil
	}
}

// BuildReActThink creates a think node for ReAct steps. It asks the LLM what
// information is still needed before proceeding.
func (b *LLMNodeBuilder) BuildReActThink(step PlanStep, bb *FactBlackboard) PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		if b.llm == nil {
			state["_noop_has_next"] = "false"
			return state, nil
		}
		// For ReAct, do a single analysis pass (no multi-iteration loop).
		prompt := b.buildStepPrompt(step, bb, state)
		result, err := b.llm.Chat(ctx, []LlmMessage{
			{Role: "system", Content: "你是一位资深专利代理人。请完成以下分析步骤。"},
			{Role: "user", Content: prompt},
		})
		if err == nil {
			prevAny, _ := bb.StageOutput("analysis")
			prev, _ := prevAny.(string)
			bb.SetStageOutput("analysis", prev+fmt.Sprintf("\n\n### 步骤%d：%s\n%s", step.Order, step.Description, result))
		}
		state["_noop_has_next"] = "false" // single pass
		return state, nil
	}
}

// BuildReActAct is a no-op for LLM builder (no external tools in this context).
func (b *LLMNodeBuilder) BuildReActAct(step PlanStep, bb *FactBlackboard) PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		return state, nil
	}
}

// BuildReActObserve signals end of ReAct iteration.
func (b *LLMNodeBuilder) BuildReActObserve(step PlanStep, bb *FactBlackboard) PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		state["_noop_has_next"] = "false"
		return state, nil
	}
}

// buildStepPrompt constructs the LLM prompt for a single analysis step,
// including accumulated context from previous steps.
func (b *LLMNodeBuilder) buildStepPrompt(step PlanStep, bb *FactBlackboard, state PregelState) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("请完成以下分析步骤：\n%s\n\n", step.Description))

	// Include relevant facts from the blackboard.
	facts := bb.ActiveFacts()
	if len(facts) > 0 {
		sb.WriteString("已知案件事实：\n")
		for i, f := range facts {
			if i >= 10 {
				break
			}
			content := f.Content
			if len(content) > 300 {
				content = content[:300] + "..."
			}
			sb.WriteString(fmt.Sprintf("- %s\n", content))
		}
		sb.WriteString("\n")
	}

	// Include analysis from previous steps (chain context).
	prevAny, ok := bb.StageOutput("analysis")
	prev, _ := prevAny.(string)
	if ok && prev != "" {
		sb.WriteString("前序步骤的分析结果：\n")
		// Truncate to avoid context explosion.
		if len(prev) > 2000 {
			prev = prev[:2000] + "\n...(前序分析已截断)"
		}
		sb.WriteString(prev)
		sb.WriteString("\n\n")
	}

	sb.WriteString("请基于以上信息，输出本步骤的分析结论。")
	return sb.String()
}
