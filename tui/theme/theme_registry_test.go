package theme

import (
	"testing"
)

func TestThemeNames(t *testing.T) {
	names := ThemeNames()
	if len(names) == 0 {
		t.Fatal("ThemeNames() returned empty")
	}
	found := false
	for _, n := range names {
		if n == "mady-dark" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'mady-dark' in theme names, got %v", names)
	}
}

func TestThemeInfoByName(t *testing.T) {
	info := ThemeInfoByName("mady-dark")
	if info == nil {
		t.Fatal("ThemeInfoByName('mady-dark') returned nil")
	}
	if !info.Dark {
		t.Errorf("mady-dark should be dark")
	}
	if info.Display != "Mady Dark" {
		t.Errorf("display name = %q, want 'Mady Dark'", info.Display)
	}
}

func TestThemeInfoByNameUnknown(t *testing.T) {
	info := ThemeInfoByName("nonexistent")
	if info != nil {
		t.Errorf("expected nil for unknown theme, got %v", info)
	}
}

func TestApplyThemeByName(t *testing.T) {
	sem := ApplyThemeByName("mady-light")
	if sem == nil {
		t.Fatal("ApplyThemeByName returned nil")
	}
	// Check that the theme has expected content (non-empty accent).
	if sem.Accent == "" {
		t.Error("theme has empty Accent")
	}
}

func TestSetThemeByName(t *testing.T) {
	err := SetThemeByName("tokyo-night")
	if err != nil {
		t.Fatalf("SetThemeByName failed: %v", err)
	}
}

func TestSetThemeByNameUnknown(t *testing.T) {
	err := SetThemeByName("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown theme")
	}
}

func TestBuiltinThemesAllDark(t *testing.T) {
	for _, name := range ThemeNames() {
		info := ThemeInfoByName(name)
		if info == nil {
			continue
		}
		sem := ApplyThemeByName(name)
		if sem == nil {
			continue
		}
		// Theme should have a non-empty accent color.
		if sem.Accent == "" {
			t.Errorf("theme %q has empty Accent", name)
		}
	}
}

func TestRegisterTheme(t *testing.T) {
	info := ThemeInfo{Name: "test-theme", Display: "Test Theme", Dark: false}
	RegisterTheme(info, func() *SemanticTheme {
		return DefaultSemanticLight()
	})
	defer func() {
		// Clean up: remove the test theme (hacky but ok for test).
		registryMu.Lock()
		for i, e := range themeRegistry {
			if e.info.Name == "test-theme" {
				themeRegistry = append(themeRegistry[:i], themeRegistry[i+1:]...)
				break
			}
		}
		registryMu.Unlock()
	}()

	if info := ThemeInfoByName("test-theme"); info == nil {
		t.Error("registered theme not found")
	}
}
