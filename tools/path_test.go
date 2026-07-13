package tools

import (
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
