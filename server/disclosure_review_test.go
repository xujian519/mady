package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/domains"
)

// newAwaitingReviewTask 构造一个停留在 awaiting_review 状态的 disclosure 任务，
// 模拟 review_gate 中断后等待人工复核的现场。
func newAwaitingReviewTask(id string) *disclosureTask {
	return &disclosureTask{
		ID:       id,
		Status:   "awaiting_review",
		Progress: &DisclosureProgress{},
		Result: &disclosure.AnalysisReport{
			ID:         "report-1",
			ReportText: "新颖性初判：该交底书具备新颖性",
		},
		CreatedAt: time.Now(),
		DoneAt:    time.Now(),
		doneCh:    make(chan struct{}),
	}
}

// doReviewRequest 向复核端点发起一次请求并返回响应 recorder。
func doReviewRequest(t *testing.T, srv *Server, taskID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/disclosure/analyze/"+taskID+"/review", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// TestDisclosureReview_RecordsDecision 验证复核端点把人工决策留痕到
// ApprovalStore，并把任务与报告状态推进到已复核。
func TestDisclosureReview_RecordsDecision(t *testing.T) {
	srv := New(agentcore.Config{})
	defer srv.Close()
	store := domains.NewMemoryApprovalStore()
	srv.SetApprovalStore(store)

	dm := srv.initDisclosureManager()
	dm.tasks.Set("task-1", newAwaitingReviewTask("task-1"))

	rec := doReviewRequest(t, srv, "task-1", DisclosureReviewRequest{
		Decision:       "modified",
		ModifiedOutput: "新颖性初判：该交底书权利要求 1 具备新颖性，权利要求 2 不具备",
		Feedback:       "补充独权与从权的区分",
		CaseID:         "case-42",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 留痕校验：与 TUI /approve /reject 同一 RecordDecision 模式。
	records, err := store.List(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 approval record, got %d", len(records))
	}
	r0 := records[0]
	if r0.Decision != domains.DecisionModified || r0.State != domains.StateModified {
		t.Errorf("record = (%q, %q), want (modified, modified)", r0.Decision, r0.State)
	}
	if r0.TriggerKeyword != "disclosure_review" {
		t.Errorf("trigger = %q, want disclosure_review", r0.TriggerKeyword)
	}
	if r0.OriginalOutput == "" {
		t.Error("original output should carry the reviewed report text")
	}
	if r0.CaseID != "case-42" {
		t.Errorf("case id = %q, want case-42", r0.CaseID)
	}
	if r0.ModifiedOutput == "" || r0.Feedback == "" {
		t.Error("modified output and feedback should be persisted")
	}

	// 任务状态校验：status 推进为 reviewed，报告标记已经人工复核。
	task, _ := dm.getTask("task-1")
	task.mu.RLock()
	defer task.mu.RUnlock()
	if task.Status != "reviewed" {
		t.Errorf("task status = %q, want reviewed", task.Status)
	}
	if task.ReviewDecision != "modified" {
		t.Errorf("review decision = %q, want modified", task.ReviewDecision)
	}
	if !task.Result.ReviewedByHuman {
		t.Error("report should be marked ReviewedByHuman after review")
	}
}

// TestDisclosureReview_StatusVisibleViaGET 验证复核结论可通过状态轮询端点读取。
func TestDisclosureReview_StatusVisibleViaGET(t *testing.T) {
	srv := New(agentcore.Config{})
	defer srv.Close()
	srv.SetApprovalStore(domains.NewMemoryApprovalStore())

	dm := srv.initDisclosureManager()
	dm.tasks.Set("task-2", newAwaitingReviewTask("task-2"))

	if rec := doReviewRequest(t, srv, "task-2", DisclosureReviewRequest{Decision: "adopted"}); rec.Code != http.StatusOK {
		t.Fatalf("review: got %d: %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/disclosure/analyze/task-2", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d", rec.Code)
	}
	var status DisclosureTaskStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status.Status != "reviewed" || status.ReviewDecision != "adopted" {
		t.Errorf("status = (%q, %q), want (reviewed, adopted)", status.Status, status.ReviewDecision)
	}
}

// TestDisclosureReview_RejectsInvalidRequests 覆盖参数与状态校验分支。
func TestDisclosureReview_RejectsInvalidRequests(t *testing.T) {
	srv := New(agentcore.Config{})
	defer srv.Close()
	srv.SetApprovalStore(domains.NewMemoryApprovalStore())

	dm := srv.initDisclosureManager()
	dm.tasks.Set("task-3", newAwaitingReviewTask("task-3"))

	// 非法 decision → 400
	if rec := doReviewRequest(t, srv, "task-3", DisclosureReviewRequest{Decision: "maybe"}); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid decision: expected 400, got %d", rec.Code)
	}
	// 任务不存在 → 404
	if rec := doReviewRequest(t, srv, "no-such-task", DisclosureReviewRequest{Decision: "adopted"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown task: expected 404, got %d", rec.Code)
	}
	// 重复复核（已 reviewed）→ 409
	if rec := doReviewRequest(t, srv, "task-3", DisclosureReviewRequest{Decision: "adopted"}); rec.Code != http.StatusOK {
		t.Fatalf("first review: got %d", rec.Code)
	}
	if rec := doReviewRequest(t, srv, "task-3", DisclosureReviewRequest{Decision: "rejected"}); rec.Code != http.StatusConflict {
		t.Errorf("second review: expected 409, got %d", rec.Code)
	}
}

// TestDisclosureReview_NoStore 验证未配置 ApprovalStore 时端点返回 503 而非静默丢数据。
func TestDisclosureReview_NoStore(t *testing.T) {
	srv := New(agentcore.Config{})
	defer srv.Close()

	dm := srv.initDisclosureManager()
	dm.tasks.Set("task-4", newAwaitingReviewTask("task-4"))

	if rec := doReviewRequest(t, srv, "task-4", DisclosureReviewRequest{Decision: "adopted"}); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 without approval store, got %d", rec.Code)
	}

	// 未留痕成功时任务状态必须保持 awaiting_review，允许配置 store 后重试。
	task, _ := dm.getTask("task-4")
	task.mu.RLock()
	defer task.mu.RUnlock()
	if task.Status != "awaiting_review" {
		t.Errorf("task status = %q, want awaiting_review (unchanged)", task.Status)
	}
}
