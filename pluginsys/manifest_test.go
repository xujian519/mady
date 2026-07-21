package pluginsys_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/pluginsys"
)

func validPlugin() pluginsys.PluginManifest {
	return pluginsys.PluginManifest{
		Name:        "novelty-check",
		Version:     "0.1.0",
		Domain:      "patent",
		Description: "新颖性检查工作流",
		Pipeline: pluginsys.PluginPipeline{
			Stages: []pluginsys.PluginStage{
				{ID: "search", Tool: "patent_search", Description: "检索对比文件"},
				{ID: "compare", Atom: "compare", Description: "特征对比"},
				{ID: "report", Tool: "generate_report", Description: "生成新颖性报告"},
			},
		},
	}
}

func TestValidatePlugin_Valid(t *testing.T) {
	p := validPlugin()
	if err := pluginsys.ValidatePlugin(p, nil); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidatePlugin_WithOptions(t *testing.T) {
	p := validPlugin()
	opts := &pluginsys.ValidateOptions{
		AtomLookupFn: func(name string) bool {
			return name == "compare"
		},
		IsValidGuardrailLevel: func(level string) bool {
			return level == "light" || level == "standard" || level == "strict"
		},
	}
	if err := pluginsys.ValidatePlugin(p, opts); err != nil {
		t.Fatalf("expected no error with valid options, got: %v", err)
	}
}

func TestValidatePlugin_MissingName(t *testing.T) {
	p := validPlugin()
	p.Name = ""
	err := pluginsys.ValidatePlugin(p, nil)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidatePlugin_InvalidName(t *testing.T) {
	p := validPlugin()
	p.Name = "Invalid Name!"
	err := pluginsys.ValidatePlugin(p, nil)
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestValidatePlugin_NameTooLong(t *testing.T) {
	p := validPlugin()
	p.Name = "a"
	for i := 0; i < 70; i++ {
		p.Name += "-toolong"
	}
	err := pluginsys.ValidatePlugin(p, nil)
	if err == nil {
		t.Fatal("expected error for too-long name")
	}
}

func TestValidatePlugin_MissingDomain(t *testing.T) {
	p := validPlugin()
	p.Domain = ""
	err := pluginsys.ValidatePlugin(p, nil)
	if err == nil {
		t.Fatal("expected error for missing domain")
	}
}

func TestValidatePlugin_MissingDescription(t *testing.T) {
	p := validPlugin()
	p.Description = ""
	err := pluginsys.ValidatePlugin(p, nil)
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}

func TestValidatePlugin_EmptyPipeline(t *testing.T) {
	p := validPlugin()
	p.Pipeline.Stages = nil
	err := pluginsys.ValidatePlugin(p, nil)
	if err == nil {
		t.Fatal("expected error for empty pipeline")
	}
}

func TestValidatePlugin_DuplicateStageID(t *testing.T) {
	p := validPlugin()
	p.Pipeline.Stages = append(p.Pipeline.Stages, pluginsys.PluginStage{ID: "search", Tool: "x", Description: "dup"})
	err := pluginsys.ValidatePlugin(p, nil)
	if err == nil {
		t.Fatal("expected error for duplicate stage id")
	}
}

func TestValidatePlugin_MissingToolAndAtom(t *testing.T) {
	p := validPlugin()
	p.Pipeline.Stages = append(p.Pipeline.Stages, pluginsys.PluginStage{ID: "extra"})
	err := pluginsys.ValidatePlugin(p, nil)
	if err == nil {
		t.Fatal("expected error for stage missing both tool and atom")
	}
}

func TestValidatePlugin_InvalidGuardrailLevel(t *testing.T) {
	p := validPlugin()
	p.GuardrailLevel = "invalid-level"
	opts := &pluginsys.ValidateOptions{
		IsValidGuardrailLevel: func(level string) bool {
			return level == "light" || level == "standard" || level == "strict"
		},
	}
	err := pluginsys.ValidatePlugin(p, opts)
	if err == nil {
		t.Fatal("expected error for invalid guardrail level")
	}
}

func TestValidatePlugin_UnknownAtom(t *testing.T) {
	p := validPlugin()
	p.Pipeline.Stages[1].Atom = "nonexistent-atom"
	opts := &pluginsys.ValidateOptions{
		AtomLookupFn: func(name string) bool { return name == "compare" || name == "search" },
	}
	err := pluginsys.ValidatePlugin(p, opts)
	if err == nil {
		t.Fatal("expected error for unknown atom")
	}
}

func TestLoadPlugin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	content := `{
		"name": "test-plugin",
		"domain": "patent",
		"description": "测试插件",
		"pipeline": {
			"stages": [
				{"id": "s1", "tool": "search", "description": "搜索"}
			]
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := pluginsys.LoadPlugin(path, nil)
	if err != nil {
		t.Fatalf("LoadPlugin failed: %v", err)
	}
	if p.Name != "test-plugin" {
		t.Errorf("expected name test-plugin, got %s", p.Name)
	}
}

func TestScanPlugins(t *testing.T) {
	dir := t.TempDir()
	// Create two plugin directories.
	for _, name := range []string{"plugin-a", "plugin-b"} {
		pdir := filepath.Join(dir, name)
		if err := os.MkdirAll(pdir, 0755); err != nil {
			t.Fatal(err)
		}
		content := `{
			"name": "` + name + `",
			"domain": "patent",
			"description": "` + name + ` plugin",
			"pipeline": {
				"stages": [
					{"id": "s1", "tool": "search", "description": "搜索"}
				]
			}
		}`
		if err := os.WriteFile(filepath.Join(pdir, "plugin.json"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	plugins, err := pluginsys.ScanPlugins([]string{dir}, nil)
	if err != nil {
		t.Fatalf("ScanPlugins failed: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}
}

func TestScanPlugins_NonexistentRoot(t *testing.T) {
	plugins, err := pluginsys.ScanPlugins([]string{"/nonexistent/path"}, nil)
	if err != nil {
		t.Fatalf("expected no error for nonexistent root, got: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestScanPlugins_FirstWins(t *testing.T) {
	dir := t.TempDir()
	// Create two plugins with the same name.
	for i, sub := range []string{"first", "second"} {
		pdir := filepath.Join(dir, sub)
		if err := os.MkdirAll(pdir, 0755); err != nil {
			t.Fatal(err)
		}
		content := `{
			"name": "same-name",
			"domain": "patent",
			"description": "` + sub + ` wins?",
			"pipeline": {
				"stages": [
					{"id": "s1", "tool": "search", "description": "搜索"}
				]
			}
		}`
		if err := os.WriteFile(filepath.Join(pdir, "plugin.json"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		_ = i
	}

	plugins, err := pluginsys.ScanPlugins([]string{dir}, nil)
	if err != nil {
		t.Fatalf("ScanPlugins failed: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin (first wins), got %d", len(plugins))
	}
}

func TestScanPlugins_SkillPathAutoDetect(t *testing.T) {
	dir := t.TempDir()
	pdir := filepath.Join(dir, "my-plugin")
	if err := os.MkdirAll(pdir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `{
		"name": "auto-skill",
		"domain": "patent",
		"description": "auto skill path",
		"pipeline": {
			"stages": [
				{"id": "s1", "tool": "search", "description": "搜索"}
			]
		}
	}`
	if err := os.WriteFile(filepath.Join(pdir, "plugin.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// Create SKILL.md.
	if err := os.WriteFile(filepath.Join(pdir, "SKILL.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}

	plugins, err := pluginsys.ScanPlugins([]string{dir}, nil)
	if err != nil {
		t.Fatalf("ScanPlugins failed: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].SkillPath != filepath.Join(pdir, "SKILL.md") {
		t.Errorf("expected skill path %s, got %s", filepath.Join(pdir, "SKILL.md"), plugins[0].SkillPath)
	}
}

func TestScanPlugins_CustomSkillPath(t *testing.T) {
	dir := t.TempDir()
	pdir := filepath.Join(dir, "my-plugin")
	if err := os.MkdirAll(pdir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `{
		"name": "custom-skill",
		"domain": "patent",
		"description": "custom skill path",
		"skill_path": "docs/skill.md",
		"pipeline": {
			"stages": [
				{"id": "s1", "tool": "search", "description": "搜索"}
			]
		}
	}`
	if err := os.WriteFile(filepath.Join(pdir, "plugin.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	plugins, err := pluginsys.ScanPlugins([]string{dir}, nil)
	if err != nil {
		t.Fatalf("ScanPlugins failed: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	expected := filepath.Join(pdir, "docs", "skill.md")
	if plugins[0].SkillPath != expected {
		t.Errorf("expected skill path %s, got %s", expected, plugins[0].SkillPath)
	}
}
