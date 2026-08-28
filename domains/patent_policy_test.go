package domains

import (
	"encoding/json"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/permission"
)

// =============================================================================
// 装配层锁定测试：防止组装回归静默移除保护层（brooks-review W3）
// =============================================================================

// TestProjectAgentPolicy_MonotonicEvidenceDeny 锁定：项目 Agent 策略必须
// 携带证据形式要件单调拒绝——删掉接线时本测试失败。
func TestProjectAgentPolicy_MonotonicEvidenceDeny(t *testing.T) {
	policy := projectAgentPolicy()

	// 域外证据未声明公证认证 → 硬拒绝（即使工具只读、Mode=Allow）。
	args := json.RawMessage(`{"source_uri":"https://example.com","evidence_type_hint":"overseas"}`)
	if got := policy.Decide("judge_type_specific", true, args); got != permission.DecisionDeny {
		t.Errorf("overseas without notarization must be denied, got %s", got)
	}

	// 声明公证认证后 → 不再被单调层拦截（回退到基础策略判定）。
	args = json.RawMessage(`{"source_uri":"https://example.com","evidence_type_hint":"overseas","notarization_status":"completed"}`)
	if got := policy.Decide("judge_type_specific", true, args); got == permission.DecisionDeny {
		t.Errorf("notarized overseas evidence must not be denied, got %s", got)
	}
}

// TestProjectAgentPolicy_BaselineUnchanged 锁定基础策略行为未被单调层破坏。
func TestProjectAgentPolicy_BaselineUnchanged(t *testing.T) {
	policy := projectAgentPolicy()
	if got := policy.Decide("read_file", true, json.RawMessage(`{"path":"x"}`)); got != permission.DecisionAllow {
		t.Errorf("read-only baseline should allow, got %s", got)
	}
	if got := policy.Decide(permission.ToolBash, false, json.RawMessage(`{"command":"ls"}`)); got != permission.DecisionAsk {
		t.Errorf("bash baseline should ask, got %s", got)
	}
}

// TestAppendResultOffload_WiresBudgetAndReadTool 锁定：结果落盘段落必须
// 同时注册预算钩子与回读工具——删掉任一接线时本测试失败。
func TestAppendResultOffload_WiresBudgetAndReadTool(t *testing.T) {
	t.Setenv("MADY_HOME", t.TempDir())
	cfg := agentcore.Config{}
	appendResultOffload(&cfg)

	if cfg.Lifecycle == nil {
		t.Fatal("lifecycle hook must be registered")
	}
	if _, ok := cfg.Lifecycle.(*agentcore.ToolResultBudget); !ok {
		t.Errorf("lifecycle must contain ToolResultBudget, got %T", cfg.Lifecycle)
	}
	found := false
	for _, tool := range cfg.Tools {
		if tool != nil && tool.Name == "offload_read" {
			found = true
		}
	}
	if !found {
		t.Error("offload_read tool must be registered alongside the budget hook")
	}
}
