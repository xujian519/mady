package theme

import (
	"testing"
	"time"
)

func TestRegisterThemeReplacesExisting(t *testing.T) {
	// Replace an existing builtin theme; after the test restore the original
	// entry so other tests keep seeing the canonical registry.
	RegisterTheme(ThemeInfo{Name: "mady-dark", Display: "Replaced Display", Dark: true}, DefaultMadyDark)
	t.Cleanup(func() {
		RegisterTheme(ThemeInfo{Name: "mady-dark", Display: "Mady Dark", Dark: true}, DefaultMadyDark)
	})

	info := ThemeInfoByName("mady-dark")
	if info == nil {
		t.Fatal("mady-dark not found after replacement")
	}
	if info.Display != "Replaced Display" {
		t.Fatalf("display = %q, want Replaced Display", info.Display)
	}
	// The replaced theme must still apply.
	if sem := ApplyThemeByName("mady-dark"); sem == nil {
		t.Fatal("ApplyThemeByName failed after replacement")
	}
}

func TestApplyThemeByNameUnknown(t *testing.T) {
	if sem := ApplyThemeByName("does-not-exist"); sem != nil {
		t.Fatalf("ApplyThemeByName(unknown) = %v, want nil", sem)
	}
}

func TestStartAutoThemeWatcher(t *testing.T) {
	savePalette(t)
	// Watches system appearance with the default 2s poll; verify it starts
	// without crashing and cancels cleanly.
	cancel := StartAutoThemeWatcher()
	time.Sleep(80 * time.Millisecond)
	cancel()
	time.Sleep(30 * time.Millisecond)
}

func TestAutoThemeFactory(t *testing.T) {
	sem := autoThemeFactory()
	if sem == nil {
		t.Fatal("autoThemeFactory returned nil")
	}
	if sem.Name != "dark" && sem.Name != "light" {
		t.Fatalf("auto theme name = %q, want dark or light", sem.Name)
	}
}
