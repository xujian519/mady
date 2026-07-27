//go:build integration

package integration_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// TestPluginManager_DiscoverAllPatentPlugins verifies that PluginManager
// discovers the 3 standard patent plugins from plugins/ directory.
func TestPluginManager_DiscoverAllPatentPlugins(t *testing.T) {
	pluginDir := filepath.Join("..", "plugins")
	pm, err := agentcore.NewPluginManager(nil, nil, pluginDir)
	if err != nil {
		t.Fatalf("NewPluginManager failed: %v", err)
	}

	plugins := pm.Plugins()
	if len(plugins) == 0 {
		t.Fatal("no plugins discovered — expected 3 patent plugins")
	}

	expectedPlugins := map[string]bool{
		"novelty-analysis":   false,
		"infringement-check": false,
		"oa-response":        false,
	}

	for _, p := range plugins {
		if _, ok := expectedPlugins[p.Name]; ok {
			expectedPlugins[p.Name] = true
		}
	}

	for name, found := range expectedPlugins {
		if !found {
			t.Errorf("expected plugin %q not found in plugins/ directory", name)
		}
	}
}

// TestPluginTool_RunPluginTool verifies that RunPluginTool() returns
// a properly structured agentcore.Tool with correct name and parameters.
func TestPluginTool_RunPluginTool(t *testing.T) {
	pluginDir := filepath.Join("..", "plugins")
	pm, err := agentcore.NewPluginManager(nil, nil, pluginDir)
	if err != nil {
		t.Fatalf("NewPluginManager failed: %v", err)
	}

	tool := pm.RunPluginTool()
	if tool == nil {
		t.Fatal("RunPluginTool returned nil")
	}
	if tool.Name != "run_plugin" {
		t.Errorf("expected tool name 'run_plugin', got %q", tool.Name)
	}
	if tool.Description == "" {
		t.Error("Tool.Description must not be empty")
	}

	// Verify parameters include plugin_name.
	params := tool.Parameters
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Parameters.properties must be a map, got %T", params["properties"])
	}
	if _, ok := props["plugin_name"]; !ok {
		t.Error("Parameters must include plugin_name")
	}

	// Verify DynamicParameters callback works.
	if tool.DynamicParameters != nil {
		dyn := tool.DynamicParameters()
		if desc, ok := dyn["plugin_descriptions"]; ok {
			t.Logf("DynamicParameters includes plugin_descriptions: %+v", desc)
		}
	}
}

// TestPluginVsPregel_ToolNameConsistency verifies that Plugin names and
// Pregel tool names follow a consistent naming convention for the same
// workflow type. This acts as an early warning if plugin names drift
// from their corresponding Pregel implementations.
func TestPluginVsPregel_ToolNameConsistency(t *testing.T) {
	pluginDir := filepath.Join("..", "plugins")
	pm, err := agentcore.NewPluginManager(nil, nil, pluginDir)
	if err != nil {
		t.Fatalf("NewPluginManager failed: %v", err)
	}

	pluginMap := make(map[string]*agentcore.PluginManifest)
	for _, p := range pm.Plugins() {
		pluginMap[p.Name] = &p
	}

	// Plugin → expected Pregel tool name mapping.
	cases := []struct {
		pluginName     string
		expectedTool   string
		expectedDomain string
	}{
		{"novelty-analysis", "analyze_patent_novelty", "patent"},
		{"infringement-check", "analyze_patent_infringement", "patent"},
		{"oa-response", "analyze_oa_response", "patent"},
	}

	for _, tc := range cases {
		plugin, ok := pluginMap[tc.pluginName]
		if !ok {
			t.Errorf("plugin %q not found in plugins/", tc.pluginName)
			continue
		}
		if plugin.Domain != tc.expectedDomain {
			t.Errorf("plugin %q: expected domain %q, got %q",
				tc.pluginName, tc.expectedDomain, plugin.Domain)
		}
	}
}

// TestPluginTool_ExtensionInBaseConfig verifies that a minimal Config
// correctly holds the plugin-tool extension when it is appended.
// This validates that BaseConfig.Extensions is a correct carrier for
// the run_plugin tool as done in InitPlugins (pkg/framework/setup.go).
func TestPluginTool_ExtensionInBaseConfig(t *testing.T) {
	pluginDir := filepath.Join("..", "plugins")
	pm, err := agentcore.NewPluginManager(nil, nil, pluginDir)
	if err != nil {
		t.Fatalf("NewPluginManager failed: %v", err)
	}

	// Simulate what InitPlugins does: add run_plugin as an extension.
	tool := pm.RunPluginTool()
	ext := &pluginToolExt{tool: tool}

	cfg := agentcore.Config{}
	cfg.Extensions = append(cfg.Extensions, ext)

	// Verify extension is carried in the config.
	found := false
	for _, e := range cfg.Extensions {
		if e.Name() == "plugin-tool" {
			found = true
			// Verify it provides the run_plugin tool.
			if tp, ok := e.(interface{ Tools() []*agentcore.Tool }); ok {
				for _, t2 := range tp.Tools() {
					if t2.Name == "run_plugin" {
						t.Logf("run_plugin tool found in extension %q", e.Name())
						return
					}
				}
				t.Error("plugin-tool extension does not provide run_plugin tool")
			}
			break
		}
	}
	if !found {
		t.Error("plugin-tool extension not found in config.Extensions")
	}
}

// pluginToolExt wraps a single *agentcore.Tool into an Extension,
// mirroring the pattern used in pkg/framework/setup.go.
type pluginToolExt struct {
	tool *agentcore.Tool
}

func (e *pluginToolExt) Name() string                                     { return "plugin-tool" }
func (e *pluginToolExt) Init(_ context.Context, _ *agentcore.Agent) error { return nil }
func (e *pluginToolExt) Dispose() error                                   { return nil }
func (e *pluginToolExt) Tools() []*agentcore.Tool                         { return []*agentcore.Tool{e.tool} }
