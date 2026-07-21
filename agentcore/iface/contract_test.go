package iface

import (
	"testing"
)

// TestInterfaceContract 验证接口的编译期契约。
// 这些测试通过类型断言确保结构体实现接口，不依赖具体实例。
func TestInterfaceContract(t *testing.T) {
	// 验证 BaseLifecycleHook 实现了 LifecycleHook 接口
	var _ LifecycleHook = BaseLifecycleHook{}
}

// TestAgentStateValues 验证 AgentState 的字段行为。
func TestAgentStateValues(t *testing.T) {
	state := AgentState{
		Status:    StatusRunning,
		TurnCount: 1,
	}
	if state.Status != StatusRunning {
		t.Errorf("AgentState.Status = %q, want %q", state.Status, StatusRunning)
	}
	if state.TurnCount != 1 {
		t.Errorf("AgentState.TurnCount = %d, want 1", state.TurnCount)
	}
}

// TestStatusConstants 验证状态常量定义。
func TestStatusConstants(t *testing.T) {
	tests := []struct {
		got  AgentStatus
		want AgentStatus
	}{
		{StatusIdle, "idle"},
		{StatusRunning, "running"},
		{StatusFinished, "finished"},
		{StatusError, "error"},
		{StatusInterrupted, "interrupted"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("status constant = %q, want %q", tt.got, tt.want)
		}
	}
}
