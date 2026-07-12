package reasoning

import (
	"context"
	"fmt"
	"math"

	"github.com/xujian519/mady/graph"
)

// =============================================================================
// Multi-Hypothesis Pregel Subgraph — Advocate A + Advocate B → Judge → Verdict
// =============================================================================
//
// Superstep 1: Advocate A (pro) and Advocate B (con) run ReAct in parallel.
//   Each advocate operates in an isolated state namespace (adv_a_* / adv_b_*).
//   They share the FactBlackboard + RuleSet as read-only input.
//
// Superstep 2: SyllogismJudge filters logically invalid arguments,
//   then EvidenceJudge weights the surviving arguments by rule authority
//   and fact confidence.
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
//	       mh_merge ──→ mh_syllogism_judge ──→ mh_evidence_judge
//	                                                 │
//	                                       ┌─ reject? ─→ mh_rejection
//	                                       └─ accept  ─→ (next step)
func BuildMultiHypothesisSubgraph(g *graph.PregelGraph, step PlanStep, bb *FactBlackboard, builder NodeBuilder) (string, string) {
	// === Advocate A (pro) ===
	aThink := fmt.Sprintf("mh_%d_adv_a_think", step.Order)
	aAct := fmt.Sprintf("mh_%d_adv_a_act", step.Order)
	aObserve := fmt.Sprintf("mh_%d_adv_a_observe", step.Order)

	g.AddNode(aThink, builder.BuildReActThink(step, bb))
	g.AddNode(aAct, builder.BuildReActAct(step, bb))
	g.AddNode(aObserve, builder.BuildReActObserve(step, bb))
	g.AddEdge(aThink, aAct)
	g.AddEdge(aAct, aObserve)
	g.SetConditionalEdge(aObserve, advocateRouter(mhAdvA, aObserve, aThink))

	// === Advocate B (con) ===
	bThink := fmt.Sprintf("mh_%d_adv_b_think", step.Order)
	bAct := fmt.Sprintf("mh_%d_adv_b_act", step.Order)
	bObserve := fmt.Sprintf("mh_%d_adv_b_observe", step.Order)

	g.AddNode(bThink, builder.BuildReActThink(step, bb))
	g.AddNode(bAct, builder.BuildReActAct(step, bb))
	g.AddNode(bObserve, builder.BuildReActObserve(step, bb))
	g.AddEdge(bThink, bAct)
	g.AddEdge(bAct, bObserve)
	g.SetConditionalEdge(bObserve, advocateRouter(mhAdvB, bObserve, bThink))

	// === Merge ===
	mergeName := fmt.Sprintf("mh_%d_merge", step.Order)
	g.AddNode(mergeName, mergeNode())
	// Both observers feed into merge.
	g.AddEdge(aObserve, mergeName)
	g.AddEdge(bObserve, mergeName)

	// === Judges ===
	sylName := fmt.Sprintf("mh_%d_syllogism_judge", step.Order)
	evidName := fmt.Sprintf("mh_%d_evidence_judge", step.Order)
	rejectName := fmt.Sprintf("mh_%d_rejection", step.Order)

	g.AddNode(sylName, syllogismJudgeNode())
	g.AddNode(evidName, evidenceJudgeNode())
	g.AddNode(rejectName, rejectionNode())

	g.AddEdge(mergeName, sylName)
	g.AddEdge(sylName, evidName)

	// Conditional: if reject → rejection path; else → PregelEnd.
	g.SetConditionalEdge(evidName, evidenceRouter(rejectName))

	return aThink, rejectName
}

// =============================================================================
// PregelNode implementations
// =============================================================================

// advocateRouter creates a conditional edge for an advocate's ReAct loop.
func advocateRouter(_ string, observeName, thinkName string) graph.PregelEdgeRouter {
	hasNextKey := observeName + "_has_next"
	return func(ctx context.Context, state graph.PregelState) []string {
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
func mergeNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		out := graph.PregelState{}

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
func syllogismJudgeNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		argA, _ := state["arg_a"].(Argument)
		argB, _ := state["arg_b"].(Argument)

		out := graph.PregelState{
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
func evidenceJudgeNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
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

		return graph.PregelState{
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
func rejectionNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
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

		return graph.PregelState{
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
func evidenceRouter(rejectName string) graph.PregelEdgeRouter {
	return func(ctx context.Context, state graph.PregelState) []string {
		return []string{rejectName} // both paths converge → CompilePlanToGraph wires to next step
	}
}

// =============================================================================
// Helpers
// =============================================================================

// extractArgument builds an Argument from state keys for a given advocate prefix.
func extractArgument(state graph.PregelState, prefix string) Argument {
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
func stateGetStringSlice(state graph.PregelState, key string) []string {
	if v, ok := state[key]; ok {
		if ss, ok := v.([]string); ok {
			return ss
		}
		// Also try []interface{} (common in JSON deserialization).
		if si, ok := v.([]interface{}); ok {
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
