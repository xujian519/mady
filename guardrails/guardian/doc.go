// Package guardian provides AI-powered safety review for tool calls.
//
// Guardian uses a dedicated LLM sub-agent to evaluate high-risk tool
// invocations before execution. It acts as a semantic upgrade to keyword-based
// guardrails, understanding context and intent rather than matching patterns.
//
// The Guardian integrates as a Middleware in the Executor chain, positioned
// after Permission so that cheap deny decisions short-circuit before the
// expensive AI review. Read-only tools are skipped entirely for performance.
//
// Circuit breaker: if the Guardian consecutively denies maxConsecutiveDenials
// calls, it trips and auto-denies all subsequent calls until manually reset.
package guardian
