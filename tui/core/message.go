package core

import (
	"context"
	"time"
)

// Msg is the marker interface for all messages in the Elm-style event loop.
// External packages can implement Msg by embedding MsgBase or by defining
// a MsgMarker() method on their types.
type Msg interface{ MsgMarker() }

// MsgBase is a zero-value struct that external packages can embed to
// satisfy the Msg interface without writing their own method.
type MsgBase struct{}

func (MsgBase) MsgMarker() {}

// KeyMsg carries a single key-press event. The Data field contains the raw
// terminal input bytes; use terminal.ParseKeys / MatchesKey to interpret it.
// KittyFlags carries the negotiated Kitty keyboard protocol flags so
// downstream parsers can decode CSI u sequences correctly.
type KeyMsg struct {
	Data       string
	KittyFlags int64
}

func (KeyMsg) MsgMarker() {}

// PasteMsg carries text pasted via the bracketed paste mechanism (CSI 2004).
// The TUI engine wraps paste content in this type so components can
// distinguish typed input from bulk paste and handle it efficiently.
type PasteMsg struct {
	Text string
}

func (PasteMsg) MsgMarker() {}

// WindowSizeMsg is emitted when the terminal is resized (SIGWINCH).
// Width and Height are the new terminal dimensions in columns and rows.
// The TUI engine automatically invalidates all components on resize.
type WindowSizeMsg struct {
	Width  int64
	Height int64
}

func (WindowSizeMsg) MsgMarker() {}

// TickMsg is emitted by a Tick Cmd after a duration delay.
// The Time field records when the tick fired.
type TickMsg struct {
	Time time.Time
}

func (TickMsg) MsgMarker() {}

// QuitMsg signals the TUI to shut down. It is typically produced by Ctrl+C
// or by calling core.Quit() from within a component's Update method.
type QuitMsg struct{}

func (QuitMsg) MsgMarker() {}

// PanicMsg is emitted when a Cmd panics during execution. The event loop
// receives this instead of silently losing the Cmd's output, so the
// application can surface the failure (log it, show an error banner, etc.).
type PanicMsg struct {
	Err      any    // the recover() value
	Stack    string // captured stack trace
	CmdIndex int    // meaningful only when emitted from a Batch/Sequence
}

func (PanicMsg) MsgMarker() {}

// MouseAction describes the type of a mouse event.
type MouseAction int64

const (
	MousePress         MouseAction = iota // button pressed
	MouseRelease                          // button released
	MouseWheelUp                          // wheel scrolled up
	MouseWheelDown                        // wheel scrolled down
	MouseWheelLeft                        // wheel scrolled left (horizontal)
	MouseWheelRight                       // wheel scrolled right (horizontal)
	MouseMotion                           // pointer moved (requires SGR mouse tracking)
	MouseBackButton                       // side "back" button (button 8)
	MouseForwardButton                    // side "forward" button (button 9)
)

// MouseMsg carries a terminal mouse event. Coordinates are 0-based
// (Row 0 = top of terminal, Col 0 = leftmost column). Button identifies
// which button was pressed/released (1=left, 2=middle, 3=right, 8=back,
// 9=forward).
type MouseMsg struct {
	Action MouseAction
	Row    int64
	Col    int64
	Button int64
	Alt    bool
	Ctrl   bool
	Shift  bool
}

func (MouseMsg) MsgMarker() {}

// BatchMsg aggregates multiple Cmd results into one message. It is produced
// by the Batch() helper and handled by the TUI event loop, which dispatches
// each Cmd result as a separate message.
type BatchMsg []Cmd

func (BatchMsg) MsgMarker() {}

// SequenceMessage carries an ordered list of Cmds. It is produced by the
// Sequence() helper; the event loop executes them in order, waiting for each
// to complete before starting the next.
type SequenceMessage []Cmd

func (SequenceMessage) MsgMarker() {}

// Cmd is a function that performs an asynchronous side effect and returns
// a Msg. Cmds are returned from Update() and executed in a separate
// goroutine by the TUI event loop. The returned Msg is delivered back to
// Update() on the event-loop goroutine.
//
// IMPORTANT: Cmd must NOT block on the event-loop goroutine (e.g., sending
// on msgCh directly) as this causes a deadlock. All goroutine management
// is handled by Batch/Sequence and the TUI engine.
type Cmd func() Msg

func Batch(cmds ...Cmd) Cmd {
	nonNil := make([]Cmd, 0, len(cmds))
	for _, c := range cmds {
		if c != nil {
			nonNil = append(nonNil, c)
		}
	}
	if len(nonNil) == 0 {
		return nil
	}
	if len(nonNil) == 1 {
		return nonNil[0]
	}
	return func() Msg {
		return BatchMsg(nonNil)
	}
}

func Sequence(cmds ...Cmd) Cmd {
	nonNil := make([]Cmd, 0, len(cmds))
	for _, c := range cmds {
		if c != nil {
			nonNil = append(nonNil, c)
		}
	}
	if len(nonNil) == 0 {
		return nil
	}
	if len(nonNil) == 1 {
		return nonNil[0]
	}
	return func() Msg {
		return SequenceMessage(nonNil)
	}
}

// Tick returns a Cmd that fires once after duration d. The timer is created
// when the Cmd runs (not when Tick is called), so the delay is measured from
// execution, not construction — avoiding drift when the Cmd is queued (e.g.
// inside Sequence) and avoiding timer leaks if the Cmd never runs.
func Tick(d time.Duration, fn func(time.Time) Msg) Cmd {
	return func() Msg {
		t := time.NewTimer(d)
		defer t.Stop()
		ts := <-t.C
		return fn(ts)
	}
}

// WithContext wraps cmd so that its result is discarded if ctx is canceled
// before the Cmd completes. The Cmd itself runs in a goroutine; cancellation
// during a long-running Cmd cannot be forced by the framework (Go has no
// goroutine kill), so the Cmd SHOULD select on ctx.Done() itself for prompt
// cancellation. WithContext guarantees that no Msg is delivered after ctx is
// done, which prevents stale results from reaching the event loop.
func WithContext(ctx context.Context, cmd Cmd) Cmd {
	if cmd == nil {
		return nil
	}
	return func() Msg {
		if err := ctx.Err(); err != nil {
			return nil
		}
		done := make(chan Msg, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					done <- PanicMsg{Err: r, Stack: CaptureStack()}
				}
			}()
			done <- cmd()
		}()
		select {
		case <-ctx.Done():
			return nil
		case m := <-done:
			if m == nil {
				return nil
			}
			return CtxMessage{ctx: ctx, inner: m}
		}
	}
}

func Quit() Msg {
	return QuitMsg{}
}

type CtxMessage struct {
	ctx   context.Context
	inner Msg
}

func (CtxMessage) MsgMarker() {}

func (m CtxMessage) Inner() Msg { return m.inner }
