package graph

import (
	"github.com/xujian519/mady/domains/reasoning"
)

// ReasoningStoreAdapter wraps a GraphStore so it satisfies the
// reasoning.KnowledgeGraphStore interface used by the ReasoningWalker for
// multi-hop traversal. It translates between the graph's rich node/edge types
// and the walker's lightweight KgNode/KgEdge/KgNodeDetail types.
//
// Usage:
//
//	store := graph.NewGraphStore()
//	// ... build graph ...
//	var kg reasoning.KnowledgeGraphStore = graph.NewReasoningStoreAdapter(store)
//	walker := reasoning.NewReasoningWalker(kg, llmClient)
type ReasoningStoreAdapter struct {
	store *GraphStore
	cache *GraphCache // optional; nil disables caching
}

// NewReasoningStoreAdapter creates an adapter without caching.
func NewReasoningStoreAdapter(store *GraphStore) *ReasoningStoreAdapter {
	return &ReasoningStoreAdapter{store: store}
}

// NewReasoningStoreAdapterWithCache creates an adapter backed by a query cache.
func NewReasoningStoreAdapterWithCache(store *GraphStore, cache *GraphCache) *ReasoningStoreAdapter {
	return &ReasoningStoreAdapter{store: store, cache: cache}
}

// SearchNodes implements reasoning.KnowledgeGraphStore. It performs a
// substring search and converts results to reasoning.KgNode.
func (a *ReasoningStoreAdapter) SearchNodes(keyword, nodeType string, limit int) ([]reasoning.KgNode, error) {
	if a.cache != nil {
		if cached := a.cache.GetSearch(searchKey(keyword, nodeType, limit)); cached != nil {
			return nodesToReasoning(cached), nil
		}
	}
	results := a.store.SearchGraphNodes(keyword, nodeType, limit)
	if a.cache != nil {
		a.cache.PutSearch(searchKey(keyword, nodeType, limit), results)
	}
	return nodesToReasoning(results), nil
}

// GetNodeDetail implements reasoning.KnowledgeGraphStore. It returns the node
// with its outgoing and incoming edges.
func (a *ReasoningStoreAdapter) GetNodeDetail(nodeID string) (*reasoning.KgNodeDetail, error) {
	if a.cache != nil {
		if cached := a.cache.GetNodeDetail(nodeID); cached != nil {
			return detailToReasoning(cached), nil
		}
	}
	detail := a.store.GetNodeDetail(nodeID)
	if detail == nil {
		return nil, nil
	}
	if a.cache != nil {
		a.cache.PutNodeDetail(nodeID, detail)
	}
	return detailToReasoning(detail), nil
}

// nodesToReasoning converts graph nodes to the walker's lightweight type.
func nodesToReasoning(nodes []*GraphNode) []reasoning.KgNode {
	result := make([]reasoning.KgNode, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, reasoning.KgNode{
			ID:       n.ID,
			NodeType: n.NodeType,
			Name:     n.Name,
			Content:  n.Content,
		})
	}
	return result
}

// detailToReasoning converts a graph node detail to the walker's type.
func detailToReasoning(d *GraphNodeDetail) *reasoning.KgNodeDetail {
	if d == nil || d.Node == nil {
		return nil
	}
	outgoing := make([]reasoning.KgEdge, 0, len(d.Outgoing))
	for _, e := range d.Outgoing {
		outgoing = append(outgoing, reasoning.KgEdge{
			TargetID: e.TargetID,
			Relation: e.Relation,
			Weight:   e.Weight,
		})
	}
	incoming := make([]reasoning.KgEdge, 0, len(d.Incoming))
	for _, e := range d.Incoming {
		incoming = append(incoming, reasoning.KgEdge{
			TargetID: e.SourceID, // note: for incoming edges, the "target" from the walker's perspective is the source
			Relation: e.Relation,
			Weight:   e.Weight,
		})
	}
	return &reasoning.KgNodeDetail{
		Node: reasoning.KgNode{
			ID:       d.Node.ID,
			NodeType: d.Node.NodeType,
			Name:     d.Node.Name,
			Content:  d.Node.Content,
		},
		Outgoing: outgoing,
		Incoming: incoming,
	}
}

// Compile-time assertion that ReasoningStoreAdapter satisfies the interface.
var _ reasoning.KnowledgeGraphStore = (*ReasoningStoreAdapter)(nil)
