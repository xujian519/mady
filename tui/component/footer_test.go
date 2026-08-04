package component

import (
	"strings"
	"testing"
)

func TestFooterDefaultGroups(t *testing.T) {
	f := NewFooter()
	lines := f.Render(80)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "help") {
		t.Fatalf("expected 'help' in footer, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "cmd") {
		t.Fatalf("expected 'cmd' in footer, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "quit") {
		t.Fatalf("expected 'quit' in footer, got %q", lines[0])
	}
}

func TestFooterCompactLayout(t *testing.T) {
	f := NewFooter()
	// Compact mode: < 80 cols, only show 3 groups.
	f.SetCompact(true)
	lines := f.Render(60)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// Should still show help, cmd, quit.
	if !strings.Contains(lines[0], "help") {
		t.Fatalf("expected 'help' in compact footer, got %q", lines[0])
	}
}

func TestFooterWidthFallback(t *testing.T) {
	f := NewFooter()
	// Very narrow: Render with width=10 should still produce a line.
	lines := f.Render(10)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// Fallback should show minimal shortcut.
	if !strings.Contains(lines[0], "?") && !strings.Contains(lines[0], "help") {
		t.Fatalf("expected fallback hint in narrow footer, got %q", lines[0])
	}
}

func TestFooterRegisterGroup(t *testing.T) {
	f := NewFooter()
	f.RegisterGroup("search", FooterItem{Key: "/", Desc: "search"})
	lines := f.Render(200) // ≥160 shows all groups
	if !strings.Contains(lines[0], "search") {
		t.Fatalf("expected 'search' in footer after RegisterGroup, got %q", lines[0])
	}
}

func TestFooterClearGroup(t *testing.T) {
	f := NewFooter()
	f.ClearGroup("help") // clear first default group
	lines := f.Render(80)
	if strings.Contains(lines[0], "help") {
		t.Fatalf("expected no 'help' after ClearGroup, got %q", lines[0])
	}
}

func TestFooterEmptyGroups(t *testing.T) {
	f := &Footer{} // no groups
	lines := f.Render(80)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0] != "" {
		t.Fatalf("expected empty line for no groups, got %q", lines[0])
	}
}

func TestFooterNoColorMode(t *testing.T) {
	f := NewFooter()
	lines := f.Render(80)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// The output should render without crashing regardless of color state.
	// This test verifies basic rendering stability.
	if len(lines[0]) == 0 {
		t.Fatalf("expected non-empty footer line")
	}
}

// TestFooterEmptyFirstGroupWidthFallback is the regression test for the
// overflow fallback path: when the first visible group was registered with
// zero items, `visible[0].Items[0]` panicked with index-out-of-range. The
// fallback must scan for the first non-empty group instead.
func TestFooterEmptyFirstGroupWidthFallback(t *testing.T) {
	f := &Footer{}
	f.RegisterGroup("empty") // empty group sorts first
	f.RegisterGroup("long", FooterItem{Key: "ctrl+q", Desc: strings.Repeat("very long description ", 12)})

	lines := f.Render(60) // width 60 → compact layout, line overflows → fallback
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "ctrl+q") {
		t.Errorf("fallback should show first non-empty group's item, got %q", lines[0])
	}
}
