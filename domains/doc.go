// Package domains provides domain-specific Agent sub-graphs and the top-level
// Router Agent for multi-domain intent classification and delegation.
//
// Architecture:
//
//	                 ┌──────────────────┐
//	                 │   Router Agent    │
//	                 │  (意图分类+路由)  │
//	                 └────────┬─────────┘
//	      ┌───────────────────┼───────────────────┐
//	      ▼                   ▼                   ▼
//	┌──────────┐      ┌──────────────┐      ┌──────────┐
//	│ 闲聊/助理 │      │   专利子图     │      │  法律子图  │
//	│ (轻量DAG) │      │ (DAG+Pregel) │      │  (DAG)   │
//	└──────────┘      └──────────────┘      └──────────┘
//
// Each domain is implemented as a graph.CompiledGraph and registered
// as a Handoff target on the Router Agent via HandoffDelegate mode.
//
// Key design principles:
//   - 重点节点必须进行人机协作 — critical decisions require human confirmation
//   - 五步工作法 — 发现事实 → 获取规则 → 规划 → 执行 → 检查
//   - 分级护栏 — different guardrail levels per domain risk profile
package domains
