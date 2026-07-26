package terminal

import (
	"os"
	"testing"
)

func TestTerminalSupportsKittyKeyboard_Detection(t *testing.T) {
	// Reset the cached terminal context before each sub-test so env
	// isolation via t.Setenv has effect. Without this, the
	// once-initialized context from an earlier sub-test (or a prior
	// test in the suite) would be reused and env changes ignored.
	reset := func() { ResetTerminalContext() }
	tests := []struct {
		name  string
		setup func(t *testing.T)
		want  bool
	}{
		{
			name: "kitty via KITTY_WINDOW_ID",
			setup: func(t *testing.T) {
				t.Setenv("KITTY_WINDOW_ID", "42")
				t.Setenv("TERM", "")
				t.Setenv("TERM_PROGRAM", "")
			},
			want: true,
		},
		{
			name: "ghostty TERM_PROGRAM",
			setup: func(t *testing.T) {
				t.Setenv("KITTY_WINDOW_ID", "")
				t.Setenv("TERM", "")
				t.Setenv("TERM_PROGRAM", "ghostty")
			},
			want: true,
		},
		{
			name: "apple terminal",
			setup: func(t *testing.T) {
				t.Setenv("KITTY_WINDOW_ID", "")
				t.Setenv("TERM", "xterm-256color")
				t.Setenv("TERM_PROGRAM", "Apple_Terminal")
			},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			reset() // clear cached context so env changes take effect
			ok, _ := CurrentTerminalContext().SupportsKittyKeyboard()
			if got := ok; got != tc.want {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestProcessTerminalKittyKbdMode(t *testing.T) {
	// SetKittyKeyboardFlags no longer writes to a package global —
	// flags are stored on the ProcessTerminal instance. No cleanup needed.

	tm := NewProcessTerminal()
	tm.SetKittyKeyboardMode("on")
	if tm.enableKittyKeyboard != kittyKbdForceOn {
		t.Fatalf("force on not set")
	}
	tm.SetKittyKeyboardMode("off")
	if tm.enableKittyKeyboard != kittyKbdForceOff {
		t.Fatalf("force off not set")
	}
	tm.SetKittyKeyboardMode("auto")
	if tm.enableKittyKeyboard != kittyKbdAuto {
		t.Fatalf("auto not set")
	}
	tm.SetKittyKeyboardFlags(5)
	if tm.kittyFlags != 5 {
		t.Fatalf("flags=%d", tm.kittyFlags)
	}
	// flag <1 clamps to 1
	tm.SetKittyKeyboardFlags(0)
	if tm.kittyFlags != 1 {
		t.Fatalf("flags=%d want 1", tm.kittyFlags)
	}
	_ = os.Stdout // keep pkg used
}
