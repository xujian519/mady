package domains

import (
	"os"
	"path/filepath"
	"testing"
)

// writeStyle 写一个最小合法 DocumentStyle YAML 到 dir/name.yaml。
func writeStyle(t *testing.T, dir, fname, name, domain string) {
	t.Helper()
	style := "name: " + name + "\ndomain: " + domain + "\nversion: \"1\"\n"
	if err := os.WriteFile(filepath.Join(dir, fname), []byte(style), 0644); err != nil {
		t.Fatalf("write %s: %v", fname, err)
	}
}

// TestLoadStylesFromPaths_MergeDistinctDomains 验证不同域风格各自保留、合并加载。
func TestLoadStylesFromPaths_MergeDistinctDomains(t *testing.T) {
	dir := t.TempDir()
	writeStyle(t, dir, "patent.yaml", "patent-std", "patent")
	writeStyle(t, dir, "legal.yaml", "legal-std", "legal")

	styles, err := loadStylesFromPaths([]string{dir})
	if err != nil {
		t.Fatalf("loadStylesFromPaths: %v", err)
	}
	if len(styles) != 2 {
		t.Fatalf("expected 2 styles, got %d", len(styles))
	}
	if len(StylesForDomain(styles, "patent")) != 1 {
		t.Error("patent domain missing")
	}
	if len(StylesForDomain(styles, "legal")) != 1 {
		t.Error("legal domain missing")
	}
}

// TestLoadStylesFromPaths_UserOverridesBuiltin 验证用户同名风格覆盖内置：
// 内置目录在前、用户目录在后，同 name 取用户的。
func TestLoadStylesFromPaths_UserOverridesBuiltin(t *testing.T) {
	builtin := t.TempDir()
	user := t.TempDir()
	// 内置 patent-standard
	writeStyle(t, builtin, "patent-standard.yaml", "patent-standard", "patent")
	// 用户用同 name 覆盖（version 标记区分）
	if err := os.WriteFile(filepath.Join(user, "patent-standard.yaml"),
		[]byte("name: patent-standard\ndomain: patent\nversion: \"user-v2\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	styles, err := loadStylesFromPaths([]string{builtin, user}) // 内置在前、用户在后
	if err != nil {
		t.Fatalf("loadStylesFromPaths: %v", err)
	}
	if len(styles) != 1 {
		t.Fatalf("expected 1 style after override, got %d", len(styles))
	}
	if styles[0].Version != "user-v2" {
		t.Errorf("expected user-v2 override, got %q", styles[0].Version)
	}
}

// TestLoadStylesFromPaths_SkipMalformed 验证坏文件被跳过，不阻断有效风格。
func TestLoadStylesFromPaths_SkipMalformed(t *testing.T) {
	dir := t.TempDir()
	writeStyle(t, dir, "good.yaml", "good-style", "patent")
	// 写一个缺少 name 的非法文件
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"),
		[]byte("domain: patent\n"), 0644); err != nil {
		t.Fatal(err)
	}

	styles, err := loadStylesFromPaths([]string{dir})
	if err != nil {
		t.Fatalf("expected malformed file skipped without error, got: %v", err)
	}
	if len(styles) != 1 || styles[0].Name != "good-style" {
		t.Fatalf("expected only good-style, got %+v", styles)
	}
}

// TestLoadStylesFromPaths_NonexistentDirSkipped 验证不存在的目录被静默跳过。
func TestLoadStylesFromPaths_NonexistentDirSkipped(t *testing.T) {
	dir := t.TempDir()
	writeStyle(t, dir, "x.yaml", "x-style", "patent")
	missing := filepath.Join(dir, "no-such-dir")

	styles, err := loadStylesFromPaths([]string{missing, dir})
	if err != nil {
		t.Fatalf("loadStylesFromPaths: %v", err)
	}
	if len(styles) != 1 {
		t.Fatalf("expected 1 style, got %d", len(styles))
	}
}
