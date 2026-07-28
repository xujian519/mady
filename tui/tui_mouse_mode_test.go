package tui

import (
	"strings"
	"sync"
	"testing"

	terminal "github.com/xujian519/mady/tui/terminal"
)

// byteSinkTerminal captures every byte written to it verbatim, so tests can
// assert on the exact DEC private-mode escape sequences emitted by
// enableMouse / disableMouse.
type byteSinkTerminal struct {
	mu      sync.Mutex
	out     []byte
	started bool
}

func (b *byteSinkTerminal) Start(func([]byte), func()) error {
	b.mu.Lock()
	b.started = true
	b.mu.Unlock()
	return nil
}

func (b *byteSinkTerminal) Stop() error {
	b.mu.Lock()
	b.started = false
	b.mu.Unlock()
	return nil
}

func (b *byteSinkTerminal) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.out = append(b.out, p...)
	return len(p), nil
}

func (b *byteSinkTerminal) Size() (int64, int64)               { return 80, 24 }
func (b *byteSinkTerminal) HideCursor()                        {}
func (b *byteSinkTerminal) ShowCursor()                        {}
func (b *byteSinkTerminal) ClearLine()                         {}
func (b *byteSinkTerminal) ClearFromCursor()                   {}
func (b *byteSinkTerminal) ClearScreen()                       {}
func (b *byteSinkTerminal) MoveBy(int64)                       {}
func (b *byteSinkTerminal) MoveTo(int64, int64)                {}
func (b *byteSinkTerminal) PushKittyKeyboard()                 {}
func (b *byteSinkTerminal) PopKittyKeyboard()                  {}
func (b *byteSinkTerminal) Context() *terminal.TerminalContext { return nil }

func (b *byteSinkTerminal) output() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := make([]byte, len(b.out))
	copy(cp, b.out)
	return cp
}

// containsSeq reports whether the captured output contains the given escape
// sequence as a substring.
func containsSeq(out []byte, seq string) bool {
	return strings.Contains(string(out), seq)
}

// TestEnableMouseDisablesAlternateScroll verifies that enabling mouse
// reporting also disables DEC 1007 (alternate scroll mode). Without
// ?1007l, terminals that default to alternate-scroll-on translate wheel
// events into ↑/↓ key sequences inside the alt screen, so the wheel never
// reaches ChatHistory.handleMouse and scrolling the transcript is broken.
func TestEnableMouseDisablesAlternateScroll(t *testing.T) {
	cases := []struct {
		mode string
	}{
		{"sgr"},
		{"auto"},
		{"on"},
		{"x11"},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			term := &byteSinkTerminal{}
			tui := NewTUI(term, TUIOptions{MouseMode: c.mode})
			if err := tui.Start(); err != nil {
				t.Fatalf("Start: %v", err)
			}
			out := term.output()
			_ = tui.Stop()

			if !containsSeq(out, "\x1b[?1007l") {
				t.Errorf("mode %q: expected \\x1b[?1007l (disable alt scroll) in output, got %q",
					c.mode, string(out))
			}
		})
	}
}

// TestEnableMouseSGRSequences asserts the full SGR enable sequence is emitted
// in the correct order: ?1007l precedes the mouse-tracking modes.
func TestEnableMouseSGRSequences(t *testing.T) {
	term := &byteSinkTerminal{}
	tui := NewTUI(term, TUIOptions{MouseMode: "sgr"})
	if err := tui.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	out := term.output()
	_ = tui.Stop()

	s := string(out)
	idx1007l := strings.Index(s, "\x1b[?1007l")
	idx1002h := strings.Index(s, "\x1b[?1002h")
	idx1006h := strings.Index(s, "\x1b[?1006h")

	for _, want := range []string{"\x1b[?1007l", "\x1b[?1002h", "\x1b[?1006h"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing enable sequence %q in output %q", want, s)
		}
	}
	if !(idx1007l < idx1002h && idx1007l < idx1006h) {
		t.Errorf("expected ?1007l before mouse-tracking modes; positions 1007l=%d 1002h=%d 1006h=%d in %q",
			idx1007l, idx1002h, idx1006h, s)
	}
}

// TestDisableMouseRestoresAlternateScroll verifies that DisableMouse
// re-enables DEC 1007 so the terminal's default wheel behavior returns when
// the TUI tears down.
func TestDisableMouseRestoresAlternateScroll(t *testing.T) {
	cases := []struct {
		mode string
	}{
		{"sgr"},
		{"x11"},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			term := &byteSinkTerminal{}
			tui := NewTUI(term, TUIOptions{MouseMode: c.mode})
			if err := tui.Start(); err != nil {
				t.Fatalf("Start: %v", err)
			}
			// Reset capture so we only inspect the Stop path.
			term.mu.Lock()
			term.out = term.out[:0]
			term.mu.Unlock()

			_ = tui.Stop()
			out := term.output()

			if !containsSeq(out, "\x1b[?1007h") {
				t.Errorf("mode %q: expected \\x1b[?1007h (restore alt scroll) on Stop, got %q",
					c.mode, string(out))
			}
		})
	}
}

// TestDisableMouseOrder verifies the disable sequence reverses enable:
// mouse-tracking modes are turned off before 1007 is restored.
func TestDisableMouseOrder(t *testing.T) {
	term := &byteSinkTerminal{}
	tui := NewTUI(term, TUIOptions{MouseMode: "sgr"})
	if err := tui.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	term.mu.Lock()
	term.out = term.out[:0]
	term.mu.Unlock()
	_ = tui.Stop()

	s := string(term.output())
	idx1006l := strings.Index(s, "\x1b[?1006l")
	idx1002l := strings.Index(s, "\x1b[?1002l")
	idx1007h := strings.Index(s, "\x1b[?1007h")

	if idx1006l < 0 || idx1002l < 0 || idx1007h < 0 {
		t.Fatalf("missing disable sequences in %q", s)
	}
	if !(idx1007h > idx1006l && idx1007h > idx1002l) {
		t.Errorf("expected ?1007h after mouse-tracking disable; positions 1006l=%d 1002l=%d 1007h=%d in %q",
			idx1006l, idx1002l, idx1007h, s)
	}
}

// TestMouseOffEmitsNothing verifies that a disabled (off) mouse mode does
// not emit any 1007 toggles — there is nothing to reverse.
func TestMouseOffEmitsNothing(t *testing.T) {
	term := &byteSinkTerminal{}
	tui := NewTUI(term, TUIOptions{MouseMode: "off"})
	if err := tui.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	out := term.output()
	_ = tui.Stop()

	if containsSeq(out, "\x1b[?1007l") || containsSeq(out, "\x1b[?1007h") {
		t.Errorf("mode off should not emit 1007 toggles, got %q", string(out))
	}
}
