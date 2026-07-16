package graph

import (
	"strconv"
	"sync"
	"time"
)

// cacheEntry holds a cached value with an expiration timestamp.
type cacheEntry struct {
	value     any
	expiresAt time.Time
}

// GraphCache is a simple TTL cache for frequent graph queries. It reduces
// repeated BFS traversals and node-detail lookups during multi-hop reasoning.
//
// The cache is best-effort: callers must invalidate it (via Invalidate) when
// the underlying store is mutated, or use a short TTL.
//
// TODO(csync): csync.Map now has Range/ForEach (added 2026-07). Migration from
// sync.RWMutex + plain maps to csync.Map is feasible. However, the current
// design uses a single mutex to atomically protect all three maps together;
// splitting into three separate csync.Map instances would require care around
// the Invalidate/Stats atomicity boundary. Consider migrating when the cache
// eviction strategy is being refactored.
type GraphCache struct {
	mu          sync.RWMutex
	nodeDetails map[string]*cacheEntry
	searches    map[string]*cacheEntry
	paths       map[string]*cacheEntry
	maxSize     int
	ttl         time.Duration
}

// NewGraphCache creates a cache with the given max entries per category and
// time-to-live duration.
func NewGraphCache(maxSize int, ttl time.Duration) *GraphCache {
	if maxSize <= 0 {
		maxSize = 1000
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &GraphCache{
		nodeDetails: make(map[string]*cacheEntry),
		searches:    make(map[string]*cacheEntry),
		paths:       make(map[string]*cacheEntry),
		maxSize:     maxSize,
		ttl:         ttl,
	}
}

// GetNodeDetail returns a cached node detail, or nil if absent/expired.
func (c *GraphCache) GetNodeDetail(id string) *GraphNodeDetail {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.nodeDetails[id]
	if !ok || time.Now().After(e.expiresAt) {
		return nil
	}
	if detail, ok := e.value.(*GraphNodeDetail); ok {
		return detail
	}
	return nil
}

// PutNodeDetail caches a node detail.
func (c *GraphCache) PutNodeDetail(id string, detail *GraphNodeDetail) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictIfNeeded(c.nodeDetails)
	c.nodeDetails[id] = &cacheEntry{value: detail, expiresAt: time.Now().Add(c.ttl)}
}

// GetSearch returns cached search results for a key, or nil if absent/expired.
func (c *GraphCache) GetSearch(key string) []*GraphNode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.searches[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil
	}
	if results, ok := e.value.([]*GraphNode); ok {
		return results
	}
	return nil
}

// PutSearch caches search results.
func (c *GraphCache) PutSearch(key string, results []*GraphNode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictIfNeeded(c.searches)
	c.searches[key] = &cacheEntry{value: results, expiresAt: time.Now().Add(c.ttl)}
}

// GetPaths returns a cached path result, or nil if absent/expired.
func (c *GraphCache) GetPaths(key string) *PathResult {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.paths[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil
	}
	if pr, ok := e.value.(*PathResult); ok {
		return pr
	}
	return nil
}

// PutPaths caches a path result.
func (c *GraphCache) PutPaths(key string, pr *PathResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictIfNeeded(c.paths)
	c.paths[key] = &cacheEntry{value: pr, expiresAt: time.Now().Add(c.ttl)}
}

// Invalidate clears all cached entries. Call this after graph mutations.
func (c *GraphCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodeDetails = make(map[string]*cacheEntry)
	c.searches = make(map[string]*cacheEntry)
	c.paths = make(map[string]*cacheEntry)
}

// InvalidateNode removes a single node's cached detail.
func (c *GraphCache) InvalidateNode(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.nodeDetails, id)
}

// Stats returns the number of entries in each cache category.
func (c *GraphCache) Stats() (details, searches, paths int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.nodeDetails), len(c.searches), len(c.paths)
}

// searchKey builds a deterministic cache key for a search query.
func searchKey(keyword, nodeType string, limit int) string {
	return nodeType + "|" + keyword + "|" + strconv.Itoa(limit)
}

// evictIfNeeded removes expired entries when a category exceeds maxSize.
func (c *GraphCache) evictIfNeeded(m map[string]*cacheEntry) {
	if len(m) < c.maxSize {
		return
	}
	now := time.Now()
	for k, e := range m {
		if now.After(e.expiresAt) {
			delete(m, k)
		}
	}
	// If still over limit, remove a random entry (simple eviction).
	if len(m) >= c.maxSize {
		for k := range m {
			delete(m, k)
			break
		}
	}
}
