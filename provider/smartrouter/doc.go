// Package smartrouter implements a task-aware [agentcore.Provider] that routes
// requests to the best-suited backend model based on task type, quality, cost,
// and latency priorities.
//
// It implements the [agentcore.Provider] interface transparently: agents
// configured with a SmartRouter delegate to it as if it were a single provider,
// while internally selecting among multiple registered model profiles.
//
// # Task awareness
//
// A [TaskClassifier] inspects the request and labels it with a [TaskType]
// (coding, reasoning, legal, patent, creative, analysis, or general). The
// router then filters registered [ModelProfile]s whose Strengths cover that
// task type and ranks them by the configured [Priority].
//
// # Priorities
//
//   - Quality:  highest QualityScore first.
//   - Cost:     lowest CostPerMTokens first.
//   - Latency:  lowest LatencyMs first.
//   - Balanced: weighted blend (50% quality, 30% cost, 20% latency).
//
// # Fallback
//
// On Complete, if the selected provider returns an error and
// [SmartRouter.EnableFallback] is true, the router retries with the next-best
// candidate. Streaming does not fallback once a stream has begun.
//
// # Route history
//
// Every routing decision is recorded in a [RouteHistory], which tracks task
// type, selected profile, success, and observed latency. This provides an audit
// trail and a foundation for future adaptive routing.
package smartrouter
