// Package doomloop provides a detector framework for identifying and breaking
// agent execution loops (infinite loops, repetition traps, cycling, etc.) in
// the Mady agent runtime.
//
// It implements six detectors inspired by XiaoNuo Agent's doom-loop detection:
//
//  1. ToolCallLoopDetector — repeated identical tool calls (same name + args)
//  2. TextRepetitionDetector — repetitive text patterns in model output
//  3. CycleDetector — execution cycles (A→B→A→B)
//  4. EmptyResultDetector — consecutive empty/no-op tool results
//  5. CircuitBreaker — global iteration limit across all criteria
//  6. CompactionBreaker — repeated compaction/summary without progress
//
// Usage:
//
//	detector := doomloop.New(doomloop.WithToolCallLoop(3), doomloop.WithCircuitBreaker(50))
//	agent.RegisterLifecycleHook(detector.AsHook())
//
// The detector implements agentcore.LifecycleHook and monitors both
// AfterModelCall and AfterToolExecution phases.
//
// # Concurrency
//
// Individual detectors are NOT internally synchronized. The DoomLoop
// coordinator serializes hook invocations by holding its own mutex across
// each detector sweep, so a single DoomLoop instance is safe for the normal
// agent lifecycle (one hook chain per agent run). A DoomLoop shared across
// multiple concurrent agent runs is also protected by this mutex, though
// OnSignal callbacks are always invoked outside the lock and must not call
// back into the DoomLoop re-entrantly.
package doomloop
