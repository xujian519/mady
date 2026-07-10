package graph

import (
	"path/filepath"
	"testing"
	"time"
)

// testStore builds a small graph for testing:
//
//	doc1 (Case) ──CITES──→ law:专利法第22条 ←──CITES── doc2 (Case)
//	doc1 ←──APPLIES── law:专利法第22条
//	doc1 ──SIMILAR_TO──→ doc3
func testStore() *GraphStore {
	s := NewGraphStore()
	s.AddNode(&GraphNode{ID: "doc1", NodeType: NodeCase, Name: "案件A", Content: "专利侵权分析"})
	s.AddNode(&GraphNode{ID: "doc2", NodeType: NodeCase, Name: "案件B", Content: "专利无效宣告"})
	s.AddNode(&GraphNode{ID: "doc3", NodeType: NodeCase, Name: "案件C", Content: "专利侵权"})
	s.AddNode(&GraphNode{ID: "law:专利法第22条", NodeType: NodeLawArticle, Name: "专利法第22条"})
	s.AddEdge(GraphEdge{SourceID: "doc1", TargetID: "law:专利法第22条", Relation: RelCites, Weight: 0.9})
	s.AddEdge(GraphEdge{SourceID: "doc2", TargetID: "law:专利法第22条", Relation: RelCites, Weight: 0.9})
	s.AddEdge(GraphEdge{SourceID: "law:专利法第22条", TargetID: "doc1", Relation: RelApplies, Weight: 0.85})
	s.AddEdge(GraphEdge{SourceID: "doc1", TargetID: "doc3", Relation: RelSimilarTo, Weight: 0.6})
	return s
}

func TestGraphStore_AddGetNode(t *testing.T) {
	s := NewGraphStore()
	s.AddNode(&GraphNode{ID: "n1", NodeType: NodeConcept, Name: "概念"})
	if n := s.GetNode("n1"); n == nil || n.Name != "概念" {
		t.Fatalf("expected node 概念, got %v", n)
	}
	if s.GetNode("missing") != nil {
		t.Fatal("expected nil for missing node")
	}
	if !s.HasNode("n1") {
		t.Fatal("HasNode should be true")
	}
}

func TestGraphStore_AddEdgeDuplicate(t *testing.T) {
	s := NewGraphStore()
	s.AddNode(&GraphNode{ID: "a"})
	s.AddNode(&GraphNode{ID: "b"})
	e1 := GraphEdge{SourceID: "a", TargetID: "b", Relation: RelCites, Weight: 0.9}
	e2 := GraphEdge{SourceID: "a", TargetID: "b", Relation: RelCites, Weight: 0.5}
	s.AddEdge(e1)
	s.AddEdge(e2) // duplicate, should be ignored
	if edges := s.GetOutgoing("a"); len(edges) != 1 {
		t.Fatalf("expected 1 edge after dup add, got %d", len(edges))
	}
}

func TestGraphStore_AddEdgeMissingNode(t *testing.T) {
	s := NewGraphStore()
	s.AddNode(&GraphNode{ID: "a"})
	// Target node doesn't exist — edge should be dropped.
	s.AddEdge(GraphEdge{SourceID: "a", TargetID: "ghost", Relation: RelCites})
	if edges := s.GetOutgoing("a"); len(edges) != 0 {
		t.Fatalf("edge to missing node should be dropped, got %d edges", len(edges))
	}
}

func TestGraphStore_RemoveNodeCleansEdges(t *testing.T) {
	s := testStore()
	s.RemoveNode("law:专利法第22条")
	if s.HasNode("law:专利法第22条") {
		t.Fatal("node should be removed")
	}
	// doc1's outgoing CITES edge should be cleaned.
	for _, e := range s.GetOutgoing("doc1") {
		if e.TargetID == "law:专利法第22条" {
			t.Fatal("dangling edge to removed node should be cleaned")
		}
	}
}

func TestGraphStore_SearchGraphNodes(t *testing.T) {
	s := testStore()
	// Keyword "侵权" matches doc1 and doc3.
	results := s.SearchGraphNodes("侵权", "", 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 侵权, got %d", len(results))
	}
	// Filter by nodeType.
	lawResults := s.SearchGraphNodes("", NodeLawArticle, 10)
	if len(lawResults) != 1 {
		t.Fatalf("expected 1 LawArticle, got %d", len(lawResults))
	}
	// Limit.
	limited := s.SearchGraphNodes("", "", 2)
	if len(limited) != 2 {
		t.Fatalf("expected limit 2, got %d", len(limited))
	}
}

func TestGraphStore_Counts(t *testing.T) {
	s := testStore()
	if s.NodeCount() != 4 {
		t.Fatalf("expected 4 nodes, got %d", s.NodeCount())
	}
	if s.EdgeCount() != 4 {
		t.Fatalf("expected 4 edges, got %d", s.EdgeCount())
	}
}

func TestGraphStore_JSONRoundTrip(t *testing.T) {
	s := testStore()
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := s.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}
	loaded := NewGraphStore()
	if err := loaded.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if loaded.NodeCount() != s.NodeCount() {
		t.Fatalf("node count mismatch: %d vs %d", loaded.NodeCount(), s.NodeCount())
	}
	if loaded.EdgeCount() != s.EdgeCount() {
		t.Fatalf("edge count mismatch: %d vs %d", loaded.EdgeCount(), s.EdgeCount())
	}
	if n := loaded.GetNode("doc1"); n == nil || n.Name != "案件A" {
		t.Fatalf("node doc1 not restored correctly: %v", n)
	}
}

func TestGraphStore_RemoveEdges(t *testing.T) {
	s := testStore()
	// Remove the SIMILAR_TO edge from doc1 to doc3.
	s.RemoveEdges("doc1", "doc3", RelSimilarTo)
	for _, e := range s.GetOutgoing("doc1") {
		if e.Relation == RelSimilarTo {
			t.Fatal("SIMILAR_TO edge should be removed")
		}
	}
}

func TestGraphStore_GetNodeDetail(t *testing.T) {
	s := testStore()
	detail := s.GetNodeDetail("doc1")
	if detail == nil || detail.Node.ID != "doc1" {
		t.Fatalf("expected detail for doc1, got %v", detail)
	}
	// doc1 has outgoing: CITES→law, SIMILAR_TO→doc3 = 2
	if len(detail.Outgoing) != 2 {
		t.Fatalf("expected 2 outgoing edges, got %d", len(detail.Outgoing))
	}
	// doc1 has incoming: APPLIES from law = 1
	if len(detail.Incoming) != 1 {
		t.Fatalf("expected 1 incoming edge, got %d", len(detail.Incoming))
	}
}

// --- Cache tests ---

func TestGraphCache_PutGet(t *testing.T) {
	c := NewGraphCache(100, 5*time.Minute)
	node := &GraphNode{ID: "x", Name: "test"}
	detail := &GraphNodeDetail{Node: node}
	c.PutNodeDetail("x", detail)
	if got := c.GetNodeDetail("x"); got == nil || got.Node.ID != "x" {
		t.Fatalf("cache miss/incorrect: %v", got)
	}
	results := []*GraphNode{node}
	c.PutSearch("kw|type|10", results)
	if got := c.GetSearch("kw|type|10"); len(got) != 1 {
		t.Fatalf("search cache miss: %v", got)
	}
	pr := &PathResult{Found: true, Paths: [][]string{{"a", "b"}}}
	c.PutPaths("a|b|3", pr)
	if got := c.GetPaths("a|b|3"); got == nil || !got.Found {
		t.Fatalf("path cache miss: %v", got)
	}
}

func TestGraphCache_Invalidate(t *testing.T) {
	c := NewGraphCache(100, 5*time.Minute)
	c.PutNodeDetail("x", &GraphNodeDetail{Node: &GraphNode{ID: "x"}})
	c.Invalidate()
	if c.GetNodeDetail("x") != nil {
		t.Fatal("cache should be empty after invalidate")
	}
}

func TestGraphCache_TTL(t *testing.T) {
	c := NewGraphCache(100, 10*time.Millisecond)
	c.PutNodeDetail("x", &GraphNodeDetail{Node: &GraphNode{ID: "x"}})
	time.Sleep(20 * time.Millisecond)
	if c.GetNodeDetail("x") != nil {
		t.Fatal("cache entry should expire after TTL")
	}
}
