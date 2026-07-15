// Package main 是 Mady 项目的统一入口，提供 tui、serve 和 acp 三个
// 子命令，支持 Router Agent 模式和 Invisible Handoff 集成模式。
//
// 子命令：
//   - tui: 启动终端交互界面（8 层 Elm 架构）
//   - serve: 启动 HTTP/SSE API 服务器
//   - acp: 启动 ACP（Agent Communication Protocol）服务器
//
// 环境变量：
//   - MADY_ROUTER_MODE=1: 启用显式 Router 模式（默认集成模式）
//   - MADY_SINGLE_AGENT=1: 回退单 Agent 模式
//
// 使用示例：
//
//	go run ./cmd/mady/ tui
//	go run ./cmd/mady/ serve --addr :8080
package main
