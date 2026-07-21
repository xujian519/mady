package i18n

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNew_DefaultLocale(t *testing.T) {
	c := New(LocaleZhCN)
	if c.Locale() != LocaleZhCN {
		t.Errorf("expected zh-CN, got %s", c.Locale())
	}
}

func TestSetLocale(t *testing.T) {
	c := New(LocaleZhCN)
	c.SetLocale(LocaleEnUS)
	if c.Locale() != LocaleEnUS {
		t.Errorf("expected en-US, got %s", c.Locale())
	}
}

func TestAddAndT(t *testing.T) {
	c := New(LocaleZhCN)
	c.Add("hello", LocaleZhCN, "你好")
	c.Add("hello", LocaleEnUS, "Hello")

	if got := c.T("hello"); got != "你好" {
		t.Errorf("expected '你好', got %q", got)
	}

	c.SetLocale(LocaleEnUS)
	if got := c.T("hello"); got != "Hello" {
		t.Errorf("expected 'Hello', got %q", got)
	}
}

func TestT_WithFormatArgs(t *testing.T) {
	c := New(LocaleZhCN)
	c.Add("greeting", LocaleZhCN, "你好，%s！")
	c.Add("count", LocaleZhCN, "共 %d 条记录")

	if got := c.T("greeting", "世界"); got != "你好，世界！" {
		t.Errorf("expected '你好，世界！', got %q", got)
	}
	if got := c.T("count", 42); got != "共 42 条记录" {
		t.Errorf("expected '共 42 条记录', got %q", got)
	}
}

func TestT_KeyNotFound(t *testing.T) {
	c := New(LocaleZhCN)
	if got := c.T("nonexistent.key"); got != "nonexistent.key" {
		t.Errorf("expected key itself, got %q", got)
	}
}

func TestT_FallbackToZhCN(t *testing.T) {
	c := New(LocaleEnUS)
	c.Add("test", LocaleZhCN, "中文文本")

	// en-US 没有翻译，应回退到 zh-CN
	if got := c.T("test"); got != "中文文本" {
		t.Errorf("expected fallback '中文文本', got %q", got)
	}
}

func TestLoadYAML_SimpleKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	content := []byte("key1: value1\nkey2: value2\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	c := New(LocaleZhCN)
	if err := c.LoadYAML(path); err != nil {
		t.Fatal(err)
	}

	if got := c.T("key1"); got != "value1" {
		t.Errorf("expected 'value1', got %q", got)
	}
	if got := c.T("key2"); got != "value2" {
		t.Errorf("expected 'value2', got %q", got)
	}
}

func TestLoadYAML_LocaleGrouped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	content := []byte("greeting:\n  zh-CN: \"你好\"\n  en-US: \"Hello\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	c := New(LocaleZhCN)
	if err := c.LoadYAML(path); err != nil {
		t.Fatal(err)
	}

	if got := c.T("greeting"); got != "你好" {
		t.Errorf("expected '你好', got %q", got)
	}

	c.SetLocale(LocaleEnUS)
	if got := c.T("greeting"); got != "Hello" {
		t.Errorf("expected 'Hello', got %q", got)
	}
}

func TestLoadYAML_NestedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	content := []byte("parent:\n  child:\n    zh-CN: \"子键\"\n    en-US: \"child key\"\n")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	c := New(LocaleZhCN)
	if err := c.LoadYAML(path); err != nil {
		t.Fatal(err)
	}

	if got := c.T("parent.child"); got != "子键" {
		t.Errorf("expected '子键', got %q", got)
	}

	c.SetLocale(LocaleEnUS)
	if got := c.T("parent.child"); got != "child key" {
		t.Errorf("expected 'child key', got %q", got)
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()

	// zh-CN locale 文件
	zhContent := []byte("greeting: \"你好\"")
	if err := os.WriteFile(filepath.Join(dir, "zh-CN.yaml"), zhContent, 0644); err != nil {
		t.Fatal(err)
	}

	c := New(LocaleZhCN)
	if err := c.LoadDir(dir); err != nil {
		t.Fatal(err)
	}

	if got := c.T("greeting"); got != "你好" {
		t.Errorf("expected '你好', got %q", got)
	}
}

func TestLoadYAML_FileNotFound(t *testing.T) {
	c := New(LocaleZhCN)
	if err := c.LoadYAML("/nonexistent/path.yaml"); err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestGlobal_Default(t *testing.T) {
	// Global() should return a non-nil catalog by default
	g := Global()
	if g == nil {
		t.Fatal("Global() returned nil")
	}
}

func TestSetGlobalAndT(t *testing.T) {
	saved := Global()
	defer SetGlobal(saved)

	c := New(LocaleZhCN)
	c.Add("test.key", LocaleZhCN, "全局翻译")
	SetGlobal(c)

	if got := T("test.key"); got != "全局翻译" {
		t.Errorf("expected '全局翻译', got %q", got)
	}
}

func TestConcurrentSafety(t *testing.T) {
	c := New(LocaleZhCN)
	var wg sync.WaitGroup

	// 并发写入
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.Add("key", LocaleZhCN, "value")
			c.SetLocale(LocaleZhCN)
			_ = c.T("key")
		}(i)
	}
	wg.Wait()
}

func TestParseLocale(t *testing.T) {
	tests := []struct {
		input string
		want  Locale
	}{
		{"zh-CN", LocaleZhCN},
		{"zh_CN", LocaleZhCN},
		{"zh", LocaleZhCN},
		{"en-US", LocaleEnUS},
		{"en_US", LocaleEnUS},
		{"en", LocaleEnUS},
		{"fr-FR", LocaleZhCN}, // 未知，回退默认
		{"", LocaleZhCN},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseLocale(tt.input); got != tt.want {
				t.Errorf("ParseLocale(%q) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestLocaleString(t *testing.T) {
	if LocaleZhCN.String() != "zh-CN" {
		t.Errorf("expected 'zh-CN', got %q", LocaleZhCN.String())
	}
	if LocaleEnUS.String() != "en-US" {
		t.Errorf("expected 'en-US', got %q", LocaleEnUS.String())
	}
}
