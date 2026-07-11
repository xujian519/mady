// Package planmode provides tool-call gating for plan mode.
//
// When plan mode is active, write-capable tools (edit, write_file, delete,
// move, bash with side effects) are blocked so the agent can only research
// and plan without modifying the workspace. Read-only tools continue to
// operate normally.
//
// The extension integrates as a LifecycleProvider hook on BeforeToolExecution.
// When inactive it is completely transparent with zero overhead.
package planmode
