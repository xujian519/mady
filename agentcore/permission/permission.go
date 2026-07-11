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
// Fallback: read-only tools → Allow; writer tools → Mode (default Ask).
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
		return DecisionAllow
	}
	if p.Mode == DecisionAllow {
		return DecisionAllow
	}
	return DecisionAsk
}
