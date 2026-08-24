package graph

import (
	"sort"
)

// PathNode is a lightweight node reference inside a path result.
type PathNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	NodeType string `json:"node_type"`
}

// PathResult holds the outcome of a path search between two nodes.
type PathResult struct {
	Found       bool         `json:"found"`
	Paths       [][]string   `json:"paths"`
	PathDetails [][]PathNode `json:"path_details"`
}

// QueryOptions controls traversal behavior for graph queries.
type QueryOptions struct {
	// IncludeWeakEdges includes SIMILAR_TO / RELATED_TO co-occurrence edges.
	// Defaults to false because XiaoNuo patent_kg.db has ~83% weak edges that
	// create noisy traversal results.
	IncludeWeakEdges bool
}

// queryOpt resolves the effective traversal options. An empty opts falls back
// to the zero value (weak co-occurrence edges filtered out), the default.
func queryOpt(opts []QueryOptions) QueryOptions {
	if len(opts) > 0 {
		return opts[0]
	}
	return QueryOptions{}
}

// weakRelations holds edge types that represent statistical co-occurrence
// rather than explicit semantic relationships. These are filtered by default.
var weakRelations = map[string]bool{
	RelSimilarTo: true,
	RelRelatedTo: true,
}

func isWeakRelation(relation string) bool {
	return weakRelations[relation]
}

func filterWeakEdges(edges []GraphEdge, includeWeak bool) []GraphEdge {
	if includeWeak {
		return edges
	}
	filtered := edges[:0]
	for _, e := range edges {
		if !isWeakRelation(e.Relation) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// QueryPaths searches for up to 5 paths from sourceID to targetID using BFS,
// bounded by maxDepth hops. Returns a PathResult with Found=false when no
// path exists within the depth limit. Weak co-occurrence edges are filtered
// by default; pass QueryOptions{IncludeWeakEdges: true} to include them.
func QueryPaths(store *GraphStore, sourceID, targetID string, maxDepth int, opts ...QueryOptions) PathResult {
	opt := queryOpt(opts)
	if !store.HasNode(sourceID) || !store.HasNode(targetID) {
		return PathResult{}
	}
	if sourceID == targetID {
		n := store.GetNode(sourceID)
		details := []PathNode{}
		if n != nil {
			details = []PathNode{{ID: n.ID, Name: n.Name, NodeType: n.NodeType}}
		}
		return PathResult{
			Found:       true,
			Paths:       [][]string{{sourceID}},
			PathDetails: [][]PathNode{details},
		}
	}

	foundPaths := bfsPaths(store, sourceID, targetID, maxDepth, opt)
	if len(foundPaths) == 0 {
		return PathResult{}
	}
	return PathResult{Found: true, Paths: foundPaths, PathDetails: buildPathDetails(store, foundPaths)}
}

// bfsPaths searches breadth-first from sourceID toward targetID, returning all
// paths found within maxDepth hops (at most 5). Weak co-occurrence edges are
// filtered per opt.IncludeWeakEdges.
func bfsPaths(store *GraphStore, sourceID, targetID string, maxDepth int, opt QueryOptions) [][]string {
	type bfsEntry struct {
		id   string
		path []string
	}
	visited := map[string]bool{}
	queue := []bfsEntry{{id: sourceID, path: []string{sourceID}}}
	var foundPaths [][]string

	for len(queue) > 0 && len(foundPaths) < 5 {
		current := queue[0]
		queue = queue[1:]

		if len(current.path) > maxDepth+1 {
			continue
		}
		if current.id == targetID {
			foundPaths = append(foundPaths, current.path)
			continue
		}
		if visited[current.id] {
			continue
		}
		visited[current.id] = true

		for _, edge := range filterWeakEdges(store.GetOutgoing(current.id), opt.IncludeWeakEdges) {
			if !visited[edge.TargetID] && !containsString(current.path, edge.TargetID) {
				newPath := make([]string, len(current.path)+1)
				copy(newPath, current.path)
				newPath[len(current.path)] = edge.TargetID
				queue = append(queue, bfsEntry{id: edge.TargetID, path: newPath})
			}
		}
	}
	return foundPaths
}

// buildPathDetails batch-fetches node details for every node across all paths.
func buildPathDetails(store *GraphStore, foundPaths [][]string) [][]PathNode {
	idSet := map[string]bool{}
	for _, p := range foundPaths {
		for _, id := range p {
			idSet[id] = true
		}
	}
	nodeMap := make(map[string]*GraphNode, len(idSet))
	for id := range idSet {
		nodeMap[id] = store.GetNode(id)
	}

	var pathDetails [][]PathNode
	for _, p := range foundPaths {
		var details []PathNode
		for _, id := range p {
			if n := nodeMap[id]; n != nil {
				details = append(details, PathNode{
					ID:       n.ID,
					Name:     n.Name,
					NodeType: n.NodeType,
				})
			}
		}
		pathDetails = append(pathDetails, details)
	}
	return pathDetails
}

// QueryNeighbors returns all nodes reachable from nodeID within the given
// BFS depth (excluding the source itself). This supports multi-hop reasoning
// traversal over the graph. Weak co-occurrence edges are filtered by default.
func QueryNeighbors(store *GraphStore, nodeID string, depth int, opts ...QueryOptions) []*GraphNode {
	opt := queryOpt(opts)
	if depth < 1 || !store.HasNode(nodeID) {
		return nil
	}
	visited := map[string]bool{nodeID: true}
	frontier := []string{nodeID}
	var neighbors []*GraphNode

	for d := 0; d < depth; d++ {
		var nextFrontier []string
		for _, fid := range frontier {
			for _, edge := range filterWeakEdges(store.GetOutgoing(fid), opt.IncludeWeakEdges) {
				if !visited[edge.TargetID] {
					visited[edge.TargetID] = true
					nextFrontier = append(nextFrontier, edge.TargetID)
					if n := store.GetNode(edge.TargetID); n != nil {
						cp := *n
						neighbors = append(neighbors, &cp)
					}
				}
			}
		}
		frontier = nextFrontier
	}

	sort.Slice(neighbors, func(i, j int) bool {
		return neighbors[i].ID < neighbors[j].ID
	})
	return neighbors
}

// QueryByRelation returns all nodes connected to nodeID via a specific
// relation type, in the given direction ("outgoing", "incoming", or "both").
// When relation is empty, weak co-occurrence edges are filtered by default;
// pass QueryOptions{IncludeWeakEdges: true} to include them.
func QueryByRelation(store *GraphStore, nodeID, relation, direction string, opts ...QueryOptions) []*GraphNode {
	opt := queryOpt(opts)
	var edges []GraphEdge
	switch direction {
	case "incoming", "in":
		edges = store.GetIncoming(nodeID)
	default:
		edges = append(edges, store.GetOutgoing(nodeID)...)
		if direction == "both" || direction == "" {
			edges = append(edges, store.GetIncoming(nodeID)...)
		}
	}

	filterWeak := relation == "" && !opt.IncludeWeakEdges
	seen := map[string]bool{}
	var result []*GraphNode
	for _, e := range edges {
		if relation != "" && e.Relation != relation {
			continue
		}
		if filterWeak && isWeakRelation(e.Relation) {
			continue
		}
		var targetID string
		if e.SourceID == nodeID {
			targetID = e.TargetID
		} else {
			targetID = e.SourceID
		}
		if seen[targetID] {
			continue
		}
		seen[targetID] = true
		if n := store.GetNode(targetID); n != nil {
			result = append(result, n)
		}
	}
	return result
}

// QueryCitationChain returns documents that cite the given law reference,
// ordered by authority weight (descending). This is useful for finding the
// strongest precedents and related cases for a statute.
func QueryCitationChain(store *GraphStore, lawRef string) []*GraphNode {
	lawID := lawNodeID(lawRef)
	if lawID == "" || !store.HasNode(lawID) {
		return nil
	}
	citing := QueryByRelation(store, lawID, RelCites, "incoming")
	citing = append(citing, QueryByRelation(store, lawID, RelApplies, "outgoing")...)

	// Deduplicate and sort by authority weight descending.
	seen := map[string]bool{}
	var result []*GraphNode
	for _, n := range citing {
		if seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		result = append(result, n)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AuthorityWeight != result[j].AuthorityWeight {
			return result[i].AuthorityWeight > result[j].AuthorityWeight
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// QuerySimilar returns documents connected to nodeID via SIMILAR_TO edges
// (in either direction), sorted by edge weight.
func QuerySimilar(store *GraphStore, nodeID string) []*GraphNode {
	similar := QueryByRelation(store, nodeID, RelSimilarTo, "both")
	sort.Slice(similar, func(i, j int) bool {
		return similar[i].AuthorityWeight > similar[j].AuthorityWeight
	})
	return similar
}

func containsString(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}
