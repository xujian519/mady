package component

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestSettingsListNavigationAndCycle(t *testing.T) {
	entries := []SettingEntry{
		{Key: "theme", Label: "Theme", Options: []SettingOption{{Value: "light", Label: "Light"}, {Value: "dark", Label: "Dark"}}, Current: 0},
		{Key: "mode", Label: "Mode", Options: []SettingOption{{Value: "auto", Label: "Auto"}, {Value: "256", Label: "256"}, {Value: "truecolor", Label: "Truecolor"}}, Current: 0},
	}
	sl := NewSettingsList(entries)
	sl.SetFocused(true)

	if v, ok := sl.GetValue("theme"); !ok || v != "light" {
		t.Fatalf("initial theme value: want light, got %q", v)
	}

	// Cycle value forward on the first entry.
	sl.Update(core.KeyMsg{Data: "\x1b[C"}) // right
	if v, ok := sl.GetValue("theme"); !ok || v != "dark" {
		t.Fatalf("after right want dark, got %q", v)
	}

	// Move down to second entry.
	sl.Update(core.KeyMsg{Data: "\x1b[B"}) // down
	if v, ok := sl.GetValue("mode"); !ok || v != "auto" {
		t.Fatalf("initial mode value: want auto, got %q", v)
	}

	// Cycle forward twice: auto -> 256 -> truecolor.
	sl.Update(core.KeyMsg{Data: "\x1b[C"})
	sl.Update(core.KeyMsg{Data: "\x1b[C"})
	if v, ok := sl.GetValue("mode"); !ok || v != "truecolor" {
		t.Fatalf("after two right want truecolor, got %q", v)
	}

	// Cycle backward: truecolor -> 256.
	sl.Update(core.KeyMsg{Data: "\x1b[D"}) // left
	if v, ok := sl.GetValue("mode"); !ok || v != "256" {
		t.Fatalf("after left want 256, got %q", v)
	}
}

func TestSettingsListCallbacks(t *testing.T) {
	entries := []SettingEntry{
		{Key: "a", Label: "A", Options: []SettingOption{{Value: "1", Label: "One"}, {Value: "2", Label: "Two"}}, Current: 0},
	}
	sl := NewSettingsList(entries)
	sl.SetFocused(true)

	var changed, submitted SettingEntry
	sl.OnChange(func(e SettingEntry) { changed = e })
	sl.OnSubmit(func(e SettingEntry) { submitted = e })

	sl.Update(core.KeyMsg{Data: "\x1b[C"})
	if changed.Key != "a" || changed.Current != 1 {
		t.Fatalf("OnChange not fired correctly: %+v", changed)
	}

	sl.Update(core.KeyMsg{Data: "\r"}) // enter
	if submitted.Key != "a" || submitted.Current != 0 {
		// enter cycles forward once more: 1 -> 0, then fires onSubmit
		t.Fatalf("OnSubmit not fired correctly: %+v", submitted)
	}
}

func TestSettingsListSetValue(t *testing.T) {
	entries := []SettingEntry{
		{Key: "a", Label: "A", Options: []SettingOption{{Value: "x", Label: "X"}, {Value: "y", Label: "Y"}}, Current: 0},
	}
	sl := NewSettingsList(entries)
	if ok := sl.SetValue("a", 1); !ok {
		t.Fatal("SetValue should return true for valid index")
	}
	if v, ok := sl.GetValue("a"); !ok || v != "y" {
		t.Fatalf("want y, got %q", v)
	}
	if ok := sl.SetValue("a", 5); ok {
		t.Fatal("SetValue should return false for out-of-range index")
	}
	if ok := sl.SetValue("missing", 0); ok {
		t.Fatal("SetValue should return false for unknown key")
	}
}

func TestSettingsListRender(t *testing.T) {
	entries := []SettingEntry{
		{Key: "a", Label: "Alpha", Options: []SettingOption{{Value: "on", Label: "On"}}, Current: 0, Description: "desc"},
	}
	sl := NewSettingsList(entries)
	lines := sl.Render(40)
	if len(lines) == 0 {
		t.Fatal("expected at least one rendered line")
	}
	if len(lines[0]) == 0 {
		t.Fatal("expected non-empty rendered line")
	}
}

func TestSettingsListEmpty(t *testing.T) {
	sl := NewSettingsList(nil)
	lines := sl.Render(40)
	if len(lines) != 1 {
		t.Fatalf("expected one line for empty settings, got %d", len(lines))
	}
}
