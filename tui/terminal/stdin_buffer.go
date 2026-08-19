package terminal

import (
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/tui/core"
)

// ---------------------------------------------------------------------------
// StdinBuffer
//
// Raw terminal input may arrive in fragmented chunks that either:
//   1. Split a multi-byte ANSI escape (e.g. "\x1b[" arrives separately from
//      "A"), breaking naive parsers.
//   2. Batch many keystrokes (or an entire paste) into one read.
//
// StdinBuffer solves both:
//   - It buffers partial escape sequences until complete.
//   - It splits batched input into logical "events" (keys / paste blobs)
//     so consumers can process them one at a time.
//   - It recognizes bracketed-paste markers ESC[200~ ... ESC[201~ and emits
//     the inner content as a single Paste event.
//
// Usage:
//
//   sb := tui.NewStdinBuffer()
//   sb.OnKey(func(data string) { ... })    // raw per-key chunks
//   sb.OnPaste(func(text string) { ... })  // bracketed-paste content
//   sb.Feed(chunkFromTerminal)
// ---------------------------------------------------------------------------

// StdinBufferOptions configures optional behavior.
type StdinBufferOptions struct {
	// MaxPasteBytes caps the in-memory paste buffer. Excess bytes are
	// truncated silently. 0 means 16 MiB (sane default).
	MaxPasteBytes int64
}

// StdinBuffer holds fragmented terminal input and emits logical events.
type StdinBuffer struct {
	opts StdinBufferOptions

	mu         sync.Mutex
	buf        []byte
	inPaste    bool
	pasteBuf   strings.Builder
	pasteBytes int64

	// escPendingAt records when a lone ESC byte first landed in b.buf.
	// FlushEsc uses this to emit a standalone ESC key event after
	// EscFlushDelay elapses, so the user pressing ESC alone is not lost
	// while the buffer waits for a (possibly nonexistent) CSI continuation.
	escPendingAt time.Time

	// stopFlush signals the background flushLoop goroutine to exit.
	stopFlush chan struct{}
	// closed guards stopFlush against a double close.
	closed bool

	onKey   func(data string)
	onPaste func(text string)
	onMouse func(msg core.MouseMsg)
}

// NewStdinBuffer returns a new buffer with defaults applied.
func NewStdinBuffer(opts ...StdinBufferOptions) *StdinBuffer {
	var o StdinBufferOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.MaxPasteBytes <= 0 {
		o.MaxPasteBytes = 16 * 1024 * 1024
	}
	b := &StdinBuffer{opts: o, stopFlush: make(chan struct{})}
	// Run a background ticker that promotes a long-waiting lone ESC byte
	// into a key event, independent of any TUI render loop. Without this
	// goroutine, a TUI handler that blocks for longer than EscFlushDelay
	// (e.g. opening a heavy overlay synchronously) would prevent the
	// render loop's per-tick FlushEsc from ever firing and ESC would
	// appear "stuck" until the next unrelated key arrived.
	go b.flushLoop()
	return b
}

// flushLoop runs for the lifetime of the buffer and ticks every
// EscFlushDelay/2 (so the deadline is never missed by more than half a
// window). It exits when stopFlush is closed.
func (b *StdinBuffer) flushLoop() {
	t := time.NewTicker(EscFlushDelay / 2)
	defer t.Stop()
	for {
		select {
		case <-b.stopFlush:
			return
		case <-t.C:
			b.FlushEsc()
		}
	}
}

// OnKey registers the callback for regular key chunks.
func (b *StdinBuffer) OnKey(fn func(data string)) {
	b.mu.Lock()
	b.onKey = fn
	b.mu.Unlock()
}

// OnPaste registers the callback for bracketed-paste content.
func (b *StdinBuffer) OnPaste(fn func(text string)) {
	b.mu.Lock()
	b.onPaste = fn
	b.mu.Unlock()
}

// OnMouse registers the callback for mouse events.
func (b *StdinBuffer) OnMouse(fn func(msg core.MouseMsg)) {
	b.mu.Lock()
	b.onMouse = fn
	b.mu.Unlock()
}

// maxBufferBytes caps the non-paste input buffer. If a malfunctioning or
// hostile terminal sends a continuous stream of incomplete escape sequences
// (each leaving the parser waiting for a terminator), the buffer would grow
// without bound. This cap drops the accumulated bytes and starts fresh,
// preventing OOM. 1 MiB is far above any legitimate key sequence.
//
// This is a var (not const) so tests can override it with a small value to
// exercise the cap without paying the O(n) scan cost of consumeKeyEvents on
// a megabyte of incomplete escapes. Production code should never modify it.
var maxBufferBytes = 1 << 20 // 1 MiB

// Feed appends raw bytes and drains any complete events.
func (b *StdinBuffer) Feed(data []byte) {
	b.mu.Lock()
	// Guard against unbounded buffer growth from incomplete escape sequences.
	if len(b.buf)+len(data) > maxBufferBytes {
		b.buf = nil
		b.escPendingAt = time.Time{}
	}
	b.buf = append(b.buf, data...)
	keys, pastes, mice := b.drainLocked()
	onKey := b.onKey
	onPaste := b.onPaste
	onMouse := b.onMouse
	b.mu.Unlock()

	for _, p := range pastes {
		if onPaste != nil {
			onPaste(p)
		}
	}
	for _, m := range mice {
		if onMouse != nil {
			onMouse(m)
		}
	}
	for _, k := range keys {
		if onKey != nil {
			onKey(k)
		}
	}
}

// FeedString is a convenience wrapper around Feed.
func (b *StdinBuffer) FeedString(data string) { b.Feed([]byte(data)) }

// EscFlushDelay is how long a lone ESC byte may sit in the buffer before
// FlushEsc forces it out as a standalone ESC key event. The value is short
// enough to feel instantaneous to the user and long enough to never
// prematurely split a real CSI sequence that the kernel happened to
// fragment across two reads.
const EscFlushDelay = 50 * time.Millisecond

// FlushEsc emits any lone ESC byte that has been pending in the buffer
// longer than EscFlushDelay. It is a no-op when the buffer has no pending
// ESC or the ESC is part of a still-completing sequence.
//
// Two callers keep this working: the TUI render loop calls it every frame
// so the common case is responsive, and a background goroutine started by
// NewStdinBuffer calls it independently of the render loop, so a lone ESC
// still fires even when the render loop is blocked in a long Update.
//
// When the Kitty keyboard protocol is active (with disambiguate flag 1),
// a lone ESC is unambiguous: every key arrives wrapped in CSI u, so ESC
// cannot be the start of a multi-byte escape. In that case we flush
// immediately with zero delay, eliminating the 50ms latency that would
// otherwise make ESC feel sluggish on Kitty-class terminals.
func (b *StdinBuffer) FlushEsc() {
	b.mu.Lock()
	pending := len(b.buf) == 1 && b.buf[0] == 0x1B
	if !pending {
		b.mu.Unlock()
		return
	}
	// Kitty protocol active: ESC is unambiguous, flush immediately.
	if IsKittyProtocolActive() {
		b.buf = nil
		b.escPendingAt = time.Time{}
		onKey := b.onKey
		b.mu.Unlock()
		if onKey != nil {
			onKey("\x1b")
		}
		return
	}
	// Legacy mode: respect the 50ms delay to disambiguate from CSI sequences.
	if b.escPendingAt.IsZero() || time.Since(b.escPendingAt) < EscFlushDelay {
		b.mu.Unlock()
		return
	}
	// Promote the lone ESC into a one-byte event and clear the buffer.
	b.buf = nil
	b.escPendingAt = time.Time{}
	onKey := b.onKey
	b.mu.Unlock()
	if onKey != nil {
		onKey("\x1b")
	}
}

// Close stops the background flushLoop goroutine. It is safe to call
// multiple times.
func (b *StdinBuffer) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	close(b.stopFlush)
	b.mu.Unlock()
}

// Reset clears all buffered bytes without emitting events.
func (b *StdinBuffer) Reset() {
	b.mu.Lock()
	b.buf = nil
	b.inPaste = false
	b.pasteBuf.Reset()
	b.pasteBytes = 0
	b.escPendingAt = time.Time{}
	b.mu.Unlock()
}

// drainLocked walks the buffer and returns completed events. It mutates
// b.buf to keep only the incomplete tail.
func (b *StdinBuffer) drainLocked() (keys []string, pastes []string, mice []core.MouseMsg) { //nolint:gocognit // 渲染/分发/状态机复杂分支，拆分列入 P3
	const (
		pasteStart = "\x1b[200~"
		pasteEnd   = "\x1b[201~"
	)

	for {
		b.updateEscPendingLocked()

		// Kitty keyboard protocol: a lone ESC is unambiguous (every key
		// arrives CSI u-wrapped), so emit it immediately with zero delay.
		// This eliminates the 50ms EscFlushDelay latency on Kitty-class
		// terminals. updateEscPendingLocked already cleared escPendingAt
		// for this case; we just need to extract the byte.
		if len(b.buf) == 1 && b.buf[0] == 0x1B && IsKittyProtocolActive() {
			keys = append(keys, "\x1b")
			b.buf = nil
			return
		}

		if len(b.buf) == 0 {
			return
		}

		if b.inPaste {
			idx := indexBytes(b.buf, []byte(pasteEnd))
			if idx < 0 {
				// keep one near-marker in buf so split markers still match
				keepTail := len(pasteEnd) - 1
				if keepTail > len(b.buf) {
					keepTail = len(b.buf)
				}
				b.appendPaste(b.buf[:len(b.buf)-keepTail])
				b.buf = append(b.buf[:0], b.buf[len(b.buf)-keepTail:]...)
				return
			}
			b.appendPaste(b.buf[:idx])
			pastes = append(pastes, b.pasteBuf.String())
			b.pasteBuf.Reset()
			b.pasteBytes = 0
			b.inPaste = false
			b.buf = append(b.buf[:0], b.buf[idx+len(pasteEnd):]...)
			continue
		}

		// X11-style mouse: \x1b[MCbCxCy (3 bytes after [M)
		if len(b.buf) >= 6 && b.buf[0] == 0x1B && b.buf[1] == '[' && b.buf[2] == 'M' {
			cb := int(b.buf[3]) - 32
			cx := int(b.buf[4]) - 32
			cy := int(b.buf[5]) - 32
			if cx < 1 {
				cx = 1
			}
			if cy < 1 {
				cy = 1
			}
			mice = append(mice, parseX11Mouse(cb, int64(cx), int64(cy)))
			b.buf = append(b.buf[:0], b.buf[6:]...)
			continue
		}

		// SGR-style mouse: \x1b[<Cb;Cx;CyM or \x1b[<Cb;Cx;Cym
		if len(b.buf) >= 6 && b.buf[0] == 0x1B && b.buf[1] == '[' && b.buf[2] == '<' {
			end := indexByteFrom(b.buf, 3, 'M', 'm')
			if end < 0 {
				if len(b.buf) > 64 {
					b.buf = nil
				}
				return
			}
			seq := string(b.buf[3 : end+1]) // include terminating M/m
			m, ok := parseSGRMouse(seq)
			if ok {
				mice = append(mice, m)
			}
			b.buf = append(b.buf[:0], b.buf[end+1:]...)
			continue
		}

		// Look for next paste-start; anything before is key data.
		idx := indexBytes(b.buf, []byte(pasteStart))
		if idx >= 0 {
			if idx > 0 {
				chunks := splitInputIntoEvents(string(b.buf[:idx]))
				keys = append(keys, chunks...)
			}
			b.buf = append(b.buf[:0], b.buf[idx+len(pasteStart):]...)
			b.inPaste = true
			continue
		}

		// No paste start in view — drain any fully-formed events, keep tail.
		consumed, chunks := consumeKeyEvents(b.buf)
		keys = append(keys, chunks...)
		if consumed == 0 {
			return
		}
		b.buf = append(b.buf[:0], b.buf[consumed:]...)
	}
}

// updateEscPendingLocked maintains escPendingAt according to current buffer
// state. A lone ESC byte starts/keeps a pending timer; any other state clears
// it. When the Kitty keyboard protocol is active, a lone ESC is unambiguous
// (every key arrives CSI u-wrapped), so it is promoted to a key event
// immediately with zero delay.
func (b *StdinBuffer) updateEscPendingLocked() {
	if len(b.buf) == 1 && b.buf[0] == 0x1B {
		if IsKittyProtocolActive() {
			// Kitty protocol: ESC is unambiguous, flush now via escPendingAt
			// sentinel. The caller (drainLocked) checks this and emits the
			// key directly so there's zero latency.
			b.escPendingAt = time.Time{}
			return
		}
		if b.escPendingAt.IsZero() {
			b.escPendingAt = time.Now()
		}
		return
	}
	b.escPendingAt = time.Time{}
}

func (b *StdinBuffer) appendPaste(p []byte) {
	if len(p) == 0 {
		return
	}
	remaining := b.opts.MaxPasteBytes - b.pasteBytes
	if remaining <= 0 {
		return
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	b.pasteBuf.Write(p)
	b.pasteBytes += int64(len(p))
}
