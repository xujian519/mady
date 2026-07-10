// Package retrieval provides document retrieval infrastructure for the Mady
// agent framework. It is a first-class infrastructure layer designed to support
// the structured retrieval needs of patent, legal, and other professional domains.
//
// Architecture:
//
//	Document → Chunker → []Chunk → Searcher → []ScoredChunk → Reranker → []ScoredChunk
//	                                                       ↑
//	                                          (KeywordSearcher, future: VectorSearcher)
//
// Current MVP implementation (Phase C):
//   - Chunker: paragraph/section-based splitting
//   - KeywordSearcher: regex + keyword matching with TF-IDF-like scoring
//   - Reranker: simple BM25-inspired scoring with position bias
//
// Future enhancements (Phase C+):
//   - VectorSearcher: embedding-based semantic search
//   - HybridSearcher: combining keyword + vector with configurable weights
//   - Indexer: persistent index for large document collections
package retrieval
