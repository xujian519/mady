package agentcore

import (
	"strconv"
	"sync"
	"testing"
)

func TestMessageQueuePushAndDrain(t *testing.T) {
	q := newMessageQueue(SteeringAll, 0)
	if err := q.Push(Message{Role: RoleUser, Content: "hi"}); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}
	if err := q.Push(Message{Role: RoleUser, Content: "there"}); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}
	msgs := q.Drain()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hi" || msgs[1].Content != "there" {
		t.Fatalf("unexpected content: %v", msgs)
	}
}

func TestMessageQueueDrainEmpty(t *testing.T) {
	q := newMessageQueue(SteeringAll, 0)
	msgs := q.Drain()
	if msgs != nil {
		t.Fatalf("expected nil, got %v", msgs)
	}
}

func TestMessageQueueOneAtATime(t *testing.T) {
	q := newMessageQueue(SteeringOneAtATime, 0)
	if err := q.Push(Message{Role: RoleUser, Content: "a"}); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}
	if err := q.Push(Message{Role: RoleUser, Content: "b"}); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}

	msgs1 := q.Drain()
	if len(msgs1) != 1 || msgs1[0].Content != "a" {
		t.Fatalf("expected [a], got %v", msgs1)
	}

	msgs2 := q.Drain()
	if len(msgs2) != 1 || msgs2[0].Content != "b" {
		t.Fatalf("expected [b], got %v", msgs2)
	}

	msgs3 := q.Drain()
	if msgs3 != nil {
		t.Fatalf("expected nil after draining all, got %v", msgs3)
	}
}

func TestMessageQueueDefaultMode(t *testing.T) {
	q := newMessageQueue("", 0) // empty should default to SteeringAll
	if err := q.Push(Message{Role: RoleUser, Content: "x"}); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}
	if err := q.Push(Message{Role: RoleUser, Content: "y"}); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}
	msgs := q.Drain()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages with default mode, got %d", len(msgs))
	}
}

func TestMessageQueueLen(t *testing.T) {
	q := newMessageQueue(SteeringAll, 0)
	if q.Len() != 0 {
		t.Fatalf("expected 0, got %d", q.Len())
	}
	if err := q.Push(Message{Role: RoleUser, Content: "a"}); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}
	if err := q.Push(Message{Role: RoleUser, Content: "b"}); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}
	if q.Len() != 2 {
		t.Fatalf("expected 2, got %d", q.Len())
	}
	q.Drain()
	if q.Len() != 0 {
		t.Fatalf("expected 0 after drain, got %d", q.Len())
	}
}

func TestMessageQueueConcurrentSafety(t *testing.T) {
	q := newMessageQueue(SteeringAll, 0)
	var wg sync.WaitGroup
	n := 100

	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = q.Push(Message{Role: RoleUser, Content: strconv.Itoa(i)})
		}(i)
	}
	wg.Wait()

	if q.Len() != int64(n) {
		t.Fatalf("expected %d messages, got %d", n, q.Len())
	}

	msgs := q.Drain()
	if len(msgs) != n {
		t.Fatalf("expected %d drained, got %d", n, len(msgs))
	}
}

func TestMessageQueueMaxSize(t *testing.T) {
	q := newMessageQueue(SteeringAll, 2) // max 2

	if err := q.Push(Message{Role: RoleUser, Content: "a"}); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}
	if err := q.Push(Message{Role: RoleUser, Content: "b"}); err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}

	// Third push should fail
	if err := q.Push(Message{Role: RoleUser, Content: "c"}); err != ErrQueueFull {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}

	// Queue should still have the first two messages
	if q.Len() != 2 {
		t.Fatalf("expected 2, got %d", q.Len())
	}

	// After drain, new pushes should succeed
	msgs := q.Drain()
	if len(msgs) != 2 {
		t.Fatalf("expected 2, got %d", len(msgs))
	}
	if err := q.Push(Message{Role: RoleUser, Content: "c"}); err != nil {
		t.Fatalf("unexpected push error after drain: %v", err)
	}
	if q.Len() != 1 {
		t.Fatalf("expected 1 after re-push, got %d", q.Len())
	}

	// Zero maxSize means unlimited
	u := newMessageQueue(SteeringAll, 0)
	for i := 0; i < 1000; i++ {
		if err := u.Push(Message{Role: RoleUser, Content: "x"}); err != nil {
			t.Fatalf("unexpected push error at %d: %v", i, err)
		}
	}
	if u.Len() != 1000 {
		t.Fatalf("expected 1000, got %d", u.Len())
	}
}
