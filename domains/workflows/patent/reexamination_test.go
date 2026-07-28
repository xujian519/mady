package patent

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

// -----------------------------------------------------------------------------
// reexamParseDecisionNode
// -----------------------------------------------------------------------------

func TestReexamParseDecisionNode_EmptyInput(t *testing.T) {
	_, err := reexamParseDecisionNode(context.Background(), graph.PregelState{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestReexamParseDecisionNode_BasicInput(t *testing.T) {
	input := `驳回决定编号：12345
决定日期：2024年3月15日
申请人：某科技有限公司
发明名称：一种智能图像处理系统
申请号：CN202310XXXXXX.X

审查员认为该申请不符合专利法第22条第2款关于新颖性的规定以及第22条第3款关于创造性的规定。
对比文件1：CN10XXXXXXXA，公开了类似的技术方案。`

	out, err := reexamParseDecisionNode(context.Background(), graph.PregelState{
		ReexamStateInput: input,
	})
	if err != nil {
		t.Fatalf("reexamParseDecisionNode: %v", err)
	}

	info, ok := out[ReexamStateDecisionInfo].(ReexamDecisionInfo)
	if !ok {
		t.Fatal("expected ReexamDecisionInfo")
	}
	if info.DecisionNumber == "" {
		t.Error("decision number should not be empty")
	}
	if info.ApplicantName == "" {
		t.Error("applicant name should not be empty")
	}

	grounds, ok := out[ReexamStateGrounds].([]ReexamGround)
	if !ok || len(grounds) < 2 {
		t.Fatalf("expected >= 2 grounds, got %d", len(grounds))
	}

	if pt := out.GetString(ReexamStatePatentType); pt != "invention" {
		t.Errorf("patent type = %q, want invention", pt)
	}
}

func TestExtractDecisionInfo_PartialInput(t *testing.T) {
	text := "驳回决定编号：98765\n申请号：2024XXXX"
	info := extractDecisionInfo(text)
	if info.DecisionNumber != "98765" {
		t.Errorf("decision number = %q, want 98765", info.DecisionNumber)
	}
}

// -----------------------------------------------------------------------------
// identifyReexaminationGrounds
// -----------------------------------------------------------------------------

func TestIdentifyReexaminationGrounds_AllTypes(t *testing.T) {
	text := `不符合专利法第22条第2款新颖性
不符合专利法第22条第3款创造性
不符合专利法第26条第3款充分公开
不符合专利法第26条第4款权利要求清楚
违反专利法第33条修改超范围`

	grounds := identifyReexaminationGrounds(text)
	if len(grounds) < 5 {
		t.Fatalf("expected >= 5 grounds, got %d", len(grounds))
	}

	seen := make(map[ReexamGroundType]bool)
	for _, g := range grounds {
		seen[g.Type] = true
	}
	for _, gt := range []ReexamGroundType{
		ReexamGroundNovelty, ReexamGroundInventiveness, ReexamGroundDisclosure,
		ReexamGroundClarity, ReexamGroundAmendment,
	} {
		if !seen[gt] {
			t.Errorf("ground type %s not identified", gt)
		}
	}
}

func TestIdentifyReexaminationGrounds_UtilityModelSubject(t *testing.T) {
	text := "该申请不符合专利法第2条第3款关于实用新型客体的规定"
	grounds := identifyReexaminationGrounds(text)
	found := false
	for _, g := range grounds {
		if g.Type == ReexamGroundSubject {
			found = true
			break
		}
	}
	if !found {
		t.Error("should identify utility model subject ground")
	}
}

// -----------------------------------------------------------------------------
// detectPatentType
// -----------------------------------------------------------------------------

func TestDetectPatentType_UtilityModel(t *testing.T) {
	pt := detectPatentType("该实用新型专利...", nil)
	if pt != "utility_model" {
		t.Errorf("want utility_model, got %s", pt)
	}
}

func TestDetectPatentType_Invention(t *testing.T) {
	pt := detectPatentType("该发明专利...", nil)
	if pt != "invention" {
		t.Errorf("want invention, got %s", pt)
	}
}

// -----------------------------------------------------------------------------
// reexamClassifyGroundsNode (utility model filtering)
// -----------------------------------------------------------------------------

func TestReexamClassifyGroundsNode_FilterInventiveness(t *testing.T) {
	state := graph.PregelState{
		ReexamStatePatentType: "utility_model",
		ReexamStateGrounds: []ReexamGround{
			{Type: ReexamGroundNovelty, Article: "A22.2", Description: "novelty"},
			{Type: ReexamGroundInventiveness, Article: "A22.3", Description: "inventiveness"},
			{Type: ReexamGroundSubject, Article: "A2.3", Description: "subject"},
		},
	}

	out, err := reexamClassifyGroundsNode(context.Background(), state)
	if err != nil {
		t.Fatalf("classifyGroundsNode: %v", err)
	}

	grounds, _ := out[ReexamStateGrounds].([]ReexamGround)
	for _, g := range grounds {
		if g.Type == ReexamGroundInventiveness {
			t.Error("inventiveness should be filtered for utility models")
		}
	}
}

// -----------------------------------------------------------------------------
// reexamDraftNode
// -----------------------------------------------------------------------------

func TestReexamDraftNode_Basic(t *testing.T) {
	state := graph.PregelState{
		ReexamStateDecisionInfo: ReexamDecisionInfo{
			ApplicantName: "测试公司",
			PatentTitle:   "测试装置",
		},
		ReexamStateGrounds: []ReexamGround{
			{Type: ReexamGroundNovelty, Article: "专利法第22条第2款", Description: "新颖性缺陷"},
		},
		ReexamStatePatentType: "invention",
	}

	out, err := reexamDraftNode(context.Background(), state)
	if err != nil {
		t.Fatalf("draftNode: %v", err)
	}

	draft := out.GetString(ReexamStateDraft)
	if !strings.Contains(draft, "复审请求书") {
		t.Error("draft should contain request title")
	}
	if !strings.Contains(draft, "第41条") {
		t.Error("draft should cite Article 41")
	}
	if !strings.Contains(draft, "单独对比") {
		t.Error("draft should mention novelty defense strategy")
	}
}

// -----------------------------------------------------------------------------
// reexamRuleCheckNode
// -----------------------------------------------------------------------------

func TestReexamRuleCheckNode_Basic(t *testing.T) {
	out, err := reexamRuleCheckNode(context.Background(), graph.PregelState{
		ReexamStateDraft: "复审理由：新颖性缺陷，采用单独对比原则",
		ReexamStateGrounds: []ReexamGround{
			{Type: ReexamGroundNovelty, Article: "A22.2", Description: "新颖性缺陷"},
		},
	})
	if err != nil {
		t.Fatalf("ruleCheckNode: %v", err)
	}
	if out.GetString(ReexamStateRuleCheck) == "" {
		t.Error("rule check report should not be empty")
	}
	if out.GetString(ReexamStateRuleVerdict) == "" {
		t.Error("rule verdict should not be empty")
	}
}

// -----------------------------------------------------------------------------
// BuildReexaminationGraph (end-to-end)
// -----------------------------------------------------------------------------

func TestBuildReexaminationGraph_EndToEnd(t *testing.T) {
	compiled, err := BuildReexaminationGraph()
	if err != nil {
		t.Fatalf("BuildReexaminationGraph: %v", err)
	}

	input := `驳回决定编号：2024-001
决定日期：2024年5月10日
申请人：某科技有限公司
发明名称：一种基于深度学习的图像识别方法
申请号：CN202310123456.X

审查员认为：
1. 权利要求1-5不符合专利法第22条第2款关于新颖性的规定。
2. 权利要求1-5不符合专利法第22条第3款关于创造性的规定。
对比文件1：CN109XXXXXXA。`

	out, err := compiled.Run(context.Background(), graph.PregelState{
		ReexamStateInput: input,
	})
	if err != nil {
		t.Fatalf("graph run: %v", err)
	}

	output := out.GetString(ReexamStateOutput)
	if output == "" {
		t.Fatal("output should not be empty")
	}
	if !strings.Contains(output, "复审请求书") {
		t.Error("output should contain request title")
	}
	if !strings.Contains(output, "第41条") {
		t.Error("output should cite Article 41")
	}
	if !strings.Contains(output, "3 个月") {
		t.Error("output should mention 3-month deadline")
	}
}

func TestBuildReexaminationGraph_EndToEnd_UtilityModel(t *testing.T) {
	compiled, err := BuildReexaminationGraph()
	if err != nil {
		t.Fatalf("BuildReexaminationGraph: %v", err)
	}

	input := `驳回决定编号：2024-002
申请人：某制造公司
实用新型名称：一种新型连接装置
申请号：CN202320XXXXXX.X

审查员认为该实用新型不符合专利法第2条第3款关于实用新型客体的规定。`

	out, err := compiled.Run(context.Background(), graph.PregelState{
		ReexamStateInput: input,
	})
	if err != nil {
		t.Fatalf("graph run: %v", err)
	}

	output := out.GetString(ReexamStateOutput)
	if !strings.Contains(output, "实用新型") {
		t.Error("output should identify as utility model")
	}
	if !strings.Contains(output, "客体") {
		t.Error("output should address subject matter ground")
	}
}
