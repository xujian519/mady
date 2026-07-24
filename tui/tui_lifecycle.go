package tui

// This file contains the TUI lifecycle: Start, Stop, and the timing helpers
// (Tick/Every) that schedule Msg delivery on the TUI's lifecycle context.
// The TUI is one-shot — after Stop it refuses to restart (see Start).

import (
	"context"
	"time"

	core "github.com/xujian519/mady/tui/core"
)

// Start begins input and render loops. Non-blocking.
//
// A TUI is one-shot: after Stop, calling Start again returns an error
// rather than silently failing. This avoids the subtle bug where a second
// Start appears to succeed but the event loop exits immediately because
// doneCh is already closed. Callers that need a fresh TUI should construct
// a new one.
func (t *TUI) Start() error {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return nil
	}
	// Detect a stopped TUI (doneCh already closed) and refuse to restart.
	select {
	case <-t.doneCh:
		t.mu.Unlock()
		return errTUIAlreadyStopped
	default:
	}
	t.started = true
	t.mu.Unlock()

	t.stdin.OnKey(func(data string) { t.onKey(data) })
	t.stdin.OnPaste(func(text string) { t.onPaste(text) })
	t.stdin.OnMouse(func(msg core.MouseMsg) { t.onMouse(msg) })

	if err := t.term.Start(t.onTerminalInput, t.onTerminalResize); err != nil {
		// Roll back started and fully tear down lifecycle signals (doneCh,
		// ctx, stdin) so the object is in a clean terminal state regardless
		// of whether the caller remembers to defer Stop(). Stop is
		// idempotent, so a later defer Stop() from the caller is a no-op.
		// We swallow Stop's error here because the term.Start failure is the
		// primary error the caller needs to see.
		t.mu.Lock()
		t.started = false
		t.mu.Unlock()
		_ = t.Stop()
		return err
	}

	if t.options.AltScreen {
		_, _ = t.term.Write([]byte("\x1b[?1049h"))
		t.outMu.Lock()
		t.altScreenOn = true
		t.outMu.Unlock()
		// Re-push Kitty keyboard after entering the alt screen. The alt
		// screen switch resets the Kitty protocol state negotiated in
		// terminal.Start; without this re-push, modifier-rich keys (e.g.
		// Cmd+C on macOS) fall back to legacy encoding and their modifiers
		// are lost — breaking shortcuts like copy. The Kitty spec
		// recommends emitting CSI >1u "when entering alternate screen
		// mode". See https://sw.kovidgoyal.net/kitty/keyboard-protocol/
		t.term.PushKittyKeyboard()
	}

	t.enableMouse(t.options.MouseMode)

	go t.eventLoop()

	cols, rows := t.term.Size()
	t.SendMsg(core.WindowSizeMsg{Width: cols, Height: rows})

	t.RequestRender()
	return nil
}

// Stop ends input/render loops and restores the terminal.
//
// Stop is safe to call even if Start was never called or failed:
//   - The stdin buffer's flushLoop goroutine is always cleaned up (it starts
//     at NewStdinBuffer time, so a never-started TUI would otherwise leak it).
//   - Done() is always closed so callers waiting on it don't block forever.
//   - The lifecycle context is always canceled so Tick/Every/WithContext
//     goroutines terminate.
//
// Stop is idempotent — subsequent calls are no-ops that return nil.
//
// The error (if any) comes from the underlying terminal's Stop; it is no
// longer swallowed so callers can surface terminal-restoration failures.
func (t *TUI) Stop() error {
	// Always close the stdin buffer, even if Start was never called or
	// failed. The buffer starts its flushLoop in its constructor, so it
	// must be explicitly closed regardless of TUI started state.
	t.stdin.Close()

	// Idempotency gate: if doneCh is already closed, we've fully stopped.
	// This must come before the !started check — a TUI where Start failed
	// has started=false but doneCh still open, and we still need to close
	// doneCh + cancel ctx below.
	t.mu.Lock()
	select {
	case <-t.doneCh:
		t.mu.Unlock()
		return nil
	default:
	}
	wasStarted := t.started
	t.started = false
	// Publish stopped BEFORE close(doneCh). sendMsgSafe observes this flag
	// before its select, so once we set it no new message can enter msgCh —
	// closing the TOCTOU window where doneCh closes mid-check and a select
	// still picks the send branch.
	t.stopped.Store(true)
	t.mu.Unlock()

	// Tear down the lifecycle context unconditionally. This covers all
	// paths: never-started, Start-failed, and normal running. ctx/cancel
	// are created in NewTUI, so cancel is never nil here.
	if t.cancel != nil {
		t.cancel()
	}

	// Stop the mouse-throttle ticker to prevent a goroutine leak from the
	// ticker's internal goroutine. The ticker is created in NewTUI and lives
	// for the full TUI lifetime unconditionally (not just while mouse is
	// enabled), so Stop is the only place to clean it up.
	if t.mouseThrottle != nil {
		t.mouseThrottle.Stop()
	}

	var stopErr error
	if wasStarted {
		t.disableMouse()

		t.outMu.Lock()
		altScreenOn := t.altScreenOn
		t.altScreenOn = false
		t.outMu.Unlock()
		if altScreenOn {
			// Pop the alt-screen Kitty push before leaving the alt screen
			// so the pop targets the correct screen context. terminal.Stop
			// will issue the final pop for the main-screen push.
			t.term.PopKittyKeyboard()
			_, _ = t.term.Write([]byte("\x1b[?1049l"))
		}

		// Restore terminal state BEFORE signaling done, so callers waiting on
		// Done() don't exit the process before termios is restored. Capture
		// the error rather than swallowing it — callers may need to surface
		// terminal-restoration failures.
		stopErr = t.term.Stop()
	}

	// Signal completion last. Callers blocked on Done() see a fully torn-down
	// TUI (terminal restored, context canceled, stdin closed).
	close(t.doneCh)
	return stopErr
}

// Quit is an alias for Stop that returns no error. Useful for Ctrl+C handlers.
func (t *TUI) Quit() { _ = t.Stop() }

// Done returns a channel closed when the TUI has exited.
func (t *TUI) Done() <-chan struct{} { return t.doneCh }

// Context returns a context canceled when the TUI stops. Use it to derive
// cancellation contexts for long-running operations triggered from Cmds.
// The context is created at NewTUI time (not Start), so Tick/Every/WithContext
// are always bound to the TUI's lifecycle — even if called before Start or if
// Start fails. Stop cancels it unconditionally.
func (t *TUI) Context() context.Context {
	return t.ctx
}

// Tick schedules fn to fire once after d, delivering its Msg to the event
// loop. The timer runs on the TUI's lifecycle context, so it is canceled
// when the TUI stops (no goroutine leak). Unlike core.Tick (which is a Cmd
// constructor), this method starts the timer immediately and is the
// preferred way to schedule one-shot delays from application code.
func (t *TUI) Tick(d time.Duration, fn func(time.Time) core.Msg) {
	ctx := t.Context()
	go func() {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case ts := <-timer.C:
			t.SendMsg(fn(ts))
		}
	}()
}

// Every schedules fn to fire repeatedly every d, delivering each Msg to the
// event loop. The ticker runs on the TUI's lifecycle context and stops when
// the TUI stops. This is the correct way to implement periodic updates
// (spinners, clock displays, polling); core.Every was removed because the
// Cmd signature (func() Msg) cannot express repeated emission.
func (t *TUI) Every(d time.Duration, fn func(time.Time) core.Msg) {
	ctx := t.Context()
	go func() {
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case ts := <-ticker.C:
				t.SendMsg(fn(ts))
			}
		}
	}()
}
