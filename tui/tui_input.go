package tui

// This file handles input: Msg dispatch (processMsg), Cmd execution, the
// terminal input callbacks (key/mouse/paste/resize), and mouse-mode
// toggling. sendMsgSafe is the safe enqueue path that observes the stopped
// flag to avoid zombie messages after Stop.

import (
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	core "github.com/xujian519/mady/tui/core"
	terminal "github.com/xujian519/mady/tui/terminal"
)

// mouseThrottlePeriod is the minimum interval between MouseMotion events.
// Matches the mouseThrottle ticker (~33ms for 30fps) and caps event throughput.
const mouseThrottlePeriod = time.Second / 30

func (t *TUI) processMsg(msg core.Msg) {
	if msg == nil {
		return
	}

	switch m := msg.(type) {
	case core.BatchMsg:
		// Run every Cmd concurrently — each result Msg flows back into the
		// event loop asynchronously. This never blocks the loop, even if a
		// Cmd performs slow IO. Order of completion is unspecified by design
		// (use Sequence when order matters).
		for i, cmd := range m {
			if cmd != nil {
				go t.execCmdIndexed(cmd, i)
			}
		}
		return
	case core.SequenceMessage:
		// Asynchronous ordered execution: run the first Cmd, and when it
		// completes, re-enqueue the remaining Cmds as a new SequenceMessage
		// so the event loop runs the next one. This preserves order without
		// blocking the loop.
		//
		// Skip leading nil Cmds defensively (core.Sequence filters them at
		// construction, but an externally-built SequenceMessage might not).
		// This mirrors BatchMsg's nil guard and avoids a panic → PanicMsg
		// round-trip for what is really a no-op.
		for len(m) > 0 && m[0] == nil {
			m = m[1:]
		}
		if len(m) == 0 {
			return
		}
		first := m[0]
		rest := m[1:]
		go func() {
			defer func() {
				if r := recover(); r != nil {
					t.SendMsg(core.PanicMsg{Err: r, Stack: captureStack(), CmdIndex: 0})
				}
			}()
			result := first()
			if result != nil {
				t.SendMsg(result)
			}
			if len(rest) > 0 {
				t.SendMsg(rest)
			}
		}()
		return
	case core.CtxMessage:
		if m.Inner() != nil {
			t.processMsg(m.Inner())
		}
		return
	case core.PanicMsg:
		slog.Default().Error("cmd panic recovered",
			"err", m.Err,
			"cmdIndex", m.CmdIndex,
			"stack", m.Stack,
		)
	case core.QuitMsg:
		t.Stop()
		return
	}

	focused := t.Focused()

	if t.options.Filter != nil {
		filtered := t.options.Filter(focused, msg)
		if filtered == nil {
			return
		}
		msg = filtered
	}

	if focused != nil {
		// Phase 4.2: translate absolute mouse coordinates to overlay-local space
		if mm, ok := msg.(core.MouseMsg); ok {
			t.mu.Lock()
			for _, ov := range t.overlays {
				if ov != nil && ov.Content == focused {
					if lr, lc, ok2 := ov.TranslateMouse(mm.Row, mm.Col); ok2 {
						mm.Row = lr
						mm.Col = lc
						msg = mm
					}
					break
				}
			}
			t.mu.Unlock()
		}
		if u, ok := focused.(core.Updatable); ok {
			start := time.Now()
			if cmd := u.Update(msg); cmd != nil {
				go t.execCmd(cmd)
			}
			if d := time.Since(start); d > 50*time.Millisecond {
				slog.Default().Warn("slow Update in processMsg",
					"component", fmt.Sprintf("%T", focused),
					"msg", fmt.Sprintf("%T", msg),
					"duration", d,
				)
			}
		}
	}

	t.mu.Lock()
	focusedIsOverlay := false
	for _, ov := range t.overlays {
		if ov != nil && ov.Content == focused {
			focusedIsOverlay = true
			break
		}
	}
	children := make([]core.Component, len(t.children))
	copy(children, t.children)
	t.mu.Unlock()

	if !focusedIsOverlay {
		for _, child := range children {
			if child == focused {
				continue
			}
			if u, ok := child.(core.Updatable); ok {
				// Non-focused children also get to run Cmds. This matches
				// the focused-component path and avoids the footgun where a
				// background component's Cmd is silently dropped.
				start := time.Now()
				if cmd := u.Update(msg); cmd != nil {
					go t.execCmd(cmd)
				}
				if d := time.Since(start); d > 50*time.Millisecond {
					slog.Default().Warn("slow Update in child component",
						"component", fmt.Sprintf("%T", child),
						"msg", fmt.Sprintf("%T", msg),
						"duration", d,
					)
				}
			}
		}
	}

	// Overlays are modal layers: once any overlay exists, only the focused
	// component receives input. The focused component (overlay content or
	// otherwise) was already updated above, so no further dispatch to
	// non-focused overlays is needed here.

	t.RequestRender()
}

func (t *TUI) execCmd(cmd core.Cmd) {
	t.execCmdIndexed(cmd, 0)
}

// execCmdIndexed runs a Cmd and forwards its result Msg to the event loop.
// If the Cmd panics, a PanicMsg is emitted instead of silently dropping the
// result. The index is preserved in PanicMsg for Batch diagnostics.
func (t *TUI) execCmdIndexed(cmd core.Cmd, idx int) {
	if cmd == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			t.sendMsgSafe(core.PanicMsg{Err: r, Stack: captureStack(), CmdIndex: idx})
		}
	}()
	msg := cmd()
	if msg == nil {
		return
	}
	t.sendMsgSafe(msg)
}

// sendMsgSafe enqueues a Msg, aborting silently if the TUI is already stopped.
//
// The stopped atomic flag is observed first. Stop sets it BEFORE closing
// doneCh, so once stopped=true is published no message can enter msgCh.
// This closes the TOCTOU window a pure channel-based check leaves: doneCh
// being closed and msgCh being writable can both be ready in a select, and
// Go's pseudorandom select could pick the send — accumulating zombie
// messages the (exited) event loop never drains.
//
// We still fall back to the doneCh select for the actual blocking send:
// once stopped is true we've already returned, so the select only runs in
// the not-stopped path, where doneCh-closed is the normal "TUI stopped
// while we were trying to send" race that the select handles correctly.
func (t *TUI) sendMsgSafe(msg core.Msg) {
	if t.stopped.Load() {
		return // already stopped — drop silently
	}
	select {
	case t.msgCh <- msg:
	case <-t.doneCh:
	}
}

// captureStack returns a truncated stack trace for panic diagnostics.
func captureStack() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// SendMsg enqueues a message for processing by the event loop.
// This is the primary way to deliver custom messages to Updatable
// components from outside the event loop (e.g. from agent callbacks).
func (t *TUI) SendMsg(msg core.Msg) {
	if msg == nil {
		return
	}
	t.sendMsgSafe(msg)
}

// EnableMouse enables SGR mouse reporting. Safe to call multiple times.
func (t *TUI) EnableMouse(mode string) { t.enableMouse(mode) }

// DisableMouse disables SGR mouse reporting.
func (t *TUI) DisableMouse() { t.disableMouse() }

// ---------------------------------------------------------------------------
// Input
// ---------------------------------------------------------------------------

func (t *TUI) onTerminalInput(data []byte) {
	t.stdin.Feed(data)
}

func (t *TUI) onTerminalResize() {
	t.mu.Lock()
	t.firstFrame = true
	t.mu.Unlock()

	cols, rows := t.term.Size()
	t.SendMsg(core.WindowSizeMsg{Width: cols, Height: rows})
	t.RequestRender()
}

func (t *TUI) onKey(data string) {
	if t.OnDebug != nil && terminal.MatchesKey(data, "ctrl+shift+d") {
		t.OnDebug()
		return
	}
	t.SendMsg(core.KeyMsg{Data: data})
}

func (t *TUI) onPaste(text string) {
	t.SendMsg(core.PasteMsg{Text: text})
}

func (t *TUI) onMouse(msg core.MouseMsg) {
	// Throttle MouseMotion events: trackpad scrolling can produce 60+ events
	// per second. We drain the throttle ticker channel and compare wall time
	// to keep the effective rate at ~30fps. Non-motion events pass through.
	if msg.Action == core.MouseMotion && t.mouseThrottle != nil {
		select {
		case <-t.mouseThrottle.C:
			t.mouseLast = time.Now()
		default:
			// Ticker channel empty → events arriving faster than throttle rate.
			// Accept if at least 16ms have passed since the last accepted event
			// (a secondary time guard so a slow-ticking ticker doesn't starve
			// motion entirely when the consumer is lagging).
			if time.Since(t.mouseLast) < mouseThrottlePeriod {
				return
			}
			t.mouseLast = time.Now()
		}
	}
	t.SendMsg(msg)
}

func (t *TUI) enableMouse(mode string) {
	mode = strings.ToLower(mode)
	if mode == "" || mode == "off" {
		t.outMu.Lock()
		t.mouseMode = ""
		t.outMu.Unlock()
		return
	}
	if mode == "auto" || mode == "on" {
		mode = "sgr"
	}
	t.outMu.Lock()
	t.mouseMode = mode
	t.outMu.Unlock()
	switch mode {
	case "sgr":
		// Enable SGR positioning (?1006h) + button-event tracking (?1002h).
		//
		// ?1002h (button-event tracking) reports press, motion AND release
		// events. This gives the TUI full mouse-drag visibility so that
		// both the Editor and ChatHistory components can implement smooth
		// text selection via their own handleMouse() methods — which
		// already handle MousePress/MouseMotion/MouseRelease correctly.
		//
		// The downside is that ?1002h prevents the terminal emulator's
		// OS-level native text selection (drag-to-select). We accept this
		// trade-off because:
		//   a) The TUI renders its own selection highlight (ANSI bg).
		//   b) Selected text is copyable via ⌘+C (Editor) or right-click
		//      (ChatHistory), both of which feed the system clipboard.
		//
		// Do NOT use ?1000h (basic click tracking) here — it only reports
		// press events, starving MouseMotion handlers in both the Editor
		// and ChatHistory, which makes the TUI's own selection unusable.
		_, _ = t.term.Write([]byte("\x1b[?1002h\x1b[?1006h"))
	case "x11":
		_, _ = t.term.Write([]byte("\x1b[?1000h"))
	}
}

func (t *TUI) disableMouse() {
	t.outMu.Lock()
	mode := t.mouseMode
	t.mouseMode = ""
	t.outMu.Unlock()
	if mode == "" {
		return
	}
	switch mode {
	case "sgr":
		_, _ = t.term.Write([]byte("\x1b[?1006l\x1b[?1002l"))
	case "x11":
		_, _ = t.term.Write([]byte("\x1b[?1000l"))
	}
}
