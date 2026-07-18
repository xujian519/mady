// Package mcp provides MCP client capabilities plus install helpers that
// wire Mady as an MCP server into external coding agents.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// AgentInfo describes a detected coding-agent CLI on the system.
type AgentInfo struct {
	// Name is the canonical agent id (e.g. "claude", "codex", "cursor").
	Name string
	// DisplayName is the human-readable label.
	DisplayName string
	// ConfigPath is the absolute path to the agent's configuration file.
	ConfigPath string
	// Installed indicates whether the agent CLI was detected.
	Installed bool
}

// InstallResult holds the outcome of an install or uninstall operation.
type InstallResult struct {
	Agent      string
	Action     string // "install" or "uninstall"
	ConfigPath string
	Preview    string // JSON preview of what was written (dry-run only)
	DryRun     bool
	Error      error
}

// Well-known agent definitions. Each entry maps an agent id to the config
// file path on the user's system.
var knownAgents = []struct {
	Name        string
	DisplayName string
	ConfigPaths []string
}{
	{
		Name:        "claude",
		DisplayName: "Claude Code",
		ConfigPaths: agentConfigPaths("claude"),
	},
	{
		Name:        "codex",
		DisplayName: "Codex CLI",
		ConfigPaths: agentConfigPaths("codex"),
	},
	{
		Name:        "cursor",
		DisplayName: "Cursor",
		ConfigPaths: agentConfigPaths("cursor"),
	},
	{
		Name:        "gemini",
		DisplayName: "Gemini CLI",
		ConfigPaths: agentConfigPaths("gemini"),
	},
	{
		Name:        "copilot",
		DisplayName: "GitHub Copilot",
		ConfigPaths: agentConfigPaths("copilot"),
	},
}

// agentConfigPaths returns a list of common config-file locations for a
// given coding agent.
func agentConfigPaths(agent string) []string {
	home, _ := os.UserHomeDir()
	var paths []string
	switch agent {
	case "claude":
		paths = append(paths,
			filepath.Join(home, ".claude", "claude_desktop_config.json"),
			filepath.Join(home, ".claude.json"),
		)
	case "codex":
		paths = append(paths,
			filepath.Join(home, ".codex", "config.yaml"),
			filepath.Join(home, ".codex.yaml"),
		)
	case "cursor":
		paths = append(paths,
			filepath.Join(home, ".cursor", "mcp.json"),
		)
	case "gemini":
		paths = append(paths,
			filepath.Join(home, ".gemini", "settings.json"),
		)
	case "copilot":
		paths = append(paths,
			filepath.Join(home, ".github-copilot", "hosts.json"),
		)
	default:
		paths = append(paths, filepath.Join(home, ".mcp.json"))
	}
	return paths
}

// DetectAgents scans well-known config paths and returns the list of
// detected agents. The result always includes all known agents; the
// Installed field indicates which ones were found.
func DetectAgents() []AgentInfo {
	var out []AgentInfo
	for _, ka := range knownAgents {
		info := AgentInfo{
			Name:        ka.Name,
			DisplayName: ka.DisplayName,
		}
		for _, cp := range ka.ConfigPaths {
			// Check if the config file already exists.
			if _, err := os.Stat(cp); err == nil {
				info.ConfigPath = cp
				info.Installed = true
				break
			}
			// Or check if the parent directory exists (agent may be installed
			// without a config file yet).
			parent := filepath.Dir(cp)
			if _, err := os.Stat(parent); err == nil {
				info.ConfigPath = cp
				info.Installed = true
				break
			}
			// Fallback: check the CLI binary on PATH.
			if _, err := exec.LookPath(agentBinary(ka.Name)); err == nil {
				info.Installed = true
				info.ConfigPath = cp
				break
			}
		}
		if info.ConfigPath == "" {
			// No config path or binary found; use the first default path.
			info.ConfigPath = ka.ConfigPaths[0]
		}
		out = append(out, info)
	}
	return out
}

// agentBinary returns the typical CLI binary name for an agent.
func agentBinary(agent string) string {
	switch agent {
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	case "cursor":
		if runtime.GOOS == "windows" {
			return "cursor.exe"
		}
		return "cursor-agent"
	case "gemini":
		return "gemini"
	case "copilot":
		if runtime.GOOS == "windows" {
			return "copilot.exe"
		}
		return "gh"
	default:
		return agent
	}
}

// InstallMady installs the Mady ACP server into the target agent's MCP
// configuration file. When dryRun is true the config is generated and
// returned in InstallResult.Preview without writing to disk.
func InstallMady(agentName string, dryRun bool) (*InstallResult, error) {
	agents := DetectAgents()
	var target *AgentInfo
	for i, a := range agents {
		if a.Name == agentName {
			target = &agents[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("mcp: unknown agent %q (supported: claude, codex, cursor, gemini, copilot)", agentName)
	}

	// Build the MCP server entry for Mady.
	madyPath, err := os.Executable()
	if err != nil {
		// Fallback: look for "mady" on PATH.
		madyPath = "mady"
	}
	serverEntry := MCPServerConfig{
		Command: madyPath,
		Args:    []string{"acp"},
	}

	fullCfg := &MCPConfigFile{
		MCPServers: map[string]MCPServerConfig{
			"mady": serverEntry,
		},
	}
	preview, err := json.MarshalIndent(fullCfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("mcp: serialize preview: %w", err)
	}

	result := &InstallResult{
		Agent:      agentName,
		Action:     "install",
		ConfigPath: target.ConfigPath,
		Preview:    string(preview),
		DryRun:     dryRun,
	}

	if dryRun {
		return result, nil
	}

	// Read existing config or create new.
	cfg, err := loadOrCreateConfig(target.ConfigPath)
	if err != nil {
		return nil, err
	}
	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]MCPServerConfig)
	}
	cfg.MCPServers["mady"] = serverEntry

	if err := writeMCPConfig(target.ConfigPath, cfg); err != nil {
		return nil, fmt.Errorf("mcp: write config %s: %w", target.ConfigPath, err)
	}

	return result, nil
}

// UninstallMady removes the Mady MCP server entry from the target agent's
// configuration.
func UninstallMady(agentName string, dryRun bool) (*InstallResult, error) {
	agents := DetectAgents()
	var target *AgentInfo
	for i, a := range agents {
		if a.Name == agentName {
			target = &agents[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("mcp: unknown agent %q", agentName)
	}

	result := &InstallResult{
		Agent:      agentName,
		Action:     "uninstall",
		ConfigPath: target.ConfigPath,
		DryRun:     dryRun,
	}

	if dryRun {
		result.Preview = "would remove mady from mcpServers in " + target.ConfigPath
		return result, nil
	}

	cfg, err := LoadMCPConfig(target.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Error = fmt.Errorf("config file %s does not exist; nothing to uninstall", target.ConfigPath)
			return result, nil
		}
		return nil, fmt.Errorf("mcp: read config %s: %w", target.ConfigPath, err)
	}

	delete(cfg.MCPServers, "mady")

	if err := writeMCPConfig(target.ConfigPath, cfg); err != nil {
		return nil, fmt.Errorf("mcp: write config %s: %w", target.ConfigPath, err)
	}

	return result, nil
}

// loadOrCreateConfig loads an existing config or returns an empty one.
func loadOrCreateConfig(path string) (*MCPConfigFile, error) {
	cfg, err := LoadMCPConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &MCPConfigFile{}, nil
		}
		return nil, fmt.Errorf("mcp: read config %s: %w", path, err)
	}
	return cfg, nil
}

// writeMCPConfig writes an MCPConfigFile as indented JSON to the given path,
// creating parent directories if needed.
func writeMCPConfig(path string, cfg *MCPConfigFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
