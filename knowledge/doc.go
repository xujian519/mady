// Package knowledge provides high-level knowledge management on top of
// the retrieval infrastructure. It manages document collections organized
// by domain, handles multi-source loading, and integrates with the guardrails
// and context engine layers.
//
// Architecture:
//
//	Loader (files, URLs, text)
//	    │
//	    ▼
//	KnowledgeStore ──→ Chunker ──→ []Chunk
//	    │                              │
//	    │                              ▼
//	    │                          Searcher ←── Query
//	    │                              │
//	    ▼                              ▼
//	RetrievalHook ◄── ScoredChunks ── Reranker
//
// Usage:
//
//	store := knowledge.NewStore()
//	store.LoadDocument("patent-cn-2024", "path/to/claims.txt")
//	store.LoadURL("law-civil-code", "https://example.com/civil-code.txt")
//
//	// Get a retrieval hook for Agent integration.
//	hook := store.RetrievalHook("patent", retrieval.DefaultRetrievalConfig())
//	cfg.Lifecycle = agentcore.LifecycleChain{hook}
package knowledge
