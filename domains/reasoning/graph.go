package reasoning

import (
	"context"

	"github.com/xujian519/mady/graph"
)

// GraphBuilder is the minimal interface required by the reasoning domain to
// compile a Plan into a Pregel graph. It is intentionally small so that the
// domain layer does not depend on the concrete graph implementation.
//
// The concrete *graph.PregelGraph satisfies this interface at compile time.
type GraphBuilder interface {
	AddNode(name string, node PregelNode) error
	AddEdge(from, to string) error
	SetConditionalEdge(from string, router PregelEdgeRouter) error
}

// PregelNode, PregelState and PregelEdgeRouter are aliases of the
// infrastructure graph types. Aliases keep the domain API compatible with the
// concrete implementation while avoiding the need for domain-layer
// redefinitions.
type (
	PregelNode       = graph.PregelNode
	PregelState      = graph.PregelState
	PregelEdgeRouter = graph.PregelEdgeRouter
)

// GraphRunner is the minimal interface required to execute a compiled Pregel
// graph from the domain layer. *graph.CompiledPregelGraph satisfies it.
type GraphRunner interface {
	Run(ctx context.Context, initial PregelState) (PregelState, error)
}
