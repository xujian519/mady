//go:build darwin

package main

// app_mcp.go — MCP 服务器管理（T5.7，PilotDeck 对齐，只读）。
//
// ListMcpServers 返回已配置的 MCP 服务器列表（只读）。
// 扫描 ~/.mady/mcp.json 与项目 .mcp.json，不触碰信任存储写路径。

import (
	"path/filepath"
	"sort"

	"github.com/xujian519/mady/mcp"

	"github.com/xujian519/mady/pkg/util"
)

// McpServerEntry 是一个已配置 MCP 服务器的只读概要。
// Env 仅暴露键名，不返回值，防止 API Key 泄露到前端。
type McpServerEntry struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"` // stdio | http
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	URL     string   `json:"url,omitempty"`
	EnvKeys []string `json:"envKeys,omitempty"`
	// Source 是来源配置文件（~/.mady/mcp.json 或项目 .mcp.json）。
	Source string `json:"source"`
}

// ListMcpServers 返回已配置的 MCP 服务器列表（只读）。
// 扫描 ~/.mady/mcp.json 与项目 .mcp.json，不触碰信任存储写路径。
// 项目 .mcp.json 的来源不受信，仅作展示（实际执行仍需信任校验）。
func (a *App) ListMcpServers() ([]McpServerEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}

	var result []McpServerEntry
	collect := func(path, source string) {
		cfg, err := mcp.LoadMCPConfig(path)
		if err != nil {
			return // 文件不存在或解析失败：跳过（best-effort 展示）
		}
		for name, srv := range cfg.MCPServers {
			typ := srv.Type
			if typ == "" {
				typ = "stdio"
			}
			envKeys := make([]string, 0, len(srv.Env))
			for k := range srv.Env {
				envKeys = append(envKeys, k)
			}
			sort.Strings(envKeys)
			result = append(result, McpServerEntry{
				Name:    name,
				Type:    typ,
				Command: srv.Command,
				Args:    srv.Args,
				URL:     srv.URL,
				EnvKeys: envKeys,
				Source:  source,
			})
		}
	}

	if home, err := util.MadyHome(); err == nil {
		collect(filepath.Join(home, "mcp.json"), "global")
	}

	if cwd, err := a.resolveProjectDir(); err == nil {
		collect(filepath.Join(cwd, ".mcp.json"), "project")
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	if result == nil {
		result = []McpServerEntry{}
	}
	return result, nil
}
