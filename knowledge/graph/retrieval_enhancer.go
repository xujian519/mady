// Graph-enhanced retrieval — expands retrieval results using knowledge-graph
// relationships (similar cases, citation chains for shared statutes).
//
// The enhancer sits after the base retrieval step: given a set of seed
// ScoredChunks, it looks up the corresponding graph nodes and pulls in
// related documents (via SIMILAR_TO and shared CITES law-article nodes),
// then formats everything as a citation-backed context block.
//
// Dependency direction: knowledge/graph → retrieval (one-way). The retrieval
// package remains unaware of the graph; this enhancer is an opt-in layer that
// composes the two.
package graph

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xujian519/mady/retrieval"
)

// EnhanceConfig controls graph-enhanced retrieval behavior.
type EnhanceConfig struct {
	// MaxSimilar is the maximum number of similar-case nodes to add (default 3).
	MaxSimilar int
	// MaxCitationChain is the maximum number of citation-chain nodes to add
	// (documents citing the same law articles, default 3).
	MaxCitationChain int
	// MinAuthority filters out nodes below this authority weight (default 0.5).
	MinAuthority float64
}

// DefaultEnhanceConfig returns sensible defaults.
func DefaultEnhanceConfig() EnhanceConfig {
	return EnhanceConfig{
		MaxSimilar:       3,
		MaxCitationChain: 3,
		MinAuthority:     0.5,
	}
}

// GraphEnhancer expands retrieval results with graph-derived context.
type GraphEnhancer struct {
	store  *GraphStore
	config EnhanceConfig
}

// NewGraphEnhancer creates an enhancer bound to a graph store.
func NewGraphEnhancer(store *GraphStore, config EnhanceConfig) *GraphEnhancer {
	if config.MaxSimilar <= 0 {
		config = DefaultEnhanceConfig()
	}
	return &GraphEnhancer{store: store, config: config}
}

// EnhancementResult holds the expanded retrieval context.
type EnhancementResult struct {
	// Original chunks (unchanged).
	Seeds []retrieval.ScoredChunk
	// Additional similar-case nodes discovered via the graph.
	Similar []*GraphNode
	// Additional citation-chain nodes (documents citing shared statutes).
	CitationChain []*GraphNode
	// Formatted context block with citations, ready for injection.
	Context string
}

// GetContext returns the formatted enhancement context block, implementing
// the knowledge.GraphEnhancement interface for type-safe consumption across
// package boundaries without import cycles.
func (r EnhancementResult) GetContext() string { return r.Context }

// GetSeeds returns the original seed chunks.
func (r EnhancementResult) GetSeeds() []retrieval.ScoredChunk { return r.Seeds }

// Compile-time check: EnhancementResult satisfies GraphEnhancement.
var _ interface {
	GetContext() string
	GetSeeds() []retrieval.ScoredChunk
} = EnhancementResult{}

// Enhance expands the seed chunks using graph relationships.
//
// For each seed chunk, it resolves the corresponding graph node (by DocID),
// then pulls in similar cases and shared-statute citation chains. Results are
// deduplicated, filtered by authority, and formatted into a context block.
func (e *GraphEnhancer) Enhance(seeds []retrieval.ScoredChunk) any {
	result := EnhancementResult{Seeds: seeds}
	if e.store == nil || e.store.NodeCount() == 0 || len(seeds) == 0 {
		result.Context = e.formatSeedsOnly(seeds)
		return result
	}

	seen := map[string]bool{}
	var similar []*GraphNode
	var citations []*GraphNode

	for _, sc := range seeds {
		nodeID := sc.DocID
		if nodeID == "" || seen[nodeID] {
			continue
		}
		seen[nodeID] = true
		if !e.store.HasNode(nodeID) {
			continue
		}

		// Similar cases.
		for _, n := range QuerySimilar(e.store, nodeID) {
			if len(similar) >= e.config.MaxSimilar {
				break
			}
			if seen[n.ID] || n.AuthorityWeight < e.config.MinAuthority {
				continue
			}
			seen[n.ID] = true
			similar = append(similar, n)
		}

		// Citation chain: law articles cited by this node. Query directly by
		// relation (avoid QueryCitationChain which expects a raw law name and
		// would double-process the node ID).
		outgoing := e.store.GetOutgoing(nodeID)
		for _, edge := range outgoing {
			if edge.Relation != RelCites {
				continue
			}
			lawID := edge.TargetID
			citingNodes := QueryByRelation(e.store, lawID, RelCites, "incoming")
			for _, cn := range citingNodes {
				if len(citations) >= e.config.MaxCitationChain {
					break
				}
				if seen[cn.ID] || cn.AuthorityWeight < e.config.MinAuthority {
					continue
				}
				seen[cn.ID] = true
				citations = append(citations, cn)
			}
			if len(citations) >= e.config.MaxCitationChain {
				break
			}
		}
	}

	result.Similar = similar
	result.CitationChain = citations
	result.Context = e.formatEnhanced(seeds, similar, citations)
	return result
}

// formatSeedsOnly formats seed chunks without graph expansion.
func (e *GraphEnhancer) formatSeedsOnly(seeds []retrieval.ScoredChunk) string {
	items := retrieval.ScoredChunksToCitable(seeds)
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(retrieval.CitationPrefix(items))
	b.WriteString("\n\n")
	b.WriteString(retrieval.FormatCitations(items))
	return b.String()
}

// formatEnhanced formats the full enhancement result with citation chains.
func (e *GraphEnhancer) formatEnhanced(seeds []retrieval.ScoredChunk, similar, citations []*GraphNode) string {
	seedItems := retrieval.ScoredChunksToCitable(seeds)

	var b strings.Builder
	if len(seedItems) > 0 {
		b.WriteString(retrieval.CitationPrefix(seedItems))
		b.WriteString("\n\n")
		b.WriteString(retrieval.FormatCitations(seedItems))
	}

	// Graph-derived citation chain.
	if len(citations) > 0 {
		b.WriteString("\n\n--- 知识图谱扩展：法条引用链 ---\n\n")
		for i, n := range citations {
			b.WriteString(formatGraphNodeRef(i+1, n, "引用相同法条"))
		}
	}

	// Graph-derived similar cases.
	if len(similar) > 0 {
		b.WriteString("\n\n--- 知识图谱扩展：相似案例 ---\n\n")
		for i, n := range similar {
			b.WriteString(formatGraphNodeRef(i+1, n, "相似案例"))
		}
	}

	return b.String()
}

// formatGraphNodeRef renders a single graph node as a citation reference.
func formatGraphNodeRef(idx int, n *GraphNode, label string) string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(idx))
	b.WriteString(". ")
	b.WriteString(n.Name)
	if n.Title != "" && n.Title != n.Name {
		b.WriteString(" — ")
		b.WriteString(n.Title)
	}
	b.WriteString(" [")
	b.WriteString(label)
	b.WriteString(", 权威度: ")
	fmt.Fprintf(&b, "%.2f", n.AuthorityWeight)
	b.WriteString("]\n")
	return b.String()
}
