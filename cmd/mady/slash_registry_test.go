package main

// Tests for the slash command registry: verifies lookup precedence (prefix
// commands before exact), the Available gate (multi-domain / review modes),
// and that Suggestions reflects availability. Handlers themselves are
// exercised via the existing tui_session integration paths, so these tests
// focus on dispatch wiring rather than command behavior.

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestSlashRegistryLookupExactAndPrefix(t *testing.T) {
	s := &tuiSession{useMultiDomain: true, reviewMode: true}
	r := s.buildSlashRegistry()

	cases := []struct {
		input string
		want  string // expected command Name
	}{
		{"/help", "help"},
		{"/clear", "clear"},
		{"/new", "clear"}, // alias
		{"/thinking", "thinking"},
		{"/thinking effort high", "thinking"}, // prefix match
		{"/theme dark", "theme"},
		{"/case list", "case"},
		{"/skill:foo", "skill"},
		{"/mode", "mode"},
		{"/quit", "quit"},
		{"exit", "quit"}, // non-slash alias
	}
	for _, c := range cases {
		cmd := r.Lookup(c.input, s)
		if cmd == nil {
			t.Errorf("Lookup(%q) returned nil, want %q", c.input, c.want)
			continue
		}
		if cmd.Name != c.want {
			t.Errorf("Lookup(%q) = %q, want %q", c.input, cmd.Name, c.want)
		}
	}
}

func TestSlashRegistryAvailableGate(t *testing.T) {
	// /mode requires multi-domain; /approve and /reject require review mode.
	multiOff := &tuiSession{useMultiDomain: false, reviewMode: false}
	r := multiOff.buildSlashRegistry()

	if cmd := r.Lookup("/mode", multiOff); cmd != nil {
		t.Errorf("/mode should be unavailable without multi-domain, got %v", cmd)
	}
	if cmd := r.Lookup("/approve", multiOff); cmd != nil {
		t.Errorf("/approve should be unavailable without review mode, got %v", cmd)
	}

	multiOn := &tuiSession{useMultiDomain: true, reviewMode: true}
	r2 := multiOn.buildSlashRegistry()
	if cmd := r2.Lookup("/mode", multiOn); cmd == nil {
		t.Errorf("/mode should be available in multi-domain mode")
	}
	if cmd := r2.Lookup("/approve", multiOn); cmd == nil {
		t.Errorf("/approve should be available in review mode")
	}
}

func TestSlashRegistryUnknownCommand(t *testing.T) {
	s := &tuiSession{}
	r := s.buildSlashRegistry()
	if cmd := r.Lookup("/nonexistent", s); cmd != nil {
		t.Errorf("unknown command should return nil, got %q", cmd.Name)
	}
}

func TestSlashRegistrySuggestionsReflectAvailability(t *testing.T) {
	s := &tuiSession{useMultiDomain: false, reviewMode: false}
	r := s.buildSlashRegistry()
	sugs := r.Suggestions(s)

	hasMode := false
	for _, sg := range sugs {
		if sg.InsertText == "/mode" {
			hasMode = true
		}
	}
	if hasMode {
		t.Errorf("/mode should not appear in suggestions when multi-domain is off: %v", sugs)
	}

	// Core commands are always present.
	joined := joinCoreSuggestions(sugs)
	for _, want := range []string{"/help", "/clear", "/thinking", "/quit"} {
		if !strings.Contains(joined, want) {
			t.Errorf("suggestions missing %q: %v", want, joined)
		}
	}
}

func joinCoreSuggestions(sugs []core.Suggestion) string {
	parts := make([]string, len(sugs))
	for i, s := range sugs {
		parts[i] = s.InsertText
	}
	return strings.Join(parts, " ")
}
