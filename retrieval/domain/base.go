// Package domain provides the shared retrieval base for domain-specific
// data sources (patent databases, legal databases, etc.).
//
// Both patent and legal domains share the same retrieval pipeline:
//
//	外部数据源 → DomainQuery → DomainRetriever → DomainResults → knowledge.Store → retrieval.Searcher → Context
//
// DomainRetriever is the key abstraction: implement it once per data source,
// and the rest of the pipeline (chunking, searching, reranking, agent injection)
// works uniformly across all domains.
package domain

import (
	"context"

	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/retrieval"
)

// DomainRetriever abstracts an external domain-specific data source.
// Each implementation handles one source (CNIPA, Google Patents, legal DB, etc.).
//
// Implementations should be stateless and safe for concurrent use;
// rate-limiting and caching are the caller's responsibility.
type DomainRetriever interface {
	// Search queries the external data source and returns ranked results.
	Search(ctx context.Context, query DomainQuery) (*DomainResults, error)

	// GetDocument fetches a single document by its source-specific ID.
	GetDocument(ctx context.Context, docID string) (*DomainDocument, error)

	// SourceName returns a human-readable name for this data source
	// (e.g. "CNIPA", "Google Patents", "中国裁判文书网").
	SourceName() string
}

// DomainQuery is a structured query for domain retrieval.
// It separates the natural-language query from structured filters,
// allowing the retriever to use both for precise results.
type DomainQuery struct {
	// Text is the natural-language query (e.g. "一种基于深度学习的图像识别方法").
	Text string

	// Keywords are extracted technical terms for precise matching.
	// e.g. ["深度学习", "图像识别", "卷积神经网络"]
	Keywords []string

	// Filters are domain-specific structured constraints.
	// Patent examples: {"ipc": "G06F17/30", "applicant": "华为"}
	// Legal examples:   {"law_source": "民法典", "article": "563"}
	Filters map[string]string

	// MaxResults limits the number of results (default: 10).
	MaxResults int
}

// DomainResults is the standardized output from a domain retrieval.
type DomainResults struct {
	Query      DomainQuery
	Documents  []DomainDocument
	TotalCount int    // total matches at source (may exceed len(Documents))
	Source     string // source name
}

// DomainDocument is a single result from a domain data source,
// normalized to a common format regardless of origin.
type DomainDocument struct {
	ID       string            // source-specific identifier
	Title    string            // document title
	Snippet  string            // short summary or highlight
	Content  string            // full document text (may be empty if only snippet available)
	URL      string            // source URL for human reference
	Metadata map[string]string // domain-specific tags (IPC, law source, court, date, etc.)
	Score    float64           // relevance score from source (0.0-1.0)
}

// ImportToStore converts domain results into knowledge store documents and chunks.
// It creates one Document per result, chunks each, and registers them under the
// given domain in the store. Already-existing docIDs are skipped.
//
// Usage:
//
//	results, _ := cnipaRetriever.Search(ctx, query)
//	count, _ := domain.ImportToStore(store, results, "patent")
//	hook := store.RetrievalHook("patent", retrieval.DefaultRetrievalConfig())
func ImportToStore(store *knowledge.Store, results *DomainResults, domainName string) (imported int, err error) {
	if results == nil || store == nil {
		return 0, nil
	}

	for _, doc := range results.Documents {
		// Skip if already loaded.
		if existing, ok := store.GetDocument(doc.ID); ok && existing != nil {
			continue
		}

		content := doc.Content
		if content == "" {
			content = doc.Snippet
		}

		if err := store.AddDocument(domainName, doc.ID, doc.Title, content, doc.URL); err != nil {
			return imported, err
		}
		imported++
	}
	return imported, nil
}

// DomainReranker creates a domain-aware reranker that boosts results matching
// domain-specific metadata criteria. This composes with the existing retrieval
// reranker chain.
type DomainReranker struct {
	// MetadataKey is the chunk metadata key to check (e.g. "ipc", "law_source").
	MetadataKey string
	// PreferredValues are metadata values that get a score boost.
	PreferredValues []string
	// Boost is the multiplier applied when metadata matches (default: 1.5).
	Boost float64
}

// Rerank implements retrieval.Reranker by boosting chunks with preferred metadata.
func (dr *DomainReranker) Rerank(results []retrieval.ScoredChunk) []retrieval.ScoredChunk {
	if dr.Boost <= 0 {
		dr.Boost = 1.5
	}
	for i := range results {
		if val, ok := results[i].Metadata[dr.MetadataKey]; ok {
			for _, pref := range dr.PreferredValues {
				if val == pref {
					results[i].Score *= dr.Boost
					break
				}
			}
		}
	}
	return results
}
