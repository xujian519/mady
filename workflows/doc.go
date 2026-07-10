// Package workflows implements domain-specific, multi-step workflow pipelines
// for legal and patent analysis. Each workflow is composed of graph.Step nodes
// that can be composed into a DAG or Pregel execution graph.
//
// Sub-packages:
//
//	workflows/patent — patent analysis, novelty checking, conflict detection, rule engine
//	workflows/legal  — legal document comparison, clause reasoning
//
// Workflow steps support human-in-the-loop approval gates and can be wired
// into a domain Agent's HandoffConfig or domain graph.
package workflows
