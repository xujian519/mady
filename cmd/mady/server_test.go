package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenEvalStore_CreatesParentDir(t *testing.T) {
	evalDB := filepath.Join(t.TempDir(), "nested", "metrics", "eval.db")

	store, err := openEvalStore(evalDB)
	if err != nil {
		t.Fatalf("openEvalStore(%q): %v", evalDB, err)
	}
	defer store.Close()

	if _, err := os.Stat(evalDB); err != nil {
		t.Fatalf("expected eval db file to exist: %v", err)
	}
}
