package workflows

import (
	"testing"
)

// ---------------------------------------------------------------------------
// TemplateRegistry tests
// ---------------------------------------------------------------------------

func TestDefaultRegistry_NotEmpty(t *testing.T) {
	names := defaultRegistry.List()
	if len(names) == 0 {
		t.Fatal("expected default registry to have templates")
	}
}

func TestDefaultRegistry_ContainsBuiltin(t *testing.T) {
	builtins := []string{
		"search_analyze_draft",
		"novelty_check",
		"creativity_check",
		"infringement_analysis",
		"invalidity_analysis",
		"full_review",
		"legal_analysis",
	}
	for _, name := range builtins {
		tmpl, ok := defaultRegistry.Get(name)
		if !ok {
			t.Errorf("builtin template %q not found", name)
			continue
		}
		if tmpl.Name != name {
			t.Errorf("expected name %q, got %q", name, tmpl.Name)
		}
		if len(tmpl.Steps) == 0 {
			t.Errorf("template %q has no steps", name)
		}
	}
}

func TestDefaultRegistry_Get_NotFound(t *testing.T) {
	_, ok := defaultRegistry.Get("nonexistent_template")
	if ok {
		t.Fatal("expected false for nonexistent template")
	}
}

func TestDefaultRegistry_Register(t *testing.T) {
	r := NewTemplateRegistry()
	tmpl := WorkflowTemplate{
		Name:        "custom",
		Description: "Custom test template",
		Steps: []TemplateStep{
			{ID: "step1", Type: StepAgent, Role: "tester"},
		},
	}
	r.Register(tmpl)
	got, ok := r.Get("custom")
	if !ok {
		t.Fatal("expected to find registered template")
	}
	if got.Name != "custom" {
		t.Errorf("expected name 'custom', got %q", got.Name)
	}
}

// ---------------------------------------------------------------------------
// DefaultRegistry convenience functions
// ---------------------------------------------------------------------------

func TestDefaultRegistry_Convenience(t *testing.T) {
	_ = DefaultRegistry() // must not panic
	_, _ = LookupTemplate("novelty_check")
	list := ListTemplates()
	if list == "" || list == "(无模板)" {
		t.Fatalf("expected non-empty template list, got %q", list)
	}
}

// ---------------------------------------------------------------------------
// GetOrchestrationManifest tests
// ---------------------------------------------------------------------------

func TestGetOrchestrationManifest(t *testing.T) {
	tests := []struct {
		name    string
		wantKey string
		wantOK  bool
	}{
		{"oa_response", "oa_response", true},
		{"novelty_check", "novelty_check", true},
		{"infringement_analysis", "infringement_analysis", true},
		{"nonexistent", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, ok := GetOrchestrationManifest(tt.name)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// YAML template loading
// ---------------------------------------------------------------------------

func TestNewTemplateRegistry_LoadYAML(t *testing.T) {
	yamlData := []byte(`
templates:
  - name: test_from_yaml
    description: A YAML-loaded template
    domain: patent
    steps:
      - id: search
        type: agent
        role: retriever
      - id: draft
        type: agent
        role: writer
        depends_on: ["search"]
`)
	r := NewTemplateRegistry()
	if err := r.LoadYAML(yamlData); err != nil {
		t.Fatalf("LoadYAML error: %v", err)
	}
	tmpl, ok := r.Get("test_from_yaml")
	if !ok {
		t.Fatal("template not found after LoadYAML")
	}
	if len(tmpl.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(tmpl.Steps))
	}
	if tmpl.Steps[0].ID != "search" {
		t.Errorf("expected step 0 ID 'search', got %q", tmpl.Steps[0].ID)
	}
	if len(tmpl.Steps[1].DependsOn) != 1 || tmpl.Steps[1].DependsOn[0] != 0 {
		t.Errorf("dependency should resolve to index 0, got %v", tmpl.Steps[1].DependsOn)
	}
}

func TestNewTemplateRegistry_LoadYAML_Invalid(t *testing.T) {
	r := NewTemplateRegistry()
	err := r.LoadYAML([]byte(`invalid yaml: [`))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
