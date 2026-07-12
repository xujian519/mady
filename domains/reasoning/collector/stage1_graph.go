package collector

import (
	"context"
	"fmt"

	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/graph"
)

// Stage1State keys for Pregel state communication between collector nodes.
const (
	StateKeyInput        = "input"
	StateKeyBlackboard   = "bb"
	StateKeyCollectors   = "collectors"
	StateKeyMergeResults = "stage1_results"
)

// BuildStage1Graph constructs a Pregel graph that runs all collectors in
// parallel (same superstep) and merges their results into the blackboard.
//
// Graph structure:
//
//	collector_0 ──┐
//	collector_1 ──┼──→ merge ──→ __end__
//	collector_2 ──┘
//
// All collector nodes run concurrently in the first superstep.
// The merge node collects results and writes a summary to the blackboard.
func BuildStage1Graph(collectors []FactCollector) (*graph.PregelGraph, error) {
	if len(collectors) == 0 {
		return nil, fmt.Errorf("stage1: at least one collector is required")
	}

	g := graph.NewPregelGraph()

	for _, c := range collectors {
		cc := c // capture for closure
		name := string(cc.ID())
		if err := g.AddNode(name, buildCollectorNode(cc)); err != nil {
			return nil, fmt.Errorf("stage1: add node %s: %w", name, err)
		}
	}

	// Add merge node.
	if err := g.AddNode("stage1_merge", stage1MergeNode()); err != nil {
		return nil, fmt.Errorf("stage1: add merge node: %w", err)
	}

	// Connect all collectors → merge.
	for _, c := range collectors {
		if err := g.AddEdge(string(c.ID()), "stage1_merge"); err != nil {
			return nil, fmt.Errorf("stage1: edge %s→merge: %w", c.ID(), err)
		}
	}

	// Connect merge → end.
	if err := g.AddEdge("stage1_merge", graph.PregelEnd); err != nil {
		return nil, fmt.Errorf("stage1: edge merge→end: %w", err)
	}

	return g, nil
}

// BuildStage1Entry returns the first collector node name — the entry point.
func BuildStage1Entry(collectors []FactCollector) string {
	if len(collectors) == 0 {
		return ""
	}
	return string(collectors[0].ID())
}

// buildCollectorNode wraps a FactCollector as a PregelNode.
// It reads input from the PregelState and writes the CollectResult
// back under the collector's ID key.
func buildCollectorNode(c FactCollector) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		input := state.GetString(StateKeyInput)
		bb, ok := state[StateKeyBlackboard].(*reasoning.FactBlackboard)
		if !ok || bb == nil {
			return state, fmt.Errorf("stage1 collector %s: blackboard not in state", c.ID())
		}

		result, err := c.Collect(ctx, input, bb)
		if err != nil {
			return state, fmt.Errorf("stage1 collector %s: %w", c.ID(), err)
		}

		out := graph.PregelState{
			string(c.ID()): result,
		}
		return out, nil
	}
}

// stage1MergeNode aggregates all collector results and writes a summary
// to the blackboard's StageOutput.
func stage1MergeNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		bb, ok := state[StateKeyBlackboard].(*reasoning.FactBlackboard)
		if !ok || bb == nil {
			return state, nil
		}

		type mergeEntry struct {
			CollectorID string
			FactCount   int
			Confidence  float64
			Gaps        []string
		}

		var results []mergeEntry
		for _, cid := range []reasoning.FactCollectorID{
			reasoning.CollectorUserInput,
			reasoning.CollectorDocuments,
			reasoning.CollectorKnowledge,
			reasoning.CollectorDerived,
		} {
			if v, ok := state[string(cid)]; ok {
				if cr, ok := v.(*reasoning.CollectResult); ok {
					results = append(results, mergeEntry{
						CollectorID: string(cr.CollectorID),
						FactCount:   cr.FactCount,
						Confidence:  cr.Confidence,
						Gaps:        cr.Gaps,
					})
				}
			}
		}

		bb.SetStageOutput("stage1", results)
		return graph.PregelState{
			StateKeyMergeResults: results,
		}, nil
	}
}
