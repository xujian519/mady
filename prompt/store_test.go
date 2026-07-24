package prompt

import (
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

// mustWritePromptTemplate is defined in loader_test.go; kept as package-level helper.

func TestPromptStore_Embedded(t *testing.T) {
	store, err := NewPromptStore()
	if err != nil {
		t.Fatalf("NewPromptStore: %v", err)
	}
	if store.Count() == 0 {
		t.Fatal("expected embedded templates to be loaded")
	}

	// Every built-in template should be findable by name.
	for _, name := range []string{
		"claim-drafting",
		"spec-drafting",
		"abstract-drafting",
		"novelty-analysis",
		"inventiveness-analysis",
		"infringement-analysis",
		"validity-analysis",
		"oa-analysis",
		"oa-response",
		"disclosure-analysis",
		"feature-extraction",
		"ipc-search",
		"keyword-search",
		"semantic-search",
		"case-search",
		"contract-review",
		"statute-interpretation",
		"checker-verdict",
		"slop-filter",
		"trademark-infringement",
	} {
		tmpl, ok := store.FindByName(name)
		if !ok {
			t.Errorf("embedded template %q not found", name)
			continue
		}
		if tmpl.SystemPrompt == "" {
			t.Errorf("template %q has empty system_prompt", name)
		}
	}
}

func TestPromptStore_UserOverlay(t *testing.T) {
	userDir := t.TempDir()
	mustWritePromptTemplate(t, filepath.Join(userDir, "custom.json"), PromptTemplate{
		Name:               "custom-overlay",
		Title:              "Custom",
		Version:            "0.1.0",
		Description:        "user custom template",
		Domain:             "patent",
		Category:           "analysis",
		SystemPrompt:       "custom system",
		UserPromptTemplate: "custom user {{var}}",
	})

	store, err := NewPromptStore(userDir)
	if err != nil {
		t.Fatalf("NewPromptStore: %v", err)
	}

	tmpl, ok := store.FindByName("custom-overlay")
	if !ok {
		t.Fatal("user template not found")
	}
	if tmpl.SystemPrompt != "custom system" {
		t.Fatalf("unexpected system prompt: %q", tmpl.SystemPrompt)
	}

	resolved, ok := store.Resolve("custom-overlay", map[string]string{"var": "x"})
	if !ok {
		t.Fatal("Resolve failed")
	}
	if resolved.UserPrompt != "custom user x" {
		t.Fatalf("unexpected user prompt: %q", resolved.UserPrompt)
	}
}

func TestPromptStore_UserOverrideEmbedded(t *testing.T) {
	userDir := t.TempDir()
	mustWritePromptTemplate(t, filepath.Join(userDir, "claim-drafting.json"), PromptTemplate{
		Name:               "claim-drafting",
		Title:              "Overridden",
		Version:            "9.9.9",
		Description:        "overridden template",
		Domain:             "patent",
		Category:           "drafting",
		SystemPrompt:       "overridden system",
		UserPromptTemplate: "overridden user",
	})

	store, err := NewPromptStore(userDir)
	if err != nil {
		t.Fatalf("NewPromptStore: %v", err)
	}

	tmpl, ok := store.FindByName("claim-drafting")
	if !ok {
		t.Fatal("template not found")
	}
	if tmpl.SystemPrompt != "overridden system" {
		t.Fatalf("expected user override to win, got %q", tmpl.SystemPrompt)
	}
}

func TestPromptStore_ConcurrentReads(t *testing.T) {
	store, err := NewPromptStore()
	if err != nil {
		t.Fatalf("NewPromptStore: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = store.FindByName("claim-drafting")
			_ = store.Count()
			_ = store.List(ListOptions{})
		}()
	}
	wg.Wait()
}

func TestPromptStore_FindByTrigger(t *testing.T) {
	store, err := NewPromptStore()
	if err != nil {
		t.Fatalf("NewPromptStore: %v", err)
	}

	matched := store.FindByTrigger("权利要求")
	if len(matched) == 0 {
		t.Fatal("expected trigger match")
	}
	found := false
	for _, m := range matched {
		if m.Name == "claim-drafting" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected claim-drafting in matches, got %v", matched)
	}
}

func TestPromptStore_List(t *testing.T) {
	store, err := NewPromptStore()
	if err != nil {
		t.Fatalf("NewPromptStore: %v", err)
	}

	drafting := store.List(ListOptions{Category: "drafting"})
	if len(drafting) == 0 {
		t.Fatal("expected drafting templates")
	}
	for _, tmpl := range drafting {
		if tmpl.Category != "drafting" {
			t.Fatalf("expected category drafting, got %q", tmpl.Category)
		}
	}

	searched := store.List(ListOptions{Query: "权利要求"})
	if len(searched) == 0 {
		t.Fatal("expected query match")
	}
}

func TestPromptStore_Index(t *testing.T) {
	store, err := NewPromptStore()
	if err != nil {
		t.Fatalf("NewPromptStore: %v", err)
	}
	idx := store.Index()
	if !strings.Contains(idx, "claim-drafting") {
		t.Fatal("index missing claim-drafting")
	}
}

func TestLoadPromptsFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/a.json": &fstest.MapFile{Data: []byte(`{
			"name": "a",
			"system_prompt": "sys a",
			"user_prompt_template": "user a"
		}`)},
		"templates/b.json": &fstest.MapFile{Data: []byte(`{
			"name": "b",
			"system_prompt": "sys b",
			"user_prompt_template": "user b"
		}`)},
		"templates/bad.txt": &fstest.MapFile{Data: []byte("not json")},
	}

	templates, err := LoadPromptsFromFS(fsys, "templates")
	if err != nil {
		t.Fatalf("LoadPromptsFromFS: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(templates))
	}
}

func TestLoadPromptsFromFS_InvalidTemplate(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/bad.json": &fstest.MapFile{Data: []byte(`{"title":"bad"}`)},
	}

	_, err := LoadPromptsFromFS(fsys, "templates")
	if err == nil {
		t.Fatal("expected error for missing system_prompt")
	}
}

func TestEmbedFS_Paths(t *testing.T) {
	// Ensure the embedded filesystem actually contains the expected directory tree.
	dirs, err := fs.ReadDir(embeddedPromptsFS, embeddedPromptsDir)
	if err != nil {
		t.Fatalf("ReadDir embedded: %v", err)
	}
	if len(dirs) == 0 {
		t.Fatal("embedded templates directory is empty")
	}

	err = fs.WalkDir(embeddedPromptsFS, embeddedPromptsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) != ".json" {
			t.Errorf("unexpected non-JSON file in embedded FS: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir embedded: %v", err)
	}
}
