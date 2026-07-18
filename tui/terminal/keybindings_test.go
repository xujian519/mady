package terminal

import (
	"testing"
)

func TestKeybindingsManagerRegister(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	km.Register("app.quit", KeybindingDef{DefaultKeys: []KeyID{"ctrl+q"}, Description: "Quit"})
	if def := km.Definition("app.quit"); def.Description != "Quit" {
		t.Fatalf("registered definition not found: %+v", def)
	}
	keys := km.Keys("app.quit")
	if len(keys) != 1 || keys[0] != "ctrl+q" {
		t.Fatalf("want [ctrl+q], got %v", keys)
	}
}

func TestKeybindingsManagerMatches(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	if !km.Matches("\x1b[A", "tui.editor.cursorUp") {
		t.Fatal("should match up arrow")
	}
	if !km.Matches("\r", "tui.input.submit") {
		t.Fatal("should match enter")
	}
	if km.Matches("x", "tui.input.submit") {
		t.Fatal("should not match x as submit")
	}
}

func TestKeybindingsManagerUserOverride(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	km.SetUserBindings(map[string][]KeyID{
		"tui.input.submit": {"ctrl+enter"},
	})
	if km.Matches("\r", "tui.input.submit") {
		t.Fatal("default enter should be overridden")
	}
	if !km.Matches("\x1b[13;5u", "tui.input.submit") {
		t.Fatal("ctrl+enter should match submit after override")
	}
}

func TestKeybindingsManagerConflict(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	// Bind the same key to two different actions.
	km.SetUserBindings(map[string][]KeyID{
		"tui.input.submit":  {"ctrl+x"},
		"tui.select.cancel": {"ctrl+x"},
	})
	conflicts := km.Conflicts()
	if len(conflicts) == 0 {
		t.Fatal("expected conflict for shared key")
	}
	found := false
	for _, c := range conflicts {
		if c.Key == "ctrl+x" && len(c.Keybindings) == 2 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ctrl+x conflict, got %+v", conflicts)
	}
}

func TestKeybindingsManagerAll(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	all := km.All()
	if len(all) == 0 {
		t.Fatal("All() should return bindings")
	}
	if _, ok := all["tui.input.submit"]; !ok {
		t.Fatal("All() should include tui.input.submit")
	}
}

func TestKeybindingsManagerLoadJSON(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	data := []byte(`{"tui.input.submit": ["ctrl+enter"], "tui.select.cancel": ["escape"]}`)
	warnings, err := km.LoadUserBindingsJSON(data)
	if err != nil {
		t.Fatalf("LoadUserBindingsJSON failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !km.Matches("\x1b[13;5u", "tui.input.submit") {
		t.Fatal("loaded ctrl+enter should match submit")
	}
}

func TestKeybindingsManagerLoadJSONInvalidToken(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	data := []byte(`{"tui.input.submit": ["foobar+a"]}`)
	warnings, err := km.LoadUserBindingsJSON(data)
	if err != nil {
		t.Fatalf("LoadUserBindingsJSON failed: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warning for invalid token")
	}
}

func TestKeybindingsManagerLoadJSONEmpty(t *testing.T) {
	km := NewKeybindingsManager(DefaultKeybindings())
	warnings, err := km.LoadUserBindingsJSON(nil)
	if err != nil {
		t.Fatalf("LoadUserBindingsJSON(nil) failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}
