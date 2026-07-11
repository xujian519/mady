package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMadyHome_EnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("MADY_HOME", tmp)

	home, err := MadyHome()
	if err != nil {
		t.Fatalf("MadyHome: %v", err)
	}
	if home != tmp {
		t.Errorf("MADY_HOME override: got %q, want %q", home, tmp)
	}
	// 目录应已被创建
	if fi, err := os.Stat(home); err != nil || !fi.IsDir() {
		t.Errorf("home dir not created: %v", err)
	}
}

func TestMadyHome_RelativeEnvBecomesAbsolute(t *testing.T) {
	// 相对路径的 MADY_HOME 应被解析为绝对路径（相对当前 cwd）
	t.Setenv("MADY_HOME", "relative-mady-home")
	defer os.RemoveAll("relative-mady-home")

	home, err := MadyHome()
	if err != nil {
		t.Fatalf("MadyHome: %v", err)
	}
	if !filepath.IsAbs(home) {
		t.Errorf("expected absolute path, got %q", home)
	}
}

func TestMadyHome_DefaultFallbackToHome(t *testing.T) {
	// 未设 MADY_HOME 时，应回退到家目录下的 .mady。
	// 注意：os.UserHomeDir() 在 macOS 上不读 $HOME（用 getpwuid），
	// 所以这里只验证路径形态，不硬编码系统家目录。
	t.Setenv("MADY_HOME", "")

	home, err := MadyHome()
	if err != nil {
		t.Fatalf("MadyHome: %v", err)
	}
	// 应是绝对路径、以 .mady 结尾
	if !filepath.IsAbs(home) {
		t.Errorf("expected absolute path, got %q", home)
	}
	if filepath.Base(home) != ".mady" {
		t.Errorf("expected base name .mady, got %q", filepath.Base(home))
	}
	// 目录应已创建
	if fi, err := os.Stat(home); err != nil || !fi.IsDir() {
		t.Errorf("home dir not created: %v", err)
	}
}

func TestResolveDataDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("MADY_HOME", tmp)

	dir, err := ResolveDataDir("workspace")
	if err != nil {
		t.Fatalf("ResolveDataDir: %v", err)
	}
	want := filepath.Join(tmp, "workspace")
	if dir != want {
		t.Errorf("got %q, want %q", dir, want)
	}
	// 子目录应已创建
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("subdir not created: %v", err)
	}
}

func TestResolveDataDir_EmptySubdirReturnsHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("MADY_HOME", tmp)

	dir, err := ResolveDataDir("")
	if err != nil {
		t.Fatalf("ResolveDataDir: %v", err)
	}
	if dir != tmp {
		t.Errorf("empty subdir: got %q, want %q", dir, tmp)
	}
}

func TestEnsureDir(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "a", "b", "c")

	if err := EnsureDir(target); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		t.Errorf("nested dir not created: %v", err)
	}

	// 对已存在目录应为 no-op
	if err := EnsureDir(target); err != nil {
		t.Errorf("EnsureDir on existing: %v", err)
	}

	// 空字符串为 no-op
	if err := EnsureDir(""); err != nil {
		t.Errorf("EnsureDir empty: %v", err)
	}
}
