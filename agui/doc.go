// Package agui 实现 Agent GUI 事件协议（Agent GUI Protocol），通过 SSE
// （Server-Sent Events）将 Agent 内部事件实时推送给前端 UI。
//
// 主要功能：
//   - Agent 内部事件 → SSE 事件流的实时转换
//   - 事件类型定义（Thinking、ToolCall、ToolResult、Error 等）
//   - HTTP SSE 端点处理
//   - 事件格式协商（完整/增量/摘要）
//
// 主要类型：
//   - Converter: Agent 事件到 SSE 消息的类型安全转换
//   - Handler: SSE HTTP 处理器，管理连接和心跳
//   - Event: 事件类型枚举及数据结构
//
// 使用示例：
//
//	h := agui.NewHandler(agent)
//	mux.Handle("GET /events", h)
package agui
