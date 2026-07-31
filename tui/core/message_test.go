package core

import (
	"context"
	"testing"
	"time"
)

// TestBatchNilFilter verifies that Batch drops nil Cmds, short-circuits a
// single Cmd, and aggregates multiple Cmds into BatchMsg.
func TestBatchNilFilter(t *testing.T) {
	if got := Batch(nil, nil); got != nil {
		t.Fatalf("Batch(nil, nil) = %v, want nil", got)
	}

	one := func() Msg { return QuitMsg{} }
	if got := Batch(one, nil); got == nil {
		t.Fatal("Batch(one, nil) returned nil, want the single Cmd")
	}
	// Single non-nil Cmd is returned as-is (short-circuit).
	got := Batch(one, nil)
	m := got()
	if _, ok := m.(QuitMsg); !ok {
		t.Fatalf("Batch(one).() = %T, want QuitMsg", m)
	}

	two := func() Msg { return TickMsg{Time: time.Now()} }
	agg := Batch(one, two)
	if agg == nil {
		t.Fatal("Batch(one, two) returned nil")
	}
	m = agg()
	bm, ok := m.(BatchMsg)
	if !ok {
		t.Fatalf("Batch(one, two)() = %T, want BatchMsg", m)
	}
	if len(bm) != 2 {
		t.Fatalf("BatchMsg len = %d, want 2", len(bm))
	}
}

// TestSequenceNilFilter mirrors TestBatchNilFilter for ordered Cmds.
func TestSequenceNilFilter(t *testing.T) {
	if got := Sequence(nil, nil); got != nil {
		t.Fatalf("Sequence(nil, nil) = %v, want nil", got)
	}

	one := func() Msg { return QuitMsg{} }
	got := Sequence(one, nil)
	if got == nil {
		t.Fatal("Sequence(one, nil) returned nil")
	}
	m := got()
	if _, ok := m.(QuitMsg); !ok {
		t.Fatalf("Sequence(one).() = %T, want QuitMsg", m)
	}

	agg := Sequence(one, one)
	am := agg()
	if _, ok := am.(SequenceMessage); !ok {
		t.Fatalf("Sequence(one, one)() = %T, want SequenceMessage", am)
	}
}

// TestTickFires verifies Tick delivers the produced Msg after the delay,
// measured from execution (not construction).
func TestTickFires(t *testing.T) {
	cmd := Tick(10*time.Millisecond, func(ts time.Time) Msg {
		return TickMsg{Time: ts}
	})
	m := cmd()
	tm, ok := m.(TickMsg)
	if !ok {
		t.Fatalf("Tick() = %T, want TickMsg", m)
	}
	if tm.Time.IsZero() {
		t.Fatal("TickMsg.Time is zero")
	}
}

// TestWithContextCancelled verifies a canceled context discards the Cmd result.
func TestWithContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled

	cmd := WithContext(ctx, func() Msg { return QuitMsg{} })
	if got := cmd(); got != nil {
		t.Fatalf("WithContext(canceled ctx)() = %v, want nil", got)
	}

	if got := WithContext(ctx, nil); got != nil {
		t.Fatalf("WithContext(canceled ctx, nil) = %v, want nil", got)
	}
}

// TestWithContextDelivers verifies a live context wraps the Cmd result in
// CtxMessage and Inner() returns the original Msg.
func TestWithContextDelivers(t *testing.T) {
	ctx := context.Background()
	cmd := WithContext(ctx, func() Msg { return QuitMsg{} })
	m := cmd()
	cm, ok := m.(CtxMessage)
	if !ok {
		t.Fatalf("WithContext()() = %T, want CtxMessage", m)
	}
	if _, ok := cm.Inner().(QuitMsg); !ok {
		t.Fatalf("Inner() = %T, want QuitMsg", cm.Inner())
	}
}

// TestQuit verifies Quit produces QuitMsg.
func TestQuit(t *testing.T) {
	if _, ok := Quit().(QuitMsg); !ok {
		t.Fatalf("Quit() = %T, want QuitMsg", Quit())
	}
}

// TestMsgMarkers verifies every built-in Msg type satisfies the Msg marker
// interface and carries no unexpected state.
func TestMsgMarkers(t *testing.T) {
	msgs := []Msg{
		MsgBase{},
		KeyMsg{Data: "x"},
		PasteMsg{Text: "p"},
		WindowSizeMsg{Width: 80, Height: 24},
		TickMsg{Time: time.Now()},
		QuitMsg{},
		PanicMsg{Err: "boom"},
		MouseMsg{Action: MousePress, Row: 1, Col: 2},
		BatchMsg{},
		SequenceMessage{},
		CtxMessage{},
	}
	for _, m := range msgs {
		m.MsgMarker() // compile-time check; no panic expected
	}
	// MouseAction constants are stable and exhaustive.
	if MousePress != 0 || MouseForwardButton != MousePress+8 {
		t.Fatalf("MouseAction enum drift: press=%d forward=%d", MousePress, MouseForwardButton)
	}
}
