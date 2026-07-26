package terminal

import (
	"strings"
	"testing"
	"time"
)

func TestStdinBufferBasicKeys(t *testing.T) {
	b := NewStdinBuffer()
	var keys []string
	b.OnKey(func(d string) { keys = append(keys, d) })
	b.FeedString("abc")
	if strings.Join(keys, "") != "abc" {
		t.Errorf("want keys = abc, got %q", keys)
	}
}

func TestStdinBufferFragmentedEscape(t *testing.T) {
	b := NewStdinBuffer()
	var keys []string
	b.OnKey(func(d string) { keys = append(keys, d) })

	// Arrow-up arrives in two chunks: "\x1b[" then "A".
	b.FeedString("\x1b[")
	if len(keys) != 0 {
		t.Errorf("should buffer incomplete escape, got %q", keys)
	}
	b.FeedString("A")
	if len(keys) != 1 || keys[0] != "\x1b[A" {
		t.Errorf("expected reassembled \\x1b[A, got %q", keys)
	}
}

func TestStdinBufferBracketedPaste(t *testing.T) {
	b := NewStdinBuffer()
	var keys, pastes []string
	b.OnKey(func(d string) { keys = append(keys, d) })
	b.OnPaste(func(t string) { pastes = append(pastes, t) })

	b.FeedString("x\x1b[200~hello\nworld\x1b[201~y")

	if len(pastes) != 1 || pastes[0] != "hello\nworld" {
		t.Errorf("want paste=hello\\nworld, got %v", pastes)
	}
	if strings.Join(keys, "") != "xy" {
		t.Errorf("want keys = xy, got %v", keys)
	}
}

func TestStdinBufferFragmentedPasteMarker(t *testing.T) {
	b := NewStdinBuffer()
	var pastes []string
	b.OnPaste(func(t string) { pastes = append(pastes, t) })

	// Paste end marker split across feeds.
	b.FeedString("\x1b[200~hello")
	b.FeedString("\x1b[20")
	b.FeedString("1~")
	if len(pastes) != 1 || pastes[0] != "hello" {
		t.Errorf("want paste=hello, got %v", pastes)
	}
}

// TestStdinBufferFlushEscLone verifies that a lone ESC byte that sits in
// the buffer past EscFlushDelay is forced out as a standalone ESC key
// event when FlushEsc is called. This is what makes "press ESC to close
// a modal" actually work in a TU application.
func TestStdinBufferFlushEscLone(t *testing.T) {
	b := NewStdinBuffer()
	var keys []string
	b.OnKey(func(d string) { keys = append(keys, d) })

	b.FeedString("\x1b")
	// No continuation arrives.
	if len(keys) != 0 {
		t.Fatalf("lone ESC must not be emitted immediately, got %q", keys)
	}
	// Without waiting, FlushEsc must still NOT emit (delay not elapsed).
	b.FlushEsc()
	if len(keys) != 0 {
		t.Fatalf("FlushEsc emitted before EscFlushDelay elapsed, got %q", keys)
	}
	// Backdate the pending timestamp so the next FlushEsc sees an elapsed delay.
	b.mu.Lock()
	b.escPendingAt = time.Now().Add(-EscFlushDelay - time.Millisecond)
	b.mu.Unlock()
	b.FlushEsc()
	if len(keys) != 1 || keys[0] != "\x1b" {
		t.Fatalf("want one \\x1b key event after delay, got %q", keys)
	}
	// Buffer must be cleared so a subsequent Feed of unrelated bytes
	// does not see a stale ESC in front.
	b.mu.Lock()
	still := len(b.buf)
	b.mu.Unlock()
	if still != 0 {
		t.Fatalf("buffer should be empty after FlushEsc, has %d bytes", still)
	}
}

// TestStdinBufferFlushEscLeavesCsiAlone verifies that FlushEsc does not
// split a real CSI sequence: only a true lone ESC (buffer == [0x1B]) is
// flushed.
func TestStdinBufferFlushEscLeavesCsiAlone(t *testing.T) {
	b := NewStdinBuffer()
	var keys []string
	b.OnKey(func(d string) { keys = append(keys, d) })

	// Buffer a partial CSI arrow-up; mark the pending timestamp as
	// already-elapsed to make sure FlushEsc is tempted, then verify it
	// leaves the buffer alone.
	b.FeedString("\x1b[")
	b.mu.Lock()
	b.escPendingAt = time.Now().Add(-EscFlushDelay - time.Millisecond)
	b.mu.Unlock()
	b.FlushEsc()
	if len(keys) != 0 {
		t.Fatalf("FlushEsc must not touch a partial CSI, got %q", keys)
	}
	// Finishing the sequence still reassembles correctly.
	b.FeedString("A")
	if len(keys) != 1 || keys[0] != "\x1b[A" {
		t.Fatalf("want reassembled \\x1b[A, got %q", keys)
	}
}

func TestStdinBufferLoneEscSetsPendingTimestamp(t *testing.T) {
	b := NewStdinBuffer()
	b.FeedString("\x1b")

	b.mu.Lock()
	pendingAt := b.escPendingAt
	b.mu.Unlock()

	if pendingAt.IsZero() {
		t.Fatalf("expected lone ESC to set escPendingAt")
	}
}

func TestStdinBufferPendingEscClearsWhenSequenceContinues(t *testing.T) {
	b := NewStdinBuffer()
	b.FeedString("\x1b")
	b.FeedString("[")

	b.mu.Lock()
	pendingAt := b.escPendingAt
	b.mu.Unlock()

	if !pendingAt.IsZero() {
		t.Fatalf("expected escPendingAt to clear for non-lone ESC sequence")
	}
}

func TestStdinBufferMaxBufferBytes(t *testing.T) {
	// Use a small cap so the test runs fast. Without this override,
	// exercising the real threat (incomplete escapes accumulating) would
	// require feeding 1 MiB of "\x1b[" and waiting ~36s while
	// consumeKeyEvents scans every byte on each Feed call.
	orig := maxBufferBytes
	maxBufferBytes = 50
	t.Cleanup(func() { maxBufferBytes = orig })

	b := NewStdinBuffer()

	// Feed incomplete escape sequences in small chunks. Each "\x1b[" is an
	// incomplete CSI that consumeKeyEvents cannot drain, so they accumulate
	// in b.buf. Without the cap, this would grow unbounded.
	for i := 0; i < 30; i++ {
		b.FeedString("\x1b[") // 2 bytes each = 60 bytes total, exceeds cap of 50
	}

	b.mu.Lock()
	bufLen := len(b.buf)
	b.mu.Unlock()

	// The cap triggers when accumulated + new data exceeds the limit,
	// resetting b.buf to nil before appending the current chunk. So the
	// buffer never holds more than ~cap bytes of incomplete escapes.
	if bufLen > maxBufferBytes {
		t.Fatalf("buffer exceeded cap after accumulation: len=%d, cap=%d", bufLen, maxBufferBytes)
	}

	// Verify the buffer still works after the cap event — a normal key
	// should still be processed correctly.
	var keys []string
	b.OnKey(func(d string) { keys = append(keys, d) })
	b.FeedString("x")
	found := false
	for _, k := range keys {
		if k == "x" {
			found = true
		}
	}
	if !found {
		t.Fatalf("buffer should still process keys after cap reset")
	}
}
