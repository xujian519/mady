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
			got := determineResponseStrategy(tt.rejectionType, nil)
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

	// Should pass through unchanged.
	draft := out.GetString(OAStateResponseDraft)
	if draft != "test draft" {
		t.Error("expected draft to pass through unchanged")
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
