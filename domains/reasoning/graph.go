package reasoning

import (
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

// PregelNode is an alias of graph.PregelNode, keeping the domain API
// compatible with the concrete graph implementation.
type PregelNode = graph.PregelNode

// PregelState is an alias of graph.PregelState.
type PregelState = graph.PregelState

// PregelEdgeRouter is an alias of graph.PregelEdgeRouter.
type PregelEdgeRouter = graph.PregelEdgeRouter
