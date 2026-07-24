package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunListPrompts(t *testing.T) {
	// Capture stdout.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	errCh := make(chan error, 1)
	go func() {
		errCh <- runListPrompts()
		w.Close()
	}()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("runListPrompts: %v", err)
	}
	os.Stdout = old

	out := buf.String()
	if !strings.Contains(out, "Available prompt templates:") && !strings.Contains(out, "总计:") {
		t.Fatalf("unexpected output: %s", out)
	}
}
