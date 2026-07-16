package graph

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Step is a unit of work in a graph node. Any type implementing Run can be
// used as a graph node. agentcore.Step satisfies this interface.
type Step interface {
	Run(ctx context.Context, input string) (string, error)
}

// StreamStep is a streaming variant of Step. Nodes implementing this can
// process input and produce output as streams, enabling pipelined execution.
type StreamStep interface {
	RunStream(ctx context.Context, input any) (any, error)
}

// ErrExceedMaxSteps is returned when a graph exceeds the maximum step count.
var ErrExceedMaxSteps = fmt.Errorf("超出最大执行步数")

// StreamStepToStep adapts a StreamStep to Step by running it with the input as
// a string and collecting the stream output into a single result.
func StreamStepToStep(s StreamStep) Step {
	return &streamToStepAdapter{stream: s}
}

type streamToStepAdapter struct {
	stream StreamStep
}

func (a *streamToStepAdapter) Run(ctx context.Context, input string) (string, error) {
	out, err := a.stream.RunStream(ctx, input)
	if err != nil {
		return "", err
	}
	if s, ok := out.(string); ok {
		return s, nil
	}
	return fmt.Sprintf("%v", out), nil
}

// condRouter pairs a routing function with its valid target nodes.
type condRouter struct {
	route   func(ctx context.Context, output string) string
	targets []string
}

// Graph is a DAG-based execution engine. Nodes are named Steps connected
// by directed edges. The engine performs topological sorting at compile time
// and executes independent branches in parallel at runtime.
type Graph struct {
	nodes       map[string]Step
	streamNodes map[string]StreamStep
	edges       map[string][]string
	condEdges   map[string]condRouter // conditional edges: from → {route, targets}
}

func NewGraph() *Graph {
	return &Graph{
		nodes:       make(map[string]Step),
		streamNodes: make(map[string]StreamStep),
		edges:       make(map[string][]string),
		condEdges:   make(map[string]condRouter),
	}
}

func (g *Graph) AddNode(name string, step Step) error {
	if g.nodeExists(name) {
		return fmt.Errorf("graph: duplicate node %q", name)
	}
	g.nodes[name] = step
	return nil
}

// AddStreamNode adds a streaming node. The node processes input as a stream
// and produces output as a stream. The graph engine uses StreamStep semantics
// when executing in streaming mode (Runner.RunStream).
func (g *Graph) AddStreamNode(name string, step StreamStep) error {
	if g.nodeExists(name) {
		return fmt.Errorf("graph: duplicate node %q", name)
	}
	g.streamNodes[name] = step
	return nil
}

func (g *Graph) nodeExists(name string) bool {
	if _, ok := g.nodes[name]; ok {
		return true
	}
	if _, ok := g.streamNodes[name]; ok {
		return true
	}
	return false
}

func (g *Graph) AddEdge(from, to string) error {
	if !g.nodeExists(from) {
		return fmt.Errorf("graph: unknown source node %q", from)
	}
	if !g.nodeExists(to) {
		return fmt.Errorf("graph: unknown target node %q", to)
	}
	g.edges[from] = append(g.edges[from], to)
	return nil
}

func (g *Graph) AddConditionalEdge(from string, route func(ctx context.Context, output string) string, targets []string) error {
	if !g.nodeExists(from) {
		return fmt.Errorf("graph: unknown source node %q", from)
	}
	for _, t := range targets {
		if !g.nodeExists(t) {
			return fmt.Errorf("graph: unknown target node %q", t)
		}
	}
	g.condEdges[from] = condRouter{route: route, targets: targets}
	return nil
}

// CompileOptions configures graph compilation.
type CompileOptions struct {
	EntryNode string
	MaxSteps  int64
	// StateFn, if set, is called at the start of each Run to create a
	// GraphState that is shared across all nodes via context. Use
	// GetGraphState(ctx) inside node implementations to access it.
	// Nodes in the same layer execute concurrently; the state is
	// mutex-protected for safe concurrent reads and writes.
	StateFn GenStateFn
}

// CompiledGraph is a validated, ready-to-execute DAG.
type CompiledGraph struct {
	graph       *Graph
	Entry       string
	Sorted      [][]string
	MaxSteps    int64
	InDegree    map[string]int64
	RevEdges    map[string][]string
	StateFn     GenStateFn
	StreamNodes map[string]StreamStep
	CondEdges   map[string]condRouter // conditional routing from → {route, targets}
}

func (g *Graph) Compile(opts CompileOptions) (*CompiledGraph, error) {
	if opts.EntryNode == "" {
		return nil, fmt.Errorf("graph: EntryNode is required")
	}
	if !g.nodeExists(opts.EntryNode) {
		return nil, fmt.Errorf("graph: entry node %q not found", opts.EntryNode)
	}

	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 100
	}

	inDegree := make(map[string]int64)
	revEdges := make(map[string][]string)

	allNames := make(map[string]struct{}, len(g.nodes)+len(g.streamNodes))
	for name := range g.nodes {
		allNames[name] = struct{}{}
		inDegree[name] = 0
	}
	for name := range g.streamNodes {
		allNames[name] = struct{}{}
		inDegree[name] = 0
	}
	for from, tos := range g.edges {
		for _, to := range tos {
			inDegree[to]++
			revEdges[to] = append(revEdges[to], from)
		}
	}

	sorted, err := topoSort(allNames, g.edges, inDegree)
	if err != nil {
		return nil, err
	}

	streamNodes := make(map[string]StreamStep, len(g.streamNodes))
	for name, s := range g.streamNodes {
		streamNodes[name] = s
	}

	// Copy conditional edges to compiled graph.
	condEdges := make(map[string]condRouter, len(g.condEdges))
	for k, v := range g.condEdges {
		condEdges[k] = v
	}

	return &CompiledGraph{
		graph:       g,
		Entry:       opts.EntryNode,
		Sorted:      sorted,
		MaxSteps:    maxSteps,
		InDegree:    inDegree,
		RevEdges:    revEdges,
		StateFn:     opts.StateFn,
		StreamNodes: streamNodes,
		CondEdges:   condEdges,
	}, nil
}

func topoSort(nodeNames map[string]struct{}, edges map[string][]string, inDegreeOrig map[string]int64) ([][]string, error) {
	inDegree := make(map[string]int64)
	for k, v := range inDegreeOrig {
		inDegree[k] = v
	}

	var layers [][]string
	remaining := int64(len(nodeNames))

	for remaining > 0 {
		var layer []string
		for name := range inDegree {
			if inDegree[name] == 0 {
				layer = append(layer, name)
			}
		}
		if len(layer) == 0 {
			return nil, fmt.Errorf("graph: cycle detected — cannot topologically sort")
		}
		layers = append(layers, layer)
		for _, name := range layer {
			delete(inDegree, name)
			remaining--
			for _, to := range edges[name] {
				inDegree[to]--
			}
		}
	}
	return layers, nil
}

// Run executes the compiled graph.
func (cg *CompiledGraph) Run(ctx context.Context, input string) (string, error) {
	if cg.StateFn != nil {
		state := cg.StateFn(ctx)
		if state != nil {
			ctx = WithGraphState(ctx, state)
		}
	}
	outputs := make(map[string]string)
	outputs[cg.Entry] = ""

	var steps int64
	for _, layer := range cg.Sorted {
		var layerNodes []string
		for _, name := range layer {
			if _, reachable := outputs[name]; reachable || name == cg.Entry {
				layerNodes = append(layerNodes, name)
			}
		}
		if len(layerNodes) == 0 {
			continue
		}

		results := make(map[string]string)
		errs := make(map[string]error)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, name := range layerNodes {
			steps++
			if steps > cg.MaxSteps {
				return "", fmt.Errorf("graph: %w", ErrExceedMaxSteps)
			}

			nodeInput := input
			if preds, ok := cg.RevEdges[name]; ok && len(preds) > 0 {
				var parts []string
				for _, p := range preds {
					if out, exists := outputs[p]; exists {
						parts = append(parts, out)
					}
				}
				if len(parts) == 1 {
					nodeInput = parts[0]
				} else if len(parts) > 1 {
					nodeInput = JoinOutputs(parts)
				}
			}

			wg.Add(1)
			go func(nodeName, nodeIn string) {
				defer wg.Done()
				step := cg.getNode(nodeName)
				out, err := step.Run(ctx, nodeIn)
				mu.Lock()
				results[nodeName] = out
				errs[nodeName] = err
				mu.Unlock()
			}(name, nodeInput)
		}

		wg.Wait()

		for name, err := range errs {
			if err != nil {
				return "", fmt.Errorf("graph:%s: %w", name, err)
			}
		}
		for name, out := range results {
			// Conditional edges: auto-route to target in the same super-step.
			if router, ok := cg.CondEdges[name]; ok {
				target := router.route(ctx, out)
				routed := false
				for _, t := range router.targets {
					if t == target {
						step := cg.getNode(target)
						targetOut, err := step.Run(ctx, out)
						if err != nil {
							return "", fmt.Errorf("graph:%s: %w", target, err)
						}
						outputs[target] = targetOut
						for _, to := range cg.graph.edges[target] {
							outputs[to] = ""
						}
						routed = true
						break
					}
				}
				if routed {
					continue // target output replaces source output
				}
				// Route didn't match any target — fall through to store source output.
			}
			outputs[name] = out
			// Regular edges: mark targets as reachable for subsequent layers.
			for _, to := range cg.graph.edges[name] {
				outputs[to] = ""
			}
		}
	}

	return FindTerminalOutput(allNodes(cg.graph.nodes, cg.StreamNodes), cg.graph.edges, outputs), nil
}

var _ Step = (*CompiledGraph)(nil)

// allNodes merges Step and StreamStep node names into a single map for use
// by FindTerminalOutput (which only needs the names, not the values).
func allNodes(stepNodes map[string]Step, streamNodes map[string]StreamStep) map[string]Step {
	merged := make(map[string]Step, len(stepNodes)+len(streamNodes))
	for k, v := range stepNodes {
		merged[k] = v
	}
	for k := range streamNodes {
		merged[k] = nil // value unused, key presence is all that matters
	}
	return merged
}

// getNode returns the Step for a node name. StreamStep nodes are auto-adapted
// via StreamStepToStep so they work in the non-streaming execution path.
func (cg *CompiledGraph) getNode(name string) Step {
	if step, ok := cg.graph.nodes[name]; ok {
		return step
	}
	if ss, ok := cg.StreamNodes[name]; ok {
		return StreamStepToStep(ss)
	}
	return nil
}

// FindTerminalOutput finds the output of terminal nodes (nodes with no outgoing edges).
func FindTerminalOutput(nodes map[string]Step, edges map[string][]string, outputs map[string]string) string {
	terminal := make(map[string]bool)
	for name := range nodes {
		if _, hasEdges := edges[name]; !hasEdges || len(edges[name]) == 0 {
			terminal[name] = true
		}
	}
	var names []string
	for name := range terminal {
		if out, ok := outputs[name]; ok && out != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var parts []string
	for _, name := range names {
		parts = append(parts, outputs[name])
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return JoinOutputs(parts)
}

// JoinOutputs merges multiple outputs with a separator.
func JoinOutputs(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n---\n"
		}
		result += p
	}
	return result
}
