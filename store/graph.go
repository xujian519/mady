package store

import "context"

// GraphStore 是知识图谱存储的统一接口。
type GraphStore interface {
	// InsertNode 插入一个图谱节点。
	InsertNode(ctx context.Context, collection string, node GraphNode) error

	// InsertEdge 插入一条图谱边。
	InsertEdge(ctx context.Context, collection string, edge GraphEdge) error

	// QueryNeighbors 查询指定节点的邻居。
	// depth 指定遍历深度（1=直接邻居，2=邻居的邻居，以此类推）。
	QueryNeighbors(ctx context.Context, collection string, nodeID string, depth int) ([]GraphNode, []GraphEdge, error)

	// QueryByType 查询指定类型的所有节点。
	QueryByType(ctx context.Context, collection string, nodeType string) ([]GraphNode, error)
}

// GraphNode 表示图谱中的一个节点。
type GraphNode struct {
	ID         string
	Type       string
	Name       string
	Properties map[string]string
}

// GraphEdge 表示图谱中的一条边。
type GraphEdge struct {
	SourceID string
	TargetID string
	Relation string
	Weight   float64
	Evidence string
}
