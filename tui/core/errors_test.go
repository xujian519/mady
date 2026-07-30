package core

import (
	"errors"
	"fmt"
	"testing"
)

func TestTerminalErrorWraps(t *testing.T) {
	inner := errors.New("io timeout")
	err := &TerminalError{Op: "restore termios", Err: inner}

	// errors.Is should traverse the chain via Unwrap.
	if !errors.Is(err, inner) {
		t.Fatal("errors.Is should find inner error through TerminalError")
	}

	// errors.As should extract TerminalError.
	var target *TerminalError
	if !errors.As(err, &target) {
		t.Fatal("errors.As should extract *TerminalError")
	}
	if target.Op != "restore termios" {
		t.Fatalf("expected Op 'restore termios', got %q", target.Op)
	}

	// Error string should include the op and inner.
	got := err.Error()
	want := "tui: terminal restore termios: io timeout"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestTerminalErrorNilErr(t *testing.T) {
	err := &TerminalError{Op: "start"}
	got := err.Error()
	want := "tui: terminal start"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestClipboardErrorWraps(t *testing.T) {
	inner := errors.New("pbcopy not found")
	err := &ClipboardError{Op: "copy", Err: inner}

	if !errors.Is(err, inner) {
		t.Fatal("errors.Is should find inner error through ClipboardError")
	}

	var target *ClipboardError
	if !errors.As(err, &target) {
		t.Fatal("errors.As should extract *ClipboardError")
	}
	if target.Op != "copy" {
		t.Fatalf("expected Op 'copy', got %q", target.Op)
	}

	got := err.Error()
	want := "tui: clipboard copy: pbcopy not found"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFmtErrorfWRoundTrip(t *testing.T) {
	// Verify that fmt.Errorf with %w preserves the wrapper chain.
	inner := errors.New("underlying issue")
	wrapped := fmt.Errorf("outer: %w", &TerminalError{Op: "test", Err: inner})

	// Should unwrap through all layers.
	if !errors.Is(wrapped, inner) {
		t.Fatal("errors.Is should reach the inner error through fmt.Errorf + TerminalError")
	}

	var target *TerminalError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should extract *TerminalError through fmt.Errorf")
	}
	if target.Op != "test" {
		t.Fatalf("expected Op 'test', got %q", target.Op)
	}
}
