package plantask

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/tasklist"
)

// fakeGate 是 Gate 接口的测试实现。
type fakeGate struct{ active atomic.Bool }

func (g *fakeGate) Activate()      { g.active.Store(true) }
func (g *fakeGate) Deactivate()    { g.active.Store(false) }
func (g *fakeGate) IsActive() bool { return g.active.Load() }

// newTestExtension 构造带内存存储与假门控的扩展。
func newTestExtension(t *testing.T) (*Extension, *fakeGate, *tasklist.MemoryStore) {
	t.Helper()
	gate := &fakeGate{}
	taskStore := tasklist.NewMemoryStore()
	ext, err := NewExtension(Config{
		Store:        NewMemoryStore(),
		TaskStore:    taskStore,
		Gate:         gate,
		NewSessionID: func(caseID string) string { return caseID + "_sess" },
		DefaultExpiry: func() *SessionExpiry {
			v := DefaultExpirySettings()
			return &v
		}(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ext, gate, taskStore
}

// invokeTool 通过工具 Func 直接调用。
// NewTypedTool 将结构体结果序列化为 JSON 字符串返回，故此处反序列化回 TResult。
func invokeTool[TArgs, TResult any](t *testing.T, tool *agentcore.Tool, args TArgs) (TResult, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	var zero TResult
	res, err := tool.Func(context.Background(), raw)
	if err != nil {
		return zero, err
	}
	switch r := res.(type) {
	case string:
		if err := json.Unmarshal([]byte(r), &zero); err != nil {
			t.Fatalf("unmarshal tool result %q: %v", r, err)
		}
		return zero, nil
	default:
		out, ok := r.(TResult)
		if !ok {
			t.Fatalf("unexpected result type %T", res)
		}
		return out, nil
	}
}

// TestPlanSubmit_Flow 验证 plan_submit 全流程：创建会话、同步任务、进入等待批准。
func TestPlanSubmit_Flow(t *testing.T) {
	ext, gate, taskStore := newTestExtension(t)
	ctx := context.Background()
	gate.Activate() // 模拟 AutoEnterPlanning 已激活门控

	result, err := invokeTool[PlanSubmitArgs, PlanSubmitResult](t, ext.Tools()[0], PlanSubmitArgs{
		CaseID:   "case1",
		CaseType: "patentability",
		Steps: []PlanStepInput{
			{Order: 1, Strategy: "chain", Description: "检索现有技术"},
			{Order: 2, Strategy: "chain", Description: "比对权利要求"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(StatusAwaitingApproval) {
		t.Errorf("expected awaiting_approval, got %s", result.Status)
	}
	if len(result.TaskIDs) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(result.TaskIDs))
	}
	if !gate.IsActive() {
		t.Error("gate must stay active in awaiting approval (still read-only)")
	}
	tasks, _ := taskStore.List(ctx, false)
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks in store, got %d", len(tasks))
	}
	sess, err := ext.cfg.Store.Load(ctx, result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Plan.Steps) != 2 || sess.Plan.Steps[0].Hash == "" {
		t.Error("plan snapshot not stored correctly")
	}
}

// TestPlanApprove_DeactivatesGate 验证批准后门控解除。
func TestPlanApprove_DeactivatesGate(t *testing.T) {
	ext, gate, _ := newTestExtension(t)

	sub, _ := invokeTool[PlanSubmitArgs, PlanSubmitResult](t, ext.Tools()[0], PlanSubmitArgs{
		CaseID: "case1", CaseType: "patentability",
		Steps: []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索"}},
	})
	appr, err := invokeTool[PlanApproveArgs, PlanApproveResult](t, ext.Tools()[1], PlanApproveArgs{SessionID: sub.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if appr.Status != string(StatusExecuting) {
		t.Errorf("expected executing, got %s", appr.Status)
	}
	if gate.IsActive() {
		t.Error("gate must be deactivated after approval")
	}
}

// TestPlanReject_BackToPlanning 验证驳回回到规划态并记录理由。
func TestPlanReject_BackToPlanning(t *testing.T) {
	ext, gate, _ := newTestExtension(t)
	ctx := context.Background()

	sub, _ := invokeTool[PlanSubmitArgs, PlanSubmitResult](t, ext.Tools()[0], PlanSubmitArgs{
		CaseID: "case1", CaseType: "patentability",
		Steps: []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索"}},
	})
	rej, err := invokeTool[PlanRejectArgs, PlanRejectResult](t, ext.Tools()[2], PlanRejectArgs{
		SessionID: sub.SessionID,
		Reason:    "检索范围太窄",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rej.Status != string(StatusPlanning) {
		t.Errorf("expected planning, got %s", rej.Status)
	}
	if !gate.IsActive() {
		t.Error("gate must stay active in planning")
	}
	sess, _ := ext.cfg.Store.Load(ctx, sub.SessionID)
	if len(sess.FeedbackLog) != 1 || sess.FeedbackLog[0].Text != "检索范围太窄" {
		t.Error("reject reason not recorded in feedback log")
	}
}

// TestPlanRevise_BackToPlanning 验证修订意图回规划态并可重新提交。
func TestPlanRevise_BackToPlanning(t *testing.T) {
	ext, _, _ := newTestExtension(t)
	ctx := context.Background()

	sub, _ := invokeTool[PlanSubmitArgs, PlanSubmitResult](t, ext.Tools()[0], PlanSubmitArgs{
		CaseID: "case1", CaseType: "patentability",
		Steps: []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索"}},
	})
	rev, err := invokeTool[PlanReviseArgs, PlanReviseResult](t, ext.Tools()[3], PlanReviseArgs{
		SessionID:    sub.SessionID,
		ReviseIntent: "增加美国同族检索步骤",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rev.Status != string(StatusPlanning) {
		t.Errorf("expected planning, got %s", rev.Status)
	}
	sess, _ := ext.cfg.Store.Load(ctx, sub.SessionID)
	if sess.ReviseIntent != "增加美国同族检索步骤" {
		t.Error("revise intent not stored")
	}
	if _, err := invokeTool[PlanSubmitArgs, PlanSubmitResult](t, ext.Tools()[0], PlanSubmitArgs{
		SessionID: sub.SessionID, CaseID: "case1", CaseType: "patentability",
		Steps: []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索（含美国同族）"}},
	}); err != nil {
		t.Errorf("resubmit after revise failed: %v", err)
	}
}

// TestPlanApprove_InvalidState 验证非等待批准状态批准报错。
func TestPlanApprove_InvalidState(t *testing.T) {
	ext, _, _ := newTestExtension(t)
	sub, _ := invokeTool[PlanSubmitArgs, PlanSubmitResult](t, ext.Tools()[0], PlanSubmitArgs{
		CaseID: "case1", CaseType: "patentability",
		Steps: []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索"}},
	})
	if _, err := invokeTool[PlanApproveArgs, PlanApproveResult](t, ext.Tools()[1], PlanApproveArgs{SessionID: sub.SessionID}); err != nil {
		t.Fatal(err)
	}
	_, err := invokeTool[PlanApproveArgs, PlanApproveResult](t, ext.Tools()[1], PlanApproveArgs{SessionID: sub.SessionID})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

// TestPlanSubmit_EmptySteps 验证空步骤报错。
func TestPlanSubmit_EmptySteps(t *testing.T) {
	ext, _, _ := newTestExtension(t)
	_, err := invokeTool[PlanSubmitArgs, PlanSubmitResult](t, ext.Tools()[0], PlanSubmitArgs{
		CaseID: "case1", CaseType: "patentability",
	})
	if !errors.Is(err, ErrPlanEmpty) {
		t.Errorf("expected ErrPlanEmpty, got %v", err)
	}
}

// TestPlanReject_EmptyReason 验证空理由报错。
func TestPlanReject_EmptyReason(t *testing.T) {
	ext, _, _ := newTestExtension(t)
	sub, _ := invokeTool[PlanSubmitArgs, PlanSubmitResult](t, ext.Tools()[0], PlanSubmitArgs{
		CaseID: "case1", CaseType: "patentability",
		Steps: []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索"}},
	})
	_, err := invokeTool[PlanRejectArgs, PlanRejectResult](t, ext.Tools()[2], PlanRejectArgs{SessionID: sub.SessionID})
	if !errors.Is(err, ErrFeedbackEmpty) {
		t.Errorf("expected ErrFeedbackEmpty, got %v", err)
	}
}

// TestNoActiveSession 验证无效会话 ID。
func TestNoActiveSession(t *testing.T) {
	ext, _, _ := newTestExtension(t)
	_, err := invokeTool[PlanApproveArgs, PlanApproveResult](t, ext.Tools()[1], PlanApproveArgs{SessionID: "nope"})
	if !errors.Is(err, ErrNoActiveSession) {
		t.Errorf("expected ErrNoActiveSession, got %v", err)
	}
}

// TestStatusChangedEvent 验证状态迁移事件发射。
func TestStatusChangedEvent(t *testing.T) {
	ext, _, _ := newTestExtension(t)
	agent := agentcore.New(agentcore.Config{})
	if err := ext.Init(context.Background(), agent); err != nil {
		t.Fatal(err)
	}
	var got atomic.Int32
	agent.On(agentcore.EventPlanTaskStatusChanged, func(_ agentcore.Event) { got.Add(1) })

	if _, err := invokeTool[PlanSubmitArgs, PlanSubmitResult](t, ext.Tools()[0], PlanSubmitArgs{
		CaseID: "case1", CaseType: "patentability",
		Steps: []PlanStepInput{{Order: 1, Strategy: "chain", Description: "检索"}},
	}); err != nil {
		t.Fatal(err)
	}
	agent.EventBus().Drain()
	if got.Load() != 1 {
		t.Errorf("expected 1 status event, got %d", got.Load())
	}
}
