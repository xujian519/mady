package graph

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// GraphStore is an in-memory knowledge graph backed by adjacency lists.
// It supports concurrent reads and exclusive writes via sync.RWMutex.
//
// Persistence is provided via JSON marshal/unmarshal to a file path, keeping
// zero external dependencies. The store scales comfortably to tens of
// thousands of nodes for patent/legal workloads.
type GraphStore struct {
	mu    sync.RWMutex
	nodes map[string]*GraphNode  // id → node
	adj   map[string][]GraphEdge // id → outgoing edges
	radj  map[string][]GraphEdge // id → incoming edges (reverse index)
}

// NewGraphStore creates an empty graph store.
func NewGraphStore() *GraphStore {
	return &GraphStore{
		nodes: make(map[string]*GraphNode),
		adj:   make(map[string][]GraphEdge),
		radj:  make(map[string][]GraphEdge),
	}
}

// AddNode inserts or replaces a node by ID.
func (s *GraphStore) AddNode(node *GraphNode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[node.ID] = node
	// Ensure adjacency entries exist so iterators see the node even with no edges.
	if _, ok := s.adj[node.ID]; !ok {
		s.adj[node.ID] = nil
	}
	if _, ok := s.radj[node.ID]; !ok {
		s.radj[node.ID] = nil
	}
}

// GetNode returns a copy of the node with the given ID, or nil if not found.
func (s *GraphStore) GetNode(id string) *GraphNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	if !ok {
		return nil
	}
	cp := *n
	return &cp
}

// HasNode reports whether a node with the given ID exists.
func (s *GraphStore) HasNode(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.nodes[id]
	return ok
}

// RemoveNode deletes a node and all edges connected to it.
func (s *GraphStore) RemoveNode(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nodes, id)
	delete(s.adj, id)
	delete(s.radj, id)
	// Clean up dangling edges referencing this node.
	for src, edges := range s.adj {
		filtered := edges[:0]
		for _, e := range edges {
			if e.TargetID != id {
				filtered = append(filtered, e)
			}
		}
		s.adj[src] = filtered
	}
	for dst, edges := range s.radj {
		filtered := edges[:0]
		for _, e := range edges {
			if e.SourceID != id {
				filtered = append(filtered, e)
			}
		}
		s.radj[dst] = filtered
	}
}

// AddEdge inserts a directed edge. Duplicate edges (same source+target+relation)
// are ignored. Both endpoint nodes must exist; otherwise the edge is dropped
// silently to keep the graph consistent.
func (s *GraphStore) AddEdge(edge GraphEdge) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[edge.SourceID]; !ok {
		return
	}
	if _, ok := s.nodes[edge.TargetID]; !ok {
		return
	}
	if edgeExists(s.adj[edge.SourceID], edge) {
		return
	}
	s.adj[edge.SourceID] = append(s.adj[edge.SourceID], edge)
	s.radj[edge.TargetID] = append(s.radj[edge.TargetID], edge)
}

// RemoveEdges deletes all edges matching the given source, target, and/or
// relation. A zero-value field acts as a wildcard (e.g. empty Relation removes
// all relations).
func (s *GraphStore) RemoveEdges(sourceID, targetID, relation string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adj[sourceID] = filterEdges(s.adj[sourceID], sourceID, targetID, relation, true)
	s.radj[targetID] = filterEdges(s.radj[targetID], sourceID, targetID, relation, false)
	// Also fix reverse indices for any other affected entries.
	for id, edges := range s.adj {
		s.adj[id] = filterEdges(edges, sourceID, targetID, relation, true)
	}
	for id, edges := range s.radj {
		s.radj[id] = filterEdges(edges, sourceID, targetID, relation, false)
	}
}

// GetOutgoing returns all outgoing edges of a node.
func (s *GraphStore) GetOutgoing(id string) []GraphEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]GraphEdge(nil), s.adj[id]...)
}

// GetIncoming returns all incoming edges of a node.
func (s *GraphStore) GetIncoming(id string) []GraphEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]GraphEdge(nil), s.radj[id]...)
}

// GetNodeDetail returns a node together with its outgoing and incoming edges.
func (s *GraphStore) GetNodeDetail(id string) *GraphNodeDetail {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	if !ok {
		return nil
	}
	cp := *n
	return &GraphNodeDetail{
		Node:     &cp,
		Outgoing: append([]GraphEdge(nil), s.adj[id]...),
		Incoming: append([]GraphEdge(nil), s.radj[id]...),
	}
}

// SearchGraphNodes performs a substring search over node names and content.
// If nodeType is non-empty, results are filtered to that type. Results are
// sorted by name and limited to the given count.
func (s *GraphStore) SearchGraphNodes(keyword, nodeType string, limit int) []*GraphNode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}
	kw := strings.ToLower(keyword)
	var results []*GraphNode
	for _, n := range s.nodes {
		if nodeType != "" && n.NodeType != nodeType {
			continue
		}
		if kw == "" || nodeMatchesKeyword(n, kw) {
			cp := *n
			results = append(results, &cp)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// AllNodes returns a snapshot of all nodes (sorted by ID).
func (s *GraphStore) AllNodes() []*GraphNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*GraphNode, 0, len(s.nodes))
	for _, n := range s.nodes {
		cp := *n
		result = append(result, &cp)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// NodeCount returns the total number of nodes.
func (s *GraphStore) NodeCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.nodes)
}

// EdgeCount returns the total number of edges.
func (s *GraphStore) EdgeCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, edges := range s.adj {
		count += len(edges)
	}
	return count
}

// NodeTypeCounts returns a map from NodeType to count.
func (s *GraphStore) NodeTypeCounts() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := make(map[string]int, len(s.nodes))
	for _, n := range s.nodes {
		counts[n.NodeType]++
	}
	return counts
}

// Stats returns a summary of the graph size.
func (s *GraphStore) Stats() GraphBuildResult {
	return GraphBuildResult{
		NodeCount: s.NodeCount(),
		EdgeCount: s.EdgeCount(),
	}
}

// --- JSON persistence ---

// graphSnapshot is the JSON serialization envelope.
type graphSnapshot struct {
	Nodes []*GraphNode `json:"nodes"`
	Edges []GraphEdge  `json:"edges"`
}

// SaveToFile serializes the entire graph to a JSON file.
func (s *GraphStore) SaveToFile(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := graphSnapshot{
		Nodes: make([]*GraphNode, 0, len(s.nodes)),
	}
	for _, n := range s.nodes {
		snap.Nodes = append(snap.Nodes, n)
	}
	for _, edges := range s.adj {
		snap.Edges = append(snap.Edges, edges...)
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("graph: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("graph: write %s: %w", path, err)
	}
	return nil
}

// LoadFromFile restores a graph from a JSON file produced by SaveToFile.
// It replaces any existing content in the store.
func (s *GraphStore) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("graph: read %s: %w", path, err)
	}
	var snap graphSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("graph: unmarshal: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes = make(map[string]*GraphNode, len(snap.Nodes))
	s.adj = make(map[string][]GraphEdge)
	s.radj = make(map[string][]GraphEdge)
	for _, n := range snap.Nodes {
		s.nodes[n.ID] = n
		s.adj[n.ID] = nil
		s.radj[n.ID] = nil
	}
	for _, e := range snap.Edges {
		if _, ok := s.nodes[e.SourceID]; !ok {
			continue
		}
		if _, ok := s.nodes[e.TargetID]; !ok {
			continue
		}
		s.adj[e.SourceID] = append(s.adj[e.SourceID], e)
		s.radj[e.TargetID] = append(s.radj[e.TargetID], e)
	}
	return nil
}

// --- internal helpers ---

func nodeMatchesKeyword(n *GraphNode, kw string) bool {
	if strings.Contains(strings.ToLower(n.Name), kw) {
		return true
	}
	if strings.Contains(strings.ToLower(n.Title), kw) {
		return true
	}
	if strings.Contains(strings.ToLower(n.Content), kw) {
		return true
	}
	return false
}

func edgeExists(edges []GraphEdge, candidate GraphEdge) bool {
	for _, e := range edges {
		if e.TargetID == candidate.TargetID && e.Relation == candidate.Relation {
			return true
		}
	}
	return false
}

// filterEdges removes edges matching the source/target/relation criteria.
// asSource=true filters the forward adjacency (match by TargetID); false
// filters the reverse adjacency (match by SourceID).
func filterEdges(edges []GraphEdge, sourceID, targetID, relation string, asSource bool) []GraphEdge {
	filtered := edges[:0]
	for _, e := range edges {
		match := true
		if sourceID != "" && e.SourceID != sourceID {
			match = false
		}
		if targetID != "" && e.TargetID != targetID {
			match = false
		}
		if relation != "" && e.Relation != relation {
			match = false
		}
		if !match {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
