package permission

import (
	"context"
	"encoding/json"
)

// Approver handles interactive confirmation when the policy decision is Ask.
type Approver interface {
	// Approve is called when the policy returns DecisionAsk.
	// It returns the final decision (Allow or Deny).
	Approve(ctx context.Context, toolName string, args json.RawMessage) Decision
}

// NonInteractiveApprover auto-allows Ask decisions (autonomous mode).
// Use this when there is no TTY or user interaction is not available.
type NonInteractiveApprover struct{}

func (NonInteractiveApprover) Approve(_ context.Context, _ string, _ json.RawMessage) Decision {
	return DecisionAllow
}

// AlwaysDenyApprover denies Ask decisions. Useful for testing and strict modes.
type AlwaysDenyApprover struct{}

func (AlwaysDenyApprover) Approve(_ context.Context, _ string, _ json.RawMessage) Decision {
	return DecisionDeny
}

// FuncApprover wraps a function to implement Approver.
type FuncApprover func(ctx context.Context, toolName string, args json.RawMessage) Decision

func (f FuncApprover) Approve(ctx context.Context, toolName string, args json.RawMessage) Decision {
	return f(ctx, toolName, args)
}
