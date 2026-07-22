package patent

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/retrieval/domain"
)

// mockInvRetriever implements domain.DomainRetriever for invalidation tests.
type mockInvRetriever struct {
	docs []domain.DomainDocument
	err  error
}

func (m *mockInvRetriever) Search(ctx context.Context, query domain.DomainQuery) (*domain.DomainResults, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &domain.DomainResults{
		Documents:  m.docs,
		TotalCount: len(m.docs),
		Source:     "mock",
	}, nil
}

func (m *mockInvRetriever) GetDocument(ctx context.Context, docID string) (*domain.DomainDocument, error) {
	return nil, nil
}

func (m *mockInvRetriever) SourceName() string { return "mock" }

// -----------------------------------------------------------------------------
// parsePatentNode
// -----------------------------------------------------------------------------

func TestParsePatentNode_EmptyInput(t *testing.T) {
	_, err := parsePatentNode(context.Background(), graph.PregelState{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParsePatentNode_BasicInput(t *testing.T) {
	input := `1. 一种图像处理方法，包括采集图像、对图像进行滤波处理和输出处理后的图像。
2. 根据权利要求1所述的方法，其中滤波处理采用高斯滤波。`

	out, err := parsePatentNode(context.Background(), graph.PregelState{
		InvStateInput: input,
	})
	if err != nil {
		t.Fatalf("parsePatentNode: %v", err)
	}

	claims, ok := out[InvStateClaimTree].([]InvClaimNode)
	if !ok || len(claims) < 2 {
		t.Fatalf("expected at least 2 claims, got %d", len(claims))
	}

	if claims[0].Number != 1 {
		t.Errorf("first claim number = %d, want 1", claims[0].Number)
	}

	if !claims[0].IsIndependent {
		t.Error("claim 1 should be independent")
	}
	if claims[1].IsIndependent {
		t.Error("claim 2 should be dependent (contains '根据')")
	}
}

func TestExtractClaimsFromText_Fallback(t *testing.T) {
	// No structured claim markers → should fall back to treating entire text as claim 1.
	text := "一种简单的装置描述，没有权利要求编号。"
	claims := extractClaimsFromText(text)
	if len(claims) != 1 {
		t.Fatalf("expected 1 fallback claim, got %d", len(claims))
	}
	if claims[0].Number != 1 || !claims[0].IsIndependent {
		t.Errorf("fallback should be independent claim 1, got %+v", claims[0])
	}
}

// -----------------------------------------------------------------------------
// identifyInvalidationGrounds
// -----------------------------------------------------------------------------

func TestIdentifyInvalidationGrounds_AllTypes(t *testing.T) {
	text := `请求人基于以下理由请求宣告专利无效：
1. 不符合专利法第22条第2款关于新颖性的规定
2. 不符合专利法第22条第3款关于创造性的规定
3. 不符合专利法第26条第3款关于公开充分的规定
4. 不符合专利法第26条第4款关于权利要求清楚的规定
5. 违反专利法第33条修改超范围`

	grounds := identifyInvalidationGrounds(text)
	if len(grounds) < 5 {
		t.Fatalf("expected >= 5 grounds, got %d", len(grounds))
	}

	seen := make(map[InvalidationGroundType]bool)
	for _, g := range grounds {
		seen[g.Type] = true
	}
	for _, gt := range []InvalidationGroundType{
		GroundNovelty, GroundInventiveness, GroundDisclosure,
		GroundClaimClarity, GroundAmendment,
	} {
		if !seen[gt] {
			t.Errorf("ground type %s not identified", gt)
		}
	}
}

func TestIdentifyInvalidationGrounds_Default(t *testing.T) {
	// No specific grounds mentioned → defaults to novelty.
	text := "一种简单的专利描述。"
	grounds := identifyInvalidationGrounds(text)
	if len(grounds) == 0 {
		t.Fatal("expected at least 1 default ground")
	}
	if grounds[0].Type != GroundNovelty {
		t.Errorf("default ground should be novelty, got %s", grounds[0].Type)
	}
}

// -----------------------------------------------------------------------------
// gatherEvidenceNode (degraded mode)
// -----------------------------------------------------------------------------

func TestGatherEvidenceNode_Degraded(t *testing.T) {
	state := graph.PregelState{
		InvStateClaims: "1. 一种图像处理方法。",
		InvStateGrounds: []InvGround{
			{Type: GroundNovelty, Article: "专利法第22条第2款", Description: "新颖性"},
		},
	}

	out, err := gatherEvidenceNode(context.Background(), state)
	if err != nil {
		t.Fatalf("gatherEvidenceNode: %v", err)
	}

	if !graph.IsDegraded(out, InvStateEvidence) {
		t.Error("evidence should be marked degraded without retriever")
	}
}

func TestGatherEvidenceNodeWithRetriever_Success(t *testing.T) {
	retriever := &mockInvRetriever{
		docs: []domain.DomainDocument{
			{ID: "D1", Title: "对比文件1", Snippet: "公开了滤波技术"},
			{ID: "D2", Title: "对比文件2", Snippet: "公开了特征提取"},
		},
	}

	node := newGatherEvidenceNodeWithRetriever(retriever)
	out, err := node(context.Background(), graph.PregelState{
		InvStateClaims: "1. 一种图像处理方法。",
	})
	if err != nil {
		t.Fatalf("gatherEvidence: %v", err)
	}

	evidence, ok := out[InvStateEvidence].([]string)
	if !ok || len(evidence) != 2 {
		t.Fatalf("expected 2 evidence items, got %v", out[InvStateEvidence])
	}

	if !strings.Contains(evidence[0], "D1") {
		t.Errorf("evidence should contain D1: %s", evidence[0])
	}
}

func TestGatherEvidenceNodeWithRetriever_SearchError(t *testing.T) {
	retriever := &mockInvRetriever{err: context.DeadlineExceeded}
	node := newGatherEvidenceNodeWithRetriever(retriever)
	out, err := node(context.Background(), graph.PregelState{
		InvStateClaims: "测试权利要求",
	})
	if err != nil {
		t.Fatalf("expected nil error (degraded), got %v", err)
	}

	if !graph.IsDegraded(out, InvStateEvidence) {
		t.Error("should be degraded on search error")
	}
}

func TestGatherEvidenceNodeWithRetriever_NilRetriever(t *testing.T) {
	node := newGatherEvidenceNodeWithRetriever(nil)
	out, err := node(context.Background(), graph.PregelState{
		InvStateClaims: "测试",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !graph.IsDegraded(out, InvStateEvidence) {
		t.Error("should be degraded with nil retriever")
	}
}

// -----------------------------------------------------------------------------
// analyzeGroundsNode
// -----------------------------------------------------------------------------

func TestAnalyzeGroundsNode_Basic(t *testing.T) {
	state := graph.PregelState{
		InvStateGrounds: []InvGround{
			{Type: GroundNovelty, Article: "专利法第22条第2款", Description: "新颖性无效",
				ClaimRefs: []int{1}},
		},
		InvStateClaimTree: []InvClaimNode{
			{Number: 1, IsIndependent: true, Text: "1. 一种图像处理方法。"},
		},
	}

	out, err := analyzeGroundsNode(context.Background(), state)
	if err != nil {
		t.Fatalf("analyzeGroundsNode: %v", err)
	}

	analysis := out.GetString(InvStateAnalysis)
	if analysis == "" {
		t.Fatal("analysis should not be empty")
	}
	if !strings.Contains(analysis, "单独对比") {
		t.Error("novelty analysis should mention 单独对比")
	}

	ruleCheck := out.GetString(InvStateRuleCheck)
	if ruleCheck == "" {
		t.Error("rule check report should not be empty")
	}

	verdict := out.GetString(InvStateRuleVerdict)
	if verdict == "" {
		t.Error("rule verdict should not be empty")
	}
}

// -----------------------------------------------------------------------------
// BuildInvalidationGraph (end-to-end)
// -----------------------------------------------------------------------------

func TestBuildInvalidationGraph_EndToEnd(t *testing.T) {
	compiled, err := BuildInvalidationGraph()
	if err != nil {
		t.Fatalf("BuildInvalidationGraph: %v", err)
	}

	input := `目标专利权利要求：
1. 一种基于深度学习的图像识别方法，包括采集图像数据、使用卷积神经网络提取特征和分类识别的步骤。
2. 根据权利要求1所述的方法，其中卷积神经网络为ResNet结构。

请求人主张无效理由：该专利不符合专利法第22条第2款新颖性及第22条第3款创造性。`

	out, err := compiled.Run(context.Background(), graph.PregelState{
		InvStateInput: input,
	})
	if err != nil {
		t.Fatalf("graph run: %v", err)
	}

	output := out.GetString(InvStateOutput)
	if output == "" {
		t.Fatal("output should not be empty")
	}
	if !strings.Contains(output, "无效宣告分析报告") {
		t.Error("output should contain report title")
	}
	if !strings.Contains(output, "单独对比") {
		t.Error("output should mention novelty 单独对比 principle")
	}
	if !strings.Contains(output, "三步法") {
		t.Error("output should mention inventiveness 三步法 framework")
	}
}

func TestBuildInvalidationGraphWithOpts_WithRetriever(t *testing.T) {
	retriever := &mockInvRetriever{
		docs: []domain.DomainDocument{
			{ID: "CN101", Title: "现有技术：传统滤波方法", Snippet: "使用均值滤波"},
		},
	}
	compiled, err := BuildInvalidationGraphWithOpts(WithInvRetriever(retriever))
	if err != nil {
		t.Fatalf("BuildInvalidationGraphWithOpts: %v", err)
	}

	input := `1. 一种图像滤波方法，包括采集和均值滤波的步骤。
请求人基于专利法第22条第2款主张该专利不具备新颖性。`

	out, err := compiled.Run(context.Background(), graph.PregelState{
		InvStateInput: input,
	})
	if err != nil {
		t.Fatalf("graph run: %v", err)
	}

	output := out.GetString(InvStateOutput)
	if !strings.Contains(output, "CN101") {
		t.Error("output should contain evidence from retriever")
	}
	// Should NOT be degraded since we injected a retriever.
	if graph.IsDegraded(out, InvStateEvidence) {
		t.Error("evidence should not be degraded with retriever")
	}
}

// -----------------------------------------------------------------------------
// truncate helper
// -----------------------------------------------------------------------------

func TestTruncate_ShortString(t *testing.T) {
	got := truncate("hello", 10)
	if got != "hello" {
		t.Errorf("truncate short string = %q, want %q", got, "hello")
	}
}

func TestTruncate_LongString(t *testing.T) {
	long := strings.Repeat("x", 100)
	got := truncate(long, 10)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated string should end with ellipsis: %q", got)
	}
}
