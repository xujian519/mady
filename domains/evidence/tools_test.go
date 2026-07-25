package evidence

import (
	"context"
	"testing"
)

func TestEvidenceDomainExtension_Name(t *testing.T) {
	ext := NewDomainExtension(nil)
	if ext.Name() != ExtensionNameDomain {
		t.Errorf("expected %q, got %q", ExtensionNameDomain, ext.Name())
	}
}

func TestEvidenceDomainExtension_InitDispose(t *testing.T) {
	ext := NewDomainExtension(nil)
	if err := ext.Init(context.Background(), nil); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := ext.Dispose(); err != nil {
		t.Fatalf("Dispose: %v", err)
	}
}

func TestEvidenceDomainExtension_Tools(t *testing.T) {
	ext := NewDomainExtension(nil)
	tools := ext.Tools()
	if len(tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"judge_triple", "check_burden", "assess_standard", "detect_conflict", "judge_type_specific"} {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}
