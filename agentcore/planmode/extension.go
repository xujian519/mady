package planmode

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName is the registration name for the plan mode extension.
const ExtensionName = "planmode"

// PlanModeExtension gates tool execution when plan mode is active.
// When inactive, it adds zero overhead — the lifecycle hook returns immediately.
type PlanModeExtension struct {
	policy Policy
	active atomic.Bool
	agent  *agentcore.Agent
}

var (
	_ agentcore.Extension         = (*PlanModeExtension)(nil)
	_ agentcore.LifecycleProvider = (*PlanModeExtension)(nil)
)

// NewExtension creates a plan mode extension with the given policy.
func NewExtension(policy Policy) *PlanModeExtension {
	return &PlanModeExtension{policy: policy}
}

// Name implements agentcore.Extension.
func (e *PlanModeExtension) Name() string { return ExtensionName }

// Init implements agentcore.Extension.
func (e *PlanModeExtension) Init(_ context.Context, agent *agentcore.Agent) error {
	e.agent = agent
	return nil
}

// Dispose implements agentcore.Extension.
func (e *PlanModeExtension) Dispose() error { return nil }

// Activate enables plan mode tool gating.
func (e *PlanModeExtension) Activate() { e.active.Store(true) }

// Deactivate disables plan mode tool gating.
func (e *PlanModeExtension) Deactivate() { e.active.Store(false) }

// IsActive reports whether plan mode is currently active.
func (e *PlanModeExtension) IsActive() bool { return e.active.Load() }

// LifecycleHook implements agentcore.LifecycleProvider.
func (e *PlanModeExtension) LifecycleHook() agentcore.LifecycleHook {
	return &planModeHook{ext: e}
}

type planModeHook struct {
	agentcore.BaseLifecycleHook
	ext *PlanModeExtension
}

func (h *planModeHook) BeforeToolExecution(_ context.Context, _ *agentcore.AgentRunContext, tec *agentcore.ToolExecutionContext) error {
	if !h.ext.active.Load() {
		return nil
	}

	for i, tc := range tec.ToolCalls {
		readOnly := false
		if h.ext.agent != nil {
			if tool, ok := h.ext.agent.GetTool(tc.Name); ok {
				readOnly = agentcore.ToolReadOnly(tool, json.RawMessage(tc.Arguments))
			}
		}

		decision := h.ext.policy.Decide(tc.Name, readOnly, json.RawMessage(tc.Arguments))
		if decision.Blocked && i < len(tec.Results) {
			tec.Results[i] = agentcore.ToolResult{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Result:     decision.Message,
				Err:        nil,
				Silent:     true,
			}
		}
	}
	return nil
}
