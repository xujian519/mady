package permission

import (
	"encoding/json"
	"fmt"
)

// Decision is the outcome of evaluating a tool call against policy rules.
type Decision int

const (
	// DecisionAllow permits the tool call without further checks.
	DecisionAllow Decision = iota
	// DecisionAsk requires interactive approval before proceeding.
	DecisionAsk
	// DecisionDeny blocks the tool call unconditionally.
	DecisionDeny
)

func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionAsk:
		return "ask"
	case DecisionDeny:
		return "deny"
	default:
		return fmt.Sprintf("decision(%d)", int(d))
	}
}

// Policy evaluates static rules to produce a Decision for a tool call.
// It is a pure function with no I/O — interactive confirmation is handled
// separately by the Approver when the decision is Ask.
type Policy struct {
	// Mode is the fallback decision when no rule matches (default: Ask).
	Mode  Decision
	Allow []Rule
	Ask   []Rule
	Deny  []Rule
}

// DefaultPolicy returns a conservative policy:
//   - read-only tools → Allow
//   - writer tools → Ask
//   - no explicit deny rules
func DefaultPolicy() Policy {
	return Policy{Mode: DecisionAsk}
}

// Decide evaluates the policy for the given tool call.
//
// Priority: Deny > Ask > Allow > fallback.
// Fallback: read-only tools → Allow（除非 Mode==Deny，此时改为 Ask 以尊重
// "默认拒绝"语义）；writer tools → Mode（默认 Ask）。
//
// 注意：readOnly 标记的优先级高于 Mode，但当 Mode==Deny 时，read-only 工具
// 也不再自动放行，而是降级为 Ask，避免在显式拒绝策略下仍放行只读工具。
func (p Policy) Decide(toolName string, readOnly bool, args json.RawMessage) Decision {
	for _, r := range p.Deny {
		if r.Matches(toolName, args) {
			return DecisionDeny
		}
	}
	for _, r := range p.Ask {
		if r.Matches(toolName, args) {
			return DecisionAsk
		}
	}
	for _, r := range p.Allow {
		if r.Matches(toolName, args) {
			return DecisionAllow
		}
	}

	if readOnly {
		// 显式 Deny 模式下，read-only 工具也不再自动放行，降级为 Ask。
		if p.Mode == DecisionDeny {
			return DecisionAsk
		}
		return DecisionAllow
	}
	if p.Mode == DecisionAllow {
		return DecisionAllow
	}
	return DecisionAsk
}
