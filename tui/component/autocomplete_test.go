package component

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestAutocompleteStaticProvider(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr: "/",
		Suggestions: []core.Suggestion{
			{Label: "help", InsertText: "help"},
			{Label: "clear", InsertText: "clear"},
		},
	})

	var out string
	var cursor int64
	ac.OnApply(func(newValue string, newCursor int64, _ core.Suggestion) {
		out = newValue
		cursor = newCursor
	})

	// User types "/hel" with cursor at end.
	ac.Refresh("/hel", 4)
	if !ac.Active() {
		t.Fatalf("expected autocomplete active")
	}
	cur, ok := ac.list.CurrentItem()
	if !ok || cur.Value != "help" {
		t.Fatalf("expected help, got %v", cur)
	}
	ac.Update(core.KeyMsg{Data: "\t"})
	if out != "/help" {
		t.Fatalf("expected /help, got %q", out)
	}
	if cursor != int64(len([]rune("/help"))) {
		t.Fatalf("unexpected cursor %d", cursor)
	}
}

func TestAutocompleteNoTrigger(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr:  "/",
		Suggestions: []core.Suggestion{{Label: "help", InsertText: "help"}},
	})
	ac.Refresh("hello", 5)
	if ac.Active() {
		t.Fatalf("should not activate without trigger")
	}
}

// TestAutocompleteDoubleTriggerGuard verifies that when the suggestion
// InsertText already starts with the trigger prefix, the trigger is not
// prepended again. This guards against the double-trigger bug ("//help").
func TestAutocompleteDoubleTriggerGuard(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr: "/",
		Suggestions: []core.Suggestion{
			{Label: "/chat", InsertText: "/chat"},
		},
	})

	var appliedValue string
	ac.OnApply(func(newValue string, _ int64, _ core.Suggestion) {
		appliedValue = newValue
	})

	// Type "/c" — trigger "/" found, token "c", suggestion "/chat" matched.
	ac.Refresh("/c", 2)
	if !ac.Active() {
		t.Fatalf("expected autocomplete active")
	}

	// Apply the suggestion (Tab key).
	ac.Update(core.KeyMsg{Data: "\t"})
	if appliedValue != "/chat" {
		t.Fatalf("expected %q, got %q — double-trigger bug", "/chat", appliedValue)
	}
}

// TestAutocompleteTriggersOnceForPlainValue verifies that when the
// suggestion InsertText does NOT start with the trigger, the trigger is
// prepended correctly (normal case, no regression).
func TestAutocompleteTriggersOnceForPlainValue(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr: "/",
		Suggestions: []core.Suggestion{
			{Label: "help", InsertText: "help"},
		},
	})

	var appliedValue string
	ac.OnApply(func(newValue string, _ int64, _ core.Suggestion) {
		appliedValue = newValue
	})

	ac.Refresh("/h", 2)
	if !ac.Active() {
		t.Fatalf("expected autocomplete active")
	}

	ac.Update(core.KeyMsg{Data: "\t"})
	if appliedValue != "/help" {
		t.Fatalf("expected %q, got %q", "/help", appliedValue)
	}
}
