package compiler

import (
	"errors"
	"testing"
)

func TestReviewQueue_EnqueueAndDequeue(t *testing.T) {
	q := NewReviewQueue(nil)
	c1 := RuleCandidate{ID: "c1", Status: CandidateDraft}
	c2 := RuleCandidate{ID: "c2", Status: CandidateDraft}

	added := q.Enqueue(c1, c2)
	if added != 2 {
		t.Errorf("expected 2 added, got %d", added)
	}
	if q.Pending() != 2 {
		t.Errorf("expected 2 pending, got %d", q.Pending())
	}

	first, ok := q.Dequeue()
	if !ok {
		t.Fatal("expected to dequeue")
	}
	if first.ID != "c1" {
		t.Errorf("expected c1, got %s", first.ID)
	}

	second, ok := q.Dequeue()
	if !ok {
		t.Fatal("expected to dequeue")
	}
	if second.ID != "c2" {
		t.Errorf("expected c2, got %s", second.ID)
	}

	_, ok = q.Dequeue()
	if ok {
		t.Error("expected empty queue")
	}
}

func TestReviewQueue_SkipNonDraft(t *testing.T) {
	q := NewReviewQueue(nil)
	c := RuleCandidate{ID: "c1", Status: CandidateApproved}
	added := q.Enqueue(c)
	if added != 0 {
		t.Errorf("expected 0 added for non-draft, got %d", added)
	}
}

func TestReviewQueue_List(t *testing.T) {
	q := NewReviewQueue(nil)
	q.Enqueue(
		RuleCandidate{ID: "a", Status: CandidateDraft},
		RuleCandidate{ID: "b", Status: CandidateDraft},
	)
	list := q.List()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].ID != "a" || list[1].ID != "b" {
		t.Errorf("unexpected order: %s, %s", list[0].ID, list[1].ID)
	}
	if q.Pending() != 2 {
		t.Error("List should not consume the queue")
	}
}

func TestReviewQueue_RunShadowEval_Success(t *testing.T) {
	fn := func(c RuleCandidate) (ShadowEvalResult, error) {
		return ShadowEvalResult{Passed: true, Score: 0.92, Detail: "no regression"}, nil
	}
	q := NewReviewQueue(fn)
	c := RuleCandidate{ID: "c1", Status: CandidateDraft}
	err := q.RunShadowEval(&c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.ShadowPassed {
		t.Error("expected shadow passed")
	}
}

func TestReviewQueue_RunShadowEval_Error(t *testing.T) {
	fn := func(c RuleCandidate) (ShadowEvalResult, error) {
		return ShadowEvalResult{}, errors.New("eval service down")
	}
	q := NewReviewQueue(fn)
	c := RuleCandidate{ID: "c1", Status: CandidateDraft}
	err := q.RunShadowEval(&c)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReviewQueue_RunShadowEval_NoFunc(t *testing.T) {
	q := NewReviewQueue(nil)
	c := RuleCandidate{ID: "c1", Status: CandidateDraft}
	err := q.RunShadowEval(&c)
	if err == nil {
		t.Fatal("expected error for nil shadow func")
	}
}

func TestReviewQueue_ReviewSession_Approved(t *testing.T) {
	fn := func(c RuleCandidate) (ShadowEvalResult, error) {
		return ShadowEvalResult{Passed: true, Score: 0.95}, nil
	}
	q := NewReviewQueue(fn)
	c := RuleCandidate{
		ID:          "c1",
		Status:      CandidateDraft,
		Samples:     10,
		SuccessRate: 0.9,
	}
	result, err := q.ReviewSession(&c, true, "approved by reviewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Ready {
		t.Errorf("expected ready, reasons: %v", result.Reasons)
	}
	if !c.HumanApproved {
		t.Error("expected human approved")
	}
	if !c.ShadowPassed {
		t.Error("expected shadow passed")
	}
	if c.Status != CandidateApproved {
		t.Errorf("expected approved status, got %s", c.Status)
	}
}

func TestReviewQueue_ReviewSession_Rejected(t *testing.T) {
	fn := func(c RuleCandidate) (ShadowEvalResult, error) {
		return ShadowEvalResult{Passed: true, Score: 0.95}, nil
	}
	q := NewReviewQueue(fn)
	c := RuleCandidate{
		ID:          "c1",
		Status:      CandidateDraft,
		Samples:     10,
		SuccessRate: 0.9,
	}
	result, _ := q.ReviewSession(&c, false, "rule too broad")
	if result.Ready {
		t.Error("expected not ready for rejected candidate")
	}
	if c.Status != CandidateRejected {
		t.Errorf("expected rejected status, got %s", c.Status)
	}
}

func TestReviewQueue_ReviewSession_ShadowFail(t *testing.T) {
	fn := func(c RuleCandidate) (ShadowEvalResult, error) {
		return ShadowEvalResult{Passed: false, Score: 0.3, Detail: "regression detected"}, nil
	}
	q := NewReviewQueue(fn)
	c := RuleCandidate{
		ID:          "c1",
		Status:      CandidateDraft,
		Samples:     10,
		SuccessRate: 0.9,
	}
	result, _ := q.ReviewSession(&c, true, "approved despite shadow fail")
	if result.Ready {
		t.Error("expected not ready when shadow eval failed")
	}
}

func TestReviewQueue_DrainApproved(t *testing.T) {
	q := NewReviewQueue(nil)
	q.Enqueue(
		RuleCandidate{ID: "a", Status: CandidateDraft, HumanApproved: true},
		RuleCandidate{ID: "b", Status: CandidateDraft},
		RuleCandidate{ID: "c", Status: CandidateDraft, HumanApproved: true},
	)
	approved := q.DrainApproved()
	if len(approved) != 2 {
		t.Fatalf("expected 2 approved, got %d", len(approved))
	}
	if q.Pending() != 1 {
		t.Errorf("expected 1 remaining, got %d", q.Pending())
	}
}

func TestReviewQueue_EmptyDequeue(t *testing.T) {
	q := NewReviewQueue(nil)
	_, ok := q.Dequeue()
	if ok {
		t.Error("expected false for empty queue")
	}
}
