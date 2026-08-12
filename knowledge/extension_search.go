package knowledge

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/retrieval"
)

// Search performs knowledge retrieval and returns scored chunks.
// When a SQLite backend is configured, this uses FTS + vector RRF fusion
// (with optional cross-encoder reranking). Otherwise it falls back to
// in-memory keyword search.
func (e *KnowledgeExtension) Search(ctx context.Context, query string, topK int) []retrieval.ScoredChunk {
	return e.search(ctx, query, topK)
}

// search dispatches to the SQLite backend (FTS + vector RRF fusion) when
// available, falling back to the in-memory keyword search otherwise.
func (e *KnowledgeExtension) search(ctx context.Context, query string, topK int) []retrieval.ScoredChunk {
	if e.backend != nil {
		return e.backendSearch(ctx, query, topK)
	}
	return e.memorySearch(ctx, query, topK)
}

// backendSearch performs FTS + vector RRF fusion via the SQLite backend.
// When a writable user store is configured, a third lane (user documents)
// is added to the RRF fusion.
//
//nolint:gocognit // 原因：多路融合搜索，含并行查询调度和结果合并
func (e *KnowledgeExtension) backendSearch(ctx context.Context, query string, topK int) []retrieval.ScoredChunk {
	candidateK := topK * 2
	if candidateK < 20 {
		candidateK = 20
	}

	var (
		mu    sync.Mutex
		lists [][]retrieval.ScoredChunk
		wg    sync.WaitGroup
	)

	// Lane 1: knowledge.db FTS (BM25).
	wg.Add(1)
	go func() {
		defer wg.Done()
		if ftsResults, err := e.backend.FTSSearch(query, candidateK); err == nil && len(ftsResults) > 0 {
			mu.Lock()
			lists = append(lists, ftsResults)
			mu.Unlock()
		} else if err != nil {
			slog.Error("knowledge: FTS search error", "err", err)
		}
	}()

	// Lane 2: knowledge.db vector (cosine similarity).
	if e.embedder != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Check query embedding cache before API call.
			var queryVec []float32
			if cached := e.queryCache.Get(query); cached != nil {
				queryVec = cached
			} else {
				vecs, err := e.embedder.Embed(ctx, []string{query})
				if err == nil && len(vecs) > 0 && len(vecs[0]) > 0 {
					queryVec = vecs[0]
					e.queryCache.Set(query, queryVec)
				} else if err != nil {
					slog.Error("knowledge: embed error", "err", err)
					return
				}
			}
			if vecResults, vErr := e.backend.VectorSearch(queryVec, candidateK); vErr == nil && len(vecResults) > 0 {
				mu.Lock()
				lists = append(lists, vecResults)
				mu.Unlock()
			} else if vErr != nil {
				slog.Error("knowledge: vector search error", "err", vErr)
			}
		}()
	}

	// Lane 3: user.db (FTS + Vector RRF, self-contained).
	if e.writable != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if userResults, err := e.writable.Search(ctx, query, candidateK); err == nil && len(userResults) > 0 {
				mu.Lock()
				lists = append(lists, userResults)
				mu.Unlock()
			} else if err != nil {
				slog.Error("knowledge: user store search error", "err", err)
			}
		}()
	}

	wg.Wait()

	if len(lists) == 0 {
		return nil
	}

	// If a cross-encoder reranker is configured, fuse more candidates
	// then rerank down to topK for better precision.
	var fused []retrieval.ScoredChunk
	if e.queryReranker != nil {
		fused = retrieval.FuseRRF(lists, candidateK)
		reranked := e.queryReranker.RerankWithQuery(ctx, query, fused)
		if len(reranked) > topK {
			reranked = reranked[:topK]
		}
		fused = reranked
	} else {
		fused = retrieval.FuseRRF(lists, topK)
	}

	// Graph-enhanced retrieval: expand results with similar cases and
	// citation chains from the knowledge graph. The context is cached for
	// BackendRetrievalHook to inject alongside chunk results.
	e.graphMu.Lock()
	if e.graph != nil && len(fused) > 0 {
		result := e.graph.Enhance(fused)
		if ge, ok := result.(GraphEnhancement); ok {
			e.lastGraphCtx = ge.GetContext()
		}
	} else {
		e.lastGraphCtx = ""
	}
	e.graphMu.Unlock()

	return fused
}

// memorySearch uses the in-memory Store with keyword search + reranking.
func (e *KnowledgeExtension) memorySearch(ctx context.Context, query string, topK int) []retrieval.ScoredChunk {
	if e.store == nil {
		return nil
	}
	chunks := e.store.SearchableChunksForDomain(e.domain)
	if len(chunks) == 0 {
		chunks = e.store.AllChunks()
	}
	if len(chunks) == 0 {
		return nil
	}

	// Lazy init cached searcher/reranker to avoid re-allocation on each call.
	if e.memorySearcher == nil {
		e.memorySearcher = retrieval.NewKeywordSearcher()
		// For large chunk sets, build an inverted index for O(term_postings) search.
		if len(chunks) >= 200 {
			idx := retrieval.BuildInvertedIndex(chunks)
			if kw, ok := e.memorySearcher.(*retrieval.KeywordSearcher); ok {
				kw.SetIndex(idx)
			}
		}
	}
	if e.memoryReranker == nil {
		e.memoryReranker = retrieval.NewPositionReranker()
	}

	results := e.memorySearcher.Search(ctx, query, chunks, topK)
	return e.memoryReranker.Rerank(results)
}

func formatToolResults(results []retrieval.ScoredChunk) string {
	var b strings.Builder
	b.WriteString("搜索结果:\n")
	for i, r := range results {
		fmt.Fprintf(&b, "\n[%d] (相关度: %.2f) %s\n", i+1, r.Score, r.Content)
	}
	fmt.Fprintf(&b, "\n共 %d 条结果", len(results))
	return b.String()
}
func lastUserMsg(msgs []agentcore.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == agentcore.RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Query embedding cache
// ---------------------------------------------------------------------------

// queryEmbedCache caches normalized query→vector mappings to avoid redundant
// embedding API calls. Cache key is the trimmed, lowercased query string.
// Default capacity is 64 entries. At 1024-dim float32, each entry is ~4 KB
// plus map overhead, so 64 entries ≈ 256 KB.
type queryEmbedCache struct {
	mu       sync.Mutex
	cache    map[string][]float32 // normalized query → vector
	keys     []string             // FIFO eviction order
	capacity int
}

// newQueryEmbedCache creates a query embedding cache with the given capacity.
// capacity=0 disables caching.
func newQueryEmbedCache(capacity int) *queryEmbedCache {
	if capacity <= 0 {
		capacity = 64
	}
	return &queryEmbedCache{
		cache:    make(map[string][]float32, capacity),
		keys:     make([]string, 0, capacity),
		capacity: capacity,
	}
}

// Get returns the cached vector for a query on exact match (after normalization),
// or nil on miss. Thread-safe.
func (c *queryEmbedCache) Get(query string) []float32 {
	if c == nil || c.capacity == 0 || len(c.cache) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := normalizeQuery(query)
	return c.cache[key]
}

// Set caches a query→vector pair. If the cache is at capacity, the oldest
// entry is evicted (FIFO). Thread-safe.
func (c *queryEmbedCache) Set(query string, vec []float32) {
	if c == nil || c.capacity == 0 || query == "" || len(vec) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := normalizeQuery(query)
	if _, exists := c.cache[key]; exists {
		c.cache[key] = vec
		return
	}
	if len(c.cache) >= c.capacity {
		// Evict oldest entry (FIFO).
		oldest := c.keys[0]
		delete(c.cache, oldest)
		c.keys = c.keys[1:]
	}
	c.cache[key] = vec
	c.keys = append(c.keys, key)
}

// normalizeQuery produces a canonical cache key: trimmed, lowercased,
// with consecutive whitespace collapsed to a single space.
func normalizeQuery(query string) string {
	lower := strings.ToLower(strings.TrimSpace(query))
	if lower == "" {
		return ""
	}
	// Collapse whitespace runs so that "专利 侵权" and "专利  侵权" collide.
	ws := regexp.MustCompile(`\s+`)
	return ws.ReplaceAllString(lower, " ")
}
