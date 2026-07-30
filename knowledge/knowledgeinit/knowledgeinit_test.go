package knowledgeinit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/knowledge"
)

// ---------------------------------------------------------------------------
// InitPatentKnowledge tests
// ---------------------------------------------------------------------------

func TestInitPatentKnowledge_NoDirectory(t *testing.T) {
	store := knowledge.NewStore()
	if store == nil {
		t.Fatal("NewStore returned nil")
	}

	// No knowledge directory exists — should silently skip all files.
	err := InitPatentKnowledge(store)
	if err != nil {
		t.Fatalf("InitPatentKnowledge should not error when dir missing: %v", err)
	}
}

func TestInitPatentKnowledge_EmptyDirectory(t *testing.T) {
	store := knowledge.NewStore()

	// Override the data dir resolution by setting MADY_HOME to an empty temp dir.
	tmpHome := t.TempDir()
	t.Setenv("MADY_HOME", tmpHome)

	// Create knowledge/ directory but no files (ResolveDataDir resolves to $MADY_HOME/knowledge).
	kDir := filepath.Join(tmpHome, "knowledge")
	if err := os.MkdirAll(kDir, 0750); err != nil {
		t.Fatal(err)
	}

	err := InitPatentKnowledge(store)
	if err != nil {
		t.Fatalf("InitPatentKnowledge should not error with empty dir: %v", err)
	}
}

func TestInitPatentKnowledge_LoadFile(t *testing.T) {
	store := knowledge.NewStore()

	tmpHome := t.TempDir()
	t.Setenv("MADY_HOME", tmpHome)

	kDir := filepath.Join(tmpHome, "knowledge")
	if err := os.MkdirAll(kDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kDir, "patent-law-full.md"), []byte("# Patent Law Test"), 0600); err != nil {
		t.Fatal(err)
	}

	err := InitPatentKnowledge(store)
	if err != nil {
		t.Fatalf("InitPatentKnowledge should not error: %v", err)
	}

	// Verify document was loaded
	if n := store.SearchableDocCount(); n == 0 {
		t.Fatal("expected at least 1 searchable doc after loading")
	}
}

func TestInitPatentKnowledge_MultipleFiles(t *testing.T) {
	store := knowledge.NewStore()

	tmpHome := t.TempDir()
	t.Setenv("MADY_HOME", tmpHome)

	kDir := filepath.Join(tmpHome, "knowledge")
	if err := os.MkdirAll(kDir, 0750); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"patent-law-full.md": "# Patent Law\nContent A",
		"guidelines.md":      "# Guidelines\nContent B",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(kDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	err := InitPatentKnowledge(store)
	if err != nil {
		t.Fatalf("InitPatentKnowledge: %v", err)
	}

	if n := store.SearchableDocCount(); n < 2 {
		t.Errorf("expected >=2 searchable docs, got %d", n)
	}
}

func TestInitPatentKnowledge_OnlySomeFilesExist(t *testing.T) {
	store := knowledge.NewStore()

	tmpHome := t.TempDir()
	t.Setenv("MADY_HOME", tmpHome)

	kDir := filepath.Join(tmpHome, "knowledge")
	if err := os.MkdirAll(kDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kDir, "ipc-classes.md"), []byte("# IPC Classes"), 0600); err != nil {
		t.Fatal(err)
	}

	err := InitPatentKnowledge(store)
	if err != nil {
		t.Fatalf("InitPatentKnowledge should not error with partial files: %v", err)
	}
}
