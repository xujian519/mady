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
// # Known limitations
//
// The LifecycleHook interface exceeds the ≤5 guideline (10 methods) due to
// the inherent complexity of lifecycle hooks. This is an accepted trade-off.
//
// The iface abstraction intentionally simplifies context types:
//   - BeforeMessagePersist does not expose the Message being persisted.
//   - ToolExecutionContext exposes ToolCalls count and names but not arguments.
//   - TurnInfo.ToolCount is always 0 (not populated by the adapter).
//
// Extensions that need full-fidelity context (e.g. evidence, which requires
// tool arguments and results) should depend on agentcore directly rather than
// going through iface.
//
// Usage:
//
//	var runner iface.AgentRunner = agentcore.New(cfg)
//	output, err := runner.Run(ctx, "user input")
package iface
