package planmode

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// 本文件中的 hook 测试传递 nil 作为 *agentcore.AgentRunContext 参数是
// 有意的：当前 planModeHook 的实现不依赖 ARC 中的任何字段（它只检查
// h.ext.active 和 h.ext.agent），传入 nil 验证了 nil-ARC 保护的可靠性。
// 如果未来 hook 需要从 ARC 读取数据，请为这些测试提供真实的 ARC。

// ---------------------------------------------------------------------------
// PlanModeExtension.LifecycleHook
// ---------------------------------------------------------------------------

func TestPlanModeExtension_LifecycleHook(t *testing.T) {
	ext := NewExtension(Policy{})
	hook := ext.LifecycleHook()
	if hook == nil {
		t.Fatal("expected non-nil LifecycleHook")
	}
}

func TestPlanModeExtension_LifecycleHook_ReturnsPlanModeHook(t *testing.T) {
	ext := NewExtension(Policy{})
	hook := ext.LifecycleHook()
	if hook == nil {
		t.Fatalf("expected non-nil LifecycleHook from planmode extension")
	}
}

// ---------------------------------------------------------------------------
// planModeHook.BeforeToolExecution — table-driven behavior tests
// ---------------------------------------------------------------------------

func TestPlanModeHook_BeforeToolExecution(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		toolID    string
		active    bool
		policy    Policy
		wantBlock bool
	}{
		{
			name:      "inactive does not block writer",
			toolName:  "edit",
			toolID:    "call_1",
			active:    false,
			wantBlock: false,
		},
		{
			name:      "inactive allows writer even after activate+deactivate",
			toolName:  "write_file",
			toolID:    "call_1",
			active:    false,
			wantBlock: false,
		},
		{
			name:      "active blocks writer",
			toolName:  "edit",
			toolID:    "call_1",
			active:    true,
			wantBlock: true,
		},
		{
			name:      "active allows always-allowed tool",
			toolName:  "ask",
			toolID:    "call_1",
			active:    true,
			wantBlock: false,
		},
		{
			name:      "active allows whitelisted tool",
			toolName:  "custom_tool",
			toolID:    "call_1",
			active:    true,
			policy:    Policy{AllowedTools: []string{"custom_tool"}},
			wantBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := NewExtension(tt.policy)
			if tt.active {
				ext.Activate()
			} else if tt.name == "inactive allows writer even after activate+deactivate" {
				// Verify the activate→deactivate cycle leaves the hook inactive.
				ext.Activate()
				ext.Deactivate()
			}
			hook := ext.LifecycleHook()

			results := make([]agentcore.ToolResult, 1)
			tec := &agentcore.ToolExecutionContext{
				ToolCalls: []agentcore.ToolCall{
					{ID: tt.toolID, Name: tt.toolName, Arguments: `{"key":"val"}`},
				},
				Results: results,
			}

			hook.BeforeToolExecution(context.Background(), nil, tec)

			if tt.wantBlock && tec.Results[0].ToolCallID != tt.toolID {
				t.Errorf("expected blocked tool to have ToolCallID=%q, got %q",
					tt.toolID, tec.Results[0].ToolCallID)
			}
			if !tt.wantBlock && tec.Results[0].ToolCallID != "" {
				t.Errorf("expected unblocked tool to have empty ToolCallID, got %q",
					tec.Results[0].ToolCallID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// planModeHook.BeforeToolExecution — nil / edge cases
// ---------------------------------------------------------------------------

func TestPlanModeHook_BeforeToolExecution_NilOrEmptyCalls(t *testing.T) {
	ext := NewExtension(Policy{})
	ext.Activate()
	hook := ext.LifecycleHook()

	t.Run("nil ToolCalls and nil Results", func(t *testing.T) {
		tec := &agentcore.ToolExecutionContext{
			ToolCalls: nil,
			Results:   nil,
		}
		// Must not panic.
		hook.BeforeToolExecution(context.Background(), nil, tec)
	})

	t.Run("nil Results with non-nil ToolCalls", func(t *testing.T) {
		tec := &agentcore.ToolExecutionContext{
			ToolCalls: []agentcore.ToolCall{
				{Name: "write_file", Arguments: `{"path":"out.go"}`},
			},
			Results: nil,
		}
		// Must not panic when Results is nil/short.
		hook.BeforeToolExecution(context.Background(), nil, tec)
	})
}

func TestPlanModeHook_BeforeToolExecution_NilAgent(t *testing.T) {
	ext := &PlanModeExtension{policy: Policy{}, agent: nil}
	ext.Activate()
	hook := ext.LifecycleHook()

	results := make([]agentcore.ToolResult, 1)
	tec := &agentcore.ToolExecutionContext{
		ToolCalls: []agentcore.ToolCall{
			{ID: "call_1", Name: "write_file", Arguments: `{"path":"out.go"}`},
		},
		Results: results,
	}

	// Must not panic when agent is nil (ToolReadOnly lookup is skipped).
	hook.BeforeToolExecution(context.Background(), nil, tec)
	if tec.Results[0].ToolCallID != "call_1" {
		t.Fatal("expected writer to be blocked even with nil agent")
	}
}

// ---------------------------------------------------------------------------
// PlanModeExtension state machine
// ---------------------------------------------------------------------------

func TestPlanModeExtension_ActivateDeactivate(t *testing.T) {
	ext := NewExtension(Policy{})
	if ext.IsActive() {
		t.Fatal("new extension should be inactive")
	}

	ext.Activate()
	if !ext.IsActive() {
		t.Fatal("expected active after Activate")
	}

	ext.Deactivate()
	if ext.IsActive() {
		t.Fatal("expected inactive after Deactivate")
	}
}
