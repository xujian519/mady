package mcp

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestDetectAgents(t *testing.T) {
	agents := DetectAgents()
	if len(agents) == 0 {
		t.Fatal("expected at least one agent entry")
	}
	// Verify each agent has required fields.
	seen := make(map[string]bool)
	for _, a := range agents {
		if a.Name == "" {
			t.Fatal("agent name is empty")
		}
		if a.DisplayName == "" {
			t.Fatalf("agent %s display name is empty", a.Name)
		}
		if a.ConfigPath == "" {
			t.Fatalf("agent %s config path is empty", a.Name)
		}
		if seen[a.Name] {
			t.Fatalf("duplicate agent %s", a.Name)
		}
		seen[a.Name] = true
	}
}

func TestInstallMady_DryRun(t *testing.T) {
	result, err := InstallMady("claude", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun {
		t.Fatal("expected dry run")
	}
	if result.Agent != "claude" {
		t.Fatalf("agent = %q", result.Agent)
	}
	if result.Action != "install" {
		t.Fatalf("action = %q", result.Action)
	}
	if result.Preview == "" {
		t.Fatal("expected non-empty preview")
	}
	// Verify preview is valid JSON for MCPConfigFile (full mcpServers wrapper).
	var cfg MCPConfigFile
	if err := json.Unmarshal([]byte(result.Preview), &cfg); err != nil {
		t.Fatalf("preview is not valid JSON: %v", err)
	}
	madyServer, ok := cfg.MCPServers["mady"]
	if !ok {
		t.Fatal("preview missing mady server entry")
	}
	if len(madyServer.Args) == 0 || madyServer.Args[0] != "acp" {
		t.Fatalf("args = %v", madyServer.Args)
	}
}

func TestInstallMady_UnknownAgent(t *testing.T) {
	_, err := InstallMady("nonexistent", true)
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestUninstallMady_DryRun(t *testing.T) {
	result, err := UninstallMady("claude", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun {
		t.Fatal("expected dry run")
	}
	if result.Action != "uninstall" {
		t.Fatalf("action = %q", result.Action)
	}
}

func TestInstallAndUninstall_RealFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "test-config.json")

	// Write initial config with an existing server entry.
	initial := &MCPConfigFile{
		MCPServers: map[string]MCPServerConfig{
			"other-server": {Command: "other"},
		},
	}
	if err := writeMCPConfig(configPath, initial); err != nil {
		t.Fatal(err)
	}

	// Read back and verify.
	cfg, err := LoadMCPConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCPServers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.MCPServers))
	}

	// Add mady entry.
	cfg.MCPServers["mady"] = MCPServerConfig{
		Command: "/usr/local/bin/mady",
		Args:    []string{"acp"},
	}
	if err := writeMCPConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	// Read back and verify both entries.
	cfg, err = LoadMCPConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCPServers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(cfg.MCPServers))
	}
	madyEntry, ok := cfg.MCPServers["mady"]
	if !ok {
		t.Fatal("mady key missing")
	}
	if madyEntry.Command != "/usr/local/bin/mady" {
		t.Fatalf("command = %q", madyEntry.Command)
	}
	if len(madyEntry.Args) != 1 || madyEntry.Args[0] != "acp" {
		t.Fatalf("args = %v", madyEntry.Args)
	}
	if _, ok := cfg.MCPServers["other-server"]; !ok {
		t.Fatal("other-server lost during merge")
	}

	// Uninstall mady.
	delete(cfg.MCPServers, "mady")
	if err := writeMCPConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	// Verify mady is gone, other-server remains.
	cfg, err = LoadMCPConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.MCPServers["mady"]; ok {
		t.Fatal("mady should have been removed")
	}
	if len(cfg.MCPServers) != 1 {
		t.Fatalf("expected 1 server after uninstall, got %d", len(cfg.MCPServers))
	}
}
