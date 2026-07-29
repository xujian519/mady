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
//   - sub_agent: spawn a child Agent.Run for this step.
//
// NodeBuilder is the injection point for domain-specific node logic.
// Callers provide builders that know how to construct LLM-calling and
// tool-executing PregelNodes.
type PlanCompiler struct {
	builder       NodeBuilder
	spawnSubAgent func(ctx context.Context, prompt string) (string, error) // optional
}

// SubAgentFactory creates and runs a sub-agent for the given prompt.
type SubAgentFactory func(ctx context.Context, prompt string) (string, error)

// WithSubAgentFactory sets the factory for sub-agent step execution.
// When nil (default), sub_agent steps fall back to chain behavior.
func (c *PlanCompiler) WithSubAgentFactory(fn SubAgentFactory) *PlanCompiler {
	c.spawnSubAgent = fn
	return c
}

// NodeBuilder constructs PregelNodes for each PlanStep strategy.
// Implementations wire up the provider, tool registry, and blackboard.
type NodeBuilder interface {
	BuildChainNode(step PlanStep, bb *FactBlackboard) PregelNode

	BuildReActThink(step PlanStep, bb *FactBlackboard) PregelNode

	BuildReActAct(step PlanStep, bb *FactBlackboard) PregelNode

	BuildReActObserve(step PlanStep, bb *FactBlackboard) PregelNode

	BuildArbitratedJudgeNode(step PlanStep, bb *FactBlackboard, cfg *ArbitrationConfig) PregelNode

	// BuildSubAgentNode builds a node that spawns a sub-agent for this step.
	// The default implementation (noopNodeBuilder) falls back to chain-like
	// behavior. LLMNodeBuilder delegates to the PlanCompiler's SubAgentFactory.
	BuildSubAgentNode(step PlanStep, bb *FactBlackboard) PregelNode
}

var _ NodeBuilder = (*noopNodeBuilder)(nil)

// noopNodeBuilder returns pass-through nodes for testing.
type noopNodeBuilder struct{}

func (noopNodeBuilder) BuildChainNode(step PlanStep, bb *FactBlackboard) PregelNode {
	nodeID := fmt.Sprintf("step_%d_%s", step.Order, step.Strategy)
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		state[nodeID+"_output"] = step.Description + " — 完成"
		return state, nil
	}
}

func (noopNodeBuilder) BuildReActThink(step PlanStep, bb *FactBlackboard) PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		state["_noop_has_next"] = "true"
		return state, nil
	}
}

func (noopNodeBuilder) BuildReActAct(step PlanStep, bb *FactBlackboard) PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		return state, nil
	}
}

func (noopNodeBuilder) BuildReActObserve(step PlanStep, bb *FactBlackboard) PregelNode {
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		state["_noop_has_next"] = "false"
		return state, nil
	}
}

func (noopNodeBuilder) BuildArbitratedJudgeNode(step PlanStep, bb *FactBlackboard, cfg *ArbitrationConfig) PregelNode {
	stepID := fmt.Sprintf("step_%d_arbitrated_judge", step.Order)
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		state[stepID+"_output"] = step.Description + " — 仲裁: 使用确定性裁决（无 LLM）"
		state[mhVerdict] = Verdict{
			Resolved:         false,
			UnresolvedReason: "确定性仲裁（无 LLM）",
			Confidence:       0,
		}
		return state, nil
	}
}

func (noopNodeBuilder) BuildSubAgentNode(step PlanStep, bb *FactBlackboard) PregelNode {
	nodeID := fmt.Sprintf("step_%d_subagent", step.Order)
	return func(ctx context.Context, state PregelState) (PregelState, error) {
		state[nodeID+"_output"] = step.Description + " — 完成（noop sub-agent）"
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
// to its Strategy and DependsOn.
//
// When DependsOn is non-empty, edges are drawn from each dependency's
// terminal node to this step's entry node, creating a DAG. Steps without
// DependsOn start at the entry level. The PregelGraph execution engine
// runs independent branches in parallel.
//
// entryNode returns the name of the graph's entry node.
func (c *PlanCompiler) CompilePlanToGraph(plan *Plan, bb *FactBlackboard) (*graph.PregelGraph, string, error) {
	if plan == nil || len(plan.Steps) == 0 {
		return nil, "", fmt.Errorf("plan compiler: plan is nil or has no steps")
	}

	g := graph.NewPregelGraph()
	var entryName string

	type builtInfo struct {
		entry    string
		terminal string
	}
	built := make(map[string]builtInfo, len(plan.Steps))

	for i, step := range plan.Steps {
		stepID := step.ID
		if stepID == "" {
			stepID = fmt.Sprintf("step_%d", step.Order)
		}

		var stepEntry, stepTerminal string

		switch step.Strategy {
		case StrategySubAgent:
			entry, term, err := c.buildSubAgentStep(g, step, bb)
			if err != nil {
				return nil, "", fmt.Errorf("plan compiler: sub_agent step %d: %w", i, err)
			}
			stepEntry, stepTerminal = entry, term
		case StrategyChain:
			entry, term, err := c.buildChainStep(g, step, bb)
			if err != nil {
				return nil, "", fmt.Errorf("plan compiler: chain step %d: %w", i, err)
			}
			stepEntry, stepTerminal = entry, term
		case StrategyReact:
			entry, term, err := c.buildReActStep(g, step, bb)
			if err != nil {
				return nil, "", fmt.Errorf("plan compiler: react step %d: %w", i, err)
			}
			stepEntry, stepTerminal = entry, term
		case StrategyMultiHypothesis:
			entry, term, err := BuildMultiHypothesisSubgraph(g, step, bb, c.builder)
			if err != nil {
				return nil, "", fmt.Errorf("plan compiler: multi_hypothesis step %d: %w", i, err)
			}
			stepEntry, stepTerminal = entry, term
		default:
			entry, term, err := c.buildChainStep(g, step, bb)
			if err != nil {
				return nil, "", fmt.Errorf("plan compiler: fallback chain step %d: %w", i, err)
			}
			stepEntry, stepTerminal = entry, term
		}

		if i == 0 && entryName == "" {
			entryName = stepEntry
		}

		built[stepID] = builtInfo{entry: stepEntry, terminal: stepTerminal}
	}

	// Wire DAG edges based on DependsOn.
	for _, step := range plan.Steps {
		stepID := step.ID
		if stepID == "" {
			stepID = fmt.Sprintf("step_%d", step.Order)
		}
		info, ok := built[stepID]
		if !ok {
			continue
		}
		for _, depID := range step.DependsOn {
			depInfo, ok := built[depID]
			if !ok {
				continue
			}
			if err := g.AddEdge(depInfo.terminal, info.entry); err != nil {
				return nil, "", fmt.Errorf("plan compiler: connect %s→%s: %w", depID, stepID, err)
			}
		}
	}

	// Connect steps with no incoming edges to the entry node.
	hasIncoming := make(map[string]bool)
	for _, step := range plan.Steps {
		for _, depID := range step.DependsOn {
			if info, ok := built[depID]; ok {
				hasIncoming[info.entry] = true
			}
		}
	}

	// Steps without DependsOn: connect to entry if they are not the entry themselves.
	for _, step := range plan.Steps {
		if len(step.DependsOn) > 0 {
			continue
		}
		stepID := step.ID
		if stepID == "" {
			stepID = fmt.Sprintf("step_%d", step.Order)
		}
		info := built[stepID]
		if info.entry == entryName {
			continue
		}
		if hasIncoming[info.entry] {
			continue
		}
		// If this is the first step overall, it IS the entry already above.
		// Otherwise link the previous-sequential chain to the entry.
		_ = g.AddEdge(entryName, info.entry)
	}

	// Connect all terminal nodes without outgoing edges to PregelEnd.
	terminalSet := make(map[string]bool)
	for _, info := range built {
		terminalSet[info.terminal] = true
	}
	// Determine the maximum Order among plan steps, used below to
	// avoid adding PregelEnd to non-terminal parallel-out steps.
	maxOrder := 0
	for _, s := range plan.Steps {
		if s.Order > maxOrder {
			maxOrder = s.Order
		}
	}

	for _, step := range plan.Steps {
		stepID := step.ID
		if stepID == "" {
			stepID = fmt.Sprintf("step_%d", step.Order)
		}
		info, ok := built[stepID]
		if !ok {
			continue
		}
		// This terminal has a static edge if another step listed it as a dependency.
		hasOutgoing := false
		for _, step2 := range plan.Steps {
			for _, depID := range step2.DependsOn {
				if depID == stepID {
					hasOutgoing = true
					break
				}
			}
			if hasOutgoing {
				break
			}
		}
		if !hasOutgoing {
			// For steps without DependsOn (fan-out parallel), only the last step
			// by Order gets a PregelEnd edge. Otherwise the Pregel engine
			// terminates the whole graph immediately upon encountering PregelEnd
			// from the entry node, before sibling/parallel steps execute.
			if len(step.DependsOn) == 0 && step.Order < maxOrder {
				continue
			}
			_ = g.AddEdge(info.terminal, graph.PregelEnd)
		}
	}

	return g, entryName, nil
}

// buildSubAgentStep creates a single node that runs the step via a sub-agent.
// If no SubAgentFactory is configured, falls back to the NodeBuilder.
// Returns (entryNodeName, terminalNodeName, error).
func (c *PlanCompiler) buildSubAgentStep(g GraphBuilder, step PlanStep, bb *FactBlackboard) (string, string, error) {
	name := fmt.Sprintf("subagent_%d", step.Order)
	if c.spawnSubAgent != nil {
		fn := c.spawnSubAgent
		if err := g.AddNode(name, func(ctx context.Context, state PregelState) (PregelState, error) {
			result, err := fn(ctx, step.Description)
			if err != nil {
				state[name+"_error"] = err.Error()
				state[name+"_output"] = step.Description + " — 子 agent 执行失败"
			} else {
				state[name+"_output"] = result
			}
			return state, nil
		}); err != nil {
			return "", "", err
		}
	} else {
		if err := g.AddNode(name, c.builder.BuildSubAgentNode(step, bb)); err != nil {
			return "", "", fmt.Errorf("add subagent node: %w", err)
		}
	}
	return name, name, nil
}

// buildChainStep creates a single linear node for a chain-strategy step.
// Returns (entryNodeName, terminalNodeName, error).
func (c *PlanCompiler) buildChainStep(g GraphBuilder, step PlanStep, bb *FactBlackboard) (string, string, error) {
	name := fmt.Sprintf("chain_%d", step.Order)
	if err := g.AddNode(name, c.builder.BuildChainNode(step, bb)); err != nil {
		return "", "", fmt.Errorf("add chain node: %w", err)
	}
	return name, name, nil
}

// buildReActStep creates a think → act → observe cycle for a ReAct step.
// Returns (entryNodeName, terminalNodeName, error).
func (c *PlanCompiler) buildReActStep(g GraphBuilder, step PlanStep, bb *FactBlackboard) (string, string, error) {
	think := fmt.Sprintf("react_%d_think", step.Order)
	act := fmt.Sprintf("react_%d_act", step.Order)
	observe := fmt.Sprintf("react_%d_observe", step.Order)

	if err := g.AddNode(think, c.builder.BuildReActThink(step, bb)); err != nil {
		return "", "", fmt.Errorf("add react think: %w", err)
	}
	if err := g.AddNode(act, c.builder.BuildReActAct(step, bb)); err != nil {
		return "", "", fmt.Errorf("add react act: %w", err)
	}
	if err := g.AddNode(observe, c.builder.BuildReActObserve(step, bb)); err != nil {
		return "", "", fmt.Errorf("add react observe: %w", err)
	}

	if err := g.AddEdge(think, act); err != nil {
		return "", "", fmt.Errorf("connect think→act: %w", err)
	}
	if err := g.AddEdge(act, observe); err != nil {
		return "", "", fmt.Errorf("connect act→observe: %w", err)
	}

	hasNextKey := observe + "_has_next"
	if err := g.SetConditionalEdge(observe, func(ctx context.Context, state PregelState) []string {
		hn := state.GetString(hasNextKey)
		if hn == "" {
			hn = state.GetString("_noop_has_next")
		}
		if strings.EqualFold(hn, "true") {
			return []string{think}
		}
		return nil
	}); err != nil {
		return "", "", fmt.Errorf("set react conditional edge: %w", err)
	}

	return think, observe, nil
}
