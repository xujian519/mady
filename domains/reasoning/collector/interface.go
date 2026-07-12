package collector

import (
	"context"

	"github.com/xujian519/mady/domains/reasoning"
)

// FactCollector gathers facts from a specific source and writes them
// to the shared FactBlackboard. Collectors are designed to run in parallel
// via Pregel orchestration.
type FactCollector interface {
	// ID returns the collector's unique identifier.
	ID() reasoning.FactCollectorID

	// Collect gathers facts from the source and writes them to the blackboard.
	// The input parameter carries the raw user message or document content.
	// Returns a summary of what was collected.
	Collect(ctx context.Context, input string, bb *reasoning.FactBlackboard) (*reasoning.CollectResult, error)
}

// LLMClient is a minimal LLM interface used by collectors that need
// natural-language understanding (user_input, derived).
// Note: this is intentionally a different signature from reasoning.LLMClient
// (which takes []LlmMessage) — the collector interface takes a plain prompt string.
type LLMClient interface {
	Chat(ctx context.Context, prompt string) (string, error)
}

// DocReader reads a document from a file path and returns its text content.
type DocReader interface {
	ReadText(ctx context.Context, path string) (string, error)
}

// KnowledgeStore is a minimal interface for the knowledge collector
// to query the knowledge graph for related facts.
type KnowledgeStore interface {
	SearchFacts(ctx context.Context, query string, topK int) ([]KnowledgeFact, error)
}

// KnowledgeFact is a fact returned from the knowledge store.
type KnowledgeFact struct {
	ID         string
	Content    string
	Source     string // document/article name
	Confidence float64
}
