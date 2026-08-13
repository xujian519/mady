package piagent

import (
	"strings"
	"testing"
)

func TestSchemaFromMap_RoundTrip(t *testing.T) {
	raw := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "glob 模式"},
			"limit":   map[string]any{"type": "integer", "minimum": float64(1)},
			"mode":    map[string]any{"type": "string", "enum": []any{"fast", "full"}},
		},
		"required": []any{"pattern"},
	}
	s, err := schemaFromMap(raw)
	if err != nil {
		t.Fatalf("schemaFromMap: %v", err)
	}
	if s == nil {
		t.Fatal("nil schema")
	}
	if s.Type != "object" {
		t.Errorf("type = %q, want object", s.Type)
	}
	if len(s.Properties) != 3 {
		t.Fatalf("properties = %d, want 3", len(s.Properties))
	}
	if len(s.Required) != 1 || s.Required[0] != "pattern" {
		t.Errorf("required = %v, want [pattern]", s.Required)
	}
	limit := s.Properties["limit"]
	if limit == nil || limit.Minimum == nil || *limit.Minimum != 1 {
		t.Errorf("limit.Minimum = %v, want 1", limit)
	}
	mode := s.Properties["mode"]
	if mode == nil || len(mode.Enum) != 2 {
		t.Errorf("mode.Enum = %v, want 2 items", mode)
	}
}

func TestSchemaFromMap_UnknownKeywordPassthrough(t *testing.T) {
	raw := map[string]any{
		"type":   "object",
		"x-mady": "custom-metadata",
	}
	s, err := schemaFromMap(raw)
	if err != nil {
		t.Fatalf("schemaFromMap: %v", err)
	}
	if s.Extra == nil || s.Extra["x-mady"] != "custom-metadata" {
		t.Errorf("Extra = %v, want x-mady passthrough", s.Extra)
	}
}

func TestSchemaFromMap_RejectsRef(t *testing.T) {
	raw := map[string]any{"$ref": "#/definitions/Foo"}
	_, err := schemaFromMap(raw)
	if err == nil || !strings.Contains(err.Error(), "$ref") {
		t.Fatalf("want $ref rejection, got %v", err)
	}
}

func TestSchemaFromMap_NestedItems(t *testing.T) {
	raw := map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "string"},
	}
	s, err := schemaFromMap(raw)
	if err != nil {
		t.Fatalf("schemaFromMap: %v", err)
	}
	if s.Type != "array" || s.Items == nil || s.Items.Type != "string" {
		t.Errorf("array/items mismatch: %+v", s)
	}
}
