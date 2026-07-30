package main

// mcp_command.go implements /mcp slash command for listing registered MCP
// servers from the TUI. MCP servers are discovered and injected as
// agentcore.Extension instances during bootstrap.

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// handleMCPCommand implements /mcp [status|list].
// Lists registered MCP extensions from the framework context.
func (s *tuiSession) handleMCPCommand() {
	exts := s.collectMCPExtensions()
	if len(exts) == 0 {
		s.app.PrintSystem("🔌 未注册 MCP 服务器。\n" +
			"MCP 服务器通过 ~/.mady/.mcp.json 或项目目录下的 .mcp.json 配置。\n" +
			"使用 mady trust-mcp 管理信任配置。")
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "🔌 MCP 服务器（共 %d 个）\n", len(exts))
	for _, ext := range exts {
		fmt.Fprintf(&b, "  · %s\n", ext.Name())
	}
	b.WriteString("\n用法: mady trust-mcp — 管理 MCP 信任配置")
	s.app.PrintSystem(b.String())
}

// collectMCPExtensions returns MCP extensions by naming convention.
// MCP server extensions registered by mcp.DiscoverMCPExtensions carry
// names like "mcp-stdio-<name>" or "mcp-http-<name>".
func (s *tuiSession) collectMCPExtensions() []agentcore.Extension {
	if s.fc == nil {
		return nil
	}
	seen := make(map[string]bool)
	var exts []agentcore.Extension
	for _, ext := range s.fc.BaseConfig.Extensions {
		name := strings.ToLower(ext.Name())
		if !strings.Contains(name, "mcp") || seen[name] {
			continue
		}
		seen[name] = true
		exts = append(exts, ext)
	}
	return exts
}
