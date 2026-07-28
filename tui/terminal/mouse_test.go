package terminal

import (
	"strconv"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// TestParseSGRMouseWheelDirections verifies all four wheel directions are
// correctly decoded from SGR mouse sequences.
func TestParseSGRMouseWheelDirections(t *testing.T) {
	cases := []struct {
		name   string
		cb     int
		action core.MouseAction
	}{
		{"up", 64, core.MouseWheelUp},
		{"down", 65, core.MouseWheelDown},
		{"left", 66, core.MouseWheelLeft},
		{"right", 67, core.MouseWheelRight},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// SGR format: <cb;cx;cyM (M = press/motion)
			seq := strconv.Itoa(c.cb) + ";10;20M"
			m, ok := parseSGRMouse(seq)
			if !ok {
				t.Fatalf("parseSGRMouse(%q) returned ok=false", seq)
			}
			if m.Action != c.action {
				t.Errorf("action = %d, want %d", m.Action, c.action)
			}
			// Coordinates are 1-based in the sequence, 0-based in MouseMsg
			if m.Row != 19 || m.Col != 9 {
				t.Errorf("coords = (%d,%d), want (19,9)", m.Row, m.Col)
			}
		})
	}
}

// TestParseSGRMouseExtendedButtons verifies side buttons (8=back, 9=forward)
// are correctly decoded.
func TestParseSGRMouseExtendedButtons(t *testing.T) {
	cases := []struct {
		name   string
		cb     int
		action core.MouseAction
		button int64
	}{
		{"back", 128, core.MouseBackButton, 8},
		{"forward", 129, core.MouseForwardButton, 9},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seq := strconv.Itoa(c.cb) + ";5;5M"
			m, ok := parseSGRMouse(seq)
			if !ok {
				t.Fatalf("parseSGRMouse(%q) returned ok=false", seq)
			}
			if m.Action != c.action {
				t.Errorf("action = %d, want %d", m.Action, c.action)
			}
			if m.Button != c.button {
				t.Errorf("button = %d, want %d", m.Button, c.button)
			}
		})
	}
}

// TestParseSGRMouseStandardButtons verifies the standard buttons still work
// (regression check after the switch reordering).
func TestParseSGRMouseStandardButtons(t *testing.T) {
	cases := []struct {
		name   string
		cb     int
		press  bool // true = M (press), false = m (release)
		action core.MouseAction
		button int64
	}{
		{"left_press", 0, true, core.MousePress, 0},
		{"middle_press", 1, true, core.MousePress, 1},
		{"right_press", 2, true, core.MousePress, 2},
		{"left_release", 0, false, core.MouseRelease, 0},
		{"right_release", 2, false, core.MouseRelease, 0},
		{"motion_left", 32, true, core.MouseMotion, 0}, // cb=32 = 0x20 = motion, button 0
		{"motion_right", 34, true, core.MouseMotion, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			term := "M"
			if !c.press {
				term = "m"
			}
			seq := strconv.Itoa(c.cb) + ";3;3" + term
			m, ok := parseSGRMouse(seq)
			if !ok {
				t.Fatalf("parseSGRMouse(%q) returned ok=false", seq)
			}
			if m.Action != c.action {
				t.Errorf("action = %d, want %d", m.Action, c.action)
			}
			if m.Button != c.button {
				t.Errorf("button = %d, want %d", m.Button, c.button)
			}
		})
	}
}

// TestParseX11MouseWheelDirections verifies X11-style wheel decoding for
// all four directions.
func TestParseX11MouseWheelDirections(t *testing.T) {
	cases := []struct {
		name   string
		cb     int // raw byte value (before -32 subtraction)
		action core.MouseAction
	}{
		{"up", 32 + 64, core.MouseWheelUp},
		{"down", 32 + 65, core.MouseWheelDown},
		{"left", 32 + 66, core.MouseWheelLeft},
		{"right", 32 + 67, core.MouseWheelRight},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// parseX11Mouse expects cb already subtracted by 32
			m := parseX11Mouse(c.cb-32, 1, 1)
			if m.Action != c.action {
				t.Errorf("action = %d, want %d", m.Action, c.action)
			}
		})
	}
}

// TestParseX11MouseExtendedButtons verifies X11-style extended button decoding.
func TestParseX11MouseExtendedButtons(t *testing.T) {
	cases := []struct {
		name   string
		cb     int
		action core.MouseAction
		button int64
	}{
		{"back", 128, core.MouseBackButton, 8},
		{"forward", 129, core.MouseForwardButton, 9},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := parseX11Mouse(c.cb, 1, 1)
			if m.Action != c.action {
				t.Errorf("action = %d, want %d", m.Action, c.action)
			}
			if m.Button != c.button {
				t.Errorf("button = %d, want %d", m.Button, c.button)
			}
		})
	}
}

// TestParseX11MouseStandardButtons regression check for standard X11 buttons.
func TestParseX11MouseStandardButtons(t *testing.T) {
	cases := []struct {
		name   string
		cb     int
		action core.MouseAction
		button int64
	}{
		{"left_press", 0, core.MousePress, 0},
		{"middle_press", 1, core.MousePress, 1},
		{"right_press", 2, core.MousePress, 2},
		{"release", 3, core.MouseRelease, 0},
		{"motion", 32, core.MouseMotion, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := parseX11Mouse(c.cb, 1, 1)
			if m.Action != c.action {
				t.Errorf("action = %d, want %d", m.Action, c.action)
			}
			if m.Button != c.button {
				t.Errorf("button = %d, want %d", m.Button, c.button)
			}
		})
	}
}
