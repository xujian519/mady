package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreflightWritableSQLitePath_CreatesParentDir(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "workspace", "approvals.db")

	if err := preflightWritableSQLitePath(dbPath); err != nil {
		t.Fatalf("preflightWritableSQLitePath(%q): %v", dbPath, err)
	}

	if _, err := os.Stat(filepath.Dir(dbPath)); err != nil {
		t.Fatalf("expected parent dir to exist: %v", err)
	}
}

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
