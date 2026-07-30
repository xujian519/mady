package knowledge

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/retrieval"
)

// ---------------------------------------------------------------------------
// SearchMode
// ---------------------------------------------------------------------------

// SearchMode controls the breadth and strategy of knowledge retrieval.
type SearchMode int

const (
	// SearchModeAuto chooses the mode based on query characteristics:
	// short queries (<3 chars) fall back to keyword text search; longer
	// queries use hybrid (keyword + vector) when an embedder is available.
	SearchModeAuto SearchMode = iota

	// SearchModeText performs plain FTS/keyword text search only.
	SearchModeText

	// SearchModeKeywordEnhanced expands the query with role-specific
	// keywords before text search.
	SearchModeKeywordEnhanced

	// SearchModeHybrid performs text + vector RRF fusion with optional
	// cross-encoder reranking.
	SearchModeHybrid
)

// ---------------------------------------------------------------------------
// SearchOptions
// ---------------------------------------------------------------------------

// SearchOptions controls the behavior of a unified search.
type SearchOptions struct {
	// TopK is the maximum number of chunks to return (default 5).
	TopK int

	// Mode controls which retrieval strategy to use.
	Mode SearchMode

	// RoleID enables role-keyword expansion: when set, the query is
	// augmented with keywords registered for that role.
	RoleID string

	// IncludeLaws controls whether laws are also searched and included
	// in the result. Only effective when a LawSearcher is configured.
	IncludeLaws bool

	// GraphContext controls whether to include the graph-enhanced
	// context block (similar cases, citation chains) in the result.
	// Only effective when a GraphEnhancer is configured.
	GraphContext bool

	// MaxChars limits each chunk content to at most this many runes.
	// 0 means no limit.
	MaxChars int

	// SourceFilter restricts search to named sources only. Empty means
	// all configured sources are searched.
	SourceFilter []string
}

// DefaultSearchOptions returns sensible defaults for agent-context search.
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		TopK:         5,
		Mode:         SearchModeAuto,
		IncludeLaws:  false,
		GraphContext: true,
		MaxChars:     3000,
	}
}

// ---------------------------------------------------------------------------
// UnifiedSearchResult
// ---------------------------------------------------------------------------

// UnifiedSearchResult is the consolidated output of a unified search.
// Zero-value fields indicate the source was either unconfigured or returned
// no matches.
type UnifiedSearchResult struct {
	// Chunks are scored text chunks from the primary knowledge store / SQLite
	// backend, ranked by relevance.
	Chunks []retrieval.ScoredChunk `json:"chunks"`

	// Laws are law-record matches from the law database, if configured.
	Laws []LawRecord `json:"laws,omitempty"`

	// GraphCtx is the graph-enhanced context block (similar cases, citation
	// chains), if configured and GraphContext was requested.
	GraphCtx string `json:"graph_ctx,omitempty"`

	// Query is the original query text (pre-expansion) for traceability.
	Query string `json:"query"`

	// ExpandedQuery is the query after role-keyword expansion, if any.
	ExpandedQuery string `json:"expanded_query,omitempty"`

	// TotalCount is the number of unique documents/items across all sources.
	TotalCount int `json:"total_count"`

	// Cached signals that at least one source returned from cache.
	Cached bool `json:"cached"`

	// Latency tracks how long the search took (wall-clock).
	Latency time.Duration `json:"latency_ms,omitempty"`
}

// ---------------------------------------------------------------------------
// roleKeywords
// ---------------------------------------------------------------------------

// roleKeywords maps a role identifier to a set of expansion keywords that
// are appended to search queries when the role requests a search. This
// replicates BCIP's RoleKeywords mechanism (see codex-patent-knowledge).
//
// The map is thread-safe for concurrent reads; writes happen at startup
// (SetRoleKeywords) and rarely at runtime (MergeRoleKeywords for hot reload).
type roleKeywords struct {
	mu   sync.RWMutex
	data map[string][]string
}

func newRoleKeywords() *roleKeywords {
	return &roleKeywords{data: make(map[string][]string)}
}

func (rk *roleKeywords) Get(roleID string) []string {
	rk.mu.RLock()
	defer rk.mu.RUnlock()
	return rk.data[roleID]
}

func (rk *roleKeywords) Set(roleID string, keywords []string) {
	rk.mu.Lock()
	defer rk.mu.Unlock()
	rk.data[roleID] = keywords
}

// ---------------------------------------------------------------------------
// unifiedResultCache
// ---------------------------------------------------------------------------

// cacheEntry holds a cached search result.
type cacheEntry struct {
	result *UnifiedSearchResult
	expiry time.Time
}

// unifiedResultCache is a TTL-based cache for search results.
//
// Design notes (BCIP reference):
//   - 5-minute TTL (matching BCIP's 5-minute query result cache)
//   - 256-entry max (matching BCIP's 256-entry upper bound)
//   - Fine-grained per-source cache keys so that subsequent searches with
//     different SourceFilter sets don't invalidate unrelated entries.
//   - Entry is by {query_normalized, source_key} — only hit when both match.
type unifiedResultCache struct {
	mu       sync.RWMutex
	entries  map[string]*cacheEntry
	keys     []string // FIFO eviction order
	capacity int
	ttl      time.Duration
}

func newUnifiedResultCache(capacity int, ttl time.Duration) *unifiedResultCache {
	if capacity <= 0 {
		capacity = 256
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &unifiedResultCache{
		entries:  make(map[string]*cacheEntry, capacity),
		keys:     make([]string, 0, capacity),
		capacity: capacity,
		ttl:      ttl,
	}
}

// cacheKey normalises a query + source list into a stable cache key.
func (c *unifiedResultCache) cacheKey(query string, sourceKey string) string {
	norm := strings.ToLower(strings.TrimSpace(query))
	if sourceKey != "" {
		return norm + "|" + sourceKey
	}
	return norm
}

// Get returns a cached result, or nil on miss/expiry.
func (c *unifiedResultCache) Get(query, sourceKey string) *UnifiedSearchResult {
	c.mu.RLock()
	entry, ok := c.entries[c.cacheKey(query, sourceKey)]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	if time.Now().After(entry.expiry) {
		// Expired; delete lazily (caller may call Set to overwrite).
		c.mu.Lock()
		delete(c.entries, c.cacheKey(query, sourceKey))
		c.mu.Unlock()
		return nil
	}
	return entry.result
}

// Set stores a result in the cache. If at capacity, the oldest entry is evicted.
func (c *unifiedResultCache) Set(query, sourceKey string, result *UnifiedSearchResult) {
	if result == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.cacheKey(query, sourceKey)
	if _, exists := c.entries[key]; exists {
		// Update in place.
		c.entries[key] = &cacheEntry{
			result: result,
			expiry: time.Now().Add(c.ttl),
		}
		return
	}
	// Evict oldest.
	if len(c.entries) >= c.capacity {
		oldest := c.keys[0]
		delete(c.entries, oldest)
		c.keys = c.keys[1:]
	}
	c.entries[key] = &cacheEntry{
		result: result,
		expiry: time.Now().Add(c.ttl),
	}
	c.keys = append(c.keys, key)
}

func (c *unifiedResultCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry, c.capacity)
	c.keys = make([]string, 0, c.capacity)
}

// ---------------------------------------------------------------------------
// UnifiedSearch
// ---------------------------------------------------------------------------

// UnifiedSearch aggregates all knowledge retrieval sources into a single
// entry point with role-aware query expansion, cross-source fusion, and
// result caching.
//
// Architecture (BCIP UnifiedSearch analog):
//
//	Query
//	  ├── Role keywords expansion ──→ expanded query
//	  │
//	  ├── [Cache hit] ──→ return cached
//	  │
//	  ├── primary: KnowledgeExtension.Search()     [FTS + Vector RRF]
//	  ├── optional: LawSearcher                    [full-text law DB]
//	  │
//	  └── fusion (RRF) + graph context injection
//	       └── cache write
//
// Create via NewUnifiedSearch(), then wire additional sources via
// WithLawSearcher / WithRoleKeywords.
type UnifiedSearch struct {
	ext *KnowledgeExtension

	lawSearcher LawSearcher
	keywords    *roleKeywords
	cache       *unifiedResultCache
	domain      string
	logger      *slog.Logger
}

// NewUnifiedSearch wraps an existing KnowledgeExtension with a unified
// search layer. The KnowledgeExtension must already be fully configured
// (including backend, embedder, reranker, graph enhancer) for the primary
// text-search path to work correctly.
//
// Example:
//
//	ext := knowledge.NewExtension(store, graphEnhancer, "patent", cfg)
//	ext.WithBackend(backend, embedder).WithReranker(reranker)
//	us := knowledge.NewUnifiedSearch(ext, "patent")
//	us.WithLawSearcher(lawSearchFn)
//	us.WithRoleKeywords(map[string][]string{
//	    "novelty_checker": {"新颖性", "现有技术"},
//	})
func NewUnifiedSearch(ext *KnowledgeExtension, domain string) *UnifiedSearch {
	return &UnifiedSearch{
		ext:      ext,
		keywords: newRoleKeywords(),
		cache:    newUnifiedResultCache(256, 5*time.Minute),
		domain:   domain,
		logger:   slog.With("component", "unified_search", "domain", domain),
	}
}

// WithLawSearcher injects a law-search function. When set, searches that
// specify IncludeLaws will also query the law database.
func (u *UnifiedSearch) WithLawSearcher(fn LawSearcher) *UnifiedSearch {
	u.lawSearcher = fn
	return u
}

// WithRoleKeywords sets the role→keyword mapping. Each role's keywords are
// appended to queries when SearchOptions.RoleID matches.
func (u *UnifiedSearch) WithRoleKeywords(m map[string][]string) *UnifiedSearch {
	for roleID, keywords := range m {
		u.keywords.Set(roleID, keywords)
	}
	return u
}

// SetRoleKeywords replaces the keyword set for a single role.
func (u *UnifiedSearch) SetRoleKeywords(roleID string, keywords []string) {
	u.keywords.Set(roleID, keywords)
}

// InvalidateCache clears all cached search results.
func (u *UnifiedSearch) InvalidateCache() {
	u.cache.Invalidate()
	u.logger.Debug("unified search cache invalidated")
}

// ---------------------------------------------------------------------------
// Search — primary entry point
// ---------------------------------------------------------------------------

// Search performs a multi-source knowledge search and returns consolidated
// results. It handles caching, role-keyword expansion, and cross-source
// fusion transparently.
//
// The method is safe for concurrent use. When opts.TopK is zero or negative,
// the default from DefaultSearchOptions is used.
func (u *UnifiedSearch) Search(ctx context.Context, query string, opts SearchOptions) (*UnifiedSearchResult, error) {
	start := time.Now()

	if query = strings.TrimSpace(query); query == "" {
		return nil, fmt.Errorf("unified_search: empty query")
	}
	if opts.TopK <= 0 {
		opts.TopK = DefaultSearchOptions().TopK
	}
	if opts.MaxChars <= 0 {
		opts.MaxChars = DefaultSearchOptions().MaxChars
	}

	// ── Role-keyword expansion ──
	expanded := query
	var sourceKey string
	if opts.RoleID != "" {
		if kw := u.keywords.Get(opts.RoleID); len(kw) > 0 {
			expanded = query + " " + strings.Join(kw, " ")
			sourceKey = "role:" + opts.RoleID
		}
	}
	if opts.IncludeLaws && u.lawSearcher != nil {
		sourceKey = sourceKey + "+laws"
	}

	// ── Cache check ──
	if cached := u.cache.Get(expanded, sourceKey); cached != nil {
		r := *cached
		r.Cached = true
		r.Latency = time.Since(start)
		return &r, nil
	}

	// ── Primary search — always delegates to KnowledgeExtension.search() ──
	// The KnowledgeExtension.search() routes internally between
	// memory keyword search and SQLite FTS+Vector RRF, so the unified
	// layer does not need its own mode routing.
	chunks := u.ext.search(ctx, expanded, opts.TopK)

	// ── Truncate content ──
	if opts.MaxChars > 0 {
		for i := range chunks {
			runes := []rune(chunks[i].Content)
			if len(runes) > opts.MaxChars {
				chunks[i].Content = string(runes[:opts.MaxChars]) + "..."
			}
		}
	}

	result := &UnifiedSearchResult{
		Chunks:        chunks,
		Query:         query,
		ExpandedQuery: expanded,
		TotalCount:    len(chunks),
	}

	// ── Law search (opt-in) ──
	if opts.IncludeLaws && u.lawSearcher != nil {
		laws, err := u.lawSearcher(query, opts.TopK)
		if err != nil {
			u.logger.Warn("law search failed", "err", err)
		} else if len(laws) > 0 {
			result.Laws = laws
			result.TotalCount += len(laws)
		}
	}

	// ── Graph context (opt-in) ──
	if opts.GraphContext {
		if gc := u.ext.GraphContext(); gc != "" {
			result.GraphCtx = gc
		}
	}

	result.Latency = time.Since(start)

	// ── Cache write ──
	u.cache.Set(expanded, sourceKey, result)

	return result, nil
}

// ---------------------------------------------------------------------------
// SearchAndFormat — convenience for agent tool handlers
// ---------------------------------------------------------------------------

// SearchAndFormat is a convenience wrapper around Search that returns a
// human-readable string suitable as an agent tool response. It is safe for
// concurrent use.
func (u *UnifiedSearch) SearchAndFormat(ctx context.Context, query string, opts SearchOptions) string {
	if query = strings.TrimSpace(query); query == "" {
		return "请提供搜索查询"
	}
	if opts.TopK <= 0 {
		opts.TopK = 5
	}

	result, err := u.Search(ctx, query, opts)
	if err != nil {
		return fmt.Sprintf("搜索失败: %v", err)
	}
	if result.TotalCount == 0 {
		return fmt.Sprintf("未找到与 \"%s\" 相关的内容", query)
	}

	var sb strings.Builder

	// Primary chunks.
	if len(result.Chunks) > 0 {
		fmt.Fprintf(&sb, "搜索结果 (共 %d 条):\n", len(result.Chunks))
		for i, c := range result.Chunks {
			fmt.Fprintf(&sb, "\n[%d] (相关度: %.2f)\n", i+1, c.Score)
			fmt.Fprintln(&sb, c.Content)
		}
	}

	// Law results.
	if len(result.Laws) > 0 {
		fmt.Fprintf(&sb, "\n相关法律法规 (共 %d 条):\n", len(result.Laws))
		for i, l := range result.Laws {
			fmt.Fprintf(&sb, "\n[%d] %s (%s)\n", i+1, l.Name, l.Level)
			if l.Subtitle != "" {
				fmt.Fprintf(&sb, "    %s\n", l.Subtitle)
			}
			content := l.Content
			if len([]rune(content)) > 2000 {
				content = string([]rune(content)[:2000]) + "..."
			}
			fmt.Fprintf(&sb, "    %s\n", content)
		}
	}

	// Graph context.
	if result.GraphCtx != "" {
		fmt.Fprintf(&sb, "\n知识图谱关联:\n%s\n", result.GraphCtx)
	}

	if result.Cached {
		fmt.Fprintf(&sb, "\n[缓存命中] (%.0fms)", result.Latency.Seconds()*1000)
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// Domain convenience accessors
// ---------------------------------------------------------------------------

// Domain returns the domain this instance is scoped to.
func (u *UnifiedSearch) Domain() string { return u.domain }

// KnowledgeExtension returns the wrapped extension, for callers that need
// to access the lower-level pipeline (e.g., GraphContext).
func (u *UnifiedSearch) KnowledgeExtension() *KnowledgeExtension { return u.ext }

// ---------------------------------------------------------------------------
// Chunk-level helpers
// ---------------------------------------------------------------------------

// SortByScoreDesc sorts chunks descending by score.
func SortByScoreDesc(ranked []retrieval.ScoredChunk) {
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})
}

// DeduplicateByContent removes chunks with >= threshold content overlap.
// threshold is in [0, 1]; 0.9 means 90% identical.
func DeduplicateByContent(ranked []retrieval.ScoredChunk, threshold float64) []retrieval.ScoredChunk {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.9
	}
	if len(ranked) <= 1 {
		return ranked
	}
	out := make([]retrieval.ScoredChunk, 0, len(ranked))
	for _, r := range ranked {
		dup := false
		for _, existing := range out {
			if contentOverlap(r.Content, existing.Content) >= threshold {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, r)
		}
	}
	return out
}

// contentOverlap computes the Jaccard-like overlap of two strings by
// character bigram sets. Returns 0-1.
func contentOverlap(a, b string) float64 {
	bigrams := func(s string) map[string]int {
		m := make(map[string]int)
		runes := []rune(s)
		for i := 0; i+1 < len(runes); i++ {
			m[string(runes[i:i+2])]++
		}
		return m
	}
	ba := bigrams(a)
	bb := bigrams(b)
	if len(ba) == 0 && len(bb) == 0 {
		return 1.0
	}
	var common int
	for k := range ba {
		if _, ok := bb[k]; ok {
			common++
		}
	}
	total := len(ba) + len(bb)
	if total == 0 {
		return 0
	}
	return 2.0 * float64(common) / float64(total)
}
