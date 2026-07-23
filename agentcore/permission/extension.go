package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName is the registration name for the permission extension.
const ExtensionName = "permission"

// PermissionExtension gates tool execution with rule-based access control.
// It registers as a Middleware positioned early in the chain so that
// cheap deny decisions avoid downstream overhead (e.g. AI guardian review).
type PermissionExtension struct {
	// policy and agent are immutable after construction/Init.
	policy Policy
	agent  *agentcore.Agent

	// mu guards only approver (the sole mutable field).
	mu       sync.RWMutex
	approver Approver
}

var (
	_ agentcore.Extension          = (*PermissionExtension)(nil)
	_ agentcore.MiddlewareProvider = (*PermissionExtension)(nil)
)

// NewExtension creates a permission extension with the given policy and approver.
// If approver is nil, DecisionAsk falls back to Allow (autonomous mode).
func NewExtension(policy Policy, approver Approver) *PermissionExtension {
	if approver == nil {
		approver = NonInteractiveApprover{}
	}
	return &PermissionExtension{policy: policy, approver: approver}
}

// Name implements agentcore.Extension.
func (e *PermissionExtension) Name() string { return ExtensionName }

// Init implements agentcore.Extension.
func (e *PermissionExtension) Init(_ context.Context, agent *agentcore.Agent) error {
	e.agent = agent
	return nil
}

// Dispose implements agentcore.Extension.
func (e *PermissionExtension) Dispose() error { return nil }

// SetApprover replaces the current approver at runtime. Thread-safe.
func (e *PermissionExtension) SetApprover(a Approver) {
	e.mu.Lock()
	e.approver = a
	e.mu.Unlock()
}

// getApprover returns the current approver in a thread-safe manner.
func (e *PermissionExtension) getApprover() Approver {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.approver
}

// Middleware implements agentcore.MiddlewareProvider.
func (e *PermissionExtension) Middleware() []agentcore.Middleware {
	return []agentcore.Middleware{e.permissionMiddleware}
}

func (e *PermissionExtension) permissionMiddleware(next agentcore.ExecuteFunc) agentcore.ExecuteFunc {
	return func(ctx context.Context, tc agentcore.ToolCall) (string, error) {
		readOnly := false
		if e.agent != nil {
			if tool, ok := e.agent.GetTool(tc.Name); ok {
				readOnly = agentcore.ToolReadOnly(tool, json.RawMessage(tc.Arguments))
			}
		}

		decision := e.policy.Decide(tc.Name, readOnly, json.RawMessage(tc.Arguments))

		switch decision {
		case DecisionDeny:
			return fmt.Sprintf("blocked: 权限策略拒绝了 %s 的调用", tc.Name), nil
		case DecisionAsk:
			if approver := e.getApprover(); approver != nil {
				d := approver.Approve(ctx, tc.Name, json.RawMessage(tc.Arguments))
				if d == DecisionDeny {
					return fmt.Sprintf("blocked: 用户拒绝了 %s 的调用", tc.Name), nil
				}
			}
		}
		return next(ctx, tc)
	}
}
