package doctmpl

import (
	"strings"
	"testing"
)

func TestVarSchema_Validate_AllRequired(t *testing.T) {
	schema := NewVarSchema([]VarDefinition{
		{Name: "a", Type: VarString, Required: true},
		{Name: "b", Type: VarString, Required: true},
	})
	errs := schema.Validate(map[string]string{"a": "1", "b": "2"})
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %v", errs)
	}
}

func TestVarSchema_Validate_MissingRequired(t *testing.T) {
	schema := NewVarSchema([]VarDefinition{
		{Name: "required", Type: VarString, Required: true},
		{Name: "optional", Type: VarString, Required: false},
	})
	errs := schema.Validate(map[string]string{"optional": "x"})
	if len(errs) == 0 {
		t.Fatal("expected error for missing required")
	}
	if errs[0].Code != "missing_required" {
		t.Errorf("code = %q", errs[0].Code)
	}
}

func TestVarSchema_Validate_NumberType(t *testing.T) {
	schema := NewVarSchema([]VarDefinition{
		{Name: "count", Type: VarNumber, Required: true},
	})
	errs := schema.Validate(map[string]string{"count": "not-a-number"})
	if len(errs) == 0 {
		t.Fatal("expected type error")
	}
	if errs[0].Code != "invalid_type" {
		t.Errorf("code = %q", errs[0].Code)
	}
	errs = schema.Validate(map[string]string{"count": "3.14"})
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %v", errs)
	}
}

func TestVarSchema_Validate_BoolType(t *testing.T) {
	schema := NewVarSchema([]VarDefinition{
		{Name: "enabled", Type: VarBool, Required: true},
	})
	errs := schema.Validate(map[string]string{"enabled": "yes"})
	if len(errs) == 0 {
		t.Fatal("expected type error")
	}
	errs = schema.Validate(map[string]string{"enabled": "true"})
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %v", errs)
	}
}

func TestVarSchema_ApplyDefaults(t *testing.T) {
	schema := NewVarSchema([]VarDefinition{
		{Name: "a", Type: VarString, Required: true},
		{Name: "b", Type: VarString, Required: false, Default: "default-b"},
	})
	vars := map[string]string{"a": "value-a"}
	filled := schema.ApplyDefaults(vars)
	if filled["a"] != "value-a" {
		t.Errorf("a = %q", filled["a"])
	}
	if filled["b"] != "default-b" {
		t.Errorf("b = %q", filled["b"])
	}
}

func TestVarSchema_ApplyDefaults_NoOverride(t *testing.T) {
	schema := NewVarSchema([]VarDefinition{
		{Name: "a", Type: VarString, Default: "default"},
	})
	filled := schema.ApplyDefaults(map[string]string{"a": "custom"})
	if filled["a"] != "custom" {
		t.Errorf("default overrode custom: %q", filled["a"])
	}
}

func TestVarSchema_Empty(t *testing.T) {
	var s *VarSchema
	if !s.Empty() {
		t.Error("nil should be empty")
	}
	s = NewVarSchema(nil)
	if !s.Empty() {
		t.Error("nil defs should be empty")
	}
	s = NewVarSchema([]VarDefinition{})
	if !s.Empty() {
		t.Error("empty defs should be empty")
	}
}

func TestVarSchema_RequiredNames(t *testing.T) {
	schema := NewVarSchema([]VarDefinition{
		{Name: "a", Required: true},
		{Name: "b", Required: false},
		{Name: "c", Required: true},
	})
	req := schema.RequiredNames()
	if len(req) != 2 {
		t.Fatalf("len = %d", len(req))
	}
}

func TestExtractPlaceholders(t *testing.T) {
	body := "{{a}} and {{b}} again {{a}}"
	names := ExtractPlaceholders(body)
	if len(names) != 2 {
		t.Fatalf("len = %d: %v", len(names), names)
	}
	if names[0] != "a" || names[1] != "b" {
		t.Errorf("names = %v", names)
	}
}

func TestExtractPlaceholders_None(t *testing.T) {
	names := ExtractPlaceholders("no placeholders here")
	if len(names) != 0 {
		t.Fatalf("len = %d", len(names))
	}
}

func TestValidatedResolve_AllPass(t *testing.T) {
	tmpl := DocTemplate{
		Name: "test",
		Body: "# {{title}}\n\n发明：{{invention}}",
		VarSchema: NewVarSchema([]VarDefinition{
			{Name: "title", Type: VarString, Required: true},
			{Name: "invention", Type: VarString, Required: true},
		}),
	}
	result := ValidatedResolve(tmpl, map[string]string{
		"title": "测试", "invention": "一种方法",
	})
	if len(result.Warnings) != 0 {
		t.Errorf("warnings = %v", result.Warnings)
	}
	if len(result.Residual) != 0 {
		t.Errorf("residual = %v", result.Residual)
	}
}

func TestValidatedResolve_Residual(t *testing.T) {
	tmpl := DocTemplate{
		Name: "test",
		Body: "# {{title}}\n\n作者：{{author}}",
	}
	result := ValidatedResolve(tmpl, map[string]string{"title": "测试"})
	if len(result.Residual) != 1 || result.Residual[0] != "author" {
		t.Errorf("residual = %v", result.Residual)
	}
	if !strings.Contains(result.Output, "{{author}}") {
		t.Error("unresolved should remain in output")
	}
}

func TestValidatedResolve_DefaultApplied(t *testing.T) {
	tmpl := DocTemplate{
		Name: "test",
		Body: "代理人：{{agent_name}}",
		VarSchema: NewVarSchema([]VarDefinition{
			{Name: "agent_name", Type: VarString, Default: "专利代理人"},
		}),
	}
	result := ValidatedResolve(tmpl, map[string]string{})
	if !strings.Contains(result.Output, "专利代理人") {
		t.Errorf("default not applied: %q", result.Output)
	}
}

func TestVarError_Error(t *testing.T) {
	e := VarError{Variable: "x", Code: "missing_required", Message: "必填"}
	if !strings.Contains(e.Error(), "x") || !strings.Contains(e.Error(), "必填") {
		t.Errorf("Error() = %q", e.Error())
	}
}
