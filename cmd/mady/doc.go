// Package main 是 Mady 项目的统一入口，提供 tui、serve 和 acp 三个子命令。
// 默认使用 UnifiedAgent（统一 Agent 模式）：内置工具集 + Invisible Handoff 到专利/法律领域。
//
// 子命令：
//   - tui: 启动终端交互界面（8 层 Elm 架构）
//   - serve: 启动 HTTP/SSE API 服务器
//   - acp: 启动 ACP（Agent Communication Protocol）服务器
//
// 使用示例：
//
//	go run ./cmd/mady/ tui
//	go run ./cmd/mady/ serve --addr :8080
package main
