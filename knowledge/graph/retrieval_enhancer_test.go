package graph

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/retrieval"
)

func TestGraphEnhancer_EmptyStore(t *testing.T) {
	store := NewGraphStore()
	enhancer := NewGraphEnhancer(store, DefaultEnhanceConfig())

	seeds := []retrieval.ScoredChunk{
		{Chunk: retrieval.Chunk{ID: "c1", DocID: "doc1", Content: "专利分析"}, Score: 0.9},
	}
	result := enhancer.Enhance(seeds).(EnhancementResult)
	if len(result.Similar) != 0 || len(result.CitationChain) != 0 {
		t.Error("empty store should yield no expansion")
	}
	if result.Context == "" {
		t.Error("should still format seed chunks")
	}
}

func TestGraphEnhancer_WithExpansion(t *testing.T) {
	store := NewGraphStore()
	// Seed doc + similar doc + separate citation doc + law article.
	store.AddNode(&GraphNode{ID: "doc1", NodeType: NodeCase, Name: "案例A", AuthorityWeight: 0.8})
	store.AddNode(&GraphNode{ID: "doc2", NodeType: NodeCase, Name: "案例B", AuthorityWeight: 0.7})
	store.AddNode(&GraphNode{ID: "doc3", NodeType: NodeCase, Name: "案例C", AuthorityWeight: 0.75})
	lawID := lawNodeID("专利法第22条第3款")
	store.AddNode(&GraphNode{ID: lawID, NodeType: NodeLawArticle, Name: "专利法第22条第3款", AuthorityWeight: 1.0})
	// doc1 → doc2 (similar), doc1 → law (cites), doc3 → law (cites, same statute).
	store.AddEdge(GraphEdge{SourceID: "doc1", TargetID: "doc2", Relation: RelSimilarTo, Weight: 0.6})
	store.AddEdge(GraphEdge{SourceID: "doc1", TargetID: lawID, Relation: RelCites, Weight: 0.9})
	store.AddEdge(GraphEdge{SourceID: "doc3", TargetID: lawID, Relation: RelCites, Weight: 0.85})

	enhancer := NewGraphEnhancer(store, EnhanceConfig{MaxSimilar: 3, MaxCitationChain: 3, MinAuthority: 0.5})

	seeds := []retrieval.ScoredChunk{
		{Chunk: retrieval.Chunk{ID: "c1", DocID: "doc1", Content: "专利创造性分析"}, Score: 0.9},
	}
	result := enhancer.Enhance(seeds).(EnhancementResult)

	if len(result.Similar) == 0 {
		t.Error("expected similar nodes from SIMILAR_TO edge")
	}
	if len(result.CitationChain) == 0 {
		t.Error("expected citation chain nodes")
	}
	if result.Context == "" {
		t.Error("expected non-empty context")
	}
	// Context should mention graph expansion sections.
	if !strings.Contains(result.Context, "知识图谱扩展") {
		t.Error("expected graph expansion section in context")
	}
}

func TestGraphEnhancer_AuthorityFilter(t *testing.T) {
	store := NewGraphStore()
	store.AddNode(&GraphNode{ID: "doc1", NodeType: NodeCase, Name: "案例A", AuthorityWeight: 0.8})
	store.AddNode(&GraphNode{ID: "doc2", NodeType: NodeCase, Name: "低权威", AuthorityWeight: 0.3})
	store.AddEdge(GraphEdge{SourceID: "doc1", TargetID: "doc2", Relation: RelSimilarTo, Weight: 0.6})

	// MinAuthority 0.5 filters out doc2 (0.3).
	enhancer := NewGraphEnhancer(store, EnhanceConfig{MaxSimilar: 5, MaxCitationChain: 5, MinAuthority: 0.5})
	seeds := []retrieval.ScoredChunk{
		{Chunk: retrieval.Chunk{ID: "c1", DocID: "doc1", Content: "内容"}, Score: 0.9},
	}
	result := enhancer.Enhance(seeds).(EnhancementResult)
	if len(result.Similar) != 0 {
		t.Errorf("expected 0 similar after authority filter, got %d", len(result.Similar))
	}
}
