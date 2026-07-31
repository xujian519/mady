package plantask

import (
	"context"
	"errors"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/tasklist"
)

// recorderReplanner 是确定性 Replanner 测试实现：
// 用给定新步骤 + ReplanMerge 合并，写回会话（recorder 记录调用）。
type recorderReplanner struct {
	calls    int
	newSteps []StepSnapshot
}

func (r *recorderReplanner) Replan(_ context.Context, s *PlanTaskSession, feedback string) ([]string, []string, error) {
	r.calls++
	skip, removed := ReplanMerge(s.Plan.Steps, s.CompletedIDs, r.newSteps, feedback)
	s.Plan = PlanSnapshot{Steps: r.newSteps}
	kept := make([]string, 0, len(r.newSteps))
	for _, st := range r.newSteps {
		if skip[st.Hash] {
			kept = append(kept, st.Hash)
		}
	}
	s.CompletedIDs = kept
	return sliceKeys(skip), sliceKeys(removed), nil
}

func sliceKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// newLoopExtension 构造带 recorder replanner 的扩展。
func newLoopExtension(t *testing.T, newSteps []StepSnapshot) (*Extension, *recorderReplanner, *fakeGate) {
	t.Helper()
	gate := &fakeGate{}
	rr := &recorderReplanner{newSteps: newSteps}
	ext, err := NewExtension(Config{
		Store:        NewMemoryStore(),
		TaskStore:    tasklist.NewMemoryStore(),
		Gate:         gate,
		Replanner:    rr,
		NewSessionID: func(caseID string) string { return caseID + "_sess" },
	})
	if err != nil {
		t.Fatal(err)
	}
	return ext, rr, gate
}

// submitAndApprove 走完 submit → approve，返回会话 ID。
func submitAndApprove(t *testing.T, ext *Extension, steps []PlanStepInput) string {
	t.Helper()
	sub, err := invokeTool[PlanSubmitArgs, PlanSubmitResult](t, ext.Tools()[0], PlanSubmitArgs{
		CaseID: "case1", CaseType: "patentability", Steps: steps,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invokeTool[PlanApproveArgs, PlanApproveResult](t, ext.Tools()[1], PlanApproveArgs{SessionID: sub.SessionID}); err != nil {
		t.Fatal(err)
	}
	return sub.SessionID
}

// TestInterrupt_Transitions 验证中断工具：会话迁移 + 返回 InterruptError。
func TestInterrupt_Transitions(t *testing.T) {
	ext, _, _ := newLoopExtension(t, nil)
	sid := submitAndApprove(t, ext, []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索"}})

	_, err := invokeTool[WorkflowInterruptArgs, WorkflowInterruptResult](t, ext.Tools()[4], WorkflowInterruptArgs{
		SessionID: sid,
		Reason:    "用户请求暂停",
	})
	if !errors.Is(err, agentcore.ErrInterrupt) {
		t.Fatalf("expected ErrInterrupt, got %v", err)
	}
	sess, _ := ext.cfg.Store.Load(context.Background(), sid)
	if sess.Status != StatusAwaitingFeedback {
		t.Errorf("expected awaiting_feedback, got %s", sess.Status)
	}
	if sess.Interrupt == nil || sess.Interrupt.Reason != "用户请求暂停" {
		t.Error("interrupt context not recorded")
	}
}

// TestInterrupt_InvalidState 验证非执行态中断报错。
func TestInterrupt_InvalidState(t *testing.T) {
	ext, _, _ := newLoopExtension(t, nil)
	sub, _ := invokeTool[PlanSubmitArgs, PlanSubmitResult](t, ext.Tools()[0], PlanSubmitArgs{
		CaseID: "case1", CaseType: "patentability",
		Steps: []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索"}},
	})
	_, err := invokeTool[WorkflowInterruptArgs, WorkflowInterruptResult](t, ext.Tools()[4], WorkflowInterruptArgs{SessionID: sub.SessionID})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

// TestResume_FromFeedback 验证无改动直接续跑。
func TestResume_FromFeedback(t *testing.T) {
	ext, _, _ := newLoopExtension(t, nil)
	sid := submitAndApprove(t, ext, []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索"}})
	if _, err := invokeTool[WorkflowInterruptArgs, WorkflowInterruptResult](t, ext.Tools()[4], WorkflowInterruptArgs{SessionID: sid}); err == nil {
		t.Fatal("expected interrupt error")
	}
	res, err := invokeTool[WorkflowResumeArgs, WorkflowResumeResult](t, ext.Tools()[5], WorkflowResumeArgs{SessionID: sid})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(StatusExecuting) {
		t.Errorf("expected executing, got %s", res.Status)
	}
}

// TestFeedback_Replan 验证反馈 → replan 合并 → 回执行态。
func TestFeedback_Replan(t *testing.T) {
	s1 := stepSnapshot(1, "检索")
	s2 := stepSnapshot(2, "比对")
	s3 := stepSnapshot(3, "结论")
	ext, rr, _ := newLoopExtension(t, []StepSnapshot{s1, s2, s3})

	sid := submitAndApprove(t, ext, []PlanStepInput{
		{Order: 1, Strategy: "chain", Description: "检索"},
		{Order: 2, Strategy: "chain", Description: "比对"},
		{Order: 3, Strategy: "chain", Description: "结论"},
	})
	// 模拟执行：完成步骤 1、2。
	sess, _ := ext.cfg.Store.Load(context.Background(), sid)
	sess.MarkCompleted(s1.Hash)
	sess.MarkCompleted(s2.Hash)
	_ = ext.cfg.Store.Save(context.Background(), sess)

	if _, err := invokeTool[WorkflowInterruptArgs, WorkflowInterruptResult](t, ext.Tools()[4], WorkflowInterruptArgs{SessionID: sid}); err == nil {
		t.Fatal("expected interrupt error")
	}
	res, err := invokeTool[WorkflowFeedbackArgs, WorkflowFeedbackResult](t, ext.Tools()[6], WorkflowFeedbackArgs{
		SessionID: sid,
		Feedback:  "重跑:检索",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(StatusExecuting) {
		t.Errorf("expected executing after replan, got %s", res.Status)
	}
	if rr.calls != 1 {
		t.Errorf("expected 1 replan call, got %d", rr.calls)
	}
	// 重跑的步骤 1 在 removed，步骤 2 在 skip。
	has := func(list []string, v string) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	if !has(res.RemovedHashes, s1.Hash) {
		t.Error("step 1 must be in removed set (explicit rerun)")
	}
	if !has(res.SkipHashes, s2.Hash) {
		t.Error("step 2 must be in skip set (unchanged)")
	}
	// 会话 CompletedIDs 只剩步骤 2。
	sess2, _ := ext.cfg.Store.Load(context.Background(), sid)
	if len(sess2.CompletedIDs) != 1 || sess2.CompletedIDs[0] != s2.Hash {
		t.Errorf("completed after replan = %v, want [%s]", sess2.CompletedIDs, s2.Hash)
	}
}

// TestFeedback_Empty 验证空反馈报错。
func TestFeedback_Empty(t *testing.T) {
	ext, _, _ := newLoopExtension(t, nil)
	sid := submitAndApprove(t, ext, []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索"}})
	if _, err := invokeTool[WorkflowInterruptArgs, WorkflowInterruptResult](t, ext.Tools()[4], WorkflowInterruptArgs{SessionID: sid}); err == nil {
		t.Fatal("expected interrupt error")
	}
	_, err := invokeTool[WorkflowFeedbackArgs, WorkflowFeedbackResult](t, ext.Tools()[6], WorkflowFeedbackArgs{SessionID: sid})
	if !errors.Is(err, ErrFeedbackEmpty) {
		t.Errorf("expected ErrFeedbackEmpty, got %v", err)
	}
}

// TestFeedback_NoReplanner 验证无 Replanner 时停在 Replanning。
func TestFeedback_NoReplanner(t *testing.T) {
	ext, _, _ := newTestExtension(t)
	sid := submitAndApprove(t, ext, []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索"}})
	if _, err := invokeTool[WorkflowInterruptArgs, WorkflowInterruptResult](t, ext.Tools()[4], WorkflowInterruptArgs{SessionID: sid}); err == nil {
		t.Fatal("expected interrupt error")
	}
	res, err := invokeTool[WorkflowFeedbackArgs, WorkflowFeedbackResult](t, ext.Tools()[6], WorkflowFeedbackArgs{
		SessionID: sid, Feedback: "加一步",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(StatusReplanning) {
		t.Errorf("expected replanning without replanner, got %s", res.Status)
	}
}
