package legal

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

func TestIdentifyStatutes(t *testing.T) {
	tests := []struct {
		facts    string
		contains string
	}{
		{"某公司侵犯了我的专利权", "专利法"},
		{"合同违约导致损失", "民法典"},
		{"商标侵权纠纷案件", "商标法"},
		{"著作权被侵犯", "著作权法"},
		{"商业秘密被泄露", "反不正当竞争法"},
		{"普通民事纠纷", "需进一步检索"},
	}

	for _, tt := range tests {
		t.Run(tt.facts, func(t *testing.T) {
			stats := identifyStatutes(tt.facts)
			found := false
			for _, s := range stats {
				if strings.Contains(s, tt.contains) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("identifyStatutes(%q) should contain %q, got %v", tt.facts, tt.contains, stats)
			}
		})
	}
}

func TestStatuteNode(t *testing.T) {
	state := graph.PregelState{
		StateCaseFacts: "原告起诉被告侵犯专利权，要求赔偿经济损失。",
	}

	out, err := statuteNode(context.Background(), state)
	if err != nil {
		t.Fatalf("statuteNode: %v", err)
	}

	statutes, ok := out[StateStatutes].([]string)
	if !ok || len(statutes) == 0 {
		t.Fatal("expected statutes")
	}
	if out.GetString(StateCaseFacts) == "" {
		t.Error("case facts should be preserved")
	}
}

func TestStatuteNode_EmptyFacts(t *testing.T) {
	_, err := statuteNode(context.Background(), graph.PregelState{})
	if err == nil {
		t.Error("expected error for empty facts")
	}
}

func TestCaseSearchNode(t *testing.T) {
	state := graph.PregelState{
		StateCaseFacts: "合同违约纠纷，被告未按约定履行义务。",
		StateStatutes:  []string{"民法典"},
	}

	out, err := caseSearchNode(context.Background(), state)
	if err != nil {
		t.Fatalf("caseSearchNode: %v", err)
	}

	// 判例检索尚未实现，应标记降级。
	if !graph.IsDegraded(out, StateSimilarCases) {
		t.Fatal("expected degraded similar cases (not implemented)")
	}
	if mark := graph.GetDegradationMark(out, StateSimilarCases); mark == nil {
		t.Fatal("expected degradation mark")
	} else if mark.Reason != graph.DegradationNotImplemented {
		t.Errorf("expected not_implemented, got %s", mark.Reason)
	}
}

func TestCompareNode(t *testing.T) {
	state := graph.PregelState{
		StateCaseFacts:    "原告与被告签订技术开发合同，被告未按约定交付成果。",
		StateStatutes:     []string{"民法典", "专利法"},
		StateSimilarCases: []string{"(2023)最高法知民终字第XX号"},
	}

	out, err := compareNode(context.Background(), state)
	if err != nil {
		t.Fatalf("compareNode: %v", err)
	}

	comparison := out.GetString(StateComparison)
	if !strings.Contains(comparison, "法律分析") {
		t.Error("comparison should have analysis header")
	}
	if !strings.Contains(comparison, "民法典") {
		t.Error("comparison should reference statutes")
	}
}

func TestConcludeNode(t *testing.T) {
	state := graph.PregelState{
		StateComparison: "## 法律分析\n\n适用民法典合同编相关规定。",
		StateStatutes:   []string{"民法典"},
	}

	out, err := concludeNode(context.Background(), state)
	if err != nil {
		t.Fatalf("concludeNode: %v", err)
	}

	conclusion := out.GetString(StateConclusion)
	if !strings.Contains(conclusion, "法律分析报告") {
		t.Error("conclusion should be a report")
	}
	if !strings.Contains(conclusion, "不构成正式法律意见") {
		t.Error("conclusion must include legal disclaimer")
	}
	if !strings.Contains(conclusion, "诉讼策略") {
		t.Error("conclusion should include strategy considerations")
	}
}

func TestBuildComparisonGraph(t *testing.T) {
	g, err := BuildComparisonGraph()
	if err != nil {
		t.Fatalf("BuildComparisonGraph: %v", err)
	}
	if g == nil {
		t.Fatal("graph should not be nil")
	}

	// Run end-to-end.
	state := graph.PregelState{
		StateCaseFacts: "被告未经许可，在相同商品上使用与原告注册商标近似的标识，造成消费者混淆，构成商标侵权和不正当竞争。",
	}

	finalState, err := g.Run(context.Background(), state)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := finalState.GetString(StateOutput)
	if output == "" {
		t.Error("final output should not be empty")
	}
	if !strings.Contains(output, "法律分析报告") {
		t.Error("output should be a legal analysis report")
	}
	if !strings.Contains(output, "不构成正式法律意见") {
		t.Error("output must include disclaimer")
	}

	t.Logf("Output: %s", output[:min(len(output), 300)])
}
