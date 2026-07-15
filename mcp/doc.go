// Package mcp 实现 Model Context Protocol（MCP）客户端，支持与 MCP 服务端
// 通过 stdio 或 HTTP/SSE 传输层进行通信，动态发现工具和资源。
//
// 主要功能：
//   - 客户端生命周期管理（初始化/运行/关闭）
//   - stdio 与 HTTP/SSE 双传输层支持
//   - 工具列表动态发现与刷新（Tools/Resources）
//   - 服务端配置发现（ConfigDiscovery）
//   - 事件驱动重连与心跳保活
//
// 主要类型：
//   - Client: MCP 客户端，管理连接和消息收发
//   - Config: 客户端配置（命令/URL/传输模式等）
//   - Capabilities: 服务端能力声明
//   - ConfigDiscoverer: 按优先级链发现 MCP 服务端配置
//
// 使用示例：
//
//	client, _ := mcp.NewClient(cfg)
//	defer client.Close()
//	tools, _ := client.ListTools(ctx)
package mcp
