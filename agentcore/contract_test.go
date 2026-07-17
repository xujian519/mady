package agentcore

import (
	"testing"
)

// TestCanonicalizeSchema verifies that schema normalization produces
// stable output independent of key order and empty-value presence.
func TestCanonicalizeSchema(t *testing.T) {
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The file path",
				"":            nil,
			},
			"content": map[string]any{
				"type": "string",
			},
		},
		"required":             []any{"path"},
		"additionalProperties": false,
	}

	out := CanonicalizeSchema(input)

	// Nil and empty values should be stripped.
	if props, ok := out["properties"].(map[string]any); ok {
		if path, ok := props["path"].(map[string]any); ok {
			if _, exists := path[""]; exists {
				t.Error("empty key should be stripped")
			}
		}
	}

	// additionalProperties: false should be stripped (empty value).
	if _, exists := out["additionalProperties"]; exists {
		t.Error("false boolean should be stripped as empty value")
	}

	// required should remain.
	if _, exists := out["required"]; !exists {
		t.Error("required array should remain")
	}
}

// TestSchemaDigest verifies that SchemaDigest is deterministic.
func TestSchemaDigest(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"path": map[string]any{"type": "string"}},
		"required":   []any{"path"},
	}

	d1 := SchemaDigest(schema)
	d2 := SchemaDigest(schema)

	if d1 != d2 {
		t.Errorf("SchemaDigest not deterministic: %s != %s", d1, d2)
	}
	if len(d1) != 64 {
		t.Errorf("SchemaDigest length: expected 64, got %d", len(d1))
	}
}

// TestSchemaDigestDifferentInput verifies that different schemas produce different digests.
func TestSchemaDigestDifferentInput(t *testing.T) {
	s1 := map[string]any{
		"type":       "object",
		"properties": map[string]any{"a": map[string]any{"type": "string"}},
	}
	s2 := map[string]any{
		"type":       "object",
		"properties": map[string]any{"b": map[string]any{"type": "number"}},
	}

	if SchemaDigest(s1) == SchemaDigest(s2) {
		t.Error("different schemas should produce different digests")
	}
}

// TestClassifyTool verifies tool classification logic.
func TestClassifyTool(t *testing.T) {
	tests := []struct {
		name     string
		readOnly bool
		want     string
	}{
		{"read", true, "read"},
		{"write_file", false, "write"},
		{"edit", false, "write"},
		{"delete", false, "write"},
		{"bash", false, "command"},
		{"web_search", true, "network"},
		{"web_fetch", true, "network"},
		{"unknown_tool", false, "other"},
	}
	for _, tt := range tests {
		got := ClassifyTool(tt.name, tt.readOnly)
		if got != tt.want {
			t.Errorf("ClassifyTool(%q, %v): got %q, want %q", tt.name, tt.readOnly, got, tt.want)
		}
	}
}
