// Package infringement implements a comprehensive patent infringement
// determination engine as a domains/ Agent module.
//
// Architecture:
//
//	The module follows the same Pregel sub-graph pattern as
//	domains/inventiveness/ and domains/enablement/.  A 10-node Pregel
//	graph is compiled and wrapped as an agentcore.Tool
//	("evaluate_infringement"), making it callable by the Patent Agent
//	via the standard Handoff mechanism.
//
// Dual Perspective:
//
//	Every node accepts a Perspective parameter (patentee | defendant) and
//	adapts its analysis accordingly.  The patentee perspective builds
//	affirmative infringement arguments; the defendant perspective builds
//	defense strategies.  This is implemented by injecting
//	perspective-specific instructions into each node's System Prompt
//	rather than by forking the graph topology.
//
// Four-Layer Coverage:
//
//	L1 - Core determination: claim scope → feature decomposition →
//	     all-elements rule → doctrine of equivalents (means/function/
//	     effect) → prosecution history estoppel → dedication rule →
//	     infringement verdict
//	L2 - Defense review: prior art defense (A62) → prior use defense
//	     (A69(2)) → legal source defense (A70) → exhaustion (A69(1))
//	     → rights conflict
//	L3 - Remedy assessment: damages (4-tier cascade per A65) →
//	     preliminary injunction (5 factors per A66) → permanent
//	     injunction → punitive damages risk
//	L4 - Strategy: litigation actions → jurisdiction → timeline →
//	     settlement → invalidation route
//
// Integration Points:
//
//   - domains/router.go: register tool in PatentAgent tool set
//   - domains/rules/data/: YAML article frameworks + rule definitions
//   - guardrails/citation_table.go: law article topic mappings
//   - knowledge/store.go: seed data enrichment
//   - styles/patent-standard.yaml: infringement anti-patterns
//
// Supplements:
//
//	workflows/patent/infringement.go — CLI deterministic engine, this module provides LLM-enhanced multi-perspective analysis for Agent use.
package infringement
