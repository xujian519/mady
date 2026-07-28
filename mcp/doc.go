// Package mcp 实现 Model Context Protocol（MCP）客户端，支持与 MCP 服务端
// 通过 stdio 或 HTTP/SSE 传输层进行通信，动态发现工具和资源。
//
// 主要功能：
//   - 客户端生命周期管理（初始化/运行/关闭）
//   - stdio 与 HTTP/SSE 双传输层支持
//   - 工具列表动态发现与刷新（tools/list_changed 通知）
//   - 资源与 Prompt 发现（resources/list、prompts/list）
//   - 服务端配置自动发现（DiscoverMCPExtensions）
//   - 信任存储与 C7 安全检查（config_trust.go）
//   - 事件驱动重连与会话续期
//
// 主要类型：
//   - Client: stdio 传输 MCP 客户端
//   - HTTPClient: HTTP/SSE 传输 MCP 客户端
//   - StdioConfig / HTTPConfig: 客户端配置
//   - ServerCapabilities: 服务端能力声明
//   - MCPServerConfig: 可序列化的服务端配置条目
//
// 使用示例：
//
//	// stdio 客户端
//	client, _ := mcp.NewStdioClient(ctx, mcp.StdioConfig{
//	    Command: "my-mcp-server",
//	    Args:    []string{"--verbose"},
//	})
//	defer client.Close()
//	tools, _ := client.ListTools(ctx)
package mcp
