package reasoning

import (
	"context"
	"fmt"
	"math"
)

// =============================================================================
// Multi-Hypothesis Pregel Subgraph — Advocate A + Advocate B → Judge → Verdict
// =============================================================================
//
// Superstep 1: Advocate A (pro) and Advocate B (con) run ReAct in parallel.
//   Each advocate operates in an isolated state namespace (adv_a_* / adv_b_*).
//   They share the FactBlackboard + RuleSet as read-only input.
//
// Superstep 2: The arbitrated judge node (from builder.BuildArbitratedJudgeNode)
//   runs either deterministic logic (noop builder) or multi-LLM arbitration
//   (LLMNodeBuilder with ArbitrationConfig) to evaluate both arguments.
//
// Superstep 3: Conditional — if the verdict is unresolved and the cause
//   is missing facts (not poor reasoning), trigger a retrieval back-edge.
//   Otherwise, route to the rejection path (human escalation) or end.

// state key prefixes for multi-hypothesis nodes.
const (
	mhAdvA         = "adv_a"
	mhAdvB         = "adv_b"
	mhReject       = "mh_rejection"
	mhVerdict      = "mh_verdict"
	mhTieThreshold = 0.05 // evidence score difference below this is a tie
)

// authorityMap maps Requirement to authority weight for evidence scoring.
var authorityMap = map[Requirement]float64{
	ReqMust:   1.0,
	ReqShould: 0.7,
	ReqNote:   0.4,
}

// BuildMultiHypothesisSubgraph builds the advocate + judge Pregel subgraph
// for a multi_hypothesis PlanStep.
//
// Returns (entryNode, terminalNode) for connecting into the parent graph.
//
// Graph structure:
//
//	adv_a_think → adv_a_act → adv_a_observe (→ back to think, conditional)
//	adv_b_think → adv_b_act → adv_b_observe (→ back to think, conditional)
//	     │                        │
//	     └────────┬───────────────┘
//	              ↓
//	       mh_merge ──→ mh_arbitrated_judge
//	                           │
//	                 ┌─ reject? → mh_rejection
//	                 └─ accept  → (next step)
//
// The judge node is delegated to builder.BuildArbitratedJudgeNode, which enables
// multi-LLM arbitration when an ArbitrationConfig is provided (LLMNodeBuilder),
// or deterministic fallback (noopNodeBuilder for testing).
func BuildMultiHypothesisSubgraph(g GraphBuilder, step PlanStep, bb *FactBlackboard, builder NodeBuilder) (string, string, error) {
	// === Advocate A (pro) ===
	aThink := fmt.Sprintf("mh_%d_adv_a_think", step.Order)
	aAct := fmt.Sprintf("mh_%d_adv_a_act", step.Order)
	aObserve := fmt.Sprintf("mh_%d_adv_a_observe", step.Order)

	if err := g.AddNode(aThink, builder.BuildReActThink(step, bb)); err != nil {
		return "", "", fmt.Errorf("add adv_a think: %w", err)
	}
	if err := g.AddNode(aAct, builder.BuildReActAct(step, bb)); err != nil {
		return "", "", fmt.Errorf("add adv_a act: %w", err)
	}
	if err := g.AddNode(aObserve, builder.BuildReActObserve(step, bb)); err != nil {
		return "", "", fmt.Errorf("add adv_a observe: %w", err)
	}
	if err := g.AddEdge(aThink, aAct); err != nil {
		return "", "", fmt.Errorf("connect adv_a think→act: %w", err)
	}
	if err := g.AddEdge(aAct, aObserve); err != nil {
		return "", "", fmt.Errorf("connect adv_a act→observe: %w", err)
	}
	if err := g.SetConditionalEdge(aObserve, advocateRouter(mhAdvA, aObserve, aThink)); err != nil {
		return "", "", fmt.Errorf("set adv_a conditional edge: %w", err)
	}

	// === Advocate B (con) ===
	bThink := fmt.Sprintf("mh_%d_adv_b_think", step.Order)
	bAct := fmt.Sprintf("mh_%d_adv_b_act", step.Order)
	bObserve := fmt.Sprintf("mh_%d_adv_b_observe", step.Order)

	if err := g.AddNode(bThink, builder.BuildReActThink(step, bb)); err != nil {
		return "", "", fmt.Errorf("add adv_b think: %w", err)
	}
	if err := g.AddNode(bAct, builder.BuildReActAct(step, bb)); err != nil {
		return "", "", fmt.Errorf("add adv_b act: %w", err)
	}
	if err := g.AddNode(bObserve, builder.BuildReActObserve(step, bb)); err != nil {
		return "", "", fmt.Errorf("add adv_b observe: %w", err)
	}
	if err := g.AddEdge(bThink, bAct); err != nil {
		return "", "", fmt.Errorf("connect adv_b think→act: %w", err)
	}
	if err := g.AddEdge(bAct, bObserve); err != nil {
		return "", "", fmt.Errorf("connect adv_b act→observe: %w", err)
	}
	if err := g.SetConditionalEdge(bObserve, advocateRouter(mhAdvB, bObserve, bThink)); err != nil {
		return "", "", fmt.Errorf("set adv_b conditional edge: %w", err)
	}

	// === Merge ===
	mergeName := fmt.Sprintf("mh_%d_merge", step.Order)
	if err := g.AddNode(mergeName, mergeNode()); err != nil {
		return "", "", fmt.Errorf("add merge node: %w", err)
	}
	// Both observers feed into merge.
	if err := g.AddEdge(aObserve, mergeName); err != nil {
		return "", "", fmt.Errorf("connect a_observe→merge: %w", err)
	}
	if err := g.AddEdge(bObserve, mergeName); err != nil {
		return "", "", fmt.Errorf("connect b_observe→merge: %w", err)
	}

	// === Judge ===
	// Delegate to the builder's arbitrated judge — replaces the old hardcoded
	// syllogismJudgeNode + evidenceJudgeNode chain. Pass nil ArbitrationConfig
	// so the builder uses its default logic (deterministic for noop, or the
	// caller can inject via LLMNodeBuilder's cfg at the higher level).
	judgeName := fmt.Sprintf("mh_%d_arbitrated_judge", step.Order)
	rejectName := fmt.Sprintf("mh_%d_rejection", step.Order)

	if err := g.AddNode(judgeName, builder.BuildArbitratedJudgeNode(step, bb, nil)); err != nil {
		return "", "", fmt.Errorf("add arbitrated judge: %w", err)
	}
	if err := g.AddNode(rejectName, rejectionNode()); err != nil {
		return "", "", fmt.Errorf("add rejection node: %w", err)
	}

	if err := g.AddEdge(mergeName, judgeName); err != nil {
		return "", "", fmt.Errorf("connect merge→judge: %w", err)
	}

	// Conditional: both resolved and unresolved verdicts go through rejectionNode.
	// rejectionNode passes through for resolved verdicts, produces escalation
	// message for unresolved ones.
	if err := g.SetConditionalEdge(judgeName, evidenceRouter(rejectName)); err != nil {
		return "", "", fmt.Errorf("set judge conditional edge: %w", err)
	}

	return aThink, rejectName, nil
}

// =============================================================================
// PregelNode implementations
// =============================================================================

// advocateRouter creates a conditional edge for an advocate's ReAct loop.
func advocateRouter(_ string, observeName, thinkName string) PregelEdgeRouter {
	hasNextKey := observeName + "_has_next"
	return func(ctx context.Context, state PregelState) []string {
		hn := state.GetString(hasNextKey)
		if hn == "" {
			hn = state.GetString("_noop_has_next") // fallback for test/noop builder
		}
		if hn == "true" {
			return []string{thinkName}
		}
		return nil // advance to next static edge
	}
}

// mergeNode collects arguments from both advocates.
func mergeNode() PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		out := PregelState{}

		// Extract Advocate A argument.
		argA := extractArgument(state, mhAdvA)
		out["arg_a"] = argA

		// Extract Advocate B argument.
		argB := extractArgument(state, mhAdvB)
		out["arg_b"] = argB

		return out, nil
	}
}

// syllogismJudgeNode filters logically invalid arguments.
// Kept as an exported helper; use BuildMultiHypothesisSubgraph's
// arbitrated judge path for new code.
func syllogismJudgeNode() PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		argA, _ := state["arg_a"].(Argument)
		argB, _ := state["arg_b"].(Argument)

		out := PregelState{
			"arg_a": argA,
			"arg_b": argB,
		}

		// Level 1: Both args must have supporting facts and rules.
		validA := len(argA.SupportingFacts) > 0 || len(argA.SupportingRules) > 0
		validB := len(argB.SupportingFacts) > 0 || len(argB.SupportingRules) > 0

		if !validA {
			out["adv_a_valid"] = false
		} else {
			out["adv_a_valid"] = true
		}
		if !validB {
			out["adv_b_valid"] = false
		} else {
			out["adv_b_valid"] = true
		}

		return out, nil
	}
}

// evidenceJudgeNode compares surviving arguments by evidence weight.
// Kept as an exported helper; use BuildMultiHypothesisSubgraph's
// arbitrated judge path for new code.
func evidenceJudgeNode() PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		argA, _ := state["arg_a"].(Argument)
		argB, _ := state["arg_b"].(Argument)
		validA, _ := state["adv_a_valid"].(bool)
		validB, _ := state["adv_b_valid"].(bool)

		bb, _ := state["bb"].(*FactBlackboard)

		// Compute evidence scores.
		scoreA := computeEvidenceScore(argA, bb)
		scoreB := computeEvidenceScore(argB, bb)

		var verdict Verdict

		switch {
		case !validA && !validB:
			verdict = Verdict{
				Resolved:         false,
				UnresolvedReason: "双方论证均存在逻辑漏洞，需人工重新分析",
				Confidence:       0,
			}
		case !validA:
			verdict = Verdict{
				Resolved:          true,
				WinningHypothesis: argB.HypothesisID,
				Confidence:        scoreB,
				Rationale:         "正方论证逻辑不自洽，反方论证通过校验",
			}
		case !validB:
			verdict = Verdict{
				Resolved:          true,
				WinningHypothesis: argA.HypothesisID,
				Confidence:        scoreA,
				Rationale:         "反方论证逻辑不自洽，正方论证通过校验",
			}
		case math.Abs(scoreA-scoreB) < mhTieThreshold:
			verdict = Verdict{
				Resolved:         false,
				UnresolvedReason: fmt.Sprintf("双方证据强度势均力敌（差距 %.1f%%），需人工定夺", math.Abs(scoreA-scoreB)*100),
				Confidence:       math.Max(scoreA, scoreB),
				Rationale:        "双方各自提出了有依据的论证，证据强度相当",
				DissentNotes:     fmt.Sprintf("正方: %s | 反方: %s", argA.AcknowledgedCounterpoints, argB.AcknowledgedCounterpoints),
			}
		case scoreA > scoreB:
			verdict = Verdict{
				Resolved:          true,
				WinningHypothesis: argA.HypothesisID,
				Confidence:        scoreA,
				Rationale:         fmt.Sprintf("正方论证证据强度 %.2f 高于反方 %.2f", scoreA, scoreB),
				DissentNotes:      argB.AcknowledgedCounterpoints,
			}
		default:
			verdict = Verdict{
				Resolved:          true,
				WinningHypothesis: argB.HypothesisID,
				Confidence:        scoreB,
				Rationale:         fmt.Sprintf("反方论证证据强度 %.2f 高于正方 %.2f", scoreB, scoreA),
				DissentNotes:      argA.AcknowledgedCounterpoints,
			}
		}

		return PregelState{
			mhVerdict: verdict,
			"score_a": scoreA,
			"score_b": scoreB,
			"valid_a": validA,
			"valid_b": validB,
		}, nil
	}
}

// rejectionNode produces a human-escalation message for unresolved verdicts,
// or passes through cleanly for resolved verdicts so the graph advances.
func rejectionNode() PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		verdict, ok := state[mhVerdict].(Verdict)
		if ok && verdict.Resolved {
			// Resolved verdict — pass through, graph advances to next PlanStep.
			return state, nil
		}

		unresolvedReason := "无法自动裁决"
		if ok && verdict.UnresolvedReason != "" {
			unresolvedReason = verdict.UnresolvedReason
		}

		msg := fmt.Sprintf("> ⚠️ **需人工复核**：%s\n\n", unresolvedReason)
		if ok && verdict.DissentNotes != "" {
			msg += fmt.Sprintf("双方论证摘要：%s\n", verdict.DissentNotes)
		}
		msg += "请律师/代理人审查以上论证后做出专业判断。"

		return PregelState{
			"output":              msg,
			mhVerdict:             verdict,
			mhReject + "_message": msg,
		}, nil
	}
}

// evidenceRouter sends both resolved and unresolved verdicts to rejectName.
// The rejectionNode handles both cases — for resolved verdicts it's a pass-through,
// for unresolved it produces a human-escalation message.
// This ensures the multi_hypothesis subgraph terminates at rejectName, which
// CompilePlanToGraph connects to the next PlanStep via its static edge chain.
func evidenceRouter(rejectName string) PregelEdgeRouter {
	return func(ctx context.Context, state PregelState) []string {
		return []string{rejectName} // both paths converge → CompilePlanToGraph wires to next step
	}
}

// =============================================================================
// Helpers
// =============================================================================

// extractArgument builds an Argument from state keys for a given advocate prefix.
func extractArgument(state PregelState, prefix string) Argument {
	claim := state.GetString(prefix + "_claim")
	reasoning := state.GetString(prefix + "_output")
	counterpoints := state.GetString(prefix + "_counterpoints")

	// Fallback: construct a minimal argument if state is incomplete.
	if claim == "" {
		claim = prefix + " 论证"
	}
	if reasoning == "" {
		reasoning = claim + " — 论证过程"
	}

	return Argument{
		HypothesisID:              prefix,
		Claim:                     claim,
		Reasoning:                 reasoning,
		AcknowledgedCounterpoints: counterpoints,
		SupportingFacts:           stateGetStringSlice(state, prefix+"_facts"),
		SupportingRules:           stateGetStringSlice(state, prefix+"_rules"),
	}
}

// stateGetStringSlice retrieves a []string from PregelState by type assertion.
func stateGetStringSlice(state PregelState, key string) []string {
	if v, ok := state[key]; ok {
		if ss, ok := v.([]string); ok {
			return ss
		}
		// Also try []any (common in JSON deserialization).
		if si, ok := v.([]any); ok {
			ss := make([]string, 0, len(si))
			for _, e := range si {
				if s, ok := e.(string); ok {
					ss = append(ss, s)
				}
			}
			return ss
		}
	}
	return nil
}

// computeEvidenceScore calculates a weighted evidence score for an argument.
//
//	score = Σ(rule_authority × fact_confidence × chain_depth_weight)
//
// where chain_depth_weight = sqrt(len(facts)+len(rules)) / sqrt(maxChain).
func computeEvidenceScore(arg Argument, bb *FactBlackboard) float64 {
	if bb == nil {
		return 0.5
	}

	var totalScore float64
	count := 0

	// Score facts by confidence.
	for _, fid := range arg.SupportingFacts {
		if f, ok := bb.GetFact(fid); ok {
			totalScore += f.Confidence
			count++
		}
	}

	// Score rules by authority.
	for _, rid := range arg.SupportingRules {
		for _, rc := range bb.RuleConstraints() {
			if rc.ArticleID == rid {
				auth := authorityMap[rc.Requirement]
				totalScore += auth
				count++
				break
			}
		}
	}

	if count == 0 {
		return 0.3
	}

	rawScore := totalScore / float64(count)

	// Chain depth weight: longer chains have diminishing returns.
	chainLen := len(arg.SupportingFacts) + len(arg.SupportingRules)
	depthWeight := math.Sqrt(float64(chainLen)) / math.Sqrt(10.0) // normalized against maxChain=10
	if depthWeight > 1.0 {
		depthWeight = 1.0
	}

	return rawScore * depthWeight
}
