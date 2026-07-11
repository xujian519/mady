// Package permission provides fine-grained tool-call access control.
//
// A Policy evaluates each tool invocation against static rules and produces
// one of three decisions: Allow, Ask, or Deny. When the decision is Ask,
// an optional Approver handles interactive confirmation.
//
// The extension integrates as a Middleware in the Executor chain, positioned
// before Guardian so that cheap deny decisions avoid expensive AI review.
package permission
