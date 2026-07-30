package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"runtime/debug"
	"sync"
	"time"
)

// PregelState is the shared mutable state passed between Pregel nodes.
type PregelState map[string]any

// Clone returns a deep copy of the PregelState.
func (s PregelState) Clone() PregelState {
	cp := make(PregelState, len(s))
	for k, v := range s {
		cp[k] = deepCopyValue(v)
	}
	return cp
}

// deepCopyValue recursively deep-copies a value that may contain nested slices
// and maps (the typical shape of PregelState values). It avoids the silent data
// loss of JSON round-tripping (e.g. channels, functions, int64 → float64).
func deepCopyValue(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[string]any:
		cp := make(map[string]any, len(val))
		for k, v := range val {
			cp[k] = deepCopyValue(v)
		}
		return cp
	case []any:
		cp := make([]any, len(val))
		for i, v := range val {
			cp[i] = deepCopyValue(v)
		}
		return cp
	}

	// Handle typed maps, slices, and structs via reflection.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		cp := reflect.MakeMap(rv.Type())
		iter := rv.MapRange()
		for iter.Next() {
			key := iter.Key()
			val := deepCopyValue(iter.Value().Interface())
			cp.SetMapIndex(key, reflect.ValueOf(val))
		}
		return cp.Interface()
	case reflect.Slice:
		cp := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Cap())
		for i := 0; i < rv.Len(); i++ {
			elem := deepCopyValue(rv.Index(i).Interface())
			cp.Index(i).Set(reflect.ValueOf(elem))
		}
		return cp.Interface()
	case reflect.Struct:
		cp := reflect.New(rv.Type()).Elem()
		for i := 0; i < rv.NumField(); i++ {
			if cp.Field(i).CanSet() {
				cp.Field(i).Set(reflect.ValueOf(deepCopyValue(rv.Field(i).Interface())))
			}
		}
		return cp.Interface()
	}

	// Immutable types (string, int, float, bool, struct without pointers)
	// are safe to share.
	return v
}

// GetString returns the string value for a key, or empty string if the key
// is missing or the value is not a string.
func (s PregelState) GetString(key string) string {
	v, _ := s[key].(string)
	return v
}

// GetMessages returns the slice value for a key, supporting both []any and
// typed slices (e.g. []agentcore.Message) via reflection. Returns nil when
// the key is missing or the value is not a slice.
func (s PregelState) GetMessages(key string) []any {
	raw, ok := s[key]
	if !ok {
		return nil
	}
	// Fast path: already []any.
	if msgs, ok := raw.([]any); ok {
		return msgs
	}
	// Reflection path: convert typed slice (e.g., []agentcore.Message) to []any.
	rv := reflect.ValueOf(raw)
	if rv.Kind() == reflect.Slice {
		result := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = rv.Index(i).Interface()
		}
		return result
	}
	return nil
}

// PregelEnd is the reserved terminal node sentinel value. Edges to PregelEnd
// cause the graph to return immediately without scheduling further nodes.
const PregelEnd = "__end__"

// PregelStep adapts a CompiledPregelGraph to the Step interface,
// enabling it to be used as a node in DAG graphs, Router branches, Pipeline
// steps, and as a Handoff delegate target.
//
// Input is placed in PregelState["input"] and output is read from
// PregelState["output"]. Domain sub-graphs should ensure their final
// node writes the result to state["output"].
type PregelStep struct {
	Graph *CompiledPregelGraph
}

// Run executes the PregelStep as a Step interface, reading the input from
// PregelState["input"] and writing the output to PregelState["output"].
func (ps *PregelStep) Run(ctx context.Context, input string) (string, error) {
	return ps.Graph.RunString(ctx, input)
}

var _ Step = (*PregelStep)(nil)

// PregelNode is a single processing step in a Pregel graph. It receives the
// current shared state and returns an updated state fragment that is merged
// back deterministically.
type PregelNode func(ctx context.Context, state PregelState) (PregelState, error)

// PregelEdgeRouter is a dynamic edge router that decides which nodes to
// activate next based on the current state. It returns a list of target
// node names.
type PregelEdgeRouter func(ctx context.Context, state PregelState) []string

// PregelGraph is a directed graph of PregelNode steps with support for
// static edges, conditional routing, node-level policies, and configurable
// state merge schemas. Use NewPregelGraph to create, then Compile to produce
// a CompiledPregelGraph for execution.
type PregelGraph struct {
	nodes            map[string]PregelNode
	edges            map[string][]string
	conditionalEdges map[string]PregelEdgeRouter
	nodePolicies     map[string]NodePolicy // 按节点名配置的运行时策略
	schema           *StateSchema          // 编译时传递到 CompiledPregelGraph
}

// NewPregelGraph creates a new empty Pregel graph with no nodes or edges.
func NewPregelGraph() *PregelGraph {
	return &PregelGraph{
		nodes:            make(map[string]PregelNode),
		edges:            make(map[string][]string),
		conditionalEdges: make(map[string]PregelEdgeRouter),
	}
}

// AddNode registers a named node in the graph. Names must not be PregelEnd.
func (pg *PregelGraph) AddNode(name string, node PregelNode) error {
	if name == PregelEnd {
		return fmt.Errorf("pregel: %q is a reserved name", PregelEnd)
	}
	if _, exists := pg.nodes[name]; exists {
		return fmt.Errorf("pregel: duplicate node %q", name)
	}
	pg.nodes[name] = node
	return nil
}

// AddEdge adds a static edge from one node to another. The target may be
// PregelEnd to signal termination.
func (pg *PregelGraph) AddEdge(from, to string) error {
	if _, ok := pg.nodes[from]; !ok {
		return fmt.Errorf("pregel: unknown source node %q", from)
	}
	if to != PregelEnd {
		if _, ok := pg.nodes[to]; !ok {
			return fmt.Errorf("pregel: unknown target node %q", to)
		}
	}
	pg.edges[from] = append(pg.edges[from], to)
	return nil
}

// SetConditionalEdge replaces the outgoing edges of a node with a dynamic
// router function that decides targets at runtime based on the current state.
func (pg *PregelGraph) SetConditionalEdge(from string, router PregelEdgeRouter) error {
	if _, ok := pg.nodes[from]; !ok {
		return fmt.Errorf("pregel: unknown source node %q", from)
	}
	pg.conditionalEdges[from] = router
	return nil
}

// SetSchema 配置状态合并策略。必须在 Compile 之前调用。
func (pg *PregelGraph) SetSchema(schema *StateSchema) {
	pg.schema = schema
}

// SetNodePolicy 为指定节点配置运行时策略（重试、超时、副作用标记）。
// 必须在 Compile 之前调用。
func (pg *PregelGraph) SetNodePolicy(name string, policy NodePolicy) error {
	if _, ok := pg.nodes[name]; !ok {
		return fmt.Errorf("pregel: unknown node %q", name)
	}
	if pg.nodePolicies == nil {
		pg.nodePolicies = make(map[string]NodePolicy)
	}
	pg.nodePolicies[name] = policy
	return nil
}

// CompiledPregelGraph is the immutable, ready-to-execute form of a PregelGraph.
// It is created by calling PregelGraph.Compile and contains a frozen snapshot
// of the node policies and schema. Run it repeatedly to execute the graph.
type CompiledPregelGraph struct {
	pg       *PregelGraph
	entry    string
	maxSteps int64
	// Schema 配置并行节点输出合并策略。nil 时使用确定性最后写入者胜出（按节点名排序）。
	Schema       *StateSchema
	nodePolicies map[string]NodePolicy // 编译时从 PregelGraph 复制
}

// Compile validates the graph and produces an immutable compiled instance
// ready for execution. The entryNode specifies the first node to execute.
// An optional maxSteps parameter limits the number of sequential steps
// (defaults to 100).
func (pg *PregelGraph) Compile(entryNode string, maxSteps ...int64) (*CompiledPregelGraph, error) {
	if _, ok := pg.nodes[entryNode]; !ok {
		return nil, fmt.Errorf("pregel: entry node %q not found", entryNode)
	}

	limit := int64(100)
	if len(maxSteps) > 0 && maxSteps[0] > 0 {
		limit = maxSteps[0]
	}

	// 编译时复制节点策略，避免后续修改 PregelGraph 影响已编译的图。
	policies := make(map[string]NodePolicy, len(pg.nodePolicies))
	for k, v := range pg.nodePolicies {
		policies[k] = v
	}

	return &CompiledPregelGraph{
		pg:           pg,
		entry:        entryNode,
		maxSteps:     limit,
		Schema:       pg.schema,
		nodePolicies: policies,
	}, nil
}

// Run executes the compiled Pregel graph starting from the initial state.
// Nodes at the same depth level are executed in parallel, and their outputs
// are merged using the configured StateSchema (or last-write-wins by
// alphabetical node name when Schema is nil).
func (cpg *CompiledPregelGraph) Run(ctx context.Context, initial PregelState) (PregelState, error) {
	state := initial.Clone()
	active := []string{cpg.entry}
	var steps int64

	for len(active) > 0 {
		steps++
		if steps > cpg.maxSteps {
			return state, fmt.Errorf("pregel: %w", ErrExceedMaxSteps)
		}

		var nextActive []string
		nextSet := make(map[string]bool)

		results := make(map[string]PregelState)
		errs := make(map[string]error)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, name := range active {
			node, ok := cpg.pg.nodes[name]
			if !ok {
				return state, fmt.Errorf("pregel: node %s not found", name)
			}

			wg.Add(1)
			go func(nodeName string, nodeFn PregelNode) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[PANIC] pregel: node %q panicked: %v\n%s", nodeName, r, debug.Stack())
					}
				}()
				snapshot := state.Clone()
				policy, hasPolicy := cpg.nodePolicies[nodeName]
				var policyPtr *NodePolicy
				if hasPolicy {
					policyPtr = &policy
				}
				out, err := executeWithPolicy(ctx, nodeName, nodeFn, snapshot, policyPtr)
				mu.Lock()
				results[nodeName] = out
				errs[nodeName] = err
				mu.Unlock()
			}(name, node)
		}

		wg.Wait()

		for name, err := range errs {
			if err != nil {
				return state, fmt.Errorf("pregel:%s: %w", name, err)
			}
		}

		// Merge results deterministically using the configured schema.
		// When Schema is nil, default last-write-wins (sorted by node name) applies.
		if err := mergeWithSchema(state, results, cpg.Schema); err != nil {
			return state, err
		}

		for _, name := range active {
			if staticTargets, ok := cpg.pg.edges[name]; ok {
				for _, t := range staticTargets {
					if t == PregelEnd {
						return state, nil
					}
					if !nextSet[t] {
						nextSet[t] = true
						nextActive = append(nextActive, t)
					}
				}
			}

			if router, ok := cpg.pg.conditionalEdges[name]; ok {
				targets := router(ctx, state)
				for _, t := range targets {
					if t == PregelEnd {
						return state, nil
					}
					if !nextSet[t] {
						nextSet[t] = true
						nextActive = append(nextActive, t)
					}
				}
			}
		}

		active = nextActive
	}

	return state, nil
}

// RunString is a convenience wrapper around Run that places the input in
// PregelState["input"] and extracts the result from PregelState["output"].
func (cpg *CompiledPregelGraph) RunString(ctx context.Context, input string) (string, error) {
	initial := PregelState{"input": input}
	final, err := cpg.Run(ctx, initial)
	if err != nil {
		return "", err
	}
	return final.GetString("output"), nil
}

// PregelCheckpointer adds checkpoint support to Pregel execution.
type PregelCheckpointer struct {
	graph *CompiledPregelGraph
	store CheckpointStore
}

// NewPregelCheckpointer creates a PregelCheckpointer backed by the given
// CheckpointStore.
func NewPregelCheckpointer(cpg *CompiledPregelGraph, store CheckpointStore) *PregelCheckpointer {
	return &PregelCheckpointer{graph: cpg, store: store}
}

// RunWithCheckpoints executes the Pregel graph with checkpointing. A checkpoint
// is saved before each node execution, allowing the graph to be resumed from
// the last saved state if interrupted.
func (pc *PregelCheckpointer) RunWithCheckpoints(ctx context.Context, initial PregelState, graphID string) (PregelState, error) {
	state := initial.Clone()
	active := []string{pc.graph.entry}
	return pc.runWithCheckpointsFrom(ctx, graphID, state, active, 0, "pregel_checkpointed")
}

// Resume continues execution from the latest checkpoint for the given graphID.
func (pc *PregelCheckpointer) Resume(ctx context.Context, graphID string) (PregelState, error) {
	cp, err := pc.store.LoadLatest(ctx, graphID)
	if err != nil {
		return nil, fmt.Errorf("pregel resume: %w", err)
	}
	if cp == nil {
		return nil, fmt.Errorf("pregel resume: no checkpoint found for graph %q", graphID)
	}

	var state PregelState
	if err := json.Unmarshal(cp.State, &state); err != nil {
		return nil, fmt.Errorf("pregel resume: unmarshal state: %w", err)
	}
	var active []string
	if rawNodes, ok := cp.Metadata["active_nodes"]; ok {
		switch nodes := rawNodes.(type) {
		case []string:
			active = nodes
		case []interface{}:
			for _, n := range nodes {
				if s, ok := n.(string); ok {
					active = append(active, s)
				}
			}
		}
	}
	if len(active) == 0 {
		return nil, fmt.Errorf("pregel resume: no active nodes in checkpoint %q", cp.ID)
	}
	return pc.resumeLoop(ctx, graphID, state, active, cp.StepIndex)
}

// resumeLoop continues the Pregel execution loop from a specific state.
// It delegates to runWithCheckpointsFrom to share the execution logic
// with RunWithCheckpoints, ensuring consistent node policy lookup and
// error handling.
func (pc *PregelCheckpointer) resumeLoop(ctx context.Context, graphID string, state PregelState, active []string, steps int64) (PregelState, error) {
	return pc.runWithCheckpointsFrom(ctx, graphID, state, active, steps, "pregel_resume")
}

// runWithCheckpointsFrom is the shared execution loop used by both
// RunWithCheckpoints and resumeLoop. It executes supersteps from the
// given active nodes, saving checkpoints before each step and looking
// up node policies from pc.graph.nodePolicies.
func (pc *PregelCheckpointer) runWithCheckpointsFrom(
	ctx context.Context,
	graphID string,
	state PregelState,
	active []string,
	steps int64,
	errPrefix string,
) (PregelState, error) {
	for len(active) > 0 {
		steps++
		if steps > pc.graph.maxSteps {
			return state, fmt.Errorf("%s: %w", errPrefix, ErrExceedMaxSteps)
		}

		stateBytes, err := json.Marshal(state)
		if err != nil {
			return state, fmt.Errorf("%s: marshal state: %w", errPrefix, err)
		}
		cp := Checkpoint{
			ID:        fmt.Sprintf("pregel_%s_step_%d", graphID, steps),
			GraphID:   graphID,
			StepIndex: steps,
			State:     stateBytes,
			Metadata:  map[string]any{"active_nodes": active},
			CreatedAt: time.Now(),
		}
		if err := pc.store.Save(ctx, cp); err != nil {
			return state, fmt.Errorf("%s: save checkpoint: %w", errPrefix, err)
		}

		var nextActive []string
		nextSet := make(map[string]bool)

		for _, name := range active {
			node, ok := pc.graph.pg.nodes[name]
			if !ok {
				return state, fmt.Errorf("%s: node %s not found", errPrefix, name)
			}
			snapshot := state.Clone()
			policy, hasPolicy := pc.graph.nodePolicies[name]
			var policyPtr *NodePolicy
			if hasPolicy {
				policyPtr = &policy
			}
			out, err := executeWithPolicy(ctx, name, node, snapshot, policyPtr)
			if err != nil {
				return state, fmt.Errorf("%s:%s: %w", errPrefix, name, err)
			}
			singleResult := map[string]PregelState{name: out}
			if err := mergeWithSchema(state, singleResult, pc.graph.Schema); err != nil {
				return state, fmt.Errorf("%s: merge %s: %w", errPrefix, name, err)
			}

			if targets, ok := pc.graph.pg.edges[name]; ok {
				for _, t := range targets {
					if t == PregelEnd {
						return state, nil
					}
					if !nextSet[t] {
						nextSet[t] = true
						nextActive = append(nextActive, t)
					}
				}
			}
			if router, ok := pc.graph.pg.conditionalEdges[name]; ok {
				for _, t := range router(ctx, state) {
					if t == PregelEnd {
						return state, nil
					}
					if !nextSet[t] {
						nextSet[t] = true
						nextActive = append(nextActive, t)
					}
				}
			}
		}

		active = nextActive
	}

	return state, nil
}
