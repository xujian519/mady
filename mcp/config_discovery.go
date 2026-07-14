package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// MCPServerConfig represents a single MCP server entry in a config file.
// Follows the Claude Desktop / Claude Code config format.
type MCPServerConfig struct {
	Command string            `json:"command,omitempty"` // stdio: executable
	Args    []string          `json:"args,omitempty"`    // stdio: arguments
	Env     map[string]string `json:"env,omitempty"`     // stdio: extra environment
	Type    string            `json:"type,omitempty"`    // "http" for HTTP/SSE servers, empty/"stdio" for stdio
	URL     string            `json:"url,omitempty"`     // http: endpoint URL
	Headers map[string]string `json:"headers,omitempty"` // http: extra request headers
}

// MCPConfigFile represents the top-level MCP config file structure.
// Compatible with Claude Desktop mcpServers format.
type MCPConfigFile struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// LoadMCPConfig reads and parses an MCP config JSON file.
func LoadMCPConfig(path string) (*MCPConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg MCPConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("mcp: parse %s: %w", path, err)
	}
	return &cfg, nil
}

// DiscoverMCPExtensions scans standard locations for MCP config files and
// creates agentcore.Extension instances for each configured server.
//
// Scan order (first successful config wins per server name):
//  1. $MCP_CONFIG (explicit file path)
//  2. ~/.mady/mcp.json
//  3. $PWD/.mcp.json
//  4. ~/.claude.json (Claude Desktop format, only if it contains "mcpServers")
//
// Returns the list of successfully created extensions and any non-fatal
// warnings (e.g. parse errors for optional config files).
func DiscoverMCPExtensions(ctx context.Context, madyHome string) ([]agentcore.Extension, []error) {
	var configPaths []string

	// 1. $MCP_CONFIG — explicit override
	if p := os.Getenv("MCP_CONFIG"); p != "" {
		configPaths = append(configPaths, p)
	}

	// 2. ~/.mady/mcp.json
	if madyHome != "" {
		configPaths = append(configPaths, filepath.Join(madyHome, "mcp.json"))
	}

	// 3. $PWD/.mcp.json
	if cwd, err := os.Getwd(); err == nil {
		configPaths = append(configPaths, filepath.Join(cwd, ".mcp.json"))
	}

	// 4. ~/.claude.json (Claude Desktop format)
	if homeDir, err := os.UserHomeDir(); err == nil {
		configPaths = append(configPaths, filepath.Join(homeDir, ".claude.json"))
	}

	var extensions []agentcore.Extension
	var warnings []error
	seen := make(map[string]string) // server name -> config path

	for _, cfgPath := range configPaths {
		if cfgPath == "" {
			continue
		}
		// Skip $PWD/.mcp.json if not owned by current user (security:
		// prevents command execution via malicious config in shared dirs).
		if strings.HasSuffix(cfgPath, string(filepath.Separator)+".mcp.json") {
			if !isOwnedByCurrentUser(cfgPath) {
				warnings = append(warnings,
					fmt.Errorf("mcp: skipping %s — not owned by current user (security)", cfgPath))
				continue
			}
		}
		cfg, err := LoadMCPConfig(cfgPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // silently skip missing optional config files
			}
			warnings = append(warnings, fmt.Errorf("mcp: %s: %w", cfgPath, err))
			continue
		}
		if len(cfg.MCPServers) == 0 {
			continue
		}

		for name, serverCfg := range cfg.MCPServers {
			if prev, exists := seen[name]; exists {
				warnings = append(warnings, fmt.Errorf(
					"mcp: server %q from %s collides with %s; keeping the first one",
					name, cfgPath, prev,
				))
				continue
			}

			ext, err := createExtension(ctx, name, serverCfg)
			if err != nil {
				warnings = append(warnings, fmt.Errorf("mcp: server %q (%s): %w", name, cfgPath, err))
				continue
			}
			if ext != nil {
				seen[name] = cfgPath
				extensions = append(extensions, ext)
			}
		}
	}

	return extensions, warnings
}

// createExtension builds the appropriate agentcore.Extension from a server config.
func createExtension(ctx context.Context, name string, cfg MCPServerConfig) (agentcore.Extension, error) {
	switch cfg.Type {
	case "http", "sse":
		return createHTTPExtension(ctx, name, cfg)
	default:
		return createStdioExtension(ctx, name, cfg)
	}
}

func createStdioExtension(ctx context.Context, name string, cfg MCPServerConfig) (agentcore.Extension, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("missing command for stdio server")
	}
	var env []string
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	return NewStdioExtension(ctx, StdioConfig{
		Name:    name,
		Command: cfg.Command,
		Args:    cfg.Args,
		Env:     env,
	})
}

func createHTTPExtension(ctx context.Context, name string, cfg MCPServerConfig) (agentcore.Extension, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("missing url for HTTP server")
	}
	return NewHTTPExtension(ctx, HTTPConfig{
		Name:     name,
		Endpoint: cfg.URL,
		Headers:  cfg.Headers,
	})
}
