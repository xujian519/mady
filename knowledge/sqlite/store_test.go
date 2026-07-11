package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

func testDBPath(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(home, ".mady", "knowledge", "knowledge.db")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("knowledge.db not found at %s", p)
	}
	return p
}

func TestFTSSearch(t *testing.T) {
	store, err := NewSQLiteStore(testDBPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	results, err := store.FTSSearch("新颖性", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected FTS results, got 0")
	}
	t.Logf("FTS search returned %d results", len(results))
	for i, r := range results {
		t.Logf("  [%d] score=%.4f doc=%s content=%.80s", i, r.Score, r.DocID, r.Content)
	}
}

func TestLoadGraph(t *testing.T) {
	store, err := NewSQLiteStore(testDBPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	gs, err := store.LoadGraph()
	if err != nil {
		t.Fatal(err)
	}
	if gs.NodeCount() == 0 {
		t.Fatal("expected graph nodes, got 0")
	}
	t.Logf("Graph loaded: %d nodes, %d edges", gs.NodeCount(), gs.EdgeCount())

	stats := gs.Stats()
	t.Logf("Graph stats: %+v", stats)
}

func TestSearchLaws(t *testing.T) {
	store, err := NewSQLiteStore(testDBPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	lawsPath := filepath.Join(filepath.Dir(testDBPath(t)), "laws-full.db")
	if err := store.OpenLawsDB(lawsPath); err != nil {
		t.Fatal(err)
	}

	results, err := store.SearchLaws("专利法", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected law results, got 0")
	}
	t.Logf("Law search returned %d results", len(results))
	for i, r := range results {
		t.Logf("  [%d] %s (%s) - %s", i, r.Name, r.Level, r.Category)
	}
}
