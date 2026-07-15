// Package a2a 实现 Google Agent-to-Agent（A2A）协议，使 Agent 之间能够
// 跨网络进行能力协商、任务委派和结果回传。
//
// 主要功能：
//   - Agent Card 发布与能力发现（Server/Client）
//   - 任务委派与状态同步（Task/Session）
//   - 多模态消息传输（Multimodal）
//   - 速率限制与退避重试（RateLimit）
//   - WebSocket 全双工通信（WS）
//
// 主要类型：
//   - Server: A2A 协议服务端，发布 Agent Card 并处理远程调用
//   - Client: A2A 客户端，发现远端 Agent 并委派任务
//   - Session: 会话状态管理
//   - Handoff: 与 agentcore.HandoffConfig 的桥接适配
//   - RateLimiter: 令牌桶速率限制器
//
// 使用示例：
//
//	srv := a2a.NewServer(agent, opts)
//	client := a2a.NewClient(httpClient)
//	result, err := client.SendTask(ctx, card.URL, task)
package a2a
