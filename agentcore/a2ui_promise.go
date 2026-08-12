package agentcore

import "sync"

// A2UIAction represents a user action from the A2UI protocol (e.g. approval
// approve/reject, button click). It is intentionally decoupled from the a2ui
// package to keep agentcore dependency-free. Server.desktop.go converts
// *a2ui.ClientAction to *A2UIAction before delivering via A2UIPromise.
type A2UIAction struct {
	Name    string
	Context map[string]any
}

// A2UIPromise provides goroutine-safe one-shot delivery of an A2UI action
// from the SendAction caller (a UI goroutine) to the agent run loop.
//
// Usage:
//
//	promise := NewA2UIPromise()
//	agent.SetA2UIPromise(promise)
//	// ... on some UI event:
//	promise.Set(action)
//	// ... agent's runPreTurn calls promise.TryGet() — returns once.
//
// TryGet returns the action exactly once (the second call returns nil),
// preventing the same action from being processed in multiple turns.
type A2UIPromise struct {
	mu       sync.Mutex
	action   *A2UIAction
	consumed bool
}

// NewA2UIPromise creates a promise ready for Set/TryGet.
func NewA2UIPromise() *A2UIPromise {
	return &A2UIPromise{}
}

// Set delivers an action to the promise. Idempotent: the first Set wins,
// subsequent calls are silently ignored.
func (p *A2UIPromise) Set(action *A2UIAction) {
	if action == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.action != nil {
		return // first Set wins
	}
	p.action = action
}

// TryGet 返回已 Set 且尚未消费的 action，并标记为已消费。
// 已消费的 action 不会再次返回。
func (p *A2UIPromise) TryGet() *A2UIAction {
	p.mu.Lock()
	defer p.mu.Unlock()
	act := p.peekLocked()
	if act == nil {
		return nil
	}
	p.consumed = true
	return act
}

// Peek 返回待处理的 action，但不改变 consumed 状态。
// 供测试与诊断使用：与 TryGet 不同，Peek 之后 action 仍可被
// consumePendingA2UIActions 消费。
func (p *A2UIPromise) Peek() *A2UIAction {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peekLocked()
}

// peekLocked 返回未消费的 action；调用方须持有 mu。
func (p *A2UIPromise) peekLocked() *A2UIAction {
	if p.consumed || p.action == nil {
		return nil
	}
	return p.action
}
