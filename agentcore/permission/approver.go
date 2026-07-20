package permission

import (
	"context"
	"encoding/json"
	"sync"
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

// ApprovalRequest carries the details of a pending tool approval request
// from the PermissionExtension middleware to the TUI.
type ApprovalRequest struct {
	ToolName string
	Args     json.RawMessage
	response chan Decision
}

// TUIChannelApprover implements Approver for interactive TUI environments.
// It blocks the calling goroutine (the agent run loop) until the TUI main
// loop calls Respond() with Allow or Deny.
//
// Usage:
//
//	approver := permission.NewTUIChannelApprover()
//	// Agent goroutine blocks in Approve() ...
//	// TUI goroutine polls or is notified: use PollPending() to detect
//	// the pending request, then show a prompt and call Respond().
type TUIChannelApprover struct {
	mu      sync.Mutex
	cond    *sync.Cond
	pending *ApprovalRequest
}

// NewTUIChannelApprover creates a TUIChannelApprover ready for use.
func NewTUIChannelApprover() *TUIChannelApprover {
	a := &TUIChannelApprover{}
	a.cond = sync.NewCond(&a.mu)
	return a
}

// Approve blocks until the TUI user responds via Respond(), or until
// ctx is canceled (returns DecisionDeny).
func (a *TUIChannelApprover) Approve(ctx context.Context, toolName string, args json.RawMessage) Decision {
	req := &ApprovalRequest{
		ToolName: toolName,
		Args:     args,
		response: make(chan Decision, 1),
	}

	a.mu.Lock()
	a.pending = req
	a.cond.Broadcast() // wake any waiter in WaitPending
	a.mu.Unlock()

	// Wait for the user's response or context cancellation.
	select {
	case d := <-req.response:
		// clean up pending reference
		a.mu.Lock()
		a.pending = nil
		a.mu.Unlock()
		return d

	case <-ctx.Done():
		// context canceled → deny the call
		a.mu.Lock()
		a.pending = nil
		a.mu.Unlock()
		return DecisionDeny
	}
}

// PollPending returns the current pending ApprovalRequest, or nil if no
// approval is outstanding. Non-blocking — designed for TUI main-loop polling.
func (a *TUIChannelApprover) PollPending() *ApprovalRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pending
}

// Respond sends the user's decision and unblocks Approve().
// It is safe to call from a different goroutine than Approve().
func (a *TUIChannelApprover) Respond(decision Decision) {
	a.mu.Lock()
	req := a.pending
	a.mu.Unlock()

	if req == nil {
		return // no pending request to respond to
	}

	// Non-blocking send: the response channel is buffered (cap 1),
	// and Approve() is the sole reader.
	select {
	case req.response <- decision:
	default:
	}
}
