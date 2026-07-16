package graph

// KgNode is a node in the knowledge graph.
type KgNode struct {
	ID       string `json:"id"`
	NodeType string `json:"node_type"`
	Name     string `json:"name"`
	Content  string `json:"content,omitempty"`
}

// KgEdge is a directed, weighted edge in the knowledge graph.
type KgEdge struct {
	TargetID string  `json:"target_id"`
	Relation string  `json:"relation"`
	Weight   float64 `json:"weight"`
}

// KgNodeDetail is a node together with its outgoing and incoming edges.
type KgNodeDetail struct {
	Node     KgNode   `json:"node"`
	Outgoing []KgEdge `json:"outgoing"`
	Incoming []KgEdge `json:"incoming"`
}

// KnowledgeGraphStore is the storage interface for multi-hop reasoning
// traversal. Concrete implementations may live in knowledge/graph or
// other packages.
type KnowledgeGraphStore interface {
	SearchNodes(keyword, nodeType string, limit int) ([]KgNode, error)
	GetNodeDetail(nodeID string) (*KgNodeDetail, error)
}
