package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

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

const defaultDiscoveryTimeout = 3 * time.Second

type discoveryTimeoutContextKey struct{}

// WithDiscoveryTimeout attaches a discovery timeout override to ctx.
// It is intended for startup-sensitive entry points such as `mady tui`
// or `mady serve`, allowing them to keep MCP auto-discovery best-effort
// without paying the full default timeout on every launch.
func WithDiscoveryTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if timeout <= 0 {
		return ctx
	}
	return context.WithValue(ctx, discoveryTimeoutContextKey{}, timeout)
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
// 安全说明（C7）：$PWD/.mcp.json 属于不可信目录来源，须为当前用户所有且
// 内容已通过信任校验（`mady trust-mcp <path>` 或 MADY_MCP_TRUST_CWD=1），
// 否则跳过并给出警告——防止克隆的恶意仓库借配置静默执行 stdio command。
//
// Returns the list of successfully created extensions and any non-fatal
// warnings (e.g. parse errors for optional config files).
func DiscoverMCPExtensions(ctx context.Context, madyHome string) ([]agentcore.Extension, []error) {
	// Fast path for ACP/Zed integration: the editor passes MCP servers via
	// session/new params, and scanning local configs (especially .claude.json)
	// can block startup on slow/hung servers.
	if os.Getenv("MADY_SKIP_MCP_DISCOVERY") == "1" {
		return nil, nil
	}
	var configPaths []string
	// cwdConfigPath 标记由 $PWD 发现的配置文件（不可信目录来源，C7）；
	// 与 $MCP_CONFIG 等显式用户意图来源区分，仅对它施加信任校验。
	var cwdConfigPath string

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
		cwdConfigPath = filepath.Join(cwd, ".mcp.json")
		configPaths = append(configPaths, cwdConfigPath)
	}

	// 4. ~/.claude.json (Claude Desktop format)
	if homeDir, err := os.UserHomeDir(); err == nil {
		configPaths = append(configPaths, filepath.Join(homeDir, ".claude.json"))
	}

	var warnings []error
	seen := make(map[string]string) // server name -> config path

	// Load all configs sequentially (fast, no external processes).
	type serverEntry struct {
		name string
		path string
		cfg  MCPServerConfig
	}
	var entries []serverEntry
	for _, cfgPath := range configPaths {
		if cfgPath == "" {
			continue
		}
		// $PWD/.mcp.json 属于不可信目录来源（C7），执行其中 stdio command 前：
		// 1) 文件须为当前用户所有（防共享目录投毒）；
		// 2) 文件内容须已通过信任校验（防克隆的恶意仓库借 .mcp.json 静默执行命令）。
		// $MCP_CONFIG / ~/.mady/mcp.json / ~/.claude.json 是显式用户意图来源，不受影响。
		if cfgPath == cwdConfigPath {
			if _, err := os.Stat(cfgPath); err != nil {
				continue // 不存在：与既往行为一致，静默跳过
			}
			if !isOwnedByCurrentUser(cfgPath) {
				warnings = append(warnings,
					fmt.Errorf("mcp: skipping %s — not owned by current user (security)", cfgPath))
				continue
			}
			if !cwdTrustBypassed() && !isConfigTrusted(cfgPath, madyHome) {
				warnings = append(warnings, fmt.Errorf(
					"mcp: skipping %s — untrusted project config: "+
						"run `mady trust-mcp %s` to allow its commands, or set MADY_MCP_TRUST_CWD=1",
					cfgPath, cfgPath))
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
			entries = append(entries, serverEntry{name: name, path: cfgPath, cfg: serverCfg})
		}
	}

	if len(entries) == 0 {
		return nil, warnings
	}

	discoveryTimeout := discoveryTimeout(ctx)

	// Create extensions in parallel with a bounded total discovery timeout.
	// A single hung stdio server should not block mady startup for the rest.
	discCtx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()

	var mu sync.Mutex
	var extensions []agentcore.Extension
	var wg sync.WaitGroup
	for _, e := range entries {
		wg.Add(1)
		go func(e serverEntry) {
			defer wg.Done()
			ext, err := createExtension(discCtx, e.name, e.cfg)
			mu.Lock()
			if err != nil {
				warnings = append(warnings, fmt.Errorf("mcp: server %q (%s): %w", e.name, e.path, err))
			} else if ext != nil {
				if prev, exists := seen[e.name]; exists {
					warnings = append(warnings, fmt.Errorf(
						"mcp: server %q from %s collides with %s; keeping the first one",
						e.name, e.path, prev,
					))
				} else {
					seen[e.name] = e.path
					extensions = append(extensions, ext)
				}
			}
			mu.Unlock()
		}(e)
	}

	// Wait for discovery workers, but never block startup longer than the
	// discovery deadline. A misbehaving Close() in createExtension could
	// otherwise keep wg.Wait() from returning.
	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		wg.Wait()
	}()
	select {
	case <-waitDone:
	case <-discCtx.Done():
		warnings = append(warnings, fmt.Errorf("mcp: discovery timed out after %v; some servers may still be starting", discoveryTimeout))
	}

	return extensions, warnings
}

func discoveryTimeout(ctx context.Context) time.Duration {
	if ctx != nil {
		if timeout, ok := ctx.Value(discoveryTimeoutContextKey{}).(time.Duration); ok && timeout > 0 {
			return timeout
		}
	}
	if raw := os.Getenv("MADY_MCP_DISCOVERY_TIMEOUT_MS"); raw != "" {
		ms, err := strconv.Atoi(raw)
		if err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return defaultDiscoveryTimeout
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
	// Guard against misbehaving stdio servers that never respond: default a
	// request timeout so a single hung server cannot block mady startup.
	return NewStdioExtension(ctx, StdioConfig{
		Name:           name,
		Command:        cfg.Command,
		Args:           cfg.Args,
		Env:            env,
		RequestTimeout: 15 * time.Second,
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
