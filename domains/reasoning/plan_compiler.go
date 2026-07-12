package reasoning

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
)

// PlanCompiler compiles a Plan (Stage ③ output) into an executable
// PregelGraph (Stage ④ runtime).
//
// Three strategies are supported:
//   - chain: linear single-node step.
//   - react: think → act → observe cycle with conditional back-edge.
//   - multi_hypothesis: dual-advocate + judge subgraph (Phase 4).
//
// NodeBuilder is the injection point for domain-specific node logic.
// Callers provide builders that know how to construct LLM-calling and
// tool-executing PregelNodes.
type PlanCompiler struct {
	builder NodeBuilder
}

// NodeBuilder constructs PregelNodes for each PlanStep strategy.
// Implementations wire up the provider, tool registry, and blackboard.
type NodeBuilder interface {
	// BuildChainNode returns a PregelNode that runs a single chain step.
	BuildChainNode(step PlanStep, bb *FactBlackboard) graph.PregelNode

	// BuildReActThink returns the "think" node for a ReAct step.
	BuildReActThink(step PlanStep, bb *FactBlackboard) graph.PregelNode

	// BuildReActAct returns the "act" (tool-calling) node for a ReAct step.
	BuildReActAct(step PlanStep, bb *FactBlackboard) graph.PregelNode

	// BuildReActObserve returns the "observe" (process result) node.
	BuildReActObserve(step PlanStep, bb *FactBlackboard) graph.PregelNode
}

// noopNodeBuilder returns pass-through nodes for testing.
type noopNodeBuilder struct{}

func (noopNodeBuilder) BuildChainNode(step PlanStep, bb *FactBlackboard) graph.PregelNode {
	nodeID := fmt.Sprintf("step_%d_%s", step.Order, step.Strategy)
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		state[nodeID+"_output"] = step.Description + " — 完成"
		return state, nil
	}
}

func (noopNodeBuilder) BuildReActThink(step PlanStep, bb *FactBlackboard) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		state["_noop_has_next"] = "true" // first iteration: always proceed
		return state, nil
	}
}

func (noopNodeBuilder) BuildReActAct(step PlanStep, bb *FactBlackboard) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		return state, nil
	}
}

func (noopNodeBuilder) BuildReActObserve(step PlanStep, bb *FactBlackboard) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		state["_noop_has_next"] = "false" // generic key that all routers check as fallback
		return state, nil
	}
}

// NewPlanCompiler creates a PlanCompiler with the given NodeBuilder.
// Pass nil to use the no-op builder (for testing).
func NewPlanCompiler(builder NodeBuilder) *PlanCompiler {
	if builder == nil {
		builder = noopNodeBuilder{}
	}
	return &PlanCompiler{builder: builder}
}

// CompilePlanToGraph converts a Plan into a CompiledPregelGraph.
// Each PlanStep becomes one or more Pregel nodes, connected according
// to its Strategy.
//
// entryNode returns the name of the graph's entry node. The caller
// should pass this to PregelGraph.Compile().
func (c *PlanCompiler) CompilePlanToGraph(plan *Plan, bb *FactBlackboard) (*graph.PregelGraph, string, error) {
	if plan == nil || len(plan.Steps) == 0 {
		return nil, "", fmt.Errorf("plan compiler: plan is nil or has no steps")
	}

	g := graph.NewPregelGraph()
	var entryName string
	var prevTerminal string // last node in the previous step's subgraph

	for i, step := range plan.Steps {
		var terminal string

		switch step.Strategy {
		case StrategyChain:
			name := c.buildChainStep(g, step, bb)
			if i == 0 {
				entryName = name
			}
			terminal = name
		case StrategyReact:
			entry, term := c.buildReActStep(g, step, bb)
			if i == 0 {
				entryName = entry
			}
			terminal = term
		case StrategyMultiHypothesis:
			entry, term := BuildMultiHypothesisSubgraph(g, step, bb, c.builder)
			if i == 0 {
				entryName = entry
			}
			terminal = term
		default:
			// Fallback: treat unknown strategies as chain.
			name := c.buildChainStep(g, step, bb)
			if i == 0 {
				entryName = name
			}
			terminal = name
		}

		if i > 0 && prevTerminal != "" {
			if err := g.AddEdge(prevTerminal, terminal); err != nil {
				return nil, "", fmt.Errorf("plan compiler: connect step %d→%d: %w", i, i+1, err)
			}
		}

		prevTerminal = terminal
	}

	// Connect final terminal to PregelEnd.
	if prevTerminal != "" {
		if err := g.AddEdge(prevTerminal, graph.PregelEnd); err != nil {
			return nil, "", fmt.Errorf("plan compiler: connect final step to end: %w", err)
		}
	}

	return g, entryName, nil
}

// buildChainStep creates a single linear node for a chain-strategy step.
func (c *PlanCompiler) buildChainStep(g *graph.PregelGraph, step PlanStep, bb *FactBlackboard) string {
	name := fmt.Sprintf("chain_%d", step.Order)
	g.AddNode(name, c.builder.BuildChainNode(step, bb))
	return name
}

// buildReActStep creates a think → act → observe cycle for a ReAct step.
// Returns (entryNodeName, terminalNodeName).
//
// The observe node has a conditional edge back to think, controlled by
// the state key "<prefix>_has_next". When the observer signals "false",
// the conditional router returns nil → the step advances to the next via
// the static edges.
func (c *PlanCompiler) buildReActStep(g *graph.PregelGraph, step PlanStep, bb *FactBlackboard) (string, string) {
	think := fmt.Sprintf("react_%d_think", step.Order)
	act := fmt.Sprintf("react_%d_act", step.Order)
	observe := fmt.Sprintf("react_%d_observe", step.Order)

	g.AddNode(think, c.builder.BuildReActThink(step, bb))
	g.AddNode(act, c.builder.BuildReActAct(step, bb))
	g.AddNode(observe, c.builder.BuildReActObserve(step, bb))

	g.AddEdge(think, act)
	g.AddEdge(act, observe)

	// Conditional edge: if has_next, loop back to think.
	hasNextKey := observe + "_has_next"
	_ = g.SetConditionalEdge(observe, func(ctx context.Context, state graph.PregelState) []string {
		hn := state.GetString(hasNextKey)
		if hn == "" {
			hn = state.GetString("_noop_has_next") // fallback for test/noop builder
		}
		if strings.EqualFold(hn, "true") {
			return []string{think}
		}
		return nil // no more iterations → rely on static edges to advance
	})

	return think, observe
}
