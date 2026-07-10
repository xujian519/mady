package reasoning

import (
	"context"
	"testing"
)

type mockKGStore struct {
	searchResults []KgNode
	detail        *KgNodeDetail
}

func (m *mockKGStore) SearchNodes(keyword, nodeType string, limit int) ([]KgNode, error) {
	return m.searchResults, nil
}

func (m *mockKGStore) GetNodeDetail(nodeID string) (*KgNodeDetail, error) {
	return m.detail, nil
}

type mockLLM struct{ resp string }

func (m *mockLLM) Chat(_ context.Context, _ []LlmMessage) (string, error) {
	return m.resp, nil
}

func TestWalker_Walk_EmptyFacts(t *testing.T) {
	w := NewReasoningWalker(&mockKGStore{}, nil)
	res, err := w.Walk(context.Background(), ReasoningWalkInput{Facts: nil, CaseType: CasePatentability})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(res.Chains) != 0 || res.Coverage != 0 {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestWalker_Walk_NilStore(t *testing.T) {
	w := NewReasoningWalker(nil, nil)
	res, err := w.Walk(context.Background(), ReasoningWalkInput{Facts: []string{"权利要求1"}, CaseType: CasePatentability})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(res.Chains) != 0 {
		t.Fatalf("nil store should yield empty chains, got %d", len(res.Chains))
	}
}

func TestWalker_Walk_ProducesChains(t *testing.T) {
	store := &mockKGStore{
		searchResults: []KgNode{
			{ID: "n1", NodeType: "Concept", Name: "创造性", Content: "..."},
			{ID: "n2", NodeType: "LawArticle", Name: "A22.3", Content: "创造性条款"},
		},
		detail: &KgNodeDetail{
			Node:     KgNode{ID: "n1", NodeType: "Concept", Name: "创造性"},
			Outgoing: []KgEdge{{TargetID: "n2", Relation: "APPLIES", Weight: 0.9}},
			Incoming: nil,
		},
	}
	w := NewReasoningWalker(store, &mockLLM{resp: "创造性, 三步法"})
	res, err := w.Walk(context.Background(), ReasoningWalkInput{
		Facts:     []string{"权利要求1包含技术特征A"},
		CaseType:  CasePatentability,
		MaxChains: 2,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(res.Chains) == 0 {
		t.Fatal("expected at least one chain")
	}
	if len(res.Chains) > 2 {
		t.Fatalf("maxChains exceeded: %d", len(res.Chains))
	}
	// First node is the seed.
	if res.Chains[0].Nodes[0].Relation != "SEED" {
		t.Fatalf("first node should be SEED, got %s", res.Chains[0].Nodes[0].Relation)
	}
}

func TestWalker_Walk_RespectsMaxChains(t *testing.T) {
	store := &mockKGStore{
		searchResults: []KgNode{
			{ID: "n1", Name: "a"},
			{ID: "n2", Name: "b"},
			{ID: "n3", Name: "c"},
			{ID: "n4", Name: "d"},
		},
		detail: &KgNodeDetail{Node: KgNode{ID: "n1"}},
	}
	w := NewReasoningWalker(store, nil)
	res, _ := w.Walk(context.Background(), ReasoningWalkInput{
		Facts:     []string{"x"},
		MaxChains: 2,
	})
	if len(res.Chains) > 2 {
		t.Fatalf("expected <= 2 chains, got %d", len(res.Chains))
	}
}

func TestWalker_CollectAll_ProducesConstraints(t *testing.T) {
	store := &mockKGStore{
		searchResults: []KgNode{
			{ID: "A22.3", NodeType: "LawArticle", Name: "创造性", Content: "必须具备创造性"},
		},
		detail: &KgNodeDetail{},
	}
	w := NewReasoningWalker(store, &mockLLM{resp: "方向1\n方向2"})
	res, err := w.CollectAll(context.Background(), CollectAllInput{
		Facts:    []string{"专利撰写"},
		CaseType: CaseDrafting,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(res.Constraints) == 0 {
		t.Fatal("expected constraints")
	}
	// must-type article should appear in RelatedArticles.
	found := false
	for _, a := range res.RelatedArticles {
		if a == "A22.3" {
			found = true
		}
	}
	if !found {
		t.Fatal("A22.3 (must) should be in RelatedArticles")
	}
	if len(res.SearchDirections) != 2 {
		t.Fatalf("expected 2 search directions, got %d", len(res.SearchDirections))
	}
}

func TestWalker_CollectAll_EmptyFacts(t *testing.T) {
	w := NewReasoningWalker(&mockKGStore{}, nil)
	res, err := w.CollectAll(context.Background(), CollectAllInput{Facts: nil})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(res.Constraints) != 0 {
		t.Fatalf("expected empty constraints")
	}
}

func TestClassifyRequirement(t *testing.T) {
	if classifyRequirement("LawArticle") != ReqMust {
		t.Fatal("LawArticle should be must")
	}
	if classifyRequirement("IPC") != ReqNote {
		t.Fatal("IPC should be note")
	}
	if classifyRequirement("Case") != ReqShould {
		t.Fatal("Case should be should")
	}
}

func TestSelectBestEdge_PrefersStrategyRelations(t *testing.T) {
	edges := []KgEdge{
		{TargetID: "c1", Relation: "CITES", Weight: 0.5},
		{TargetID: "c2", Relation: "APPLIES", Weight: 0.3},
		{TargetID: "c3", Relation: "SIMILAR_TO", Weight: 0.9},
	}
	// legal strategy prefers APPLIES/INTERPRETED_BY/DEFINES → c2
	best := selectBestEdge(edges, CaseInvalidation)
	if best.TargetID != "c2" {
		t.Fatalf("legal strategy should prefer APPLIES edge, got %s", best.TargetID)
	}
	// technical strategy prefers SIMILAR_TO/CITES → c3 (weight 0.9 > 0.5)
	best = selectBestEdge(edges, CaseDrafting)
	if best.TargetID != "c3" {
		t.Fatalf("technical strategy should pick highest SIMILAR_TO/CITES, got %s", best.TargetID)
	}
}

func TestExtractLegalBasis(t *testing.T) {
	nodes := []ReasoningChainNode{
		{NodeType: "Concept", Name: "创造性"},
		{NodeType: "LawArticle", Name: "A22.3"},
		{NodeType: "Judgment", Name: "(2020)最高法行再1号"},
	}
	lb := extractLegalBasis(nodes)
	if lb.LawArticle != "A22.3" || lb.PrecedentCase != "(2020)最高法行再1号" {
		t.Fatalf("legal basis extraction mismatch: %+v", lb)
	}
}
