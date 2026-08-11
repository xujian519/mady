package main

// tui_theme_command_test.go 覆盖 handleThemeCommand 的 legacy 主题别名。
//
// 背景：SettingsStore 默认主题为 "light"/"dark"（settings_store.go
// DefaultTheme），但主题注册表内没有这两个名字（只有 mady-light/mady-dark）。
// applyStoredTheme 有 legacy 兜底，而 handleThemeCommand 缺失，导致每次启动
// 执行 "/theme light"（tui.go 启动路径）都打印 "未知主题: light" 警告。
// 本测试锁定：light/dark 别名被正确应用、持久化且不产生未知主题警告。

import (
	"strings"
	"testing"
)

func TestHandleThemeCommandLegacyAlias(t *testing.T) {
	cases := []struct {
		name  string // 别名
		store string // 应持久化的值
	}{
		{"light", "light"},
		{"dark", "dark"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newTestSession()
			s.app = testAppForSession(t)

			s.handleThemeCommand("/theme " + c.name)

			// 1) 不产生"未知主题"警告。
			for _, m := range s.app.History().Messages() {
				if strings.Contains(m.Text, "未知主题") {
					t.Fatalf("unexpected unknown-theme warning: %q", m.Text)
				}
			}
			// 2) 输出"已切换主题"确认。
			if got := lastSystemMessage(s); !strings.Contains(got, "已切换主题") {
				t.Fatalf("expected switch confirmation, got %q", got)
			}
			// 3) store 持久化为原值（保持兼容，不改写为注册表名）。
			if got := s.store.Get(SettingKeyTheme); got != c.store {
				t.Fatalf("stored theme = %q, want %q", got, c.store)
			}
			if got := s.themeName(); got != c.store {
				t.Fatalf("themeName() = %q, want %q", got, c.store)
			}
		})
	}
}

// TestHandleThemeCommandUnknownStillWarns 回归：无效主题名仍应提示未知主题。
func TestHandleThemeCommandUnknownStillWarns(t *testing.T) {
	s := newTestSession()
	s.app = testAppForSession(t)

	s.handleThemeCommand("/theme no-such-theme")

	last := lastSystemMessage(s)
	if !strings.Contains(last, "未知主题") {
		t.Fatalf("expected unknown-theme warning for bad name, got %q", last)
	}
	// store 不应被污染为无效值。
	if got := s.store.Get(SettingKeyTheme); got == "no-such-theme" {
		t.Fatal("invalid theme name must not be persisted")
	}
}

// TestHandleThemeCommandDefaultThemeStartup 模拟 tui.go 启动路径：
// store 默认值为 "light" 时执行 "/theme light" 不应有警告。
func TestHandleThemeCommandDefaultThemeStartup(t *testing.T) {
	s := newTestSession()
	s.app = testAppForSession(t)

	// SettingsStore 默认 theme 即 "light"（defaultValues）。
	s.handleThemeCommand("/theme " + s.store.Get(SettingKeyTheme))

	for _, m := range s.app.History().Messages() {
		if strings.Contains(m.Text, "未知主题") {
			t.Fatalf("startup with default theme must not warn: %q", m.Text)
		}
	}
}
