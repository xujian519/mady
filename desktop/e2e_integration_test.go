//go:build darwin && e2e

package main

// e2e_integration_test.go — 桌面端冒烟测试（smoke，非完整端到端）。
//
// 本文件通过 build tag "e2e" 隔离，避免在常规 `go test` 中执行。
// 运行方式：go test -tags e2e -count=1 ./desktop/
//
// 覆盖（纯 Go 环境可完整执行的部分）：
//   1. App 构造 + ready() 状态检查
//   2. Health / ListThreads（无 Wails runtime 也可执行）
//   3. 文件操作（CreateFolder / ListDirectory / RenameFolder，t.TempDir 隔离）
//   4. SendAction / Cancel（尽力而为，验证不 panic）
//   5. shutdown（不 panic）
//
// 说明（M-4）：Chat 需要 Wails runtime（runtime.EventsEmit 依赖 OnStartup 注入的
// context），纯 Go test 环境无法真实跑通，测试中显式 Skip 并注明需实机验证
// （make desktop-run 手动走查）；不再使用恒 false 的 isWailsContext 伪检测。

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/xujian519/mady/a2ui"
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/bootstrap"
	madyserver "github.com/xujian519/mady/server"
)

// TestDesktopAppLifecycle 测试桌面端 App 的生命周期冒烟：
// setup → startup → Health → ListThreads → SendAction → Cancel → 文件操作 → shutdown
func TestDesktopAppLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 设置测试环境变量
	t.Setenv("MADY_HOME", t.TempDir())

	// 1. 初始化 framework
	fc, err := bootstrap.Setup(ctx, bootstrap.Options{
		Mode:    bootstrap.ModeSync,
		CmdName: "desktop-test",
	})
	if err != nil {
		// 无 Provider 配置时跳过完整生命周期测试
		t.Skipf("bootstrap.Setup failed (SKIP - no provider config): %v", err)
	}

	// 2. 构造 App 并模拟 startup
	app := NewApp()
	app.fc = fc
	app.server = madyserver.New(buildDesktopAgentConfig(fc))
	app.ctx.Store(context.Background())

	// 3. 验证 App 就绪
	if err := app.ready(); err != nil {
		t.Fatalf("app.ready() failed after startup: %v", err)
	}

	// 4. 测试 Health
	health, err := app.Health()
	if err != nil {
		t.Fatalf("Health() failed: %v", err)
	}
	if health.Model == "" {
		t.Error("Health().Model should not be empty")
	}
	t.Logf("Health: provider=%s model=%s version=%s", health.Provider, health.Model, health.Version)

	// 5. 测试 ListThreads（可能因无 thread store 返回错误，非阻塞）
	t.Run("ListThreads", func(t *testing.T) {
		_, err := app.ListThreads()
		if err != nil {
			t.Logf("ListThreads() returned (expected without store): %v", err)
		}
	})

	// 6. Chat：需要 Wails runtime（runtime.EventsEmit 依赖 OnStartup 注入的 context），
	// 纯 Go test -tags e2e 环境下无法真实跑通，显式跳过；实机走查见 make desktop-run。
	t.Run("Chat", func(t *testing.T) {
		t.Skip("SKIP: requires Wails runtime (EventsEmit needs OnStartup context); verify manually via make desktop-run")
	})

	// 7. 测试 SendAction（尽力而为，验证不 panic）
	t.Run("SendAction", func(t *testing.T) {
		err := app.SendAction("surface_test", &a2ui.ClientAction{
			Name:              "test_action",
			SurfaceID:         "surface_test",
			SourceComponentID: "test_component",
			Timestamp:         time.Now().UTC().Format(time.RFC3339),
			Context:           map[string]any{"key": "value"},
		})
		if err != nil {
			t.Logf("SendAction() returned error (expected with mock): %v", err)
		}
	})

	// 8. 测试 Cancel（验证不 panic）
	t.Run("Cancel", func(t *testing.T) {
		err := app.Cancel("non-existent-run")
		if err != nil {
			t.Logf("Cancel(non-existent) returned error (expected): %v", err)
		}
	})

	// 9. 测试文件操作（使用独立临时目录避免污染工作区）
	t.Run("FileOperations", func(t *testing.T) {
		testDir := t.TempDir()
		// 临时覆盖 app 的 ProjectDir
		origCWD := fc.BaseConfig.ProjectDir
		fc.BaseConfig.ProjectDir = testDir
		defer func() { fc.BaseConfig.ProjectDir = origCWD }()

		// CreateFolder
		path, err := app.CreateFolder("", "test-folder")
		if err != nil {
			t.Fatalf("CreateFolder failed: %v", err)
		}
		t.Logf("Created folder: %s", path)

		// ListDirectory
		entries, err := app.ListDirectory("")
		if err != nil {
			t.Fatalf("ListDirectory failed: %v", err)
		}
		found := false
		for _, e := range entries {
			if e.Name == "test-folder" && e.IsDir {
				found = true
				break
			}
		}
		if !found {
			t.Error("ListDirectory should contain test-folder")
		}

		// RenameFolder
		err = app.RenameFolder("test-folder", "renamed-folder")
		if err != nil {
			t.Fatalf("RenameFolder failed: %v", err)
		}
		t.Log("Renamed folder successfully")

		// 验证重命名后目录存在
		entries2, err := app.ListDirectory("")
		if err != nil {
			t.Fatalf("ListDirectory after rename failed: %v", err)
		}
		renamed := false
		for _, e := range entries2 {
			if e.Name == "renamed-folder" && e.IsDir {
				renamed = true
				break
			}
		}
		if !renamed {
			t.Error("ListDirectory should contain renamed-folder")
		}
	})

	// 10. 测试 shutdown（不 panic）
	t.Run("Shutdown", func(t *testing.T) {
		app.shutdown(context.Background())
		t.Log("shutdown completed successfully")
	})

	log.Println("[test] desktop smoke test passed")
}

// TestAppReady 测试 App ready() 状态检查。
func TestAppReady(t *testing.T) {
	app := NewApp()

	// 未初始化的 app 应返回错误
	if err := app.ready(); err == nil {
		t.Error("ready() should return error when server is nil")
	}

	// 用 agentcore.Config 初始化
	app.server = madyserver.New(agentcore.Config{})
	if err := app.ready(); err != nil {
		t.Error("ready() should return nil after server is set")
	}

	t.Log("App ready() state checks passed")
}

// TestMapAguiEvent 测试 AGUI 事件名映射。
func TestMapAguiEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    string
		expected string
	}{
		{"SCREAMING_SNAKE", "RUN_STARTED", "run-started"},
		{"handoff_start", "handoff_start", "handoff-start"},
		{"empty", "", ""},
		{"simple", "ok", "ok"},
		{"camelCase", "myEvent", "my-event"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toKebabCase(tt.event)
			if got != tt.expected {
				t.Errorf("toKebabCase(%q) = %q, want %q", tt.event, got, tt.expected)
			}
		})
	}
}
