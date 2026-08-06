package patent

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

func TestOAResponseGraph_BuildAndCompile(t *testing.T) {
	g, err := BuildOAResponseGraph()
	if err != nil {
		t.Fatalf("BuildOAResponseGraph() error = %v", err)
	}
	if g == nil {
		t.Fatal("BuildOAResponseGraph() returned nil")
	}
}

func TestParseOANode_NoveltyRejection(t *testing.T) {
	oaText := `审查意见通知书

本申请涉及一种智能灌溉装置。审查员认为：

权利要求1-3相对于对比文件1（CN123456A）不具备新颖性（专利法第22条第2款）。
权利要求4-5相对于对比文件2（US789012B）不具备创造性（专利法第22条第3款）。

审查员认为对比文件1公开了权利要求1的全部技术特征。`

	state := graph.PregelState{OAStateInput: oaText}
	out, err := parseOANode(context.Background(), state)
	if err != nil {
		t.Fatalf("parseOANode() error = %v", err)
	}

	// Verify rejection type detection
	rejectionType := out.GetString(OAStateRejectionType)
	if rejectionType == "" {
		t.Error("expected rejection type to be detected")
	}

	// Verify citations extraction
	citations, ok := out[OAStateCitations].([]CitedReference)
	if !ok {
		t.Fatal("expected OAStateCitations to be []CitedReference")
	}
	if len(citations) == 0 {
		t.Error("expected at least 1 citation extracted")
	}

	// Verify affected claims
	claims, ok := out[OAStateAffectedClaims].([]int)
	if !ok {
		t.Fatal("expected OAStateAffectedClaims to be []int")
	}
	if len(claims) == 0 {
		t.Error("expected at least 1 affected claim")
	}

	// Verify parsed struct
	parsed, ok := out[OAStateParsed].(*ParsedOfficeAction)
	if !ok || parsed == nil {
		t.Fatal("expected OAStateParsed to be *ParsedOfficeAction")
	}
}

func TestParseOANode_EmptyInput(t *testing.T) {
	state := graph.PregelState{OAStateInput: ""}
	_, err := parseOANode(context.Background(), state)
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParseOANode_InventivenessRejection(t *testing.T) {
	oaText := `权利要求1-5相对于对比文件1（CN111111A）和对比文件2（CN222222B）的结合不具备创造性。
审查员认为区别特征是本领域常规技术手段。`

	state := graph.PregelState{OAStateInput: oaText}
	out, err := parseOANode(context.Background(), state)
	if err != nil {
		t.Fatalf("parseOANode() error = %v", err)
	}

	rejectionType := out.GetString(OAStateRejectionType)
	if rejectionType != string(OaInventiveness) {
		t.Errorf("expected rejection type %q, got %q", string(OaInventiveness), rejectionType)
	}
}

func TestClassifyRejectionNode(t *testing.T) {
	tests := []struct {
		name           string
		rejectionType  string
		citations      []CitedReference
		affectedClaims []int
		wantStrategy   string
		wantTemplate   string
	}{
		{
			name:          "novelty → argument strategy",
			rejectionType: string(OaNovelty),
			wantStrategy:  "argument",
			wantTemplate:  "novelty-defense",
		},
		{
			name:          "inventiveness → argument strategy",
			rejectionType: string(OaInventiveness),
			wantStrategy:  "argument",
			wantTemplate:  "inventiveness-defense",
		},
		{
			name:          "clarity → amendment strategy",
			rejectionType: string(OaClarity),
			wantStrategy:  "amendment",
			wantTemplate:  "clarity-amendment",
		},
		{
			name:          "support → amendment strategy",
			rejectionType: string(OaSupport),
			wantStrategy:  "amendment",
			wantTemplate:  "clarity-amendment",
		},
		{
			name:          "scope → amendment strategy",
			rejectionType: string(OaScope),
			wantStrategy:  "amendment",
			wantTemplate:  "clarity-amendment",
		},
		{
			name:          "disclosure → argument strategy",
			rejectionType: string(OaDisclosure),
			wantStrategy:  "argument",
			wantTemplate:  "novelty-defense",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := &ParsedOfficeAction{
				RejectionType:  string(OaRejectionType(tt.rejectionType)),
				Citations:      tt.citations,
				AffectedClaims: tt.affectedClaims,
			}
			state := graph.PregelState{
				OAStateInput:          "test OA text",
				OAStateParsed:         parsed,
				OAStateRejectionType:  tt.rejectionType,
				OAStateCitations:      tt.citations,
				OAStateAffectedClaims: tt.affectedClaims,
			}

			out, err := classifyRejectionNode(context.Background(), state)
			if err != nil {
				t.Fatalf("classifyRejectionNode() error = %v", err)
			}

			if got := out.GetString(OAStateResponseStrategy); got != tt.wantStrategy {
				t.Errorf("strategy = %q, want %q", got, tt.wantStrategy)
			}
			if got := out.GetString(OAStateTemplateUsed); got != tt.wantTemplate {
				t.Errorf("template = %q, want %q", got, tt.wantTemplate)
			}
		})
	}
}

func TestAnalyzeClaimsNode(t *testing.T) {
	parsed := &ParsedOfficeAction{
		RejectionType: string(OaNovelty),
		Citations: []CitedReference{
			{DocumentNumber: "CN123456A", Relevancy: "X"},
			{DocumentNumber: "US789012B", Relevancy: "A"},
		},
		AffectedClaims: []int{1, 2, 3},
	}
	state := graph.PregelState{
		OAStateParsed:           parsed,
		OAStateRejectionType:    string(OaNovelty),
		OAStateResponseStrategy: "argument",
		OAStateCitations:        parsed.Citations,
		OAStateAffectedClaims:   parsed.AffectedClaims,
		OAStateInput:            "test OA text",
		OAStateTemplateUsed:     "novelty-defense",
	}

	out, err := analyzeClaimsNode(context.Background(), state)
	if err != nil {
		t.Fatalf("analyzeClaimsNode() error = %v", err)
	}

	amendments := out.GetString(OAStateClaimAmendments)
	if amendments == "" {
		t.Error("expected non-empty claim amendments")
	}
	if !strings.Contains(amendments, "权利要求修改对照表") {
		t.Error("expected claim amendment table header")
	}
	if !strings.Contains(amendments, "单独对比原则") {
		t.Error("expected novelty strategy guidance")
	}
	if !strings.Contains(amendments, "CN123456A") {
		t.Error("expected citation analysis for CN123456A")
	}
}

func TestDraftResponseNode(t *testing.T) {
	parsed := &ParsedOfficeAction{
		RejectionType:     string(OaInventiveness),
		Citations:         []CitedReference{{DocumentNumber: "CN123456A", Relevancy: "X"}},
		AffectedClaims:    []int{1},
		ExaminerArguments: []string{"对比文件1公开了本发明的全部技术特征。"},
	}
	state := graph.PregelState{
		OAStateParsed:           parsed,
		OAStateRejectionType:    string(OaInventiveness),
		OAStateResponseStrategy: "argument",
		OAStateTemplateUsed:     "inventiveness-defense",
		OAStateClaimAmendments:  "## 权利要求修改对照表\n\n无需修改\n",
		OAStateInput:            "test OA text",
	}

	out, err := draftResponseNode(context.Background(), state)
	if err != nil {
		t.Fatalf("draftResponseNode() error = %v", err)
	}

	draft := out.GetString(OAStateResponseDraft)
	if draft == "" {
		t.Error("expected non-empty response draft")
	}
	if !strings.Contains(draft, "审查意见答复书") {
		t.Error("expected response header")
	}
	if !strings.Contains(draft, "创造性") {
		t.Error("expected inventiveness reference")
	}
	if !strings.Contains(draft, "第一步：最接近的现有技术") {
		t.Error("expected three-step method analysis")
	}
	if !strings.Contains(draft, "人工审核提醒") {
		t.Error("expected human review notice")
	}
	if !strings.Contains(draft, "不构成正式法律意见") {
		t.Error("expected legal disclaimer")
	}
}

func TestFullOAResponsePipeline(t *testing.T) {
	oaText := `审查意见通知书

本申请涉及一种基于深度学习的图像识别方法。审查员认为：

1. 权利要求1-3相对于对比文件1（CN202410001A）不具备新颖性，不符合专利法第22条第2款的规定。
2. 权利要求4相对于对比文件1和对比文件2（CN202410002B）的结合不具备创造性，不符合专利法第22条第3款的规定。
3. 权利要求5不清楚，不符合专利法第26条第4款的规定。`

	g, err := BuildOAResponseGraph()
	if err != nil {
		t.Fatalf("BuildOAResponseGraph() error = %v", err)
	}

	state, err := g.Run(context.Background(), graph.PregelState{
		OAStateInput: oaText,
	})
	if err != nil {
		t.Fatalf("graph.Run() error = %v", err)
	}

	output := state.GetString(OAStateOutput)
	if output == "" {
		t.Fatal("expected non-empty output from full pipeline")
	}

	// Verify key sections exist in the output.
	sections := []string{
		"审查意见答复书",
		"审查意见概述",
		"权利要求修改对照表",
		"答复策略建议",
		"引用对比文件分析",
		"意见陈述",
		"人工审核提醒",
	}
	for _, section := range sections {
		if !strings.Contains(output, section) {
			t.Errorf("expected section %q in output", section)
		}
	}
}

func TestExtractExaminerArguments(t *testing.T) {
	text := "审查员认为对比文件1公开了本发明的技术特征。因此权利要求1不具备新颖性。"
	args := extractExaminerArguments(text)
	if len(args) == 0 {
		t.Error("expected at least 1 examiner argument")
	}
}

func TestDetermineResponseStrategy(t *testing.T) {
	tests := []struct {
		rejectionType string
		want          string
	}{
		{string(OaNovelty), "argument"},
		{string(OaInventiveness), "argument"},
		{string(OaClarity), "amendment"},
		{string(OaSupport), "amendment"},
		{string(OaScope), "amendment"},
		{string(OaDisclosure), "argument"},
		{string(OaFormal), "amendment"},
	}

	for _, tt := range tests {
		t.Run(tt.rejectionType, func(t *testing.T) {
			got := determineResponseStrategy(tt.rejectionType)
			if got != tt.want {
				t.Errorf("determineResponseStrategy(%q) = %q, want %q", tt.rejectionType, got, tt.want)
			}
		})
	}
}

func TestSelectOATemplate(t *testing.T) {
	tests := []struct {
		rejectionType string
		strategy      string
		want          string
	}{
		{string(OaNovelty), "argument", "novelty-defense"},
		{string(OaInventiveness), "argument", "inventiveness-defense"},
		{string(OaClarity), "amendment", "clarity-amendment"},
		{string(OaSupport), "amendment", "clarity-amendment"},
		{string(OaFormal), "amendment", "clarity-amendment"},
	}

	for _, tt := range tests {
		t.Run(tt.rejectionType, func(t *testing.T) {
			got := selectOATemplate(tt.rejectionType, tt.strategy)
			if got != tt.want {
				t.Errorf("selectOATemplate(%q, %q) = %q, want %q",
					tt.rejectionType, tt.strategy, got, tt.want)
			}
		})
	}
}

func TestApprovalGateNode(t *testing.T) {
	state := graph.PregelState{
		OAStateResponseDraft: "test draft content",
		OAStateOutput:        "test draft content",
	}

	out, err := approvalGateNode(context.Background(), state)
	if err != nil {
		t.Fatalf("approvalGateNode() error = %v", err)
	}

	if out.GetString(OAStateOutput) == "" {
		t.Error("expected output to be passed through")
	}
}

func TestApprovalGateNode_EmptyDraft(t *testing.T) {
	state := graph.PregelState{
		OAStateResponseDraft: "",
	}

	_, err := approvalGateNode(context.Background(), state)
	if err == nil {
		t.Error("expected error for empty draft")
	}
}

func TestBuildOAResponseGraphWithOpts_NoProvider(t *testing.T) {
	// Without provider, graph should be identical to BuildOAResponseGraph.
	g, err := BuildOAResponseGraphWithOpts()
	if err != nil {
		t.Fatalf("BuildOAResponseGraphWithOpts() error = %v", err)
	}
	if g == nil {
		t.Fatal("expected non-nil graph")
	}

	// Verify it runs without LLM enhancement.
	state, err := g.Run(context.Background(), graph.PregelState{
		OAStateInput: "审查员认为权利要求1不具备新颖性。",
	})
	if err != nil {
		t.Fatalf("graph.Run() error = %v", err)
	}
	output := state.GetString(OAStateOutput)
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if enhanced, ok := state[OAStateLLMEnhanced].(bool); ok && enhanced {
		t.Error("expected no LLM enhancement without provider")
	}
}

func TestOAEnhanceNode_NoopOnNilProvider(t *testing.T) {
	node := newOAEnhanceNode(nil)
	if node == nil {
		t.Fatal("newOAEnhanceNode(nil) should return non-nil node")
	}

	state := graph.PregelState{
		OAStateResponseDraft: "test draft",
		OAStateInput:         "test input",
	}
	out, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("noop enhance node error = %v", err)
	}

	// 增量语义：no-op 节点不触碰 draft，只标记 LLMEnhanced=false。
	if _, exists := out[OAStateResponseDraft]; exists {
		t.Error("no-op node should not touch the draft (incremental state)")
	}
	enhanced, ok := out[OAStateLLMEnhanced].(bool)
	if !ok || enhanced {
		t.Error("expected OAStateLLMEnhanced=false for nil provider")
	}
}

func TestOAEnhanceNode_WithNilProviderGraph(t *testing.T) {
	// BuildOAResponseGraphWithOpts with no provider should produce
	// the same output as BuildOAResponseGraph.
	g1, err := BuildOAResponseGraph()
	if err != nil {
		t.Fatalf("BuildOAResponseGraph() error = %v", err)
	}
	g2, err := BuildOAResponseGraphWithOpts()
	if err != nil {
		t.Fatalf("BuildOAResponseGraphWithOpts() error = %v", err)
	}

	state1, err1 := g1.Run(context.Background(), graph.PregelState{
		OAStateInput: "审查员认为权利要求1不具备专利法第22条第2款规定的新颖性。",
	})
	state2, err2 := g2.Run(context.Background(), graph.PregelState{
		OAStateInput: "审查员认为权利要求1不具备专利法第22条第2款规定的新颖性。",
	})

	if err1 != nil || err2 != nil {
		t.Fatalf("graph.Run() error g1=%v g2=%v", err1, err2)
	}
	if state1.GetString(OAStateOutput) != state2.GetString(OAStateOutput) {
		t.Error("expected identical output with and without opts (no provider)")
	}
}

// mockOARuleRetriever implements OARuleRetriever for testing.
type mockOARuleRetriever struct {
	articles []OALawArticle
	err      error
}

func (m *mockOARuleRetriever) RetrieveRules(ctx context.Context, rejectionType string) ([]OALawArticle, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.articles, nil
}

func TestRuleRetrievalNode_NilRetriever(t *testing.T) {
	node := newRuleRetrievalNode(nil)
	state := graph.PregelState{
		OAStateRejectionType: string(OaNovelty),
	}
	out, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("ruleRetrievalNode nil: %v", err)
	}
	// No retriever → no applicable rules.
	if out.GetString(OAStateApplicableRules) != "" {
		t.Error("should not have applicable rules when retriever is nil")
	}
}

func TestRuleRetrievalNode_WithRetriever(t *testing.T) {
	retriever := &mockOARuleRetriever{
		articles: []OALawArticle{
			{
				ArticleRef: "专利法第22条第2款",
				Title:      "新颖性",
				Content:    "新颖性，是指该发明或者实用新型不属于现有技术。",
				Source:     "专利法",
			},
			{
				ArticleRef: "审查指南第二部分第三章",
				Title:      "新颖性审查",
				Content:    "新颖性判断应当遵循单独对比原则。",
				Source:     "审查指南",
			},
		},
	}
	node := newRuleRetrievalNode(retriever)
	state := graph.PregelState{
		OAStateRejectionType: string(OaNovelty),
	}
	out, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("ruleRetrievalNode: %v", err)
	}
	rules := out.GetString(OAStateApplicableRules)
	if rules == "" {
		t.Fatal("expected applicable rules in state")
	}
	if !strings.Contains(rules, "专利法第22条第2款") {
		t.Error("should contain the article reference")
	}
	if !strings.Contains(rules, "单独对比原则") {
		t.Error("should contain guideline excerpt")
	}
}

func TestRuleRetrievalNode_RetrievalError(t *testing.T) {
	retriever := &mockOARuleRetriever{err: context.DeadlineExceeded}
	node := newRuleRetrievalNode(retriever)
	state := graph.PregelState{
		OAStateRejectionType: string(OaInventiveness),
	}
	out, err := node(context.Background(), state)
	if err != nil {
		t.Fatalf("retrieval error should not return error: %v", err)
	}
	if !graph.IsDegraded(out, OAStateApplicableRules) {
		t.Error("expected degraded mark on retrieval failure")
	}
}

func TestBuildOAResponseGraphWithOpts_WithRetriever(t *testing.T) {
	retriever := &mockOARuleRetriever{
		articles: []OALawArticle{
			{
				ArticleRef: "专利法第22条第3款",
				Title:      "创造性",
				Content:    "创造性，是指与现有技术相比具有突出的实质性特点和显著的进步。",
				Source:     "专利法",
			},
		},
	}
	g, err := BuildOAResponseGraphWithOpts(WithOARuleRetriever(retriever))
	if err != nil {
		t.Fatalf("BuildOAResponseGraphWithOpts: %v", err)
	}

	state, err := g.Run(context.Background(), graph.PregelState{
		OAStateInput: "审查员认为权利要求1-3相对于对比文件1（CN123456A）不具备创造性（专利法第22条第3款）。",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := state.GetString(OAStateOutput)
	if output == "" {
		t.Fatal("output should not be empty")
	}
	// The dynamically retrieved article should appear in the output.
	if !strings.Contains(output, "专利法第22条第3款") {
		t.Error("output should contain dynamically retrieved article reference")
	}
	if !strings.Contains(output, "适用法条") {
		t.Error("output should contain applicable rules section header")
	}
}

func TestBuildOAResponseGraphWithOpts_WithoutRetriever(t *testing.T) {
	// No retriever — backward compatibility.
	g, err := BuildOAResponseGraphWithOpts()
	if err != nil {
		t.Fatalf("BuildOAResponseGraphWithOpts: %v", err)
	}

	state, err := g.Run(context.Background(), graph.PregelState{
		OAStateInput: "审查员认为权利要求1不具备新颖性（专利法第22条第2款）。",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := state.GetString(OAStateOutput)
	if output == "" {
		t.Fatal("output should not be empty")
	}
	// No dynamic retrieval section should appear.
	if strings.Contains(output, "适用法条与审查指南") {
		t.Error("should not contain dynamic rules section when no retriever")
	}
}

// =============================================================================
// 多驳回类型（mixed grounds）测试
// =============================================================================

func TestClassifyRejectionNode_MultipleTypes(t *testing.T) {
	parsed := &ParsedOfficeAction{
		RejectionType:  string(OaNovelty),
		RejectionTypes: []OaRejectionType{OaNovelty, OaInventiveness, OaClarity},
	}
	state := graph.PregelState{
		OAStateInput:          "test OA text",
		OAStateParsed:         parsed,
		OAStateRejectionType:  string(OaNovelty),
		OAStateRejectionTypes: []OaRejectionType{OaNovelty, OaInventiveness, OaClarity},
	}

	out, err := classifyRejectionNode(context.Background(), state)
	if err != nil {
		t.Fatalf("classifyRejectionNode() error = %v", err)
	}

	// Primary (first) strategy/template preserved for backward compatibility.
	if got := out.GetString(OAStateResponseStrategy); got != "argument" {
		t.Errorf("primary strategy = %q, want argument", got)
	}
	if got := out.GetString(OAStateTemplateUsed); got != "novelty-defense" {
		t.Errorf("primary template = %q, want novelty-defense", got)
	}

	// Per-type strategy list.
	strategies, ok := out[OAStateStrategies].([]string)
	if !ok {
		t.Fatalf("expected OAStateStrategies []string, got %T", out[OAStateStrategies])
	}
	wantStrategies := []string{"argument", "argument", "amendment"}
	if len(strategies) != len(wantStrategies) {
		t.Fatalf("strategies = %v, want %v", strategies, wantStrategies)
	}
	for i, s := range wantStrategies {
		if strategies[i] != s {
			t.Errorf("strategies[%d] = %q, want %q", i, strategies[i], s)
		}
	}

	// Per-type template list.
	templates, ok := out[OAStateTemplates].([]string)
	if !ok {
		t.Fatalf("expected OAStateTemplates []string, got %T", out[OAStateTemplates])
	}
	wantTemplates := []string{"novelty-defense", "inventiveness-defense", "clarity-amendment"}
	for i, tmpl := range wantTemplates {
		if templates[i] != tmpl {
			t.Errorf("templates[%d] = %q, want %q", i, templates[i], tmpl)
		}
	}
}

func TestAnalyzeClaimsNode_MixedTypes(t *testing.T) {
	parsed := &ParsedOfficeAction{
		RejectionType:  string(OaInventiveness),
		RejectionTypes: []OaRejectionType{OaInventiveness, OaClarity},
		AffectedClaims: []int{1, 2, 3},
	}
	state := graph.PregelState{
		OAStateParsed:           parsed,
		OAStateRejectionType:    string(OaInventiveness),
		OAStateRejectionTypes:   []OaRejectionType{OaInventiveness, OaClarity},
		OAStateResponseStrategy: "argument",
		OAStateStrategies:       []string{"argument", "amendment"},
		OAStateCitations:        parsed.Citations,
		OAStateAffectedClaims:   parsed.AffectedClaims,
		OAStateInput:            "test OA text",
		OAStateTemplateUsed:     "inventiveness-defense",
	}

	out, err := analyzeClaimsNode(context.Background(), state)
	if err != nil {
		t.Fatalf("analyzeClaimsNode() error = %v", err)
	}

	amendments := out.GetString(OAStateClaimAmendments)
	if amendments == "" {
		t.Fatal("expected non-empty claim amendments")
	}
	// 创造性（争辩）不生成修改行，不清楚（修改）生成一行。
	if !strings.Contains(amendments, "| 驳回类型 | 修改类型 |") {
		t.Error("expected rejection type column in amendment table")
	}
	if !strings.Contains(amendments, "| 不清楚 | 澄清限定 | [原内容] | [建议修改] | 专利法第26条第4款（清楚） | 1, 2, 3 |") {
		t.Errorf("expected clarity amendment row with claims 1, 2, 3, got:\n%s", amendments)
	}
	if strings.Contains(amendments, "| — | 无需修改 |") {
		t.Errorf("argument+amendment mix should not show the no-amendment placeholder:\n%s", amendments)
	}
	// 每个驳回类型一段策略建议。
	if !strings.Contains(amendments, "### 创造性（策略：争辩）") {
		t.Error("expected inventiveness strategy block")
	}
	if !strings.Contains(amendments, "### 不清楚（策略：修改）") {
		t.Error("expected clarity strategy block")
	}
}

func TestDraftResponseBodies_Numbering(t *testing.T) {
	body := draftResponseBodies([]OaRejectionType{OaNovelty, OaInventiveness, OaClarity})

	for _, want := range []string{
		"### 一、关于新颖性（专利法第22条第2款）",
		"### 二、关于创造性（专利法第22条第3款）",
		"### 三、关于权利要求不清楚（专利法第26条第4款）",
		"### 结论",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in multi-type response body:\n%s", want, body)
		}
	}
	// 结论只出现一次。
	if strings.Count(body, "### 结论") != 1 {
		t.Errorf("expected exactly one conclusion, got %d", strings.Count(body, "### 结论"))
	}
}

func TestSummarizeStrategies(t *testing.T) {
	got := summarizeStrategies(
		[]OaRejectionType{OaNovelty, OaClarity},
		[]string{"argument", "amendment"},
	)
	want := "新颖性→争辩、不清楚→修改"
	if got != want {
		t.Errorf("summarizeStrategies() = %q, want %q", got, want)
	}
	if got := summarizeStrategies(nil, nil); got != "综合答复" {
		t.Errorf("empty summarizeStrategies() = %q, want 综合答复", got)
	}
}

func TestFullOAResponsePipeline_MixedTypes(t *testing.T) {
	oaText := `审查意见通知书

本申请涉及一种基于深度学习的图像识别方法。审查员认为：

1. 权利要求1-3相对于对比文件1（CN202410001A）不具备新颖性，不符合专利法第22条第2款的规定。
2. 权利要求4相对于对比文件1和对比文件2（CN202410002B）的结合不具备创造性，不符合专利法第22条第3款的规定。
3. 权利要求5不清楚，不符合专利法第26条第4款的规定。`

	g, err := BuildOAResponseGraph()
	if err != nil {
		t.Fatalf("BuildOAResponseGraph() error = %v", err)
	}

	state, err := g.Run(context.Background(), graph.PregelState{
		OAStateInput: oaText,
	})
	if err != nil {
		t.Fatalf("graph.Run() error = %v", err)
	}

	// 区间展开："权利要求1-3" 应展开为 1、2、3。
	claims, ok := state[OAStateAffectedClaims].([]int)
	if !ok {
		t.Fatal("expected OAStateAffectedClaims []int")
	}
	wantClaims := []int{1, 2, 3, 4, 5}
	if len(claims) != len(wantClaims) {
		t.Fatalf("affected claims = %v, want %v", claims, wantClaims)
	}
	for i, c := range wantClaims {
		if claims[i] != c {
			t.Errorf("claims[%d] = %d, want %d", i, claims[i], c)
		}
	}

	// 多驳回类型全部识别。
	types, ok := state[OAStateRejectionTypes].([]OaRejectionType)
	if !ok || len(types) != 3 {
		t.Fatalf("expected 3 rejection types, got %v", state[OAStateRejectionTypes])
	}

	output := state.GetString(OAStateOutput)
	if output == "" {
		t.Fatal("expected non-empty output")
	}

	// 三段意见陈述（每种驳回类型一段）+ 单一结论。
	for _, want := range []string{
		"### 一、关于新颖性（专利法第22条第2款）",
		"### 二、关于创造性（专利法第22条第3款）",
		"### 三、关于权利要求不清楚（专利法第26条第4款）",
		"### 结论",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output", want)
		}
	}
	// 策略摘要包含所有类型的映射。
	for _, want := range []string{"新颖性→争辩", "创造性→争辩", "不清楚→修改"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in strategy summary", want)
		}
	}
	// 清楚性驳回类型应有修改行（每类型一行，涉及权利要求列表展示）。
	if !strings.Contains(output, "| 不清楚 | 澄清限定 | [原内容] | [建议修改] | 专利法第26条第4款（清楚） | 1, 2, 3, 4, 5 |") {
		t.Errorf("expected clarity amendment row with all claims in output")
	}
}
