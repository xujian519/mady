package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSandboxConfig_NonExistentFile(t *testing.T) {
	cfg, err := LoadSandboxConfigFromPath("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("expected nil error for non-existent file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.AllowRead) != 0 {
		t.Fatalf("expected empty AllowRead, got: %v", cfg.AllowRead)
	}
}

func TestSaveAndLoadSandboxConfig(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")

	cfg := &SandboxConfig{
		AllowRead:  []string{"/path/to/knowledge"},
		AllowWrite: []string{"/tmp/mady"},
	}

	if err := SaveSandboxConfigToPath(path, cfg); err != nil {
		t.Fatalf("SaveSandboxConfigToPath: %v", err)
	}

	loaded, err := LoadSandboxConfigFromPath(path)
	if err != nil {
		t.Fatalf("LoadSandboxConfigFromPath: %v", err)
	}
	if len(loaded.AllowRead) != 1 || loaded.AllowRead[0] != "/path/to/knowledge" {
		t.Fatalf("expected AllowRead=[/path/to/knowledge], got: %v", loaded.AllowRead)
	}
	if len(loaded.AllowWrite) != 1 || loaded.AllowWrite[0] != "/tmp/mady" {
		t.Fatalf("expected AllowWrite=[/tmp/mady], got: %v", loaded.AllowWrite)
	}
}

func TestSaveSandboxConfig_PreservesOtherSections(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")

	// 先写入一个带其他段的配置
	original := "foo: bar\nbaz: 42\n"
	os.WriteFile(path, []byte(original), 0644)

	// 再写入 sandbox 段
	cfg := &SandboxConfig{AllowRead: []string{"/wiki"}}
	if err := SaveSandboxConfigToPath(path, cfg); err != nil {
		t.Fatalf("SaveSandboxConfigToPath: %v", err)
	}

	loaded, err := LoadSandboxConfigFromPath(path)
	if err != nil {
		t.Fatalf("LoadSandboxConfigFromPath: %v", err)
	}
	if len(loaded.AllowRead) != 1 || loaded.AllowRead[0] != "/wiki" {
		t.Fatalf("expected AllowRead=[/wiki], got: %v", loaded.AllowRead)
	}

	// 验证其他段保留（读取原始内容检查）
	data, _ := os.ReadFile(path)
	if !contains(string(data), "foo") {
		t.Fatalf("expected other config sections preserved, got:\n%s", data)
	}
}

func TestLoadKnowledgeDirsFromEnv(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	t.Setenv("KNOWLEDGE_DIRS", dir1+":"+dir2)
	dirs := LoadKnowledgeDirsFromEnv()
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d: %v", len(dirs), dirs)
	}
}

func TestLoadKnowledgeDirsFromEnv_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir not available")
	}
	t.Setenv("KNOWLEDGE_DIRS", "~/Documents")
	dirs := LoadKnowledgeDirsFromEnv()
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}
	expected := filepath.Join(home, "Documents")
	if dirs[0] != expected {
		t.Fatalf("expected %q, got %q", expected, dirs[0])
	}
}

func TestMergeAllowRead_Dedup(t *testing.T) {
	merged := MergeAllowRead(
		[]string{"/a", "/b"},
		[]string{"/b", "/c"},
		nil,
		[]string{"/d"},
	)
	if len(merged) != 4 {
		t.Fatalf("expected 4 unique dirs, got %d: %v", len(merged), merged)
	}
}

func TestAddKnowledgeDir(t *testing.T) {
	// 临时设置 MADY_HOME
	tmp := t.TempDir()
	t.Setenv("MADY_HOME", tmp)

	dir := t.TempDir()
	if err := AddKnowledgeDir(dir); err != nil {
		t.Fatalf("AddKnowledgeDir: %v", err)
	}

	cfg, err := LoadSandboxConfig()
	if err != nil {
		t.Fatalf("LoadSandboxConfig: %v", err)
	}
	found := false
	for _, d := range cfg.AllowRead {
		if d == dir {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %q in AllowRead, got: %v", dir, cfg.AllowRead)
	}
}

func TestAddKnowledgeDir_DuplicateNoop(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("MADY_HOME", tmp)

	dir := t.TempDir()
	if err := AddKnowledgeDir(dir); err != nil {
		t.Fatalf("first AddKnowledgeDir: %v", err)
	}
	if err := AddKnowledgeDir(dir); err != nil {
		t.Fatalf("second AddKnowledgeDir: %v", err)
	}

	cfg, _ := LoadSandboxConfig()
	count := 0
	for _, d := range cfg.AllowRead {
		if d == dir {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 entry for %q, got %d", dir, count)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
