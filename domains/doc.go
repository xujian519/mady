// Package domains provides domain-specific Agent configurations and the
// top-level Router Agent for multi-domain intent classification and Handoff.
//
// Architecture (v1 — Chat / Assistant split):
//
//	                    ┌──────────────────┐
//	                    │   Router Agent    │
//	                    │  (意图分类+路由)  │
//	                    └────────┬─────────┘
//	     ┌──────────┬───────────┼───────────┬──────────┐
//	     ▼          ▼           ▼           ▼          ▼
//	┌─────────┐ ┌─────────┐ ┌─────────┐ ┌──────────┐
//	│  chat   │ │assistant│ │ patent  │ │  legal   │
//	│ (Level  │ │ (Level  │ │ (Level  │ │ (Level   │
//	│  Light) │ │Standard)│ │ Strict) │ │ Strict)  │
//	└─────────┘ └─────────┘ └─────────┘ └──────────┘
//	  纯聊天      工具执行     专利分析     法律分析
//	  +心理引擎   +检索工具    +审批关卡    +审批关卡
//
// Domain overview:
//   - chat-agent:     Daily conversation & emotional support (no tools, LevelLight)
//   - assistant-agent: Task execution with tools (web_search, file ops, LevelStandard)
//   - patent-agent:   Patent search, claim analysis, drafting (LevelStrict + approval gate)
//   - legal-advisor:  Statute search, case law, legal analysis (LevelStrict + approval gate)
//
// Each domain is registered as a HandoffDelegate target on the Router Agent.
// The Router dispatches requests based on keyword classification (with LLM fallback).
//
// Key design principles:
//   - 重点节点必须进行人机协作 — critical decisions require human confirmation
//   - 五步工作法 — 发现事实 → 获取规则 → 规划 → 执行 → 检查
//   - 分级护栏 — different guardrail levels per domain risk profile
//   - 关注点分离 — Chat and Assistant are separate agents, not one "chat-assistant"
package domains
