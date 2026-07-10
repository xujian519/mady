package graph

import (
	"testing"

	"github.com/xujian519/mady/knowledge"
)

func TestParseKnowledgeDocument(t *testing.T) {
	doc := &knowledge.Document{
		ID:      "case001",
		Title:   "测试案例文档",
		Domain:  "patent",
		Content: "本发明涉及一种通信装置",
		Source:  "case",
		Metadata: map[string]string{
			"type":        "case",
			"level":       "一般案例",
			"law_refs":    "专利法第22条第3款,专利法第26条第3款",
			"cross_refs":  "相关wiki1",
			"ipc_codes":   "G06F,H04L",
			"case_number": "(2020)京73行初1234",
			"court":       "北京知识产权法院",
			"priority":    "P2",
		},
	}
	parsed := ParseKnowledgeDocument(doc)

	if parsed.ID != "case001" {
		t.Errorf("ID = %q, want case001", parsed.ID)
	}
	if parsed.DocType != "case" {
		t.Errorf("DocType = %q, want case", parsed.DocType)
	}
	if len(parsed.Metadata.LawRefs) != 2 {
		t.Fatalf("LawRefs len = %d, want 2", len(parsed.Metadata.LawRefs))
	}
	if parsed.Metadata.LawRefs[0] != "专利法第22条第3款" {
		t.Errorf("LawRefs[0] = %q", parsed.Metadata.LawRefs[0])
	}
	if len(parsed.Metadata.IPCCodes) != 2 {
		t.Errorf("IPCCodes len = %d, want 2", len(parsed.Metadata.IPCCodes))
	}
	if parsed.Metadata.CaseNumber != "(2020)京73行初1234" {
		t.Errorf("CaseNumber = %q", parsed.Metadata.CaseNumber)
	}
	if parsed.Metadata.Priority != "P2" {
		t.Errorf("Priority = %q", parsed.Metadata.Priority)
	}
}

func TestParseKnowledgeDocument_FallbackIPCAndLaw(t *testing.T) {
	doc := &knowledge.Document{
		ID:       "doc1",
		Title:    "fallback",
		Domain:   "patent",
		Content:  "内容",
		Source:   "wiki",
		Metadata: map[string]string{"ipc": "A61K", "law": "专利法第2条"},
	}
	parsed := ParseKnowledgeDocument(doc)
	if len(parsed.Metadata.IPCCodes) != 1 || parsed.Metadata.IPCCodes[0] != "A61K" {
		t.Errorf("fallback IPCCodes = %v", parsed.Metadata.IPCCodes)
	}
	if len(parsed.Metadata.LawRefs) != 1 || parsed.Metadata.LawRefs[0] != "专利法第2条" {
		t.Errorf("fallback LawRefs = %v", parsed.Metadata.LawRefs)
	}
}

func TestParseKnowledgeDocument_InferDocType(t *testing.T) {
	cases := []struct {
		level string
		want  string
	}{
		{"法律", "law_article"},
		{"审查指南", "guideline_rule"},
		{"指导性案例", "case"},
	}
	for _, c := range cases {
		doc := &knowledge.Document{
			ID:       "d",
			Domain:   "patent",
			Metadata: map[string]string{"level": c.level},
		}
		parsed := ParseKnowledgeDocument(doc)
		if parsed.DocType != c.want {
			t.Errorf("level=%q → DocType=%q, want %q", c.level, parsed.DocType, c.want)
		}
	}
}

func TestLawNodeID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"专利法第22条第3款", "law:专利法第22条第3款"},
		{" 专利法 第22条 ", "law:专利法第22条"},
		{"", ""},
		{"  ", ""},
	}
	for _, c := range cases {
		got := lawNodeID(c.in)
		if got != c.want {
			t.Errorf("lawNodeID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func caseDoc(id, title, lawRef string) ParsedDoc {
	return ParsedDoc{
		ID:      id,
		Source:  "case",
		DocType: "case",
		Domain:  "patent",
		Title:   title,
		Content: "案件内容",
		Metadata: ParsedMetadata{
			Level:   "一般案例",
			LawRefs: []string{lawRef},
		},
	}
}

func TestBuild_CITESandAPPLIES(t *testing.T) {
	store := NewGraphStore()
	b := NewGraphBuilder(store)
	b.Build([]ParsedDoc{caseDoc("case001", "案例一", "专利法第22条第3款")})

	lawID := "law:专利法第22条第3款"
	// case001 CITES law node
	outgoing := store.GetOutgoing("case001")
	foundCites := false
	for _, e := range outgoing {
		if e.Relation == RelCites && e.TargetID == lawID {
			foundCites = true
		}
	}
	if !foundCites {
		t.Error("expected CITES edge case001 → law node")
	}

	// law node APPLIES case001 (reverse)
	incoming := store.GetOutgoing(lawID)
	foundApplies := false
	for _, e := range incoming {
		if e.Relation == RelApplies && e.TargetID == "case001" {
			foundApplies = true
		}
	}
	if !foundApplies {
		t.Error("expected APPLIES edge law → case001")
	}

	// law node auto-created
	node := store.GetNode(lawID)
	if node == nil {
		t.Fatal("law node not auto-created")
	}
	if node.NodeType != NodeLawArticle {
		t.Errorf("law node type = %q, want %q", node.NodeType, NodeLawArticle)
	}
}

func TestBuild_SIMILAR_TO(t *testing.T) {
	store := NewGraphStore()
	b := NewGraphBuilder(store)
	docs := []ParsedDoc{
		caseDoc("case001", "案例一", "专利法第22条第3款"),
		caseDoc("case002", "案例二", "专利法第22条第3款"),
	}
	b.Build(docs)

	// case001 → SIMILAR_TO → case002 (case001 < case002, no reverse edge added)
	similar := QuerySimilar(store, "case001")
	found := false
	for _, n := range similar {
		if n.ID == "case002" {
			found = true
		}
	}
	if !found {
		t.Error("expected SIMILAR_TO from case001 to case002")
	}

	// case002 can see case001 via the bidirectional QuerySimilar (both dir),
	// which is correct for a symmetric "similar" relationship.
	similarBack := QuerySimilar(store, "case002")
	foundBack := false
	for _, n := range similarBack {
		if n.ID == "case001" {
			foundBack = true
		}
	}
	if !foundBack {
		t.Error("expected case002 to find case001 via bidirectional SIMILAR_TO")
	}
}

func TestIncrementalUpdate(t *testing.T) {
	store := NewGraphStore()
	b := NewGraphBuilder(store)
	b.Build([]ParsedDoc{caseDoc("case001", "一", "专利法第22条第3款")})
	if store.NodeCount() < 2 {
		t.Fatalf("after Build, node count = %d, want >= 2", store.NodeCount())
	}

	// Incremental: add case002, remove case001
	b2 := NewGraphBuilder(store)
	b2.IncrementalUpdate(
		[]ParsedDoc{caseDoc("case002", "二", "专利法第22条第3款")},
		[]string{"case001"},
	)
	if store.HasNode("case001") {
		t.Error("case001 should be removed")
	}
	if !store.HasNode("case002") {
		t.Error("case002 should be added")
	}
	// law node survives (case002 still cites it)
	lawID := "law:专利法第22条第3款"
	if !store.HasNode(lawID) {
		t.Error("law node should survive")
	}
}

func TestQueryPaths(t *testing.T) {
	store := NewGraphStore()
	b := NewGraphBuilder(store)
	b.Build([]ParsedDoc{
		caseDoc("case001", "一", "专利法第22条第3款"),
		caseDoc("case002", "二", "专利法第22条第3款"),
	})

	// case001 → SIMILAR_TO → case002 (1 hop) or via law node (2 hops)
	result := QueryPaths(store, "case001", "case002", 3)
	if !result.Found {
		t.Fatal("expected path found case001 → case002")
	}
	if len(result.Paths) == 0 {
		t.Fatal("expected at least one path")
	}
	// The last node of the first path must be case002
	p := result.Paths[0]
	if p[len(p)-1] != "case002" || p[0] != "case001" {
		t.Errorf("unexpected path: %v", p)
	}
}

func TestQueryPaths_NotFound(t *testing.T) {
	store := NewGraphStore()
	store.AddNode(&GraphNode{ID: "a", NodeType: NodeConcept, Name: "A"})
	store.AddNode(&GraphNode{ID: "z", NodeType: NodeConcept, Name: "Z"})
	result := QueryPaths(store, "a", "z", 3)
	if result.Found {
		t.Error("expected no path between disconnected nodes")
	}
}

func TestQueryNeighbors(t *testing.T) {
	store := NewGraphStore()
	store.AddNode(&GraphNode{ID: "center", NodeType: NodeConcept, Name: "中心"})
	store.AddNode(&GraphNode{ID: "n1", NodeType: NodeConcept, Name: "邻居1"})
	store.AddNode(&GraphNode{ID: "n2", NodeType: NodeConcept, Name: "邻居2"})
	store.AddEdge(GraphEdge{SourceID: "center", TargetID: "n1", Relation: RelRelatedTo, Weight: 0.5})
	store.AddEdge(GraphEdge{SourceID: "center", TargetID: "n2", Relation: RelRelatedTo, Weight: 0.5})

	neighbors := QueryNeighbors(store, "center", 1)
	if len(neighbors) != 2 {
		t.Errorf("neighbors count = %d, want 2", len(neighbors))
	}
}

func TestQueryByRelation(t *testing.T) {
	store := NewGraphStore()
	b := NewGraphBuilder(store)
	b.Build([]ParsedDoc{caseDoc("case001", "一", "专利法第22条第3款")})

	// Outgoing CITES from case001
	outCites := QueryByRelation(store, "case001", RelCites, "outgoing")
	if len(outCites) != 1 {
		t.Errorf("outgoing CITES = %d, want 1", len(outCites))
	}

	// Both directions of APPLIES from law node
	lawID := "law:专利法第22条第3款"
	bothApplies := QueryByRelation(store, lawID, RelApplies, "both")
	if len(bothApplies) != 1 {
		t.Errorf("both APPLIES = %d, want 1", len(bothApplies))
	}
}

func TestQueryCitationChain(t *testing.T) {
	store := NewGraphStore()
	b := NewGraphBuilder(store)
	b.Build([]ParsedDoc{
		caseDoc("case001", "指导案例", "专利法第22条第3款"),
		caseDoc("case002", "普通案例", "专利法第22条第3款"),
	})

	chain := QueryCitationChain(store, "专利法第22条第3款")
	if len(chain) < 2 {
		t.Fatalf("citation chain = %d, want >= 2", len(chain))
	}
	// Both cases should appear
	seen := map[string]bool{}
	for _, n := range chain {
		seen[n.ID] = true
	}
	if !seen["case001"] || !seen["case002"] {
		t.Errorf("expected case001 and case002 in chain, got %v", seen)
	}
}

func TestReasoningStoreAdapter_SearchNodes(t *testing.T) {
	store := NewGraphStore()
	store.AddNode(&GraphNode{ID: "case001", NodeType: NodeCase, Name: "专利新颖性案例", Content: "关于新颖性"})
	store.AddNode(&GraphNode{ID: "law1", NodeType: NodeLawArticle, Name: "专利法第22条", Content: "法条内容"})

	adapter := NewReasoningStoreAdapter(store)
	nodes, err := adapter.SearchNodes("专利", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Errorf("SearchNodes count = %d, want 2", len(nodes))
	}

	// Filter by node type
	caseNodes, _ := adapter.SearchNodes("专利", NodeCase, 10)
	if len(caseNodes) != 1 || caseNodes[0].ID != "case001" {
		t.Errorf("filtered SearchNodes = %v", caseNodes)
	}
}

func TestReasoningStoreAdapter_GetNodeDetail(t *testing.T) {
	store := NewGraphStore()
	store.AddNode(&GraphNode{ID: "case001", NodeType: NodeCase, Name: "案例1"})
	store.AddNode(&GraphNode{ID: "law1", NodeType: NodeLawArticle, Name: "专利法22条"})
	store.AddEdge(GraphEdge{SourceID: "case001", TargetID: "law1", Relation: RelCites, Weight: 0.9})
	store.AddEdge(GraphEdge{SourceID: "law1", TargetID: "case001", Relation: RelApplies, Weight: 0.85})

	adapter := NewReasoningStoreAdapter(store)
	detail, err := adapter.GetNodeDetail("case001")
	if err != nil {
		t.Fatal(err)
	}
	if detail == nil {
		t.Fatal("detail is nil")
	}
	if detail.Node.ID != "case001" {
		t.Errorf("detail node ID = %q", detail.Node.ID)
	}
	if len(detail.Outgoing) != 1 {
		t.Errorf("outgoing edges = %d, want 1", len(detail.Outgoing))
	}
	if len(detail.Incoming) != 1 {
		t.Errorf("incoming edges = %d, want 1", len(detail.Incoming))
	}
}

func TestReasoningStoreAdapter_NotFound(t *testing.T) {
	store := NewGraphStore()
	adapter := NewReasoningStoreAdapter(store)
	detail, err := adapter.GetNodeDetail("nonexistent")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if detail != nil {
		t.Error("expected nil detail for nonexistent node")
	}
}
