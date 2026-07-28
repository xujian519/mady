package tui

// This file handles input: Msg dispatch (processMsg), Cmd execution, the
// terminal input callbacks (key/mouse/paste/resize), and mouse-mode
// toggling. sendMsgSafe is the safe enqueue path that observes the stopped
// flag to avoid zombie messages after Stop.

import (
	"fmt"
	"log/slog"
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

	t.mu.Lock()
	t.msgCount++
	t.mu.Unlock()

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
					t.SendMsg(core.PanicMsg{Err: r, Stack: core.CaptureStack(), CmdIndex: 0})
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
		slog.Error("cmd panic recovered",
			"err", m.Err,
			"cmdIndex", m.CmdIndex,
			"stack", m.Stack,
		)
	case core.QuitMsg:
		if err := t.Stop(); err != nil {
			slog.Warn("tui: stop failed on QuitMsg", "err", err)
		}
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
				slog.Warn("slow Update in processMsg",
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
					slog.Warn("slow Update in child component",
						"component", fmt.Sprintf("%T", child),
						"msg", fmt.Sprintf("%T", msg),
						"duration", d,
					)
				}
				// Mouse consumption: if this child consumed the MouseMsg,
				// stop broadcasting to remaining siblings. This prevents
				// every component from processing the same mouse event.
				if _, isMouse := msg.(core.MouseMsg); isMouse {
					if mc, ok := child.(core.MouseConsumer); ok && mc.MouseConsumed() {
						break
					}
				}
			}
		}
	}

	// Overlays are modal layers: once any overlay exists, only the focused
	// component receives input. The focused component (overlay content or
	// otherwise) was already updated above, so no further dispatch to
	// non-focused overlays is needed here.

	// Log event type for debug overlay (avoid flooding with frequent events).
	t.logEvent(msg)

	t.RequestRender()
}

// logEvent records a summary of the given message in the debug event ring.
// High-frequency events (MouseMotion, WindowSize, TickMsg) are thinned to
// avoid flooding the ring buffer.
func (t *TUI) logEvent(msg core.Msg) {
	label := fmt.Sprintf("%T", msg)
	// Strip package prefix for readability: "chat.AgentStartChatEvent" → "AgentStartChatEvent".
	if idx := strings.LastIndex(label, "."); idx >= 0 && len(label) > idx+1 {
		label = label[idx+1:]
	}

	// Throttle high-frequency events.
	switch msg.(type) {
	case core.TickMsg:
		// Log TickMsg only every 10th message.
		if t.msgCount%10 != 0 {
			return
		}
	case core.WindowSizeMsg:
		// Thin resize events: only log every 120th message.
		if t.msgCount%120 != 0 {
			return
		}
	}

	t.mu.Lock()
	t.eventLog[t.eventLogIdx] = label
	t.eventLogIdx = (t.eventLogIdx + 1) % len(t.eventLog)
	t.mu.Unlock()
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
			t.sendMsgSafe(core.PanicMsg{Err: r, Stack: core.CaptureStack(), CmdIndex: idx})
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
	t.SendMsg(core.KeyMsg{Data: data, KittyFlags: t.kittyFlags})
}

func (t *TUI) onPaste(text string) {
	t.SendMsg(core.PasteMsg{Text: text})
}

func (t *TUI) onMouse(msg core.MouseMsg) {
	if msg.Action == core.MouseMotion && t.mouseThrottle != nil {
		t.onThrottledMotion(msg)
		return
	}
	// Non-motion events flush any pending coalesced motion first, so a
	// press/release right after a drag burst sees the correct final position.
	if t.pendingMotion != nil {
		flushed := *t.pendingMotion
		t.pendingMotion = nil
		t.SendMsg(flushed)
	}
	t.SendMsg(msg)
}

// onThrottledMotion implements the MouseMotion throttle/coalesce logic.
//
// Trackpad scrolling can produce 60+ motion events per second. We drain the
// throttle ticker channel and compare wall time to keep the effective rate at
// ~30fps. Events arriving faster than the throttle rate are stored in
// pendingMotion instead of dropped; the event loop flushes the pending motion
// on the next ticker tick, so the final drag position is never lost
// (merge, not drop). This keeps text-selection endpoints accurate during fast
// drags.
//
// Caller: onMouse — only when msg.Action == MouseMotion and mouseThrottle != nil.
func (t *TUI) onThrottledMotion(msg core.MouseMsg) {
	select {
	case <-t.mouseThrottle.C:
		t.mouseLast = time.Now()
		t.pendingMotion = nil // consumed a tick, send directly
	default:
		// Ticker channel empty → events arriving faster than throttle rate.
		// Accept if at least mouseThrottlePeriod (~33ms) has passed since the
		// last accepted event (secondary time guard so a slow-ticking ticker
		// doesn't starve motion entirely when the consumer is lagging).
		if time.Since(t.mouseLast) < mouseThrottlePeriod {
			// Throttle: remember as pending. The next ticker tick will
			// flush this so the final position is never lost.
			t.pendingMotion = &msg
			return
		}
		t.mouseLast = time.Now()
		t.pendingMotion = nil
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
	// Order matters: disable alternate scroll mode (?1007l) BEFORE enabling
	// mouse tracking. DEC 1007 (alternate scroll mode) is on by default in
	// most terminals (Terminal.app, iTerm2, GNOME Terminal, xterm). While it
	// is active inside the alternate screen buffer, the terminal intercepts
	// wheel events and translates them into ↑/↓ key sequences instead of
	// reporting them as mouse events — so the wheel never reaches
	// ChatHistory.handleMouse and scrolling the transcript appears broken
	// (the stray arrow keys either do nothing or move the editor cursor).
	// Disabling 1007 forces real wheel events through. On terminals that
	// don't implement 1007 the set/reset is a no-op, so this is safe.
	//
	// ?1002h (button-event tracking) reports press, motion AND release
	// events, giving the TUI full mouse-drag visibility for smooth text
	// selection in the Editor and ChatHistory (which implement their own
	// handleMouse and handle MousePress/MouseMotion/MouseRelease). The
	// trade-off is that it disables the terminal's OS-level native
	// drag-to-select — acceptable because the TUI renders its own selection
	// highlight and copies via ⌘+C (Editor) or right-click (ChatHistory).
	// Do NOT use ?1000h (basic click) for SGR mode: it reports only press
	// events, starving MouseMotion handlers and breaking selection.
	switch mode {
	case "sgr":
		// ?1007l disable alt scroll · ?1002h button-event tracking · ?1006h SGR positioning.
		_, _ = t.term.Write([]byte("\x1b[?1007l\x1b[?1002h\x1b[?1006h"))
	case "x11":
		// ?1007l disable alt scroll · ?1000h basic click tracking.
		_, _ = t.term.Write([]byte("\x1b[?1007l\x1b[?1000h"))
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
	// Reverse order of enableMouse: disable mouse tracking FIRST, then
	// re-enable alternate scroll mode (?1007h) so the terminal's default
	// wheel behavior (native scrollback in the main screen) is restored.
	switch mode {
	case "sgr":
		_, _ = t.term.Write([]byte("\x1b[?1006l\x1b[?1002l\x1b[?1007h"))
	case "x11":
		_, _ = t.term.Write([]byte("\x1b[?1000l\x1b[?1007h"))
	}
}
