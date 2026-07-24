package main

import (
	"testing"
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
