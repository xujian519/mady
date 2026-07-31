package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/knowledge/fileindex"
)

// TestStableUserID_EnvOverride 验证 $MADY_USER_ID 优先级最高。
func TestStableUserID_EnvOverride(t *testing.T) {
	t.Setenv("MADY_USER_ID", "explicit-user")
	if got := stableUserID(); got != "explicit-user" {
		t.Errorf("stableUserID() = %q, want explicit-user", got)
	}
}

// TestStableUserID_DefaultNonEmpty 验证无环境变量时回退值非空且稳定。
// 关键不变量：不能等于某个会话 threadID（即必须是跨会话稳定的身份）。
func TestStableUserID_DefaultNonEmpty(t *testing.T) {
	t.Setenv("MADY_USER_ID", "")
	first := stableUserID()
	if first == "" {
		t.Fatal("stableUserID() returned empty")
	}
	// 同一进程内多次调用必须一致（稳定性）
	second := stableUserID()
	if first != second {
		t.Errorf("stableUserID() not stable: %q vs %q", first, second)
	}
}

// TestBuildAgentConfig_PreservesMaxTokens 验证 buildAgentConfig 在组装统一 Agent
// 配置时不会把 BaseConfig 中已解析好的 MaxTokens 清零或覆盖。
//
// 这是受保护的不变量：MaxTokens 由 bootstrap.NewBaseConfig / 用户配置在启动阶段决定，
// TUI 会话层只能保留，不能二次赋值。
func TestBuildAgentConfig_PreservesMaxTokens(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	store, err := NewSettingsStore(filepath.Join(tmp, "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	const sentinelMaxTokens int64 = 12345
	fc := &frameworkContext{
		WorkspaceDir: tmp,
		BaseConfig: agentcore.Config{
			ModelConfig: agentcore.ModelConfig{
				Name:      "mady-router",
				Model:     "deepseek-v4-flash",
				Streaming: true,
				MaxTokens: sentinelMaxTokens,
			},
		},
	}

	s := &tuiSession{
		ctx:             context.Background(),
		fc:              fc,
		provider:        nil,
		model:           "deepseek-v4-flash",
		providerName:    "deepseek",
		fileIndexExt:    fileindex.NewExtension(fileindex.ExtensionConfig{FallbackDir: tmp}),
		currentThreadID: "test-thread-maxtokens",
		store:           store,
	}

	cfg := s.buildAgentConfig()

	if cfg.MaxTokens != sentinelMaxTokens {
		t.Errorf("buildAgentConfig() MaxTokens = %d, want %d; BaseConfig.MaxTokens 在装配过程中被覆盖", cfg.MaxTokens, sentinelMaxTokens)
	}
}
