package reasoning

import (
	"context"
	"errors"
	"testing"
)

// failingGraphBuilder is a GraphBuilder that always fails AddNode.
type failingGraphBuilder struct{}

func (f *failingGraphBuilder) AddNode(name string, node PregelNode) error {
	return errors.New("intentional AddNode failure")
}

func (f *failingGraphBuilder) AddEdge(from, to string) error { return nil }

func (f *failingGraphBuilder) SetConditionalEdge(from string, router PregelEdgeRouter) error {
	return nil
}

func TestBuildMultiHypothesisSubgraph_PropagatesError(t *testing.T) {
	bb := &FactBlackboard{}
	step := PlanStep{Order: 1, Strategy: StrategyMultiHypothesis}
	builder := noopNodeBuilder{}

	_, _, err := BuildMultiHypothesisSubgraph(&failingGraphBuilder{}, step, bb, builder)
	if err == nil {
		t.Fatal("expected error from failing GraphBuilder, got nil")
	}
}

func TestPlanCompiler_CompilePlanToGraph_PropagatesError(t *testing.T) {
	plan := &Plan{
		Steps: []PlanStep{
			{Order: 1, Strategy: StrategyChain, Description: "step1"},
			{Order: 2, Strategy: StrategyChain, Description: "step2"},
		},
	}
	compiler := NewPlanCompiler(nil)
	_, _, err := compiler.CompilePlanToGraph(plan, &FactBlackboard{})
	if err != nil {
		t.Fatalf("unexpected error for valid plan: %v", err)
	}
}

func TestBuildChainStep_PropagatesError(t *testing.T) {
	compiler := NewPlanCompiler(nil)
	_, err := compiler.buildChainStep(&failingGraphBuilder{}, PlanStep{Order: 1}, &FactBlackboard{})
	if err == nil {
		t.Fatal("expected error from failing GraphBuilder, got nil")
	}
}

func TestBuildReActStep_PropagatesError(t *testing.T) {
	compiler := NewPlanCompiler(nil)
	_, _, err := compiler.buildReActStep(&failingGraphBuilder{}, PlanStep{Order: 1}, &FactBlackboard{})
	if err == nil {
		t.Fatal("expected error from failing GraphBuilder, got nil")
	}
}

// compileGraphBuilder is a minimal GraphBuilder that records nodes and edges for testing.
type compileGraphBuilder struct {
	nodes map[string]PregelNode
	edges map[string][]string
	conds map[string]PregelEdgeRouter
}

func newCompileGraphBuilder() *compileGraphBuilder {
	return &compileGraphBuilder{
		nodes: make(map[string]PregelNode),
		edges: make(map[string][]string),
		conds: make(map[string]PregelEdgeRouter),
	}
}

func (c *compileGraphBuilder) AddNode(name string, node PregelNode) error {
	if _, exists := c.nodes[name]; exists {
		return errors.New("duplicate node")
	}
	c.nodes[name] = node
	return nil
}

func (c *compileGraphBuilder) AddEdge(from, to string) error {
	c.edges[from] = append(c.edges[from], to)
	return nil
}

func (c *compileGraphBuilder) SetConditionalEdge(from string, router PregelEdgeRouter) error {
	c.conds[from] = router
	return nil
}

func TestBuildReActStep_CompilesAgainstInterface(t *testing.T) {
	compiler := NewPlanCompiler(nil)
	gb := newCompileGraphBuilder()
	entry, term, err := compiler.buildReActStep(gb, PlanStep{Order: 1}, &FactBlackboard{})
	if err != nil {
		t.Fatalf("buildReActStep: %v", err)
	}
	if entry == "" || term == "" {
		t.Fatalf("empty entry/terminal: %q / %q", entry, term)
	}
	if len(gb.nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(gb.nodes))
	}
	if len(gb.conds) != 1 {
		t.Fatalf("expected 1 conditional edge, got %d", len(gb.conds))
	}
}

func TestBuildMultiHypothesisSubgraph_CompilesAgainstInterface(t *testing.T) {
	gb := newCompileGraphBuilder()
	entry, term, err := BuildMultiHypothesisSubgraph(gb, PlanStep{Order: 1}, &FactBlackboard{}, noopNodeBuilder{})
	if err != nil {
		t.Fatalf("BuildMultiHypothesisSubgraph: %v", err)
	}
	if entry == "" || term == "" {
		t.Fatalf("empty entry/terminal: %q / %q", entry, term)
	}
	if len(gb.nodes) < 9 {
		t.Fatalf("expected at least 9 nodes, got %d", len(gb.nodes))
	}
}

// Ensure the noop builder returns a working node.
func TestNoopNodeBuilder_Run(t *testing.T) {
	builder := noopNodeBuilder{}
	node := builder.BuildChainNode(PlanStep{Order: 1, Strategy: StrategyChain, Description: "test"}, &FactBlackboard{})
	state, err := node(context.Background(), PregelState{"input": "hello"})
	if err != nil {
		t.Fatalf("noop node failed: %v", err)
	}
	if state["step_1_chain_output"] == nil {
		t.Fatal("expected output key in state")
	}
}
