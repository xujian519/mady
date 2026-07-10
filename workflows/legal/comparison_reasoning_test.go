package legal

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/domains/reasoning"
	"github.com/xujian519/mady/graph"
)

func TestBuildComparisonGraphWithReasoning(t *testing.T) {
	compiled, bb, err := BuildComparisonGraphWithReasoning("case-001", reasoning.CaseGeneralLegal)
	if err != nil {
		t.Fatalf("BuildComparisonGraphWithReasoning: %v", err)
	}
	if compiled == nil || bb == nil {
		t.Fatal("compiled graph and blackboard must not be nil")
	}

	state := graph.PregelState{
		StateCaseFacts: "被告未经许可，在相同商品上使用与原告注册商标近似的标识，造成消费者混淆，构成商标侵权和不正当竞争。",
	}

	final, err := compiled.Run(context.Background(), state)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := final.GetString(StateOutput)
	if output == "" {
		t.Fatal("output must not be empty")
	}

	// The output must include the auditable syllogism section.
	if !strings.Contains(output, "三段论推理链") {
		t.Error("output should contain the syllogism reasoning chain section")
	}
	if !strings.Contains(output, "大前提") || !strings.Contains(output, "小前提") || !strings.Contains(output, "结论") {
		t.Error("output should contain major/minor/conclusion of a syllogism")
	}
	if !strings.Contains(output, "已校验") {
		t.Error("output should mark validated syllogisms")
	}
	if !strings.Contains(output, "推理审计") {
		t.Error("output should contain the reasoning audit summary")
	}
}

func TestReasoningGraphBlackboardAuditable(t *testing.T) {
	compiled, bb, err := BuildComparisonGraphWithReasoning("case-002", reasoning.CaseInfringement)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	_, err = compiled.Run(context.Background(), graph.PregelState{
		StateCaseFacts: "原告起诉被告侵犯其发明专利权，制造销售落入权利要求保护范围的产品。",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Blackboard should hold the case facts and at least one rule constraint.
	if _, ok := bb.GetFact("case_facts"); !ok {
		t.Error("blackboard should contain the case_facts entry")
	}
	constraints := bb.RuleConstraints()
	if len(constraints) == 0 {
		t.Fatal("blackboard should contain rule constraints for identified laws")
	}
	foundPatent := false
	for _, c := range constraints {
		if c.ArticleID == "专利法" {
			foundPatent = true
		}
	}
	if !foundPatent {
		t.Error("expected 专利法 rule constraint on blackboard")
	}

	// The technical field should be detected from the facts.
	if bb.TechnicalField != "专利" {
		t.Errorf("expected technical field 专利, got %q", bb.TechnicalField)
	}

	// Reasoning chains recorded on the blackboard must be validated.
	chains := bb.ReasoningChains()
	if len(chains) == 0 {
		t.Fatal("expected reasoning chains on the blackboard")
	}
	for _, c := range chains {
		if c.FactRef == "" {
			t.Error("every reasoning chain must reference a fact")
		}
	}

	// The blackboard should be locked after conclusion.
	if !bb.Locked {
		t.Error("blackboard should be locked after conclude")
	}
}

func TestFilterStatutes(t *testing.T) {
	out := filterStatutes([]string{"专利法", "需进一步检索适用法律", "", "  "})
	if len(out) != 1 || out[0] != "专利法" {
		t.Fatalf("expected only 专利法, got %v", out)
	}
}

func TestDetectTechnicalField(t *testing.T) {
	cases := map[string]string{
		"侵犯专利权":  "专利",
		"注册商标侵权": "商标",
		"著作权被侵犯": "著作权",
		"商业秘密泄露": "反不正当竞争",
		"普通合同违约": "",
	}
	for facts, want := range cases {
		if got := detectTechnicalField(facts); got != want {
			t.Errorf("detectTechnicalField(%q) = %q, want %q", facts, got, want)
		}
	}
}

func TestReasoningGraphNoStatutes(t *testing.T) {
	// Facts that match no keyword set → placeholder statute, no syllogisms,
	// but the graph should still complete without error.
	compiled, bb, err := BuildComparisonGraphWithReasoning("case-003", reasoning.CaseGeneralLegal)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	final, err := compiled.Run(context.Background(), graph.PregelState{
		StateCaseFacts: "一件普通的邻里琐事。",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	output := final.GetString(StateOutput)
	if !strings.Contains(output, "需进一步检索") {
		t.Error("output should indicate further statute search is needed")
	}
	if len(bb.ReasoningChains()) != 0 {
		t.Errorf("expected no reasoning chains, got %d", len(bb.ReasoningChains()))
	}
}
