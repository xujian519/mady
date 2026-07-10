// Package reasoning implements structured legal/patent reasoning primitives:
// the shared FactBlackboard, the categorical Syllogism engine, the multi-hop
// ReasoningWalker, and the RuleAssertion validator.
//
// Architecture:
//
//	                  ┌─────────────────────┐
//	    facts ──────▶ │   FactBlackboard    │ ◀──── reasoning chains
//	    rules ──────▶ │  (shared memory)    │ ◀──── rule constraints
//	                  └──────────┬──────────┘
//	                             │
//	           ┌─────────────────┼─────────────────┐
//	           ▼                 ▼                 ▼
//	   ┌───────────────┐ ┌──────────────┐ ┌─────────────────┐
//	   │   Syllogism   │ │ RuleAssertion│ │ ReasoningWalker │
//	   │ 大前提→小前提  │ │  引用校验      │ │  KG 多跳遍历      │
//	   │   →结论        │ │  (结论必溯源)  │ │                 │
//	   └───────────────┘ └──────────────┘ └─────────────────┘
//
// Design principles:
//   - 三段论 — every conclusion must trace back to a blackboard fact and a legal article
//   - 可溯源 — references (FactRef + ArticleRef) are validated, not assumed
//   - 知识图谱遍历 — ReasoningWalker walks the KG once it is available (Week 5)
package reasoning
