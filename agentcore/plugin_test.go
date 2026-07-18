package agentcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePlugin_Valid(t *testing.T) {
	p := PluginManifest{
		Name:        "novelty-analysis",
		Version:     "0.1.0",
		Domain:      "patent",
		Description: "Patent novelty analysis workflow",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				{ID: "search", Tool: "patent_search", Description: "Search prior art"},
				{ID: "compare", Tool: "reasoning", Description: "Compare features"},
			},
		},
	}
	if err := ValidatePlugin(p); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePlugin_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		p    PluginManifest
	}{
		{"missing name", PluginManifest{Domain: "patent", Description: "desc", Pipeline: PluginPipeline{Stages: []PluginStage{{ID: "s", Tool: "t"}}}}},
		{"missing domain", PluginManifest{Name: "test", Description: "desc", Pipeline: PluginPipeline{Stages: []PluginStage{{ID: "s", Tool: "t"}}}}},
		{"missing description", PluginManifest{Name: "test", Domain: "patent", Pipeline: PluginPipeline{Stages: []PluginStage{{ID: "s", Tool: "t"}}}}},
		{"no stages", PluginManifest{Name: "test", Domain: "patent", Description: "desc"}},
		{"duplicate stage id", PluginManifest{Name: "test", Domain: "patent", Description: "desc",
			Pipeline: PluginPipeline{Stages: []PluginStage{{ID: "s", Tool: "t"}, {ID: "s", Tool: "t2"}}}}},
		{"empty stage id", PluginManifest{Name: "test", Domain: "patent", Description: "desc",
			Pipeline: PluginPipeline{Stages: []PluginStage{{ID: "", Tool: "t"}}}}},
		{"invalid name", PluginManifest{Name: "Invalid_Name", Domain: "patent", Description: "desc",
			Pipeline: PluginPipeline{Stages: []PluginStage{{ID: "s", Tool: "t"}}}}},
		{"name too long", PluginManifest{Name: "a" + string(make([]byte, 65)), Domain: "patent", Description: "desc",
			Pipeline: PluginPipeline{Stages: []PluginStage{{ID: "s", Tool: "t"}}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePlugin(tt.p); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadPlugin_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	p := PluginManifest{
		Name:        "novelty-analysis",
		Version:     "0.1.0",
		Domain:      "patent",
		Description: "Patent novelty analysis",
		Pipeline: PluginPipeline{
			Stages: []PluginStage{
				{ID: "search", Tool: "patent_search", Description: "Search prior art"},
				{ID: "compare", Tool: "reasoning", Description: "Compare features"},
				{ID: "approval", Tool: "approval_gate", Description: "Human review"},
			},
		},
		AllowedSources: []string{"mady-router"},
		HandoffTargets: []string{"patent-agent"},
	}
	data, _ := json.MarshalIndent(p, "", "  ")
	os.WriteFile(path, data, 0o644)

	loaded, err := LoadPlugin(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != p.Name {
		t.Fatalf("name = %q", loaded.Name)
	}
	if len(loaded.Pipeline.Stages) != 3 {
		t.Fatalf("stages = %d", len(loaded.Pipeline.Stages))
	}
	if len(loaded.AllowedSources) != 1 || loaded.AllowedSources[0] != "mady-router" {
		t.Fatalf("allowed_sources = %v", loaded.AllowedSources)
	}
}

func TestScanPlugins(t *testing.T) {
	root := t.TempDir()

	// Plugin 1: novelty-analysis
	dir1 := filepath.Join(root, "patent", "novelty-analysis")
	os.MkdirAll(dir1, 0o755)
	p1 := PluginManifest{
		Name: "novelty-analysis", Version: "0.1.0", Domain: "patent", Description: "Novelty",
		Pipeline: PluginPipeline{Stages: []PluginStage{{ID: "s", Tool: "t"}}},
	}
	data1, _ := json.MarshalIndent(p1, "", "  ")
	os.WriteFile(filepath.Join(dir1, "plugin.json"), data1, 0o644)

	// Plugin 2: infringement-check
	dir2 := filepath.Join(root, "patent", "infringement-check")
	os.MkdirAll(dir2, 0o755)
	p2 := PluginManifest{
		Name: "infringement-check", Version: "0.1.0", Domain: "patent", Description: "Infringement",
		Pipeline: PluginPipeline{Stages: []PluginStage{{ID: "s", Tool: "t"}}},
	}
	data2, _ := json.MarshalIndent(p2, "", "  ")
	os.WriteFile(filepath.Join(dir2, "plugin.json"), data2, 0o644)

	plugins, err := ScanPlugins(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}
	// Verify skill path auto-resolution.
	for _, p := range plugins {
		if p.SkillPath == "" {
			t.Fatalf("plugin %s: skill path not resolved", p.Name)
		}
	}
}

func TestScanPlugins_DuplicateName(t *testing.T) {
	root := t.TempDir()

	dir1 := filepath.Join(root, "a")
	os.MkdirAll(dir1, 0o755)
	p := PluginManifest{
		Name: "dup", Version: "0.1.0", Domain: "patent", Description: "First",
		Pipeline: PluginPipeline{Stages: []PluginStage{{ID: "s", Tool: "t"}}},
	}
	data, _ := json.MarshalIndent(p, "", "  ")
	os.WriteFile(filepath.Join(dir1, "plugin.json"), data, 0o644)

	dir2 := filepath.Join(root, "b")
	os.MkdirAll(dir2, 0o755)
	p.Description = "Second"
	data, _ = json.MarshalIndent(p, "", "  ")
	os.WriteFile(filepath.Join(dir2, "plugin.json"), data, 0o644)

	plugins, err := ScanPlugins(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 (first wins), got %d", len(plugins))
	}
	if plugins[0].Description != "First" {
		t.Fatalf("expected first, got %s", plugins[0].Description)
	}
}
