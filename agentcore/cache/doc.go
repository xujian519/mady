// Package cache provides cache policy configuration and statistics for
// agentcore's LLM response caching layer.
//
// # Current state
//
// The package defines:
//   - Policy: per-provider cache configuration (TTL, priority, invalidation hooks)
//   - Stats: lock-free hit/miss/token-saved counters via atomic.Int64
//   - DefaultPolicy: factory that selects a policy based on provider name
//
// Note: The actual cache storage (get/set/eviction) is NOT implemented in
// this package. TTL and InvalidationHooks are declarative configuration
// fields reserved for a future cache store implementation. Today, the
// caching layer operates at the provider level (see provider/ package)
// using prompt-cache headers, not an in-process store.
package cache
