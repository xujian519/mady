package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

type testStep struct {
	name string
	fn   func(ctx context.Context, input string) (string, error)
}

func (s *testStep) Run(ctx context.Context, input string) (string, error) {
	return s.fn(ctx, input)
}

func identityStep(name string) *testStep {
	return &testStep{name: name, fn: func(_ context.Context, input string) (string, error) {
		return input, nil
	}}
}

func constStep(name, output string) *testStep {
	return &testStep{name: name, fn: func(_ context.Context, _ string) (string, error) {
		return output, nil
	}}
}

func errStep(name, errMsg string) *testStep {
	return &testStep{name: name, fn: func(_ context.Context, _ string) (string, error) {
		return "", errors.New(errMsg)
	}}
}

func Example_graphDAG() {
	g := NewGraph()
	_ = g.AddNode("a", constStep("a", "hello"))
	_ = g.AddNode("b", constStep("b", "world"))
	_ = g.AddEdge("a", "b")

	cg, err := g.Compile(CompileOptions{EntryNode: "a"})
	if err != nil {
		fmt.Println("compile error:", err)
		return
	}
	out, err := cg.Run(context.Background(), "")
	if err != nil {
		fmt.Println("run error:", err)
		return
	}
	fmt.Println(out)
	// Output: world
}

func Example_pregel() {
	pg := NewPregelGraph()
	_ = pg.AddNode("step1", func(_ context.Context, s PregelState) (PregelState, error) {
		s["output"] = s.GetString("input") + " world"
		return s, nil
	})
	_ = pg.AddEdge("step1", PregelEnd)

	cpg, err := pg.Compile("step1")
	if err != nil {
		fmt.Println("compile error:", err)
		return
	}
	out, err := cpg.RunString(context.Background(), "hello")
	if err != nil {
		fmt.Println("run error:", err)
		return
	}
	fmt.Println(out)
	// Output: hello world
}

// ---- Graph Construction ----

func TestNewGraph(t *testing.T) {
	g := NewGraph()
	if g == nil {
		t.Fatal("expected non-nil")
	}
	if len(g.nodes) != 0 {
		t.Fatal("expected empty")
	}
}

func TestAddNode(t *testing.T) {
	g := NewGraph()
	if err := g.AddNode("a", identityStep("a")); err != nil {
		t.Fatal(err)
	}
	if len(g.nodes) != 1 {
		t.Fatalf("nodes = %d", len(g.nodes))
	}
}

func TestAddNode_Duplicate(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", identityStep("a"))
	err := g.AddNode("a", identityStep("a"))
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestAddEdge(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", identityStep("a"))
	g.AddNode("b", identityStep("b"))
	if err := g.AddEdge("a", "b"); err != nil {
		t.Fatal(err)
	}
}

func TestAddEdge_UnknownSource(t *testing.T) {
	g := NewGraph()
	err := g.AddEdge("unknown", "b")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAddEdge_UnknownTarget(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", identityStep("a"))
	err := g.AddEdge("a", "unknown")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---- Compile ----

func TestCompile_NoEntry(t *testing.T) {
	g := NewGraph()
	_, err := g.Compile(CompileOptions{})
	if err == nil {
		t.Fatal("expected error without entry node")
	}
}

func TestCompile_EntryNotFound(t *testing.T) {
	g := NewGraph()
	_, err := g.Compile(CompileOptions{EntryNode: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent entry")
	}
}

func TestCompile_Simple(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", identityStep("a"))
	cg, err := g.Compile(CompileOptions{EntryNode: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if cg.Entry != "a" {
		t.Fatalf("entry = %q", cg.Entry)
	}
	if len(cg.Sorted) == 0 {
		t.Fatal("expected sorted layers")
	}
}

// ---- TopoSort ----

func TestTopoSort_Simple(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", identityStep("a"))
	g.AddNode("b", identityStep("b"))
	g.AddEdge("a", "b")

	cg, err := g.Compile(CompileOptions{EntryNode: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cg.Sorted) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(cg.Sorted))
	}
}

func TestTopoSort_Cycle(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", identityStep("a"))
	g.AddNode("b", identityStep("b"))
	g.AddEdge("a", "b")
	g.AddEdge("b", "a")

	_, err := g.Compile(CompileOptions{EntryNode: "a"})
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

// ---- Run ----

func TestRun_SingleNode(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", constStep("a", "hello"))
	cg, err := g.Compile(CompileOptions{EntryNode: "a"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := cg.Run(context.Background(), "input")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello" {
		t.Fatalf("output = %q", out)
	}
}

func TestRun_Chain(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", constStep("a", "hello"))
	g.AddNode("b", &testStep{name: "b", fn: func(_ context.Context, input string) (string, error) {
		return input + " world", nil
	}})
	g.AddEdge("a", "b")

	cg, err := g.Compile(CompileOptions{EntryNode: "a"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sorted layers: %v", cg.Sorted)
	t.Logf("rev edges: %v", cg.RevEdges)
	out, err := cg.Run(context.Background(), "input")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello world" {
		t.Fatalf("output = %q", out)
	}
}

func TestRun_Parallel(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", constStep("a", "prefix"))
	g.AddNode("b", &testStep{name: "b", fn: func(_ context.Context, input string) (string, error) {
		return input + " hello", nil
	}})
	g.AddNode("c", &testStep{name: "c", fn: func(_ context.Context, input string) (string, error) {
		return input + " world", nil
	}})
	g.AddEdge("a", "b")
	g.AddEdge("a", "c")

	cg, err := g.Compile(CompileOptions{EntryNode: "a"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := cg.Run(context.Background(), "input")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello") || !strings.Contains(out, "world") {
		t.Fatalf("output = %q", out)
	}
}

func TestRun_Error(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", errStep("a", "oops"))
	cg, err := g.Compile(CompileOptions{EntryNode: "a"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cg.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_MaxSteps(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", identityStep("a"))
	g.AddNode("b", identityStep("b"))
	g.AddEdge("a", "b")

	cg, err := g.Compile(CompileOptions{EntryNode: "a", MaxSteps: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cg.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("expected max steps error")
	}
}

// ---- Conditional Edge ----

func TestConditionalEdge_AutoRouting(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", constStep("a", "route_to_b"))
	g.AddNode("b", constStep("b", "output_b"))
	g.AddNode("c", constStep("c", "output_c"))

	err := g.AddConditionalEdge("a", func(_ context.Context, output string) string {
		if output == "route_to_b" {
			return "b"
		}
		return "c"
	}, []string{"b", "c"})
	if err != nil {
		t.Fatal(err)
	}

	cg, err := g.Compile(CompileOptions{EntryNode: "a"})
	if err != nil {
		t.Fatal(err)
	}

	// Verify condEdges are stored (no virtual __conditional_ nodes).
	if _, ok := cg.CondEdges["a"]; !ok {
		t.Fatal("conditional edge for 'a' not found")
	}

	// Run: a routes to b, which auto-executes and produces "output_b".
	out, err := cg.Run(context.Background(), "input")
	if err != nil {
		t.Fatal(err)
	}
	if out != "output_b" {
		t.Fatalf("output = %q, want output_b", out)
	}
}

func TestConditionalEdge_NoMatch(t *testing.T) {
	g := NewGraph()
	g.AddNode("a", constStep("a", "unknown_target"))
	g.AddNode("b", constStep("b", "output"))

	g.AddConditionalEdge("a", func(_ context.Context, _ string) string {
		return "nonexistent"
	}, []string{"b"})

	cg, err := g.Compile(CompileOptions{EntryNode: "a"})
	if err != nil {
		t.Fatal(err)
	}
	// When the route returns a non-target, the node's own output is used.
	out, err := cg.Run(context.Background(), "input")
	if err != nil {
		t.Fatal(err)
	}
	// Node "a" output is "unknown_target" since no target matched.
	if out != "unknown_target" {
		t.Fatalf("output = %q, want unknown_target", out)
	}
}

func TestAddConditionalEdge_UnknownSource(t *testing.T) {
	g := NewGraph()
	err := g.AddConditionalEdge("unknown", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---- FindTerminalOutput ----

func TestFindTerminalOutput(t *testing.T) {
	nodes := map[string]Step{"a": identityStep("a"), "b": identityStep("b")}
	edges := map[string][]string{"a": {"b"}}
	outputs := map[string]string{"a": "out_a", "b": "out_b"}

	result := FindTerminalOutput(nodes, edges, outputs)
	if result != "out_b" {
		t.Fatalf("result = %q", result)
	}
}

func TestFindTerminalOutput_MultipleTerminals(t *testing.T) {
	nodes := map[string]Step{"a": identityStep("a"), "b": identityStep("b")}
	edges := map[string][]string{}
	outputs := map[string]string{"a": "out_a", "b": "out_b"}

	result := FindTerminalOutput(nodes, edges, outputs)
	if !strings.Contains(result, "out_a") || !strings.Contains(result, "out_b") {
		t.Fatalf("result = %q", result)
	}
}

// ---- JoinOutputs ----

func TestJoinOutputs(t *testing.T) {
	result := JoinOutputs([]string{"a", "b"})
	if result != "a\n---\nb" {
		t.Fatalf("result = %q", result)
	}
}

func TestJoinOutputs_Single(t *testing.T) {
	result := JoinOutputs([]string{"only"})
	if result != "only" {
		t.Fatalf("result = %q", result)
	}
}

func TestJoinOutputs_Empty(t *testing.T) {
	result := JoinOutputs(nil)
	if result != "" {
		t.Fatalf("result = %q", result)
	}
}

// ---- State Sharing ----

func TestGraphStateSharing(t *testing.T) {
	g := NewGraph()

	// Node A writes to shared state.
	_ = g.AddNode("a", &testStep{name: "a", fn: func(ctx context.Context, input string) (string, error) {
		gs := GetGraphState(ctx)
		if gs == nil {
			t.Fatal("expected GraphState in context")
		}
		gs.Set("user", "alice")
		return "from_a", nil
	}})

	// Node B reads from shared state (runs in same layer, in parallel).
	_ = g.AddNode("b", &testStep{name: "b", fn: func(ctx context.Context, input string) (string, error) {
		gs := GetGraphState(ctx)
		user := gs.GetString("user")
		if user == "" {
			return "no_user", nil
		}
		return "hello " + user, nil
	}})

	_ = g.AddEdge("a", "b")

	cg, err := g.Compile(CompileOptions{
		EntryNode: "a",
		StateFn:   func(_ context.Context) *GraphState { return NewGraphState() },
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := cg.Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello alice" {
		t.Fatalf("expected 'hello alice', got %q", out)
	}
}

func TestGraphStateSharingNoStateFn(t *testing.T) {
	g := NewGraph()

	_ = g.AddNode("a", &testStep{name: "a", fn: func(ctx context.Context, input string) (string, error) {
		gs := GetGraphState(ctx)
		if gs != nil {
			t.Fatal("expected nil GraphState when StateFn is not set")
		}
		return "ok", nil
	}})

	cg, err := g.Compile(CompileOptions{EntryNode: "a"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := cg.Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Fatalf("expected 'ok', got %q", out)
	}
}

func TestGraphStateConcurrentAccess(t *testing.T) {
	g := NewGraph()

	_ = g.AddNode("entry", constStep("entry", "go"))
	_ = g.AddNode("collector", &testStep{name: "collector", fn: func(ctx context.Context, input string) (string, error) {
		gs := GetGraphState(ctx)
		for i := 0; i < 10; i++ {
			v := gs.Get(fmt.Sprintf("key_%d", i))
			if v == nil {
				t.Fatalf("missing key_%d from shared state", i)
			}
		}
		return "all_good", nil
	}})

	// 10 parallel writer nodes, all reachable from entry.
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("writer_%d", i)
		i := i
		_ = g.AddNode(name, &testStep{name: name, fn: func(ctx context.Context, input string) (string, error) {
			gs := GetGraphState(ctx)
			gs.Set(fmt.Sprintf("key_%d", i), fmt.Sprintf("val_%d", i))
			return "ok", nil
		}})
		_ = g.AddEdge("entry", name)
		_ = g.AddEdge(name, "collector")
	}

	cg, err := g.Compile(CompileOptions{
		EntryNode: "entry",
		StateFn:   func(_ context.Context) *GraphState { return NewGraphState() },
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := cg.Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if out != "all_good" {
		t.Fatalf("expected 'all_good', got %q", out)
	}
}

func TestGraphStateGetMessages(t *testing.T) {
	gs := NewGraphState()
	msgs := []any{agentcore.Message{Role: agentcore.RoleUser, Content: "hello"}}
	gs.SetMessages("history", msgs)

	got := gs.GetMessages("history")
	if len(got) != 1 {
		t.Fatal("unexpected length from GraphState")
	}
	if msg, ok := got[0].(agentcore.Message); !ok || msg.Content != "hello" {
		t.Fatal("unexpected messages from GraphState")
	}

	if gs.GetMessages("nonexistent") != nil {
		t.Fatal("expected nil for missing key")
	}
}

func TestDAGStateSharing(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode("a", &testStep{name: "a", fn: func(ctx context.Context, input string) (string, error) {
		GetGraphState(ctx).Set("key", "val")
		return "from_a", nil
	}})
	_ = g.AddNode("b", &testStep{name: "b", fn: func(ctx context.Context, input string) (string, error) {
		return GetGraphState(ctx).GetString("key"), nil
	}})
	_ = g.AddEdge("a", "b")

	cg, err := g.Compile(CompileOptions{
		EntryNode: "a",
		StateFn:   func(_ context.Context) *GraphState { return NewGraphState() },
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := cg.Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if out != "val" {
		t.Fatalf("expected 'val', got %q", out)
	}
}

func TestDAGStateFnOverride(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode("a", &testStep{name: "a", fn: func(ctx context.Context, input string) (string, error) {
		gs := GetGraphState(ctx)
		if gs == nil {
			return "no_state", nil
		}
		return gs.GetString("x"), nil
	}})

	// Set StateFn directly in CompileOptions.
	cg, err := g.Compile(CompileOptions{
		EntryNode: "a",
		StateFn: func(_ context.Context) *GraphState {
			s := NewGraphState()
			s.Set("x", "overridden")
			return s
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := cg.Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if out != "overridden" {
		t.Fatalf("expected 'overridden', got %q", out)
	}
}

// ---- Interface ----

func TestGraphCompiledGraphImplementsStep(t *testing.T) {
	var _ Step = (*CompiledGraph)(nil)
}

// ---- NodeError ----

func TestNodeError(t *testing.T) {
	err := agentcore.NewNodeError("test error", nil, "test_node", "test")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
}
