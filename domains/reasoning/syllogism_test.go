package reasoning

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRuleAssertion_MissingRefs(t *testing.T) {
	bb := NewFactBlackboard("c1", CasePatentability, "")
	s := Syllogism{ID: "s1"} // no refs
	err := RuleAssertion(bb, &s)
	if !errors.Is(err, ErrUnreferencedConclusion) {
		t.Fatalf("expected ErrUnreferencedConclusion, got %v", err)
	}
	if s.Validated {
		t.Fatal("should not be validated on failure")
	}
}

func TestRuleAssertion_FactNotFound(t *testing.T) {
	bb := NewFactBlackboard("c1", CasePatentability, "")
	bb.AddRuleConstraint(RuleConstraint{ArticleID: "A22.3"})
	s := Syllogism{ID: "s1", FactRef: "ghost", ArticleRef: "A22.3"}
	err := RuleAssertion(bb, &s)
	if err == nil {
		t.Fatal("expected error for missing fact")
	}
}

func TestRuleAssertion_ArticleNotFound(t *testing.T) {
	bb := NewFactBlackboard("c1", CasePatentability, "")
	bb.AddFact(FactEntry{ID: "f1"})
	s := Syllogism{ID: "s1", FactRef: "f1", ArticleRef: "ghost"}
	err := RuleAssertion(bb, &s)
	if err == nil {
		t.Fatal("expected error for missing article")
	}
}

func TestRuleAssertion_ValidViaConstraint(t *testing.T) {
	bb := NewFactBlackboard("c1", CasePatentability, "")
	bb.AddFact(FactEntry{ID: "f1", Content: "特征A"})
	bb.AddRuleConstraint(RuleConstraint{ArticleID: "A22.3", ArticleName: "创造性"})
	s := Syllogism{ID: "s1", FactRef: "f1", ArticleRef: "A22.3", Conclusion: "具备创造性"}
	if err := RuleAssertion(bb, &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !s.Validated {
		t.Fatal("should be validated")
	}
}

func TestRuleAssertion_ValidViaJudgment(t *testing.T) {
	bb := NewFactBlackboard("c1", CasePatentability, "")
	bb.AddFact(FactEntry{ID: "f1"})
	bb.SetArticleJudgment("A22.2", ArticleJudgment{ArticleID: "A22.2"})
	s := Syllogism{ID: "s1", FactRef: "f1", ArticleRef: "A22.2"}
	if err := RuleAssertion(bb, &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyllogismBuilder_BuildOK(t *testing.T) {
	bb := NewFactBlackboard("c1", CasePatentability, "")
	bb.AddFact(FactEntry{ID: "f1", Content: "区别特征为X"})
	bb.AddRuleConstraint(RuleConstraint{ArticleID: "A22.3", ArticleName: "创造性"})

	s, err := NewSyllogismBuilder("s1").
		Major("创造性条款", "A22.3", "发明应当具备突出的实质性特点和显著进步").
		Minor("案件事实", "f1", "权利要求与对比文件的区别特征为X").
		ConclusionText("该区别特征非显而易见，具备创造性", 0.9).
		Build(bb)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if !s.Validated || s.Confidence != 0.9 {
		t.Fatalf("unexpected syllogism: %+v", s)
	}
	if s.MajorPremise.Source != SourceStatute || s.MinorPremise.Source != SourceCaseFact {
		t.Fatalf("premise source mismatch")
	}
}

func TestSyllogismBuilder_BuildFailsUnreferenced(t *testing.T) {
	bb := NewFactBlackboard("c1", CasePatentability, "")
	_, err := NewSyllogismBuilder("s1").
		Major("法条", "A22.3", "...").
		Minor("事实", "f1", "...").
		ConclusionText("结论", 0.5).
		Build(bb)
	if err == nil {
		t.Fatal("expected validation failure (refs not on board)")
	}
}

func TestAssertChain(t *testing.T) {
	bb := NewFactBlackboard("c1", CasePatentability, "")
	bb.AddFact(FactEntry{ID: "f1"})
	bb.AddFact(FactEntry{ID: "f2"})
	bb.AddRuleConstraint(RuleConstraint{ArticleID: "A1"})

	chains := []Syllogism{
		{ID: "s1", FactRef: "f1", ArticleRef: "A1"},
		{ID: "s2", FactRef: "f2", ArticleRef: "A1"},
	}
	if err := AssertChain(bb, chains); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !chains[0].Validated || !chains[1].Validated {
		t.Fatal("all chains should be validated")
	}
}

func TestAssertChain_FailsAt(t *testing.T) {
	bb := NewFactBlackboard("c1", CasePatentability, "")
	bb.AddFact(FactEntry{ID: "f1"})
	bb.AddRuleConstraint(RuleConstraint{ArticleID: "A1"})

	chains := []Syllogism{
		{ID: "s1", FactRef: "f1", ArticleRef: "A1"},
		{ID: "s2", FactRef: "ghost", ArticleRef: "A1"}, // bad
	}
	err := AssertChain(bb, chains)
	if err == nil {
		t.Fatal("expected error on second chain")
	}
}

func TestSyllogism_SerializationRoundTrip(t *testing.T) {
	s := Syllogism{
		ID:           "s1",
		MajorPremise: Premise{Label: "大前提", Source: SourceStatute, RefID: "A22.3", Content: "法条"},
		MinorPremise: Premise{Label: "小前提", Source: SourceCaseFact, RefID: "f1", Content: "事实"},
		Conclusion:   "结论",
		FactRef:      "f1",
		ArticleRef:   "A22.3",
		Confidence:   0.9,
		Validated:    true,
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Syllogism
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != s.ID || got.Confidence != s.Confidence || !got.Validated {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}
