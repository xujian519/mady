package reasoning

import (
	"encoding/json"
	"testing"
)

func TestFactBlackboard_Empty(t *testing.T) {
	bb := NewFactBlackboard("case-1", CasePatentability, "H04W")
	if len(bb.Facts()) != 0 {
		t.Fatalf("expected 0 facts, got %d", len(bb.Facts()))
	}
	if len(bb.ReasoningChains()) != 0 {
		t.Fatalf("expected 0 chains")
	}
	if len(bb.RuleConstraints()) != 0 {
		t.Fatalf("expected 0 constraints")
	}
	if len(bb.ArticleJudgments()) != 0 {
		t.Fatalf("expected 0 judgments")
	}
	if bb.Plan() != nil {
		t.Fatalf("expected nil plan")
	}
	if bb.CaseType != CasePatentability {
		t.Fatalf("case type mismatch")
	}
	if bb.TechnicalField != "H04W" {
		t.Fatalf("field mismatch")
	}
}

func TestFactBlackboard_AddGetFact(t *testing.T) {
	bb := NewFactBlackboard("case-1", CasePatentability, "")
	f := FactEntry{ID: "f1", Source: "user_text", Content: "权利要求1包含特征A、B、C", Confidence: 0.9}
	bb.AddFact(f)
	if len(bb.Facts()) != 1 {
		t.Fatalf("expected 1 fact")
	}
	got, ok := bb.GetFact("f1")
	if !ok || got.Content != f.Content {
		t.Fatalf("GetFact mismatch: %+v ok=%v", got, ok)
	}
}

func TestFactBlackboard_GetFact_Missing(t *testing.T) {
	bb := NewFactBlackboard("case-1", CasePatentability, "")
	if _, ok := bb.GetFact("nope"); ok {
		t.Fatal("expected ok=false for missing fact")
	}
}

func TestFactBlackboard_DiscardFact(t *testing.T) {
	bb := NewFactBlackboard("case-1", CasePatentability, "")
	bb.AddFact(FactEntry{ID: "f1", Source: "user_text", Content: "旧内容", Confidence: 0.8})
	bb.DiscardFact("f1")
	got, ok := bb.GetFact("f1")
	if !ok {
		t.Fatal("fact should still exist after discard")
	}
	if !got.IsDiscarded() {
		t.Fatal("fact should be discarded")
	}
	if len(bb.ActiveFacts()) != 0 {
		t.Fatal("ActiveFacts should exclude discarded")
	}
}

func TestFactBlackboard_ReasoningChains(t *testing.T) {
	bb := NewFactBlackboard("case-1", CasePatentability, "")
	bb.AddReasoningChain(ReasoningChain{ID: "rc1", FactRef: "f1", Confidence: 0.85})
	if len(bb.ReasoningChains()) != 1 {
		t.Fatalf("expected 1 chain")
	}
	bb.ClearReasoningChains()
	if len(bb.ReasoningChains()) != 0 {
		t.Fatalf("expected 0 chains after clear")
	}
}

func TestFactBlackboard_RuleConstraints(t *testing.T) {
	bb := NewFactBlackboard("case-1", CasePatentability, "")
	bb.AddRuleConstraint(RuleConstraint{ArticleID: "A22.3", ArticleName: "创造性", Requirement: ReqMust})
	if len(bb.RuleConstraints()) != 1 {
		t.Fatalf("expected 1 constraint")
	}
	bb.SetRuleConstraints([]RuleConstraint{{ArticleID: "A22.2", Requirement: ReqShould}})
	if len(bb.RuleConstraints()) != 1 || bb.RuleConstraints()[0].ArticleID != "A22.2" {
		t.Fatalf("SetRuleConstraints failed")
	}
}

func TestFactBlackboard_ArticleJudgments(t *testing.T) {
	bb := NewFactBlackboard("case-1", CasePatentability, "")
	j := ArticleJudgment{ArticleID: "A22.3", ArticleName: "创造性", Confidence: ConfidenceHigh}
	bb.SetArticleJudgment("A22.3", j)
	if len(bb.ArticleJudgments()) != 1 {
		t.Fatalf("expected 1 judgment")
	}
	got, ok := bb.GetArticleJudgment("A22.3")
	if !ok || got.Confidence != ConfidenceHigh {
		t.Fatalf("GetArticleJudgment mismatch")
	}
}

func TestFactBlackboard_Plan(t *testing.T) {
	bb := NewFactBlackboard("case-1", CasePatentability, "")
	bb.SetPlan(ExecutionPlan{
		Steps:     []ExecutionPlanStep{{Order: 1, Description: "撰写", ToolName: "draft"}},
		Artifacts: []string{"claims.md"},
	})
	if bb.Plan() == nil || len(bb.Plan().Steps) != 1 {
		t.Fatalf("plan mismatch")
	}
}

func TestFactBlackboard_Lock(t *testing.T) {
	bb := NewFactBlackboard("case-1", CasePatentability, "")
	if bb.Locked {
		t.Fatal("should start unlocked")
	}
	bb.Lock()
	if !bb.Locked {
		t.Fatal("should be locked")
	}
}

func TestFactBlackboard_SerializationRoundTrip(t *testing.T) {
	bb := NewFactBlackboard("case-1", CasePatentability, "G06F")
	bb.AddFact(FactEntry{ID: "f1", Source: "file", Content: "交底书内容", Confidence: 1.0, ExtractedAt: "2026-06-18T10:00:00Z"})

	data, err := json.Marshal(bb)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	restored := NewFactBlackboard("", CaseGeneralLegal, "")
	if err := json.Unmarshal(data, restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(restored.Facts()) != 1 {
		t.Fatalf("restored facts: %d", len(restored.Facts()))
	}
	if restored.TechnicalField != "G06F" {
		t.Fatalf("restored field: %s", restored.TechnicalField)
	}
	got, ok := restored.GetFact("f1")
	if !ok || got.Content != "交底书内容" {
		t.Fatalf("restored fact content mismatch")
	}
	if restored.articleJudgments == nil {
		// map should be initialized (non-nil) after restore, safe to write
		t.Fatal("restored articleJudgments map should be non-nil")
	}
}
