// Package retrieval provides document retrieval infrastructure for the Mady
// agent framework. It is a first-class infrastructure layer designed to support
// the structured retrieval needs of patent, legal, and other professional domains.
//
// Architecture:
//
//	Document → Chunker → []Chunk → Searcher → []ScoredChunk → Reranker → []ScoredChunk
//	                                                       ↑
//	                        (KeywordSearcher, VectorSearcher, HybridSearcher)
//
// Implementations:
//   - Chunker: paragraph/section-based splitting with UTF-8-safe truncation
//   - KeywordSearcher: regex + keyword matching with TF-IDF-like scoring;
//     an optional inverted index keeps large chunk sets fast
//   - VectorSearcher: embedding-based cosine similarity over stored vectors
//   - HybridSearcher: weighted merge of keyword and vector scores
//   - Rerankers: position, dedup, legal/patent authority, and remote model
//     reranking (ModelReranker)
package retrieval
