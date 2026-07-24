package component

import (
	"strings"
	"testing"
)

func TestChipStateInsertAndRemove(t *testing.T) {
	var cs chipState

	chip := &Chip{Kind: ChipFile, Value: "main.go", Display: "@file:main.go"}
	cp := cs.InsertChip(0, 0, chip)
	if cp.HardRow != 0 || cp.RuneStart != 0 {
		t.Errorf("InsertChip: got (%d,%d), want (0,0)", cp.HardRow, cp.RuneStart)
	}
	if len(cs.chips) != 1 {
		t.Errorf("expected 1 chip, got %d", len(cs.chips))
	}

	// Remove the chip.
	ok := cs.RemoveChipAt(0, 0)
	if !ok {
		t.Error("RemoveChipAt returned false")
	}
	if len(cs.chips) != 0 {
		t.Errorf("expected 0 chips after remove, got %d", len(cs.chips))
	}
}

func TestChipStateChipAt(t *testing.T) {
	var cs chipState

	chip := &Chip{Kind: ChipFile, Value: "main.go", Display: "main.go"}
	cs.InsertChip(0, 6, chip)

	// Should find chip at col 6.
	found, ok := cs.ChipAt(0, 6)
	if !ok || found.Chip.Value != "main.go" {
		t.Error("ChipAt(0,6) should find chip")
	}

	// Should NOT find at col 0.
	_, ok = cs.ChipAt(0, 0)
	if ok {
		t.Error("ChipAt(0,0) should NOT find chip")
	}
}

func TestChipStateClear(t *testing.T) {
	var cs chipState
	cs.InsertChip(0, 0, &Chip{Kind: ChipFile, Value: "main.go", Display: "main.go"})
	cs.Clear()
	if len(cs.chips) != 0 {
		t.Error("expected 0 chips after Clear")
	}
}

func TestChipSort(t *testing.T) {
	var cs chipState

	// Insert out of order.
	cs.InsertChip(1, 10, &Chip{Kind: ChipFile, Value: "a", Display: "a"})
	cs.InsertChip(0, 20, &Chip{Kind: ChipFile, Value: "b", Display: "b"})
	cs.InsertChip(0, 5, &Chip{Kind: ChipFile, Value: "c", Display: "c"})

	if len(cs.chips) != 3 {
		t.Fatalf("expected 3 chips, got %d", len(cs.chips))
	}

	// Should be sorted: (0,5), (0,20), (1,10)
	if cs.chips[0].RuneStart != 5 || cs.chips[1].RuneStart != 20 || cs.chips[2].HardRow != 1 {
		t.Errorf("chips not sorted: got %+v", cs.chips)
	}
}

func TestEditorInsertChip(t *testing.T) {
	ed := NewEditor(nil)
	ed.SetTextFn(func(s string) string { return s })
	ed.SetPromptFn(func(s string) string { return s })

	chip := &Chip{Kind: ChipFile, Value: "main.go", Display: "main.go"}
	cp := ed.InsertChip(chip)
	if cp.Chip.Value != "main.go" {
		t.Errorf("expected chip value 'main.go', got %q", cp.Chip.Value)
	}

	chips := ed.Chips()
	if len(chips) != 1 {
		t.Errorf("expected 1 chip, got %d", len(chips))
	}
}

func TestEditorRemoveChip(t *testing.T) {
	ed := NewEditor(nil)
	chip := &Chip{Kind: ChipFile, Value: "main.go", Display: "main.go"}
	ed.InsertChip(chip)

	ok := ed.RemoveChipAt(0, 0)
	if !ok {
		t.Error("RemoveChipAt returned false")
	}
	if len(ed.Chips()) != 0 {
		t.Error("expected 0 chips after removal")
	}
}

func TestChipKindPrefix(t *testing.T) {
	tests := []struct {
		kind ChipKind
		want string
	}{
		{ChipFile, "@file:"},
		{ChipFolder, "@folder:"},
		{ChipSession, "@session:"},
	}

	for _, tt := range tests {
		got := chipKindPrefix(tt.kind)
		if got != tt.want {
			t.Errorf("chipKindPrefix(%d) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestChipDisplayText(t *testing.T) {
	chip := &Chip{Kind: ChipFile, Value: "main.go", Display: "main.go"}

	display := chipKindPrefix(chip.Kind) + chip.Display
	if !strings.Contains(display, "@file:") {
		t.Errorf("expected @file: prefix in display, got %q", display)
	}
	if !strings.Contains(display, "main.go") {
		t.Errorf("expected 'main.go' in display, got %q", display)
	}
}

func TestEditorChipRender(t *testing.T) {
	ed := NewEditor(nil)
	ed.SetTextFn(func(s string) string { return s })
	ed.SetPromptFn(func(s string) string { return s })
	ed.SetValue("hello world")

	// Insert a chip at position 0 (before "hello") using InsertChipAt.
	chip := &Chip{Kind: ChipFile, Value: "main.go", Display: "main.go"}
	ed.InsertChipAt(0, 0, chip)

	// Render and verify the chip is visible in the output.
	lines := ed.Render(80)
	if len(lines) == 0 {
		t.Fatal("expected rendered lines")
	}
	rendered := lines[0]
	if !strings.Contains(rendered, "@file:") {
		t.Errorf("expected chip prefix @file: in render output, got %q", rendered)
	}
	if !strings.Contains(rendered, "main.go") {
		t.Errorf("expected chip value 'main.go' in render output, got %q", rendered)
	}
}

func TestEditorChipRenderWithText(t *testing.T) {
	ed := NewEditor(nil)
	ed.SetTextFn(func(s string) string { return s })
	ed.SetPromptFn(func(s string) string { return s })
	ed.SetValue("run ")

	// Insert chip after "run " (at position 4).
	chip := &Chip{Kind: ChipFile, Value: "build.sh", Display: "build.sh"}
	ed.InsertChipAt(0, 4, chip)

	lines := ed.Render(80)
	if len(lines) == 0 {
		t.Fatal("expected rendered lines")
	}
	rendered := lines[0]
	if !strings.Contains(rendered, "run ") {
		t.Errorf("expected 'run ' in output, got %q", rendered)
	}
	if !strings.Contains(rendered, "@file:") {
		t.Errorf("expected '@file:' in output, got %q", rendered)
	}
}
