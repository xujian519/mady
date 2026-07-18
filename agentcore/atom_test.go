package agentcore

import (
	"strings"
	"testing"
)

func TestRegisterAndLookupAtom(t *testing.T) {
	// Verify built-in registration.
	atoms := []string{"search", "extract", "compare", "reasoning", "approval-gate"}
	for _, name := range atoms {
		a := LookupAtom(name)
		if a == nil {
			t.Fatalf("atom %q not found", name)
		}
		if a.Name() != name {
			t.Fatalf("atom %q: Name() = %q", name, a.Name())
		}
	}
}

func TestListAtoms(t *testing.T) {
	all := ListAtoms()
	if len(all) != 5 {
		t.Fatalf("expected 5 atoms, got %d: %v", len(all), names(all))
	}
	// Verify sorted order.
	for i := 1; i < len(all); i++ {
		if all[i-1].Name() >= all[i].Name() {
			t.Fatalf("atoms not sorted: %q >= %q", all[i-1].Name(), all[i].Name())
		}
	}
}

func TestListAtomsByCategory(t *testing.T) {
	tests := []struct {
		category string
		expected int
	}{
		{AtomCategorySearch, 1},
		{AtomCategoryExtract, 1},
		{AtomCategoryCompare, 1},
		{AtomCategoryReason, 1},
		{AtomCategoryGate, 1},
		{"nonexistent", 0},
	}
	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			atoms := ListAtomsByCategory(tt.category)
			if len(atoms) != tt.expected {
				t.Fatalf("%s: expected %d, got %d", tt.category, tt.expected, len(atoms))
			}
		})
	}
}

func TestAtomSchemas(t *testing.T) {
	// Verify each atom's schema is non-empty.
	for _, a := range ListAtoms() {
		if len(a.InputSchema()) == 0 {
			t.Fatalf("atom %q: empty input schema", a.Name())
		}
		if len(a.OutputSchema()) == 0 {
			t.Fatalf("atom %q: empty output schema", a.Name())
		}
		if a.Description() == "" {
			t.Fatalf("atom %q: empty description", a.Name())
		}
	}
}

func TestAtomIndex(t *testing.T) {
	idx := AtomIndex()
	if idx == "" {
		t.Fatal("AtomIndex returned empty string")
	}
	// Should contain all categories.
	for _, cat := range []string{"search", "extract", "compare", "reason", "gate"} {
		if !strings.Contains(idx, "["+cat+"]") {
			t.Errorf("AtomIndex missing category %q: %s", cat, idx)
		}
	}
}

func TestRegisterAtom_Override(t *testing.T) {
	// Custom atom with same name as built-in.
	custom := testAtom{name: "search", desc: "custom search"}
	RegisterAtom(custom)
	defer func() {
		// Restore original.
		RegisterAtom(searchAtom{})
	}()

	a := LookupAtom("search")
	if a == nil {
		t.Fatal("atom not found after override")
	}
	// Verify the custom one won.
	if a.Description() != "custom search" {
		t.Fatalf("expected custom, got %q", a.Description())
	}
}

func TestPluginStage_AtomValidation(t *testing.T) {
	// Valid: atom set, no tool needed.
	p := PluginManifest{
		Name:        "test",
		Version:     "0.1.0",
		Domain:      "patent",
		Description: "Test plugin with atoms",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				{ID: "s1", Atom: "search", Description: "Search"},
			},
		},
	}
	if err := ValidatePlugin(p); err != nil {
		t.Fatalf("valid atom plugin rejected: %v", err)
	}

	// Invalid: unknown atom.
	p.Pipeline.Stages[0].Atom = "nonexistent"
	if err := ValidatePlugin(p); err == nil {
		t.Fatal("expected error for unknown atom")
	}

	// Invalid: neither tool nor atom set.
	p.Pipeline.Stages[0].Atom = ""
	if err := ValidatePlugin(p); err == nil {
		t.Fatal("expected error for missing tool and atom")
	}

	// Valid: both tool and atom set (tool is optional when atom exists).
	p.Pipeline.Stages[0] = PluginStage{ID: "s1", Atom: "extract", Tool: "my_tool", Description: "Extract"}
	if err := ValidatePlugin(p); err != nil {
		t.Fatalf("stage with atom+tool rejected: %v", err)
	}
}

// testAtom is a simple Atom implementation for testing.
type testAtom struct {
	name string
	desc string
}

func (a testAtom) Name() string           { return a.name }
func (a testAtom) Description() string    { return a.desc }
func (a testAtom) Category() string       { return AtomCategorySearch }
func (a testAtom) InputSchema() []string  { return []string{"q"} }
func (a testAtom) OutputSchema() []string { return []string{"r"} }

func names(atoms []Atom) []string {
	n := make([]string, len(atoms))
	for i, a := range atoms {
		n[i] = a.Name()
	}
	return n
}
