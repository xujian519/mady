// Package cache 提供统一的缓存策略管理（Cache-First Architecture, P2）。
//
// 对齐 docs/decisions/reasonix-analysis.md §P3 Cache-First Architecture：
// 将当前散落在各模块的缓存友好模式（tiered engine prefix cache 优先、
// memory preheat、embedding/metric caching）收敛到统一抽象层。
//
// 核心类型：
//   - Policy: 缓存策略配置（TTL、优先级、失效策略）
//   - Stats: 缓存命中率统计（命中/未命中/tokens_saved）
package cache
