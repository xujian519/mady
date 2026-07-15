// Package acp 实现 Agent 通信协议（Agent Communication Protocol），基于
// JSON-RPC 2.0 提供跨进程 Agent 之间的标准消息交换机制。
//
// 主要功能：
//   - JSON-RPC 2.0 消息编码/解码（Protocol）
//   - 请求-响应生命周期管理
//   - 会话状态维护（Session）
//   - HTTP 传输层（Server）
//   - 应用级路由分发（ServerApp）
//
// 主要类型：
//   - Protocol: JSON-RPC 2.0 消息处理（Request/Response/Notification）
//   - Server: ACP HTTP 服务端，处理 JSON-RPC 请求
//   - Session: 会话上下文，维护连续对话状态
//   - ServerApp: 应用层路由分发器
//
// 使用示例：
//
//	srv := acp.NewServer(handler, acp.WithAddr(":8080"))
//	if err := srv.Start(ctx); err != nil { ... }
package acp
