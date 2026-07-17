// Package autoresearch 实现面向专利/法律长周期研究任务的自动研究协议。
//
// 对齐 docs/decisions/reasonix-analysis.md §P2 AutoResearch Protocol：
// 状态机管理、任务契约、成功标准追踪、方向追踪、心跳监控。
//
// 适用场景：专利无效宣告检索、法律条文体系性分析等需要多轮推进的长周期任务。
// 状态存储与 session 隔离，不污染 agent 对话历史。
package autoresearch
