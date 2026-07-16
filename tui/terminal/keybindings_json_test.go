package terminal

// keybindings_json_test.go verifies LoadUserBindingsJSON: valid tokens apply,
// unknown modifiers warn but are still accepted, empty tokens are skipped,
// and a malformed JSON returns an error.

import (
	"sort"
	"testing"
)

func TestLoadUserBindingsJSONAppliesValid(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	json := `{
		"tui.editor.deleteWordBackward": ["ctrl+backspace"],
		"tui.input.submit": ["enter", "ctrl+j"]
	}`
	warnings, err := km.LoadUserBindingsJSON([]byte(json))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for valid keymap, got %v", warnings)
	}
	got := km.Keys("tui.editor.deleteWordBackward")
	if len(got) != 1 || got[0] != "ctrl+backspace" {
		t.Errorf("deleteWordBackward = %v, want [ctrl+backspace]", got)
	}
}

func TestLoadUserBindingsJSONWarnsUnknownModifier(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	json := `{"tui.editor.cursorLeft": ["foobar+a"]}`
	warnings, err := km.LoadUserBindingsJSON([]byte(json))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Unknown modifier "foobar" should produce a warning, but the token is
	// still accepted (parseKeyID drops unknown modifiers silently).
	found := false
	for _, w := range warnings {
		if contains(w, "foobar") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning about unknown modifier 'foobar', got %v", warnings)
	}
}

func TestLoadUserBindingsJSONSkipsEmptyToken(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	json := `{"tui.editor.cursorLeft": ["", "ctrl+a"]}`
	warnings, err := km.LoadUserBindingsJSON([]byte(json))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty token warned, valid one applied.
	if len(warnings) == 0 {
		t.Errorf("expected a warning for empty token")
	}
	got := km.Keys("tui.editor.cursorLeft")
	if len(got) != 1 || got[0] != "ctrl+a" {
		t.Errorf("cursorLeft = %v, want [ctrl+a]", got)
	}
}

func TestLoadUserBindingsJSONMalformed(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	if _, err := km.LoadUserBindingsJSON([]byte(`{not json`)); err == nil {
		t.Errorf("expected error for malformed JSON")
	}
}

func TestLoadUserBindingsJSONEmptyClears(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	_, _ = km.LoadUserBindingsJSON([]byte(`{"tui.editor.cursorLeft": ["ctrl+a"]}`))
	// Empty payload clears overrides back to defaults.
	if _, err := km.LoadUserBindingsJSON(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	def := DefaultKeybindings()["tui.editor.cursorLeft"].DefaultKeys
	got := km.Keys("tui.editor.cursorLeft")
	if !sameKeys(got, def) {
		t.Errorf("after clear, cursorLeft = %v, want default %v", got, def)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func sameKeys(a, b []KeyID) bool {
	if len(a) != len(b) {
		return false
	}
	ac := make([]string, len(a))
	bc := make([]string, len(b))
	for i := range a {
		ac[i] = string(a[i])
		bc[i] = string(b[i])
	}
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}
