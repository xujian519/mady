package guardrails

import (
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestDeferredPersistQueue(t *testing.T) {
	q := NewDeferredPersistQueue()

	// Store two messages.
	q.Store(1, agentcore.Message{Role: "assistant", Content: "分析结果1"})
	q.Store(2, agentcore.Message{Role: "assistant", Content: "分析结果2"})

	if q.Len() != 2 {
		t.Errorf("Len: expected 2, got %d", q.Len())
	}

	// Commit msg 1.
	msg, ok := q.Commit(1)
	if !ok {
		t.Fatal("expected msg 1 to be commitable")
	}
	if msg.Content != "分析结果1" {
		t.Errorf("Content: expected '分析结果1', got %q", msg.Content)
	}
	if q.Has(1) {
		t.Error("msg 1 should be removed after commit")
	}

	// Discard msg 2.
	q.Discard(2)
	if q.Has(2) {
		t.Error("msg 2 should be removed after discard")
	}

	if q.Len() != 0 {
		t.Errorf("Len: expected 0 after clear, got %d", q.Len())
	}
}

func TestDeferredPersistQueueCommitMissing(t *testing.T) {
	q := NewDeferredPersistQueue()
	_, ok := q.Commit(99)
	if ok {
		t.Error("committing non-existent message should return false")
	}
}

func TestDeferredPersistQueueOverwrite(t *testing.T) {
	q := NewDeferredPersistQueue()
	q.Store(1, agentcore.Message{Content: "first"})
	q.Store(1, agentcore.Message{Content: "second"})

	if q.Len() != 1 {
		t.Errorf("Len: expected 1 after overwrite, got %d", q.Len())
	}

	msg, ok := q.Commit(1)
	if !ok {
		t.Fatal("expected msg to be commitable")
	}
	if msg.Content != "second" {
		t.Errorf("Content: expected 'second', got %q", msg.Content)
	}
}

func TestDeferredPersistQueuePending(t *testing.T) {
	q := NewDeferredPersistQueue()
	q.Store(3, agentcore.Message{})
	q.Store(5, agentcore.Message{})
	q.Store(7, agentcore.Message{})

	pending := q.Pending()
	if len(pending) != 3 {
		t.Errorf("Pending: expected 3, got %d", len(pending))
	}
}
