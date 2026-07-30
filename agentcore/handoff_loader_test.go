package agentcore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseHandoffRole_ValidXML(t *testing.T) {
	xmlData := []byte(`<handoff_role name="patent-analyst" mode="delegate" invisible="true">
    <description>专利分析子 Agent</description>
    <system_prompt><![CDATA[你是专利分析专家。]]></system_prompt>
    <allowed_sources>
        <source>mady-agent</source>
    </allowed_sources>
    <model>gpt-4o-mini</model>
    <temperature>0.3</temperature>
</handoff_role>`)

	cfg, err := ParseHandoffRole(xmlData)
	if err != nil {
		t.Fatalf("ParseHandoffRole failed: %v", err)
	}

	if cfg.Name != "patent-analyst" {
		t.Errorf("expected name 'patent-analyst', got %q", cfg.Name)
	}
	if cfg.Mode != HandoffDelegate {
		t.Errorf("expected delegate mode, got %s", cfg.Mode)
	}
	if !cfg.Invisible {
		t.Error("expected invisible=true")
	}
	if cfg.AgentConfig.Model != "gpt-4o-mini" {
		t.Errorf("expected model 'gpt-4o-mini', got %q", cfg.AgentConfig.Model)
	}
	if cfg.AgentConfig.Temperature != 0.3 {
		t.Errorf("expected temperature 0.3, got %f", cfg.AgentConfig.Temperature)
	}
	if len(cfg.AllowedSources) != 1 || cfg.AllowedSources[0] != "mady-agent" {
		t.Errorf("expected allowed sources [mady-agent], got %v", cfg.AllowedSources)
	}
}

func TestParseHandoffRole_TransferMode(t *testing.T) {
	xmlData := []byte(`<handoff_role name="legal-advisor" mode="transfer">
    <description>法律咨询</description>
    <system_prompt>你是法律专家。</system_prompt>
</handoff_role>`)

	cfg, err := ParseHandoffRole(xmlData)
	if err != nil {
		t.Fatalf("ParseHandoffRole failed: %v", err)
	}
	if cfg.Mode != HandoffTransfer {
		t.Errorf("expected transfer mode, got %s", cfg.Mode)
	}
}

func TestParseHandoffRole_MissingName(t *testing.T) {
	xmlData := []byte(`<handoff_role mode="delegate">
    <description>Missing name</description>
</handoff_role>`)

	_, err := ParseHandoffRole(xmlData)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required', got: %v", err)
	}
}

func TestParseHandoffRole_InvalidXML(t *testing.T) {
	_, err := ParseHandoffRole([]byte(`<handoff_role><unclosed>`))
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

func TestParseHandoffRole_EmptyDescription(t *testing.T) {
	xmlData := []byte(`<handoff_role name="test">
    <description></description>
</handoff_role>`)

	_, err := ParseHandoffRole(xmlData)
	if err == nil {
		t.Fatal("expected error for empty description")
	}
}

func TestHandoffRoleStore_LoadAndGet(t *testing.T) {
	dir := t.TempDir()

	// Create a role XML file
	writeXMLFile(t, filepath.Join(dir, "patent.xml"),
		`<handoff_role name="patent" mode="delegate" invisible="true">
    <description>专利代理</description>
    <system_prompt>你是专利代理专家。</system_prompt>
    <allowed_sources><source>mady-agent</source></allowed_sources>
</handoff_role>`)

	store := NewHandoffRoleStore(dir)
	if err := store.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cfg, ok := store.Get("patent")
	if !ok {
		t.Fatal("expected to find 'patent' role")
	}
	if cfg.Name != "patent" {
		t.Errorf("expected name 'patent', got %q", cfg.Name)
	}
}

func TestHandoffRoleStore_LoadMissingDir(t *testing.T) {
	store := NewHandoffRoleStore("/nonexistent/path")
	if err := store.Load(); err != nil {
		t.Fatalf("Load should not error on missing dir: %v", err)
	}
}

func TestHandoffRoleStore_MergeWithCode(t *testing.T) {
	dir := t.TempDir()

	writeXMLFile(t, filepath.Join(dir, "patent.xml"),
		`<handoff_role name="patent" mode="delegate">
    <description>XML Patent Role</description>
</handoff_role>`)

	store := NewHandoffRoleStore(dir)
	if err := store.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	codeConfigs := []HandoffConfig{
		{Name: "patent", Description: "Code Patent Role"},
		{Name: "legal", Description: "Code Legal Role"},
	}

	merged := store.MergeWithCode(codeConfigs)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged configs, got %d", len(merged))
	}

	// Code version should take priority for "patent"
	for _, cfg := range merged {
		if cfg.Name == "patent" && cfg.Description != "Code Patent Role" {
			t.Errorf("expected code patent role to win, got description: %q", cfg.Description)
		}
	}
}

func TestHandoffRoleStore_DefaultTemperature(t *testing.T) {
	dir := t.TempDir()

	writeXMLFile(t, filepath.Join(dir, "test.xml"),
		`<handoff_role name="test" mode="delegate">
    <description>Test</description>
</handoff_role>`)

	store := NewHandoffRoleStore(dir)
	if err := store.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	cfg, ok := store.Get("test")
	if !ok {
		t.Fatal("expected to find 'test' role")
	}
	if cfg.AgentConfig.Temperature != 0.3 {
		t.Errorf("expected default temperature 0.3, got %f", cfg.AgentConfig.Temperature)
	}
}

func writeXMLFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
