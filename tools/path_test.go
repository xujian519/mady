package tools

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathSandboxed_WithinWorkingDir(t *testing.T) {
	tmp := t.TempDir()
	sbx := WorkingDirSandbox{Enabled: true, WorkingDir: tmp}
	subdir := filepath.Join(tmp, "sub")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)

	got, err := resolvePathSandboxed("file.txt", tmp, sbx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	expected, _ := filepath.EvalSymlinks(filepath.Join(tmp, "file.txt"))
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestResolvePathSandboxed_EscapeViaDotDot(t *testing.T) {
	tmp := t.TempDir()
	sbx := WorkingDirSandbox{Enabled: true, WorkingDir: tmp}

	_, err := resolvePathSandboxed("../etc/passwd", tmp, sbx)
	if err == nil {
		t.Fatal("expected error for path escaping sandbox, got nil")
	}
}

func TestResolvePathSandboxed_AbsoluteOutside(t *testing.T) {
	tmp := t.TempDir()
	sbx := WorkingDirSandbox{Enabled: true, WorkingDir: tmp}

	_, err := resolvePathSandboxed("/etc/passwd", tmp, sbx)
	if err == nil {
		t.Fatal("expected error for absolute path outside sandbox, got nil")
	}
}

func TestResolvePathSandboxed_SymlinkEscape(t *testing.T) {
	tmp := t.TempDir()
	sbx := WorkingDirSandbox{Enabled: true, WorkingDir: tmp}

	// Create a symlink inside the working dir pointing outside.
	outsideTarget := filepath.Join(t.TempDir(), "outside.txt")
	os.WriteFile(outsideTarget, []byte("escape"), 0644)
	linkPath := filepath.Join(tmp, "escape_link")
	if err := os.Symlink(outsideTarget, linkPath); err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	_, err := resolvePathSandboxed("escape_link", tmp, sbx)
	if err == nil {
		t.Fatal("expected error for symlink escaping sandbox, got nil")
	}
}

func TestResolvePathSandboxed_DisabledSandbox(t *testing.T) {
	tmp := t.TempDir()
	sbx := WorkingDirSandbox{Enabled: false, WorkingDir: tmp}

	got, err := resolvePathSandboxed("/etc/hosts", tmp, sbx)
	if err != nil {
		t.Fatalf("expected no error when sandbox disabled, got: %v", err)
	}
	if got == "" {
		t.Fatal("expected resolved path, got empty")
	}
}

func TestResolvePathSandboxed_EmptyPath(t *testing.T) {
	tmp := t.TempDir()
	sbx := WorkingDirSandbox{Enabled: true, WorkingDir: tmp}

	got, err := resolvePathSandboxed("", tmp, sbx)
	if err != nil {
		t.Fatalf("expected no error for empty path, got: %v", err)
	}
	expected, _ := filepath.EvalSymlinks(tmp)
	if got != expected {
		t.Fatalf("expected %q for empty path, got %q", expected, got)
	}
}

func TestOpenSandboxed_ReadFile(t *testing.T) {
	tmp := t.TempDir()
	sbx := WorkingDirSandbox{Enabled: true, WorkingDir: tmp}
	content := "sandbox test content"
	os.WriteFile(filepath.Join(tmp, "test.txt"), []byte(content), 0644)

	data, err := readFileSandboxed("test.txt", sbx)
	if err != nil {
		t.Fatalf("readFileSandboxed failed: %v", err)
	}
	if data != content {
		t.Fatalf("expected %q, got %q", content, data)
	}
}

func TestVerifyOpenedInode_Consistent(t *testing.T) {
	tmp := t.TempDir()
	sbx := WorkingDirSandbox{Enabled: true, WorkingDir: tmp}
	os.WriteFile(filepath.Join(tmp, "inode_test.txt"), []byte("data"), 0644)

	resolved, err := resolvePathSandboxed("inode_test.txt", tmp, sbx)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(resolved)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := verifyOpenedInode(f, resolved); err != nil {
		t.Fatalf("verifyOpenedInode failed: %v", err)
	}
}

func TestVerifyOpenedInode_Mismatch(t *testing.T) {
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "file1.txt")
	f2 := filepath.Join(tmp, "file2.txt")
	os.WriteFile(f1, []byte("one"), 0644)
	os.WriteFile(f2, []byte("two"), 0644)

	f, err := os.Open(f1)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := verifyOpenedInode(f, f2); err == nil {
		t.Fatal("expected error for mismatched inode, got nil")
	}
}

// --- AllowRead / AllowWrite 白名单测试 ---

func TestResolvePathSandboxed_AllowRead(t *testing.T) {
	tmp := t.TempDir()   // WorkingDir
	extra := t.TempDir() // 只读白名单目录
	os.WriteFile(filepath.Join(extra, "knowledge.md"), []byte("wiki"), 0644)

	sbx := WorkingDirSandbox{
		Enabled:    true,
		WorkingDir: tmp,
		AllowRead:  resolveAllowList([]string{extra}),
	}

	got, err := resolvePathSandboxedMode(filepath.Join(extra, "knowledge.md"), tmp, sbx, AccessRead)
	if err != nil {
		t.Fatalf("expected AllowRead to permit access, got: %v", err)
	}
	expected, _ := filepath.EvalSymlinks(filepath.Join(extra, "knowledge.md"))
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestResolvePathSandboxed_AllowRead_BlocksWrite(t *testing.T) {
	tmp := t.TempDir()
	extra := t.TempDir()

	sbx := WorkingDirSandbox{
		Enabled:    true,
		WorkingDir: tmp,
		AllowRead:  resolveAllowList([]string{extra}),
	}

	// Write mode should NOT be allowed in AllowRead-only directory
	_, err := resolvePathSandboxedMode(filepath.Join(extra, "file.txt"), tmp, sbx, AccessWrite)
	if err == nil {
		t.Fatal("expected error for write in AllowRead-only directory, got nil")
	}
	if !errors.Is(err, ErrOutsideSandbox) {
		t.Fatalf("expected ErrOutsideSandbox, got: %v", err)
	}
}

func TestResolvePathSandboxed_AllowWrite(t *testing.T) {
	tmp := t.TempDir()
	tmpWrite := t.TempDir()

	sbx := WorkingDirSandbox{
		Enabled:    true,
		WorkingDir: tmp,
		AllowWrite: resolveAllowList([]string{tmpWrite}),
	}

	// Write mode should be allowed in AllowWrite directory
	got, err := resolvePathSandboxedMode(filepath.Join(tmpWrite, "output.txt"), tmp, sbx, AccessWrite)
	if err != nil {
		t.Fatalf("expected AllowWrite to permit write, got: %v", err)
	}
	if got == "" {
		t.Fatal("expected resolved path, got empty")
	}
}

func TestResolvePathSandboxed_AllowRead_NotListed(t *testing.T) {
	tmp := t.TempDir()
	unrelated := t.TempDir()
	os.WriteFile(filepath.Join(unrelated, "secret.txt"), []byte("data"), 0644)

	sbx := WorkingDirSandbox{
		Enabled:    true,
		WorkingDir: tmp,
		// unrelated NOT in AllowRead
	}

	_, err := resolvePathSandboxedMode(filepath.Join(unrelated, "secret.txt"), tmp, sbx, AccessRead)
	if err == nil {
		t.Fatal("expected error for path not in any allowlist, got nil")
	}
	if !errors.Is(err, ErrOutsideSandbox) {
		t.Fatalf("expected ErrOutsideSandbox, got: %v", err)
	}
}

func TestIsWithin(t *testing.T) {
	tests := []struct {
		base, path string
		want       bool
	}{
		{"/a", "/a/b", true},
		{"/a", "/a", true},
		{"/a", "/a/b/c", true},
		{"/a", "/ab", false},
		{"/a", "/b", false},
	}
	for _, tt := range tests {
		got := isWithin(tt.base, tt.path)
		if got != tt.want {
			t.Errorf("isWithin(%q, %q) = %v, want %v", tt.base, tt.path, got, tt.want)
		}
	}
}
