// Package evidence provides an in-memory ledger of tool-call receipts for the
// current agent turn. The ledger records every tool invocation (tool name,
// arguments, success, paths, read/write classification) so that downstream
// components — Guardian review, plan-mode gating, step-completion verification —
// can verify agent claims against concrete execution evidence.
//
// The ledger is intentionally ephemeral: it resets at the start of each user
// turn and never serializes into prompts or session state.
package evidence
