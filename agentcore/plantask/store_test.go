package plantask

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// newTestSession 构造一个测试会话。
func newTestSession(id, caseID string, status Status) *PlanTaskSession {
	s := NewSession(id, caseID, "patentability")
	s.Status = status
	return s
}

// TestMemoryStore_Crud 验证内存存储的 Save/Load/Delete。
func TestMemoryStore_Crud(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	s := newTestSession("s1", "case1", StatusAwaitingApproval)
	if err := st.Save(ctx, s); err != nil {
		t.Fatal(err)
	}
	got, err := st.Load(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusAwaitingApproval {
		t.Errorf("unexpected status: %s", got.Status)
	}
	if err := st.Delete(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load(ctx, "s1"); err == nil {
		t.Error("expected not-found error after delete")
	}
}

// TestMemoryStore_ListPending 验证只列出未终态会话。
func TestMemoryStore_ListPending(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	_ = st.Save(ctx, newTestSession("s1", "case1", StatusPlanning))
	_ = st.Save(ctx, newTestSession("s2", "case1", StatusFinished))
	_ = st.Save(ctx, newTestSession("s3", "case2", StatusAwaitingFeedback))

	pending, err := st.ListPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending sessions, got %d", len(pending))
	}
}

// TestMemoryStore_ListByCase 验证按案件过滤。
func TestMemoryStore_ListByCase(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	_ = st.Save(ctx, newTestSession("s1", "case1", StatusPlanning))
	_ = st.Save(ctx, newTestSession("s2", "case1", StatusFinished))
	_ = st.Save(ctx, newTestSession("s3", "case2", StatusPlanning))

	got, err := st.ListByCase(ctx, "case1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions for case1, got %d", len(got))
	}
}

// TestFileStore_Crud 验证文件存储持久化与恢复。
func TestFileStore_Crud(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s := newTestSession("s1", "case1", StatusAwaitingApproval)
	s.AddFeedback("需要修改", "")
	if err := st.Save(ctx, s); err != nil {
		t.Fatal(err)
	}

	// 重新打开（模拟进程重启）。
	st2, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st2.Load(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusAwaitingApproval || len(got.FeedbackLog) != 1 {
		t.Errorf("restore mismatch: status=%s feedbacks=%d", got.Status, len(got.FeedbackLog))
	}
}

// TestFileStore_PathTraversal 验证会话 ID 不逃逸目录。
func TestFileStore_PathTraversal(t *testing.T) {
	ctx := context.Background()
	st, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.Load(ctx, "../evil")
	if err == nil {
		t.Error("expected error for path-traversal ID")
	}
}

// TestFileStore_ListPending 验证文件存储的 pending 列表。
func TestFileStore_ListPending(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	st, err := NewFileStore(filepath.Join(base, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Save(ctx, newTestSession("s1", "case1", StatusPlanning))
	_ = st.Save(ctx, newTestSession("s2", "case1", StatusFinished))

	pending, err := st.ListPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if _, err := os.Stat(filepath.Join(base, "sessions", "s1.json")); err != nil {
		t.Error("expected session file on disk")
	}
}
