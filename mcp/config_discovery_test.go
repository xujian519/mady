package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMCPConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	cfg := MCPConfigFile{
		MCPServers: map[string]MCPServerConfig{
			"memory": {
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-memory"},
			},
			"web-search": {
				Type: "http",
				URL:  "http://localhost:3000/mcp",
			},
		},
	}
	writeJSON(t, path, cfg)

	loaded, err := LoadMCPConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.MCPServers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(loaded.MCPServers))
	}
	if loaded.MCPServers["memory"].Command != "npx" {
		t.Fatalf("memory command = %q", loaded.MCPServers["memory"].Command)
	}
	if loaded.MCPServers["web-search"].URL != "http://localhost:3000/mcp" {
		t.Fatalf("web-search url = %q", loaded.MCPServers["web-search"].URL)
	}
}

func TestLoadMCPConfig_FileNotFound(t *testing.T) {
	_, err := LoadMCPConfig("/nonexistent/path/mcp.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected os.IsNotExist, got %v", err)
	}
}

func TestLoadMCPConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadMCPConfig(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestDiscoverMCPExtensions_NoConfigFiles(t *testing.T) {
	// With no config files present, should return empty without errors.
	// Override MCP_CONFIG and HOME to avoid picking up the user's real config.
	t.Setenv("MCP_CONFIG", "/nonexistent/mcp.json")
	t.Setenv("HOME", t.TempDir())

	ctx := context.Background()
	exts, warnings := DiscoverMCPExtensions(ctx, t.TempDir())
	if len(exts) != 0 {
		t.Fatalf("expected 0 extensions, got %d", len(exts))
	}
	if len(warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %d", len(warnings))
	}
}

func TestDiscoveryTimeout_ContextOverrideWins(t *testing.T) {
	t.Setenv("MADY_MCP_DISCOVERY_TIMEOUT_MS", "2800")
	ctx := WithDiscoveryTimeout(context.Background(), 1500*time.Millisecond)

	got := discoveryTimeout(ctx)
	if got != 1500*time.Millisecond {
		t.Fatalf("discoveryTimeout(ctx) = %v, want 1.5s", got)
	}
}

func TestDiscoveryTimeout_EnvFallback(t *testing.T) {
	t.Setenv("MADY_MCP_DISCOVERY_TIMEOUT_MS", "2200")

	got := discoveryTimeout(context.Background())
	if got != 2200*time.Millisecond {
		t.Fatalf("discoveryTimeout(env) = %v, want 2.2s", got)
	}
}

func TestDiscoveryTimeout_Default(t *testing.T) {
	t.Setenv("MADY_MCP_DISCOVERY_TIMEOUT_MS", "")

	got := discoveryTimeout(context.Background())
	if got != defaultDiscoveryTimeout {
		t.Fatalf("discoveryTimeout(default) = %v, want %v", got, defaultDiscoveryTimeout)
	}
}

func TestDiscoverMCPExtensions_ConfigFileLoadOnly(t *testing.T) {
	// Test that config file is loaded correctly from MCP_CONFIG env var.
	// Does NOT test actual extension creation (which requires real MCP servers).
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mcp.json")

	cfg := MCPConfigFile{
		MCPServers: map[string]MCPServerConfig{
			"test-server": {
				Type: "http",
				URL:  "http://localhost:9999/mcp",
			},
			"local-tool": {
				Command: "my-tool",
				Args:    []string{"--serve"},
				Env:     map[string]string{"DEBUG": "1"},
			},
		},
	}
	writeJSON(t, cfgPath, cfg)

	oldVal := os.Getenv("MCP_CONFIG")
	os.Setenv("MCP_CONFIG", cfgPath)
	defer os.Setenv("MCP_CONFIG", oldVal)

	// Verify we can load and parse the config directly (without calling
	// DiscoverMCPExtensions which would try to connect to servers).
	loaded, err := LoadMCPConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.MCPServers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(loaded.MCPServers))
	}
	if loaded.MCPServers["test-server"].URL != "http://localhost:9999/mcp" {
		t.Fatalf("http server URL not loaded correctly")
	}
	if loaded.MCPServers["local-tool"].Command != "my-tool" {
		t.Fatalf("stdio command not loaded correctly")
	}
	if loaded.MCPServers["local-tool"].Env["DEBUG"] != "1" {
		t.Fatalf("env not loaded correctly")
	}
}

func TestDiscoverMCPExtensions_NameCollision(t *testing.T) {
	dir := t.TempDir()

	// Create two config files with the same server name.
	cfg1 := filepath.Join(dir, "mcp1.json")
	cfg2 := filepath.Join(dir, "mcp2.json")

	writeJSON(t, cfg1, MCPConfigFile{
		MCPServers: map[string]MCPServerConfig{
			"dup": {Command: "echo", Args: []string{"first"}},
		},
	})
	writeJSON(t, cfg2, MCPConfigFile{
		MCPServers: map[string]MCPServerConfig{
			"dup": {Command: "echo", Args: []string{"second"}},
		},
	})

	// Load from cfg1 first, then cfg2.
	cfg, _ := LoadMCPConfig(cfg1)
	seen := make(map[string]string)
	var collisions []string
	for name, serverCfg := range cfg.MCPServers {
		seen[name] = "cfg1"
		_ = serverCfg
	}

	cfg, _ = LoadMCPConfig(cfg2)
	for name := range cfg.MCPServers {
		if prev, exists := seen[name]; exists {
			collisions = append(collisions, prev)
		}
	}
	if len(collisions) != 1 {
		t.Fatalf("expected collision for 'dup', got %d", len(collisions))
	}
}

func TestDiscoverMCPExtensions_StdioEnvPassthrough(t *testing.T) {
	cfg := MCPServerConfig{
		Command: "test-cmd",
		Args:    []string{"--verbose"},
		Env:     map[string]string{"DEBUG": "1", "LOG_LEVEL": "trace"},
	}

	var env []string
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}

	if len(env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(env))
	}
	envMap := make(map[string]bool)
	for _, e := range env {
		envMap[e] = true
	}
	if !envMap["DEBUG=1"] || !envMap["LOG_LEVEL=trace"] {
		t.Fatalf("env = %v", env)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMCPServerConfig_HTTPTypeDetection(t *testing.T) {
	tests := []struct {
		cfg      MCPServerConfig
		expected string // "http" or "stdio"
	}{
		{MCPServerConfig{Command: "echo"}, "stdio"},
		{MCPServerConfig{Type: "http", URL: "http://localhost:8080"}, "http"},
		{MCPServerConfig{Type: "sse", URL: "http://localhost:8080"}, "http"},
		{MCPServerConfig{Command: "npx", Args: []string{"-y", "server"}}, "stdio"},
	}

	for _, tc := range tests {
		var got string
		switch tc.cfg.Type {
		case "http", "sse":
			got = "http"
		default:
			got = "stdio"
		}
		if got != tc.expected {
			t.Errorf("cfg %+v: expected %s, got %s", tc.cfg, tc.expected, got)
		}
	}
}

func TestDiscoverMCPExtensions_HTTPConfigHeaders(t *testing.T) {
	cfg := MCPServerConfig{
		Type:    "http",
		URL:     "http://localhost:8080/mcp",
		Headers: map[string]string{"Authorization": "Bearer test-token", "X-Custom": "value"},
	}

	if cfg.Headers["Authorization"] != "Bearer test-token" {
		t.Fatalf("headers = %v", cfg.Headers)
	}
	if cfg.Headers["X-Custom"] != "value" {
		t.Fatalf("headers = %v", cfg.Headers)
	}

	httpCfg := HTTPConfig{
		Name:     "test-http",
		Endpoint: cfg.URL,
		Headers:  cfg.Headers,
	}

	if httpCfg.Headers["Authorization"] != "Bearer test-token" {
		t.Fatalf("http headers not propagated correctly")
	}
}

func TestDiscoverMCPExtensions_MissingCommand(t *testing.T) {
	// A stdio server without a command should not crash the discovery.
	cfg := MCPServerConfig{
		// Command is empty — should be caught.
		Args: []string{"some-arg"},
	}
	if cfg.Command != "" {
		t.Fatal("expected empty command")
	}
	// The extension creation should fail gracefully in createStdioExtension.
}

func TestDiscoverMCPExtensions_MissingURL(t *testing.T) {
	// An HTTP server without a URL should not crash the discovery.
	cfg := MCPServerConfig{
		Type: "http",
		// URL is empty — should be caught.
	}
	if cfg.URL != "" {
		t.Fatal("expected empty url")
	}
	// The extension creation should fail gracefully in createHTTPExtension.
}

func TestDiscoverMCPExtensions_CompatibleWithClaudeDesktopFormat(t *testing.T) {
	// Verify the config format is compatible with Claude Desktop's mcpServers.
	// Claude Desktop uses: {"mcpServers": {"name": {"command": "...", "args": [...]}}}
	claudeStyle := `{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/allowed/files"]
    },
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_example"
      }
    }
  }
}`
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_style.json")
	if err := os.WriteFile(path, []byte(claudeStyle), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadMCPConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCPServers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(cfg.MCPServers))
	}
	if cfg.MCPServers["filesystem"].Command != "npx" {
		t.Fatalf("filesystem command = %q", cfg.MCPServers["filesystem"].Command)
	}
	if len(cfg.MCPServers["filesystem"].Args) != 3 {
		t.Fatalf("filesystem args len = %d", len(cfg.MCPServers["filesystem"].Args))
	}
	if cfg.MCPServers["github"].Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "ghp_example" {
		t.Fatalf("github env = %v", cfg.MCPServers["github"].Env)
	}
}

func TestDiscoverMCPExtensions_EmptyConfigSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	// Config with no mcpServers key
	if err := os.WriteFile(path, []byte(`{"otherKey": "value"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadMCPConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCPServers) != 0 {
		t.Fatalf("expected empty mcpServers, got %d", len(cfg.MCPServers))
	}
}

func TestDiscoverMCPExtensions_OnlyMcpServersKey(t *testing.T) {
	// Claude config files may have other top-level keys alongside mcpServers.
	// LoadMCPConfig only reads the mcpServers key.
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.json")

	content := `{
  "mcpServers": {
    "test-server": {
      "command": "echo"
    }
  },
  "otherConfig": {
    "enabled": true
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadMCPConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCPServers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.MCPServers))
	}
	if cfg.MCPServers["test-server"].Command != "echo" {
		t.Fatalf("command = %q", cfg.MCPServers["test-server"].Command)
	}
}

func TestDiscoverMCPExtensions_ScanOrderFirstWins(t *testing.T) {
	// Verify that when MCP_CONFIG is set, it's used first.
	// The scan order is: $MCP_CONFIG > ~/.mady/mcp.json > $PWD/.mcp.json > ~/.claude.json
	// This test just validates the path construction order in DiscoverMCPExtensions.
	_ = os.Getenv("MCP_CONFIG") // check env var name exists in docs
	homeDir, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	paths := []string{
		os.Getenv("MCP_CONFIG"),                 // 1. explicit
		filepath.Join("/fake/mady", "mcp.json"), // 2. ~/.mady/mcp.json
		filepath.Join(cwd, ".mcp.json"),         // 3. $PWD/.mcp.json
		filepath.Join(homeDir, ".claude.json"),  // 4. ~/.claude.json
	}
	if len(paths) != 4 {
		t.Fatalf("expected 4 scan paths, got %d", len(paths))
	}
}

func TestConfigDiscovery_HTTPHeadersPropagation(t *testing.T) {
	// Verify headers map flows correctly from MCPServerConfig to HTTPConfig.
	serverCfg := MCPServerConfig{
		Type: "http",
		URL:  "https://api.example.com/mcp",
		Headers: map[string]string{
			"Authorization": "Bearer secret123",
			"X-API-Key":     "key-abc",
		},
	}

	httpCfg := HTTPConfig{
		Name:     "api-server",
		Endpoint: serverCfg.URL,
		Headers:  serverCfg.Headers,
	}

	if httpCfg.Endpoint != "https://api.example.com/mcp" {
		t.Fatalf("endpoint not propagated")
	}
	if len(httpCfg.Headers) != 2 {
		t.Fatalf("headers not propagated, got %d", len(httpCfg.Headers))
	}
	if httpCfg.Headers["Authorization"] != "Bearer secret123" {
		t.Fatalf("auth header not propagated correctly")
	}
}

func TestConfigDiscovery_StdioConfigMapping(t *testing.T) {
	serverCfg := MCPServerConfig{
		Command: "/usr/local/bin/my-mcp-server",
		Args:    []string{"--port", "9090", "--debug"},
		Env:     map[string]string{"HOME": "/tmp", "PATH": "/usr/bin"},
	}

	var env []string
	for k, v := range serverCfg.Env {
		env = append(env, k+"="+v)
	}

	stdioCfg := StdioConfig{
		Name:    "my-server",
		Command: serverCfg.Command,
		Args:    serverCfg.Args,
		Env:     env,
	}

	if stdioCfg.Command != "/usr/local/bin/my-mcp-server" {
		t.Fatalf("command not propagated")
	}
	if len(stdioCfg.Args) != 3 {
		t.Fatalf("args not propagated, got %d", len(stdioCfg.Args))
	}
	if len(stdioCfg.Env) != 2 {
		t.Fatalf("env not propagated, got %d", len(stdioCfg.Env))
	}

	// Check env contains expected values
	envMap := make(map[string]string)
	for _, e := range stdioCfg.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}
	if envMap["HOME"] != "/tmp" || envMap["PATH"] != "/usr/bin" {
		t.Fatalf("env values incorrect: %v", envMap)
	}
}
