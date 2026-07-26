package terminal

import (
	"testing"
)

func TestCursorMovement(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"CursorUp(5)", CursorUp(5), "\033[5A"},
		{"CursorDown(3)", CursorDown(3), "\033[3B"},
		{"CursorRight(10)", CursorRight(10), "\033[10C"},
		{"CursorLeft(1)", CursorLeft(1), "\033[1D"},
		{"CursorUp(0) returns empty", CursorUp(0), ""},
		{"CursorUp(-1) returns empty", CursorUp(-1), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestScreenClearing(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ClearLine", ClearLine(), "\033[2K"},
		{"ClearToEnd", ClearToEnd(), "\033[0K"},
		{"ClearScreen", ClearScreen(), "\033[2J\033[H"},
		{"ClearFromCursorDown", ClearFromCursorDown(), "\033[0J"},
		{"ClearToEndOfLine", ClearToEndOfLine(), "\033[0K"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestCursorSaveRestore(t *testing.T) {
	if SaveCursor() != "\033[s" {
		t.Fatalf("SaveCursor: got %q, want %q", SaveCursor(), "\033[s")
	}
	if RestoreCursor() != "\033[u" {
		t.Fatalf("RestoreCursor: got %q, want %q", RestoreCursor(), "\033[u")
	}
}

func TestCursorVisibility(t *testing.T) {
	if HideCursor() != "\033[?25l" {
		t.Fatalf("HideCursor: got %q", HideCursor())
	}
	if ShowCursor() != "\033[?25h" {
		t.Fatalf("ShowCursor: got %q", ShowCursor())
	}
}

func TestCursorHome(t *testing.T) {
	if CursorHome() != "\033[H" {
		t.Fatalf("CursorHome: got %q, want %q", CursorHome(), "\033[H")
	}
}

func TestCursorPosition(t *testing.T) {
	cases := []struct {
		row, col int64
		want     string
	}{
		{1, 1, "\033[1;1H"},
		{10, 80, "\033[10;80H"},
		{0, 0, "\033[0;0H"}, // edge case: zero values
		{999, 999, "\033[999;999H"},
	}
	for _, tc := range cases {
		got := CursorPosition(tc.row, tc.col)
		if got != tc.want {
			t.Fatalf("CursorPosition(%d,%d): got %q, want %q", tc.row, tc.col, got, tc.want)
		}
	}
}

func TestEscnEdgeCases(t *testing.T) {
	// n <= 0 should return empty string (no-op)
	if escn("A", 0) != "" {
		t.Fatalf("escn with n=0 should return empty")
	}
	if escn("A", -5) != "" {
		t.Fatalf("escn with n=-5 should return empty")
	}
	// n = 1 should produce minimal sequence
	if got := escn("A", 1); got != "\033[1A" {
		t.Fatalf("escn(A,1): got %q, want %q", got, "\033[1A")
	}
}

func TestEscnLargeNumbers(t *testing.T) {
	// Large numbers should still format correctly
	got := CursorUp(1000000)
	want := "\033[1000000A"
	if got != want {
		t.Fatalf("CursorUp(1000000): got %q, want %q", got, want)
	}
}

func TestConstants(t *testing.T) {
	if Esc != "\033[" {
		t.Fatalf("Esc constant: got %q, want %q", Esc, "\033[")
	}
	if Reset != "\033[0m" {
		t.Fatalf("Reset constant: got %q, want %q", Reset, "\033[0m")
	}
}

// FuzzCursorPosition ensures no panic on arbitrary int64 inputs.
func FuzzCursorPosition(f *testing.F) {
	f.Add(int64(1), int64(1))
	f.Add(int64(0), int64(0))
	f.Add(int64(-1), int64(-1))
	f.Fuzz(func(t *testing.T, row, col int64) {
		s := CursorPosition(row, col)
		if len(s) == 0 {
			t.Fatalf("CursorPosition should never return empty string")
		}
		// Result must start with ESC[
		if s[:2] != Esc {
			t.Fatalf("result must start with ESC[: got %q", s)
		}
	})
}
