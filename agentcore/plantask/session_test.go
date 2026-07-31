package plantask

import (
	"errors"
	"testing"
	"time"
)

// TestTransition_Matrix 验证迁移矩阵全部合法组合。
func TestTransition_Matrix(t *testing.T) {
	legal := map[Status][]Status{
		StatusPlanning:         {StatusAwaitingApproval, StatusCanceled, StatusExpired},
		StatusAwaitingApproval: {StatusPlanning, StatusExecuting, StatusCanceled, StatusExpired},
		StatusExecuting:        {StatusAwaitingFeedback, StatusFinished, StatusCanceled, StatusExpired},
		StatusAwaitingFeedback: {StatusExecuting, StatusReplanning, StatusCanceled, StatusExpired},
		StatusReplanning:       {StatusExecuting, StatusCanceled, StatusExpired},
	}
	all := []Status{
		StatusPlanning, StatusAwaitingApproval, StatusExecuting,
		StatusAwaitingFeedback, StatusReplanning, StatusFinished,
		StatusCanceled, StatusExpired,
	}

	for _, from := range all {
		for _, to := range all {
			s := NewSession("s1", "case1", "patentability")
			s.Status = from
			err := s.Transition(to)
			expectOK := contains(legal[from], to)
			if expectOK && err != nil {
				t.Errorf("expected %s -> %s legal, got error: %v", from, to, err)
			}
			if !expectOK && err == nil {
				t.Errorf("expected %s -> %s illegal, got nil error", from, to)
			}
			if expectOK && !errors.Is(err, ErrInvalidTransition) && err == nil && s.Status != to {
				t.Errorf("expected status %s after legal transition, got %s", to, s.Status)
			}
		}
	}
}

// TestTransition_InvalidTransitionError 验证非法迁移返回类型化错误。
func TestTransition_InvalidTransitionError(t *testing.T) {
	s := NewSession("s1", "case1", "patentability")
	s.Status = StatusExecuting
	err := s.Transition(StatusPlanning)
	var ite *InvalidTransitionError
	if !errors.As(err, &ite) {
		t.Fatalf("expected *InvalidTransitionError, got %v", err)
	}
	if ite.From != StatusExecuting || ite.To != StatusPlanning {
		t.Errorf("unexpected error fields: %+v", ite)
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition in chain")
	}
}

// TestTransition_Expired 验证过期会话拒绝一切迁移。
func TestTransition_Expired(t *testing.T) {
	s := NewSession("s1", "case1", "patentability")
	s.Status = StatusExpired
	if err := s.Transition(StatusPlanning); !errors.Is(err, ErrSessionExpired) {
		t.Errorf("expected ErrSessionExpired, got %v", err)
	}
}

// TestTransition_LazyExpiry 验证 ExpiresAt 过期后自动迁移到 Expired。
func TestTransition_LazyExpiry(t *testing.T) {
	s := NewSession("s1", "case1", "patentability")
	past := time.Now().Add(-time.Hour)
	s.SetExpiresAt(past)
	if err := s.Transition(StatusAwaitingApproval); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired on expired session, got %v", err)
	}
	if s.Status != StatusExpired {
		t.Errorf("expected status Expired after lazy expiry, got %s", s.Status)
	}
}

// TestFeedback_Append 验证反馈追加与审计留痕。
func TestFeedback_Append(t *testing.T) {
	s := NewSession("s1", "case1", "patentability")
	s.AddFeedback("增加美国同族检索", "step_2")
	if len(s.FeedbackLog) != 1 {
		t.Fatalf("expected 1 feedback entry, got %d", len(s.FeedbackLog))
	}
	s.AddFeedback("重跑:step1", "")
	if len(s.FeedbackLog) != 2 {
		t.Fatalf("expected 2 feedback entries, got %d", len(s.FeedbackLog))
	}
}

// TestMarkCompleted 验证步骤完成标记去重。
func TestMarkCompleted(t *testing.T) {
	s := NewSession("s1", "case1", "patentability")
	s.MarkCompleted("step_1")
	s.MarkCompleted("step_1")
	if len(s.CompletedIDs) != 1 {
		t.Errorf("expected 1 completed id, got %d", len(s.CompletedIDs))
	}
}

// TestClone 验证深拷贝隔离。
func TestClone(t *testing.T) {
	s := NewSession("s1", "case1", "patentability")
	s.TaskIDs = []string{"1", "2"}
	cp := s.Clone()
	cp.TaskIDs[0] = "99"
	if s.TaskIDs[0] != "1" {
		t.Errorf("clone should isolate TaskIDs slice")
	}
}

func contains(list []Status, s Status) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
