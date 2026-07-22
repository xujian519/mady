package reasoning

import (
	"context"
	"testing"
)

// mockTopoKGStore implements KnowledgeGraphStore for topology testing with
// multiple node lookup support.
type mockTopoKGStore struct {
	searchFn func(keyword, nodeType string, limit int) ([]KgNode, error)
	nodes    map[string]*KgNodeDetail
}

func (m *mockTopoKGStore) SearchNodes(keyword, nodeType string, limit int) ([]KgNode, error) {
	if m.searchFn != nil {
		return m.searchFn(keyword, nodeType, limit)
	}
	return nil, nil
}

func (m *mockTopoKGStore) GetNodeDetail(nodeID string) (*KgNodeDetail, error) {
	if d, ok := m.nodes[nodeID]; ok {
		return d, nil
	}
	return nil, nil
}

// helper to create a graph node quickly.
func kgNode(id, nodeType, name, content string) KgNode {
	return KgNode{ID: id, NodeType: nodeType, Name: name, Content: content}
}

// helper to create a graph edge quickly.
func kgEdge(targetID, relation string, weight float64) KgEdge {
	return KgEdge{TargetID: targetID, Relation: relation, Weight: weight}
}

// --------------------------------------------------------------------------
// Nil / empty store
// --------------------------------------------------------------------------

func TestTopologyExtractor_NilStore(t *testing.T) {
	ext := NewTopologyExtractor(nil)
	if ext.HasStore() {
		t.Fatal("HasStore should be false for nil store")
	}
	topo, err := ext.ExtractByCaseType(context.Background(), CaseNoveltySearch, 2, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topo == nil || len(topo.Gaps) == 0 {
		t.Fatal("expected gap for nil store")
	}
}

func TestTopologyExtractor_NoMatchingNodes(t *testing.T) {
	store := &mockTopoKGStore{
		searchFn: func(_, _ string, _ int) ([]KgNode, error) {
			return nil, nil // no results
		},
	}
	ext := NewTopologyExtractor(store)
	topo, err := ext.ExtractByCaseType(context.Background(), CaseNoveltySearch, 2, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topo.Gaps) == 0 {
		t.Fatal("expected gap for no matching nodes")
	}
}

// --------------------------------------------------------------------------
// Single GuidelineRule → CITES → LawArticle chain
// --------------------------------------------------------------------------

func TestTopologyExtractor_CitesChain(t *testing.T) {
	store := &mockTopoKGStore{
		searchFn: func(keyword, _ string, _ int) ([]KgNode, error) {
			if keyword == "新颖性" {
				return []KgNode{{ID: "g1", NodeType: "GuidelineRule", Name: "新颖性审查"}}, nil
			}
			return nil, nil
		},
		nodes: map[string]*KgNodeDetail{
			"g1": {
				Node:     kgNode("g1", "GuidelineRule", "新颖性审查", "新颖性审查规则"),
				Outgoing: []KgEdge{kgEdge("a22_2", "CITES", 0.9), kgEdge("a22_3", "CITES", 0.85)},
			},
			"a22_2": {
				Node: kgNode("a22_2", "LawArticle", "专利法第22条第2款", "新颖性定义"),
			},
			"a22_3": {
				Node: kgNode("a22_3", "LawArticle", "专利法第22条第3款", "创造性定义"),
			},
		},
	}

	ext := NewTopologyExtractor(store)
	topo, err := ext.ExtractByCaseType(context.Background(), CaseNoveltySearch, 2, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if topo.RootRule != "g1" {
		t.Fatalf("expected root g1, got %s", topo.RootRule)
	}
	if len(topo.Steps) != 2 {
		t.Fatalf("expected 2 steps (2 CITES edges), got %d", len(topo.Steps))
	}

	// Both should be LawArticle CITES steps.
	for i, s := range topo.Steps {
		if s.NodeType != "LawArticle" {
			t.Errorf("step %d: expected LawArticle, got %s", i, s.NodeType)
		}
		if s.Relation != WorkflowRelCites {
			t.Errorf("step %d: expected CITES, got %s", i, s.Relation)
		}
	}
}

// --------------------------------------------------------------------------
// GuidelineRule → CITES + APPLIES + RELATED_TO edges
// --------------------------------------------------------------------------

func TestTopologyExtractor_MixedEdges(t *testing.T) {
	store := &mockTopoKGStore{
		searchFn: func(keyword, _ string, _ int) ([]KgNode, error) {
			if keyword == "侵权" {
				return []KgNode{{ID: "g1", NodeType: "GuidelineRule", Name: "侵权判定规则"}}, nil
			}
			return nil, nil
		},
		nodes: map[string]*KgNodeDetail{
			"g1": {
				Node: kgNode("g1", "GuidelineRule", "侵权判定规则", "全面覆盖原则"),
				Outgoing: []KgEdge{
					kgEdge("a59", "CITES", 0.9),           // LawArticle
					kgEdge("a64", "CITES", 0.85),          // LawArticle
					kgEdge("case1", "APPLIES", 0.7),       // Case
					kgEdge("related1", "RELATED_TO", 0.6), // GuidelineRule
				},
			},
			"a59": {
				Node: kgNode("a59", "LawArticle", "专利法第59条", "保护范围"),
			},
			"a64": {
				Node: kgNode("a64", "LawArticle", "专利法第64条", "侵权例外"),
			},
			"case1": {
				Node: kgNode("case1", "Case", "最高院指导案例XX号", "等同侵权案例"),
			},
			"related1": {
				Node: kgNode("related1", "GuidelineRule", "损害赔偿计算", "赔偿规则"),
			},
		},
	}

	ext := NewTopologyExtractor(store)
	topo, err := ext.ExtractByCaseType(context.Background(), CaseInfringement, 2, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(topo.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d: %+v", len(topo.Steps), topo.Steps)
	}

	// Verify ordering: CITES first, APPLIES second, RELATED_TO last.
	expectedRelations := []WorkflowRelation{
		WorkflowRelCites, WorkflowRelCites, // law articles first
		WorkflowRelApplies,   // case comparison
		WorkflowRelRelatedTo, // associated procedure
	}
	for i, exp := range expectedRelations {
		if topo.Steps[i].Relation != exp {
			t.Errorf("step %d: expected relation %s, got %s (name=%s)",
				i, exp, topo.Steps[i].Relation, topo.Steps[i].Name)
		}
	}

	if topo.AuthorityScore <= 0 {
		t.Error("expected positive authority score")
	}
}

// --------------------------------------------------------------------------
// Ordering: CITES before APPLIES before RELATED_TO
// --------------------------------------------------------------------------

func TestTopologyExtractor_Ordering(t *testing.T) {
	// Create a store where edges come in reverse order (RELATED_TO first,
	// APPLIES middle, CITES last) and verify they get re-sorted.
	store := &mockTopoKGStore{
		searchFn: func(keyword, _ string, _ int) ([]KgNode, error) {
			return []KgNode{{ID: "g1", NodeType: "GuidelineRule", Name: "混合规则"}}, nil
		},
		nodes: map[string]*KgNodeDetail{
			"g1": {
				Node: kgNode("g1", "GuidelineRule", "混合规则", "包含多种关系"),
				Outgoing: []KgEdge{
					kgEdge("rel", "RELATED_TO", 0.6),
					kgEdge("app", "APPLIES", 0.8),
					kgEdge("cite", "CITES", 0.9),
				},
			},
			"cite": {Node: kgNode("cite", "LawArticle", "法条A", "内容")},
			"app":  {Node: kgNode("app", "Case", "案例B", "内容")},
			"rel":  {Node: kgNode("rel", "GuidelineRule", "关联C", "内容")},
		},
	}

	ext := NewTopologyExtractor(store)
	topo, err := ext.ExtractByCaseType(context.Background(), CasePatentability, 2, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(topo.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(topo.Steps))
	}

	// Must be: CITES → APPLIES → RELATED_TO
	if topo.Steps[0].Relation != WorkflowRelCites {
		t.Errorf("expected step 0 to be CITES, got %s", topo.Steps[0].Relation)
	}
	if topo.Steps[1].Relation != WorkflowRelApplies {
		t.Errorf("expected step 1 to be APPLIES, got %s", topo.Steps[1].Relation)
	}
	if topo.Steps[2].Relation != WorkflowRelRelatedTo {
		t.Errorf("expected step 2 to be RELATED_TO, got %s", topo.Steps[2].Relation)
	}
}

// --------------------------------------------------------------------------
// Max steps truncation
// --------------------------------------------------------------------------

func TestTopologyExtractor_MaxStepsLimit(t *testing.T) {
	store := &mockTopoKGStore{
		searchFn: func(keyword, _ string, _ int) ([]KgNode, error) {
			return []KgNode{{ID: "g1", NodeType: "GuidelineRule", Name: "多步规则"}}, nil
		},
		nodes: map[string]*KgNodeDetail{
			"g1": {
				Node:     kgNode("g1", "GuidelineRule", "多步规则", ""),
				Outgoing: []KgEdge{kgEdge("a1", "CITES", 0.9), kgEdge("a2", "CITES", 0.9)},
			},
			"a1": {Node: kgNode("a1", "LawArticle", "法条1", "")},
			"a2": {Node: kgNode("a2", "LawArticle", "法条2", "")},
		},
	}

	ext := NewTopologyExtractor(store)
	// maxSteps=1 should only return 1 step.
	topo, err := ext.ExtractByCaseType(context.Background(), CaseNoveltySearch, 2, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topo.Steps) > 1 {
		t.Fatalf("expected at most 1 step with maxSteps=1, got %d", len(topo.Steps))
	}
}

// --------------------------------------------------------------------------
// Dependency matrix
// --------------------------------------------------------------------------

func TestTopologyExtractor_DependencyMatrix(t *testing.T) {
	ext := NewTopologyExtractor(nil) // extractor itself, store not needed for dep matrix test
	steps := []WorkflowStep{
		{ArticleID: "a1", Relation: WorkflowRelCites, Name: "法条1", Priority: 1},
		{ArticleID: "a2", Relation: WorkflowRelCites, Name: "法条2", Priority: 2},
		{ArticleID: "c1", Relation: WorkflowRelApplies, Name: "案例1", Priority: 2},
		{ArticleID: "r1", Relation: WorkflowRelRelatedTo, Name: "关联1", Priority: 3},
	}
	deps := ext.buildDependencyMatrix(steps)
	if len(deps) != 4 {
		t.Fatalf("expected 4 entries in dependency matrix, got %d", len(deps))
	}
	// CITES steps should have no deps.
	if deps[0] != nil {
		t.Errorf("CITES step should have no deps, got %v", deps[0])
	}
	if deps[1] != nil {
		t.Errorf("CITES step should have no deps, got %v", deps[1])
	}
	// APPLIES step should depend on all preceding CITES steps (indices 0 and 1).
	if len(deps[2]) != 2 || deps[2][0] != 0 || deps[2][1] != 1 {
		t.Errorf("APPLIES step should depend on last CITES (index 1), got %v", deps[2])
	}
	// RELATED_TO should depend on all preceding steps.
	if len(deps[3]) != 3 {
		t.Errorf("RELATED_TO should depend on 3 preceding steps, got %v", deps[3])
	}
}

// --------------------------------------------------------------------------
// Case type keyword mapping coverage
// --------------------------------------------------------------------------

func TestCaseTypeKeywords_Coverage(t *testing.T) {
	// Verify all defined CaseTypes produce non-empty keyword lists.
	cases := []CaseType{
		CaseNoveltySearch, CasePatentability, CaseOAResponse,
		CaseRejection, CaseReexamination, CaseInvalidation,
		CaseInfringement, CaseFTO, CaseValidity, CaseDrafting,
		CaseLegalStatus, CaseGeneralLegal,
	}
	for _, ct := range cases {
		kws := caseTypeKeywords(ct)
		if ct == CaseGeneralLegal && len(kws) == 1 && kws[0] == "" {
			continue // general_legal uses empty fallback, OK
		}
		if len(kws) == 0 || (len(kws) == 1 && kws[0] == "") {
			t.Errorf("case type %s produced empty keywords", ct)
		}
	}
}

// --------------------------------------------------------------------------
// Strategy mapping
// --------------------------------------------------------------------------

func TestRelationToStrategy(t *testing.T) {
	tests := []struct {
		rel      WorkflowRelation
		expected StrategyType
	}{
		{WorkflowRelCites, StrategyChain},
		{WorkflowRelApplies, StrategyMultiHypothesis},
		{WorkflowRelRelatedTo, StrategyChain},
		{WorkflowRelContains, StrategyChain},
	}
	for _, tc := range tests {
		got := relationToStrategy(tc.rel, "")
		if got != tc.expected {
			t.Errorf("relation %s: expected %s, got %s", tc.rel, tc.expected, got)
		}
	}
}

// --------------------------------------------------------------------------
// Priority mapping
// --------------------------------------------------------------------------

func TestPriorityForNodeType(t *testing.T) {
	tests := []struct {
		nodeType string
		expected int
	}{
		{"LawArticle", 1},
		{"GuidelineRule", 1},
		{"Rule", 1},
		{"Case", 2},
		{"Judgment", 2},
		{"Evidence", 2},
		{"IPC", 3},
		{"Concept", 3},
		{"UnknownType", 3},
	}
	for _, tc := range tests {
		got := priorityForNodeType(tc.nodeType)
		if got != tc.expected {
			t.Errorf("nodeType %s: expected priority %d, got %d", tc.nodeType, tc.expected, got)
		}
	}
}
