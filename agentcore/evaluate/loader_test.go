package evaluate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoader_LoadFile(t *testing.T) {
	// Create a temporary fixture file.
	dir := t.TempDir()
	path := filepath.Join(dir, "test_fixtures.json")
	content := `{
		"suite": "test_suite",
		"cases": [
			{"id": "case1", "domain": "patent", "input": "input1", "expected": "output1"},
			{"id": "case2", "domain": "general", "input": "input2", "expected": "output2"}
		]
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loader := &Loader{}
	result, err := loader.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	cases, ok := result["test_suite"]
	if !ok {
		t.Fatal("expected suite key 'test_suite'")
	}
	if len(cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(cases))
	}
	if cases[0].ID != "case1" {
		t.Errorf("case[0].ID = %q, want %q", cases[0].ID, "case1")
	}
	if cases[1].Domain != "general" {
		t.Errorf("case[1].Domain = %q, want %q", cases[1].Domain, "general")
	}
}

func TestLoader_LoadFile_Array(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test_array.json")
	content := `[
		{"suite": "suite1", "cases": [{"id": "c1", "input": "i1", "expected": "o1"}]},
		{"suite": "suite2", "cases": [{"id": "c2", "input": "i2", "expected": "o2"}]}
	]`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loader := &Loader{}
	result, err := loader.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 suites, got %d", len(result))
	}
	if len(result["suite1"]) != 1 {
		t.Errorf("suite1: expected 1 case, got %d", len(result["suite1"]))
	}
}

func TestLoader_LoadFile_RawArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "raw.json")
	content := `[
		{"id": "c1", "domain": "patent", "input": "i1", "expected": "o1"},
		{"id": "c2", "domain": "legal", "input": "i2", "expected": "o2"}
	]`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loader := &Loader{}
	result, err := loader.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	cases, ok := result["default"]
	if !ok {
		t.Fatal("expected suite key 'default'")
	}
	if len(cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(cases))
	}
}

func TestLoader_LoadDir(t *testing.T) {
	loader := &Loader{}
	// Load the bundled testdata directory.
	// From the evaluate package root, testdata/ is a sibling directory.
	result, err := loader.LoadDir("testdata")
	if err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}

	if len(result) == 0 {
		t.Fatal("expected at least one suite from testdata")
	}

	cases, ok := result["tool_accuracy"]
	if !ok {
		t.Fatal("expected suite 'tool_accuracy' from testdata/tool_accuracy_fixtures.json")
	}
	if len(cases) == 0 {
		t.Fatal("expected at least one case in tool_accuracy suite")
	}
}

func TestLoader_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := &Loader{}
	_, err := loader.LoadFile(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoader_EmptyCases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	content := `{"suite": "empty", "cases": []}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loader := &Loader{}
	_, err := loader.LoadFile(path)
	if err == nil {
		t.Error("expected error for empty cases")
	}
}

func TestLoader_MustLoad_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from MustLoad with nonexistent path")
		}
	}()
	MustLoad(&Loader{}, "nonexistent_path_xyz.json")
}
