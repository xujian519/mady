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
// It holds a map of LlmClient instances keyed by model name, enabling
// per-judge model selection in arbitrated multi-LLM scenarios (BuildArbitratedJudgeNode).
// The "default" key holds the primary client used for chain/react steps.
//
// This is what makes the five-step workflow tool produce substance rather than
// empty metadata. Without it, formatResult only outputs step names and JSON
// state — no actual patent analysis.
type LLMNodeBuilder struct {
	clients map[string]LlmClient
}

// NewLLMNodeBuilder creates a NodeBuilder that uses the given LlmClient for
// real analysis. If llm is nil, falls back to noop behavior.
// The client is stored under the "default" key for use in chain/react steps.
func NewLLMNodeBuilder(llm LlmClient) *LLMNodeBuilder {
	clients := make(map[string]LlmClient, 1)
	if llm != nil {
		clients["default"] = llm
	}
	return &LLMNodeBuilder{clients: clients}
}

// NewLLMNodeBuilderWithClients creates a NodeBuilder with multiple LlmClient
// instances keyed by model name. Use this for multi-LLM arbitration where
// different judges use different models. The "default" key is used for
// chain/react steps; arbitrated judges select their client by model name.
func NewLLMNodeBuilderWithClients(clients map[string]LlmClient) *LLMNodeBuilder {
	if clients == nil {
		clients = make(map[string]LlmClient)
	}
	return &LLMNodeBuilder{clients: clients}
}

// clientFor returns the LlmClient for the given model name. If model is empty
// or not found, falls back to the "default" client. Returns nil if no client
// is available.
func (b *LLMNodeBuilder) clientFor(model string) LlmClient {
	if b.clients == nil {
		return nil
	}
	if model != "" {
		if c, ok := b.clients[model]; ok {
			return c
		}
	}
	return b.clients["default"]
}

// BuildChainNode creates a node that asks the LLM to perform a single analysis
// step. The prompt includes the step description, all known facts from the
// blackboard, and any rules retrieved in Stage ②. The output is stored in
// state["step_{order}_analysis"] and accumulated in state["analysis_history"].
func (b *LLMNodeBuilder) BuildChainNode(step PlanStep, bb *FactBlackboard) PregelNode {
	nodeID := fmt.Sprintf("step_%d_chain", step.Order)
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		llm := b.clientFor("")
		if llm == nil {
			state[nodeID+"_output"] = step.Description + " — 完成"
			return state, nil
		}

		prompt := b.buildStepPrompt(step, bb, state)
		result, err := llm.Chat(ctx, []LlmMessage{
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
		llm := b.clientFor("")
		if llm == nil {
			state["_noop_has_next"] = "false"
			return state, nil
		}
		// For ReAct, do a single analysis pass (no multi-iteration loop).
		prompt := b.buildStepPrompt(step, bb, state)
		result, err := llm.Chat(ctx, []LlmMessage{
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

// =============================================================================
// MultiModelNodeBuilder — 多模型 Advocate 包装器
// =============================================================================

// MultiModelNodeBuilder 包装多个 NodeBuilder，每个对应一个 LLM 模型。
// 用于多模型辩论场景：Advocate A 用模型 X，Advocate B 用模型 Y。
type MultiModelNodeBuilder struct {
	builders map[string]NodeBuilder // role → NodeBuilder
	roles    map[string]string      // stepID_role → modelID
}

// NewMultiModelNodeBuilder 创建 MultiModelNodeBuilder。
// roleModels 映射 advocate_role → modelID（如 "adv_a" → "deepseek"）。
// builders 是 modelID → NodeBuilder 的预构建映射。
func NewMultiModelNodeBuilder(roleModels map[string]string, builders map[string]NodeBuilder) *MultiModelNodeBuilder {
	roles := make(map[string]string, len(roleModels))
	for role, modelID := range roleModels {
		if _, ok := builders[modelID]; ok {
			roles[role] = modelID
		}
	}
	return &MultiModelNodeBuilder{builders: builders, roles: roles}
}

// selectBuilder 根据 state 中的 advocate_role 标签选择对应的 builder。
// 无匹配时优先返回 "default" 命名的 builder（确定性选择），
// 避免 Go map 迭代随机性导致的不确定回退行为。
func (m *MultiModelNodeBuilder) selectBuilder(state PregelState) NodeBuilder {
	role, _ := state["advocate_role"].(string)
	if role != "" {
		if modelID, ok := m.roles[role]; ok {
			if b, ok := m.builders[modelID]; ok {
				return b
			}
		}
	}
	// Deterministic fallback: look for a "default" named builder first.
	if b, ok := m.builders["default"]; ok {
		return b
	}
	// Last fallback: noop
	return noopNodeBuilder{}
}

// BuildChainNode creates a chain node, selecting the builder at runtime
// so the advocate_role in the runtime state is available for delegation.
func (m *MultiModelNodeBuilder) BuildChainNode(step PlanStep, bb *FactBlackboard) PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		return m.selectBuilder(state).BuildChainNode(step, bb)(ctx, state)
	}
}

// BuildReActThink creates a think node, selecting the builder at runtime.
func (m *MultiModelNodeBuilder) BuildReActThink(step PlanStep, bb *FactBlackboard) PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		return m.selectBuilder(state).BuildReActThink(step, bb)(ctx, state)
	}
}

func (m *MultiModelNodeBuilder) BuildReActAct(step PlanStep, bb *FactBlackboard) PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		return m.selectBuilder(state).BuildReActAct(step, bb)(ctx, state)
	}
}

func (m *MultiModelNodeBuilder) BuildReActObserve(step PlanStep, bb *FactBlackboard) PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		return m.selectBuilder(state).BuildReActObserve(step, bb)(ctx, state)
	}
}

func (m *MultiModelNodeBuilder) BuildArbitratedJudgeNode(step PlanStep, bb *FactBlackboard, cfg *ArbitrationConfig) PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		return m.selectBuilder(state).BuildArbitratedJudgeNode(step, bb, cfg)(ctx, state)
	}
}

// BuildArbitratedJudgeNode creates a node that uses multi-LLM arbitration.
// When cfg is nil or has no judges, falls back to deterministic judge logic.
// Each judge uses its configured model via b.clientFor(j.Model) — if no client
// is registered for that model, the judge is skipped (graceful degradation).
func (b *LLMNodeBuilder) BuildArbitratedJudgeNode(step PlanStep, bb *FactBlackboard, cfg *ArbitrationConfig) PregelNode {
	nodeID := fmt.Sprintf("step_%d_judge", step.Order)
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		if cfg == nil || len(cfg.Judges) == 0 {
			// No arbitration configured — fall back to deterministic pass-through.
			state[nodeID+"_output"] = step.Description + " — 确定性裁决（无多 LLM 仲裁）"
			state[mhVerdict] = Verdict{
				Resolved:         false,
				UnresolvedReason: "无仲裁配置，回退确定性裁决",
				Confidence:       0,
			}
			return state, nil
		}
		// Multi-LLM arbitration: collect votes from all judges.
		var votes []JudgeVote
		totalWeight := 0.0
		resolvedCount := 0

		for _, j := range cfg.Judges {
			llm := b.clientFor(j.Model)
			if llm == nil {
				// No client registered for this judge's model — skip.
				continue
			}

			prompt := fmt.Sprintf("请对以下争议性法律问题做出裁决：\n%s\n\n"+
				"基于所提供的案件事实和法条规则，给出你的判断和置信度。",
				step.Description)

			result, err := llm.Chat(ctx, []LlmMessage{
				{Role: "system", Content: "你是一位资深专利法官。请对下列争议做出独立裁决。"},
				{Role: "user", Content: prompt},
			})

			if err != nil {
				continue
			}

			totalWeight += j.Weight
			resolvedCount++
			votes = append(votes, JudgeVote{
				JudgeName: j.Name,
				Score:     j.Weight,
				Reasoning: result,
			})
		}

		if resolvedCount == 0 {
			state[nodeID+"_output"] = step.Description + " — 全部 LLM 裁决失败，回退确定性判断"
			state[mhVerdict] = Verdict{
				Resolved:         false,
				UnresolvedReason: "全部 LLM 裁决均失败",
				Confidence:       0,
			}
			return state, nil
		}

		// Aggregate votes into a weighted verdict.
		avgScore := totalWeight / float64(resolvedCount)
		resolved := avgScore >= cfg.MinAgreement
		var unresolvedReason string
		if !resolved {
			unresolvedReason = fmt.Sprintf("加权一致度 %.2f 低于阈值 %.2f", avgScore, cfg.MinAgreement)
		}

		outputMsg := fmt.Sprintf("%s — 多 LLM 仲裁完成 (%d 个裁决)", step.Description, resolvedCount)
		state[nodeID+"_output"] = outputMsg
		state[nodeID+"_votes"] = votes
		state[mhVerdict] = Verdict{
			Resolved:         resolved,
			Confidence:       avgScore,
			Rationale:        outputMsg,
			UnresolvedReason: unresolvedReason,
		}
		return state, nil
	}
}

// buildStepPrompt constructs the LLM prompt for a single analysis step,
// including accumulated context from previous steps.
func (b *LLMNodeBuilder) buildStepPrompt(step PlanStep, bb *FactBlackboard, state PregelState) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "请完成以下分析步骤：\n%s\n\n", step.Description)

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
			fmt.Fprintf(&sb, "- %s\n", content)
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
