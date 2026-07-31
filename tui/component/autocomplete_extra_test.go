package component

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// TestFilePathProviderTriggerDefault verifies the default "@" trigger.
func TestFilePathProviderTriggerDefault(t *testing.T) {
	p := &FilePathProvider{}
	if got := p.Trigger(); got != "@" {
		t.Fatalf("expected default trigger @, got %q", got)
	}
	p.TriggerStr = "~"
	if got := p.Trigger(); got != "~" {
		t.Fatalf("expected configured trigger ~, got %q", got)
	}
}

// TestFilePathProviderComplete walks a temp directory structure.
func TestFilePathProviderComplete(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel, content string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("docs/readme.md", "x")
	mustWrite("docs/spec.md", "x")
	mustWrite("src/main.go", "x")
	mustWrite("src/util/helper.go", "x")
	mustWrite(".hidden.txt", "x")

	p := &FilePathProvider{RootDir: root}

	// Root listing: hidden file excluded by default.
	got := p.Complete("")
	labels := make([]string, len(got))
	for i, s := range got {
		labels[i] = s.Label
	}
	joined := strings.Join(labels, "|")
	if strings.Contains(joined, ".hidden") {
		t.Fatalf("hidden file should be excluded by default, got %v", labels)
	}
	if !strings.Contains(joined, "docs/") || !strings.Contains(joined, "src/") {
		t.Fatalf("expected dirs docs/ and src/, got %v", labels)
	}
	for _, s := range got {
		if s.InsertText == "" {
			t.Fatalf("expected non-empty InsertText, got %+v", s)
		}
	}

	// Prefix filter.
	got = p.Complete("d")
	if len(got) != 1 || got[0].Label != "docs/" {
		t.Fatalf("expected only docs/, got %v", got)
	}

	// Subdirectory token.
	got = p.Complete("docs/")
	if len(got) != 2 {
		t.Fatalf("expected 2 files under docs/, got %v", got)
	}
	for _, s := range got {
		if !strings.HasPrefix(s.Label, "docs/") {
			t.Fatalf("expected docs/ prefix, got %q", s.Label)
		}
		if s.Description != "file" {
			t.Fatalf("expected file description, got %q", s.Description)
		}
	}

	// IncludeHidden.
	p.IncludeHidden = true
	got = p.Complete("")
	foundHidden := false
	for _, s := range got {
		if s.Label == ".hidden.txt" {
			foundHidden = true
		}
	}
	if !foundHidden {
		t.Fatal("expected hidden file with IncludeHidden")
	}

	// Nonexistent dir -> nil.
	if got := p.Complete("nope/"); got != nil {
		t.Fatalf("expected nil for missing dir, got %v", got)
	}
}

// TestFilePathProviderCompleteGetwd covers the RootDir == "" path.
func TestFilePathProviderCompleteGetwd(t *testing.T) {
	p := &FilePathProvider{}
	got := p.Complete("")
	// Current working directory always has entries.
	if len(got) == 0 {
		t.Fatal("expected entries from cwd")
	}
}

type alwaysActiveProvider struct{}

func (alwaysActiveProvider) Trigger() string { return "" }
func (alwaysActiveProvider) Complete(token string) []core.Suggestion {
	return []core.Suggestion{{Label: "word-" + token, InsertText: "word-" + token}}
}

// TestAutocompleteAlwaysActiveProvider covers the Trigger()=="" branch.
func TestAutocompleteAlwaysActiveProvider(t *testing.T) {
	ac := NewAutocomplete(alwaysActiveProvider{})
	ac.Refresh("hel", 3)
	if !ac.Active() {
		t.Fatal("expected always-active provider to activate")
	}
	var applied string
	ac.OnApply(func(newValue string, _ int64, _ core.Suggestion) { applied = newValue })
	ac.Update(core.KeyMsg{Data: "\t"})
	if applied != "word-hel" {
		t.Fatalf("expected word-hel applied, got %q", applied)
	}
}

// TestAutocompleteAddProvider verifies runtime provider registration.
func TestAutocompleteAddProvider(t *testing.T) {
	ac := NewAutocomplete()
	ac.AddProvider(&StaticProvider{
		TriggerStr:  "/",
		Suggestions: []core.Suggestion{{Label: "run", InsertText: "run"}},
	})
	ac.Refresh("/", 1)
	if !ac.Active() {
		t.Fatal("expected provider added at runtime to activate")
	}
}

// TestAutocompleteDismiss verifies Escape dismisses and fires OnDismiss.
func TestAutocompleteDismiss(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr:  "/",
		Suggestions: []core.Suggestion{{Label: "help", InsertText: "help"}},
	})
	dismissed := false
	ac.OnDismiss(func() { dismissed = true })
	ac.Refresh("/", 1)
	if !ac.Active() {
		t.Fatal("expected active")
	}
	ac.Update(core.KeyMsg{Data: "\x1b"}) // escape
	if ac.Active() {
		t.Fatal("expected inactive after escape")
	}
	if !dismissed {
		t.Fatal("expected OnDismiss to fire")
	}
}

// TestAutocompleteHide verifies Hide deactivates and clears items.
func TestAutocompleteHide(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr:  "/",
		Suggestions: []core.Suggestion{{Label: "help", InsertText: "help"}},
	})
	ac.Refresh("/", 1)
	ac.Hide()
	if ac.Active() {
		t.Fatal("expected inactive after Hide")
	}
	if n := len(ac.list.items); n != 0 {
		t.Fatalf("expected cleared list, got %d items", n)
	}
}

// TestAutocompleteRenderActive verifies Render draws the suggestion list.
func TestAutocompleteRenderActive(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr:  "/",
		Suggestions: []core.Suggestion{{Label: "help", InsertText: "help"}},
	})
	if lines := ac.Render(20); lines != nil {
		t.Fatalf("expected nil render when inactive, got %v", lines)
	}
	ac.Refresh("/", 1)
	lines := ac.Render(20)
	if len(lines) == 0 {
		t.Fatal("expected rendered lines when active")
	}
	for _, ln := range lines {
		if w := core.VisibleWidth(ln); w > 20 {
			t.Fatalf("line width %d > 20 (line=%q)", w, ln)
		}
	}
}

// TestAutocompleteFocus verifies focus delegation to the inner list.
func TestAutocompleteFocus(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr:  "/",
		Suggestions: []core.Suggestion{{Label: "help", InsertText: "help"}},
	})
	ac.SetFocused(true)
	if !ac.IsFocused() {
		t.Fatal("expected focused after SetFocused(true)")
	}
	ac.SetFocused(false)
	if ac.IsFocused() {
		t.Fatal("expected unfocused after SetFocused(false)")
	}
	ac.Invalidate() // no-op fan-out, must not panic
}

// TestAutocompleteCursorOutOfRange covers detectTriggerLocked's bounds check.
func TestAutocompleteCursorOutOfRange(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr:  "/",
		Suggestions: []core.Suggestion{{Label: "help", InsertText: "help"}},
	})
	ac.Refresh("/hel", -1)
	if ac.Active() {
		t.Fatal("expected inactive for out-of-range cursor")
	}
	ac.Refresh("/hel", 99)
	if ac.Active() {
		t.Fatal("expected inactive for cursor beyond buffer")
	}
}

// TestAutocompleteApplyCurrentNoItem covers applyCurrent's empty-list guard.
func TestAutocompleteApplyCurrentNoItem(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr:  "/",
		Suggestions: []core.Suggestion{{Label: "help", InsertText: "help"}},
	})
	ac.mu.Lock()
	ac.active = true // force active with an empty list
	ac.mu.Unlock()
	ac.applyCurrent() // must not panic, must not call OnApply
}

// TestAutocompleteWindowSizeMsg verifies WindowSizeMsg triggers Invalidate.
func TestAutocompleteWindowSizeMsg(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr:  "/",
		Suggestions: []core.Suggestion{{Label: "help", InsertText: "help"}},
	})
	ac.Update(core.WindowSizeMsg{Width: 80, Height: 24})
}

// TestAutocompleteWordBoundary verifies detection uses the word preceding
// the cursor: no trigger in the word means no activation.
func TestAutocompleteWordBoundary(t *testing.T) {
	ac := NewAutocomplete(&StaticProvider{
		TriggerStr:  "/",
		Suggestions: []core.Suggestion{{Label: "help", InsertText: "help"}},
	})
	// Word before cursor has no trigger -> inactive.
	ac.Refresh("hello", 5)
	if ac.Active() {
		t.Fatal("expected inactive when word has no trigger")
	}
	ac.Refresh("/help", 5)
	if !ac.Active() {
		t.Fatal("expected active when trigger at word start")
	}
}
