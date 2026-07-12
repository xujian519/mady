// Package collector implements Stage ① Fact Collectors for the five-step
// reasoning workflow. Each collector gathers facts from a specific source
// and writes them to the shared FactBlackboard.
//
// Collectors:
//   - user_input:  extracts key facts from the user's natural-language message
//   - documents:   parses uploaded files (PDF, DOCX, TXT) into structured facts
//   - knowledge:   retrieves relevant background facts from the knowledge graph
//   - derived:     uses LLM reasoning to derive new facts from existing ones
//
// All collectors implement the FactCollector interface and can run in parallel
// via Pregel orchestration (see stage1_graph.go).
package collector
