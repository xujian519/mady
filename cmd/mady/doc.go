// Package main 是 Mady 项目的统一入口，提供 tui、serve、acp 等 11 个子命令。
// 默认使用 UnifiedAgent（统一 Agent 模式）：内置工具集 + Invisible Handoff 到专利/法律领域。
//
// 子命令入口（按包分布）：
//   - main:    tui, serve, acp（需要 framework shim 共享符号）
//   - subcmd:  evidence, eval, patent, mcp-install, trust-mcp,
//     trust-knowledge, ocr, util（独立子命令）
//
// 使用示例：
//
//	go run ./cmd/mady/ tui
//	go run ./cmd/mady/ serve --addr :8080
//	go run ./cmd/mady/ eval --suite p2a --mode static
package main
