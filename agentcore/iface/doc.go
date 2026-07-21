// Package iface provides the core interface contracts for the Mady agent runtime.
//
// These interfaces define the boundaries between agentcore and its consumers.
// External modules (guardrails, knowledge, memory, server, tui, etc.) should
// depend on these interfaces rather than on agentcore concrete types.
//
// Design principles:
//   - Each interface has ≤5 methods (Interface Segregation Principle)
//   - Interfaces export only methods, no structs
//   - Package-level dependencies go through iface, not agentcore
//   - All methods accept context.Context as first parameter
//
// Usage:
//
//	var runner iface.AgentRunner = agentcore.New(cfg)
//	output, err := runner.Run(ctx, "user input")
package iface
