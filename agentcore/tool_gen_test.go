package agentcore

import (
	"context"
	"encoding/json"
	"testing"
)

// --- Test types ---

type simpleArgs struct {
	Name  string `json:"name" jsonschema:"required,description=Your name"`
	Age   int    `json:"age,omitempty" jsonschema:"description=Your age"`
	Debug bool   `json:"debug,omitempty"`
}

type simpleResult struct {
	Greeting string `json:"greeting"`
}

type nestedArgs struct {
	Query  string       `json:"query" jsonschema:"required"`
	Config *searchConfig `json:"config,omitempty"`
}

type searchConfig struct {
	MaxResults int      `json:"max_results,omitempty"`
	Sources    []string `json:"sources,omitempty"`
}

type searchResult struct {
	Count int `json:"count"`
}

func TestNewTypedTool_BasicHandler(t *testing.T) {
	tool := NewTypedTool("greet", "Returns a greeting", func(ctx context.Context, args simpleArgs) (simpleResult, error) {
		return simpleResult{Greeting: "Hello, " + args.Name}, nil
	})

	if tool.Name != "greet" {
		t.Fatalf("expected name greet, got %s", tool.Name)
	}
	if tool.Func == nil {
		t.Fatal("expected Func to be set")
	}

	// Test invocation via Func.
	raw, _ := json.Marshal(map[string]any{"name": "World", "age": 30})
	result, err := tool.Func(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Result should be JSON-serialized simpleResult.
	var sr simpleResult
	if err := json.Unmarshal([]byte(result.(string)), &sr); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if sr.Greeting != "Hello, World" {
		t.Fatalf("expected 'Hello, World', got %q", sr.Greeting)
	}
}

func TestNewTypedTool_SchemaGeneration(t *testing.T) {
	tool := NewTypedTool("greet", "Returns a greeting", func(ctx context.Context, args simpleArgs) (simpleResult, error) {
		return simpleResult{}, nil
	})

	// Runtime Parameters should be lenient (no required fields).
	def := tool.Definition()
	if def.Parameters == nil {
		t.Fatal("expected Parameters to be set")
	}

	// Check that the schema has type object with properties.
	if def.Parameters["type"] != "object" {
		t.Fatalf("expected type=object, got %v", def.Parameters["type"])
	}

	props, ok := def.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	// Declaration schema should have required fields.
	if tool.declarationParams == nil {
		t.Fatal("expected declarationParams to be set for typed tool")
	}
	if _, hasName := props["name"]; !hasName {
		t.Fatal("expected 'name' property")
	}
	if _, hasAge := props["age"]; !hasAge {
		t.Fatal("expected 'age' property")
	}

	// Verify declaration schema has "required": ["name"].
	declDef := ToolDefinition{
		Name:        tool.Name,
		Description: tool.Description,
		Parameters:  tool.declarationParams,
	}
	required, _ := declDef.Parameters["required"].([]any)
	if len(required) != 1 || required[0] != "name" {
		t.Fatalf("expected declaration required=[name], got %v", required)
	}
}

func TestNewTypedTool_TypeCoercion(t *testing.T) {
	tool := NewTypedTool("greet", "Returns a greeting", func(ctx context.Context, args simpleArgs) (simpleResult, error) {
		return simpleResult{Greeting: "OK"}, nil
	})

	// LLM sends age as string "30" and debug as string "true".
	raw, _ := json.Marshal(map[string]any{
		"name":  "Test",
		"age":   "30",
		"debug": "true",
	})
	_, err := tool.Func(context.Background(), raw)
	if err != nil {
		t.Fatalf("type coercion failed: %v", err)
	}
}

func TestNewTypedTool_ParameterAliases(t *testing.T) {
	tool := NewTypedTool("greet", "Returns a greeting",
		func(ctx context.Context, args simpleArgs) (simpleResult, error) {
			if args.Age != 25 {
				t.Fatalf("expected age=25, got %d", args.Age)
			}
			return simpleResult{Greeting: "OK"}, nil
		},
		map[string]string{"path": "name"}, // alias: path → name
	)

	// LLM uses "path" instead of "name".
	raw, _ := json.Marshal(map[string]any{
		"path": "Test",
		"age":  25,
	})
	_, err := tool.Func(context.Background(), raw)
	if err != nil {
		t.Fatalf("alias resolution failed: %v", err)
	}
}

func TestNewTypedTool_NestedStruct(t *testing.T) {
	tool := NewTypedTool("search", "Search", func(ctx context.Context, args nestedArgs) (searchResult, error) {
		if args.Config != nil {
			return searchResult{Count: args.Config.MaxResults}, nil
		}
		return searchResult{Count: 10}, nil
	})

	raw, _ := json.Marshal(map[string]any{
		"query": "test",
		"config": map[string]any{
			"max_results": "20",  // string coercion
			"sources":     `["a","b"]`, // JSON string coercion
		},
	})
	_, err := tool.Func(context.Background(), raw)
	if err != nil {
		t.Fatalf("nested coercion failed: %v", err)
	}
}

func TestNewTypedTool_ExtraProperties(t *testing.T) {
	tool := NewTypedTool("greet", "Returns a greeting", func(ctx context.Context, args simpleArgs) (simpleResult, error) {
		return simpleResult{Greeting: "OK"}, nil
	})

	// LLM sends extra unknown property "foo".
	raw, _ := json.Marshal(map[string]any{
		"name": "Test",
		"foo":  "bar",
	})
	_, err := tool.Func(context.Background(), raw)
	if err != nil {
		t.Fatalf("extra properties should be tolerated: %v", err)
	}
}

func TestNewTypedTool_SchemaCaching(t *testing.T) {
	// Creating two tools with the same TArgs should reuse cached schema.
	tool1 := NewTypedTool("tool1", "desc1", func(ctx context.Context, args simpleArgs) (simpleResult, error) {
		return simpleResult{}, nil
	})
	tool2 := NewTypedTool("tool2", "desc2", func(ctx context.Context, args simpleArgs) (simpleResult, error) {
		return simpleResult{}, nil
	})

	// Both should have identical declaration params.
	if len(tool1.declarationParams) == 0 || len(tool2.declarationParams) == 0 {
		t.Fatal("both tools should have declaration params")
	}

	// Check they're the same by serializing.
	d1, _ := json.Marshal(tool1.declarationParams)
	d2, _ := json.Marshal(tool2.declarationParams)
	if string(d1) != string(d2) {
		t.Fatal("cached schemas should be identical")
	}
}

func TestNewTypedTool_BackwardCompatibility(t *testing.T) {
	// Legacy tools (built without NewTypedTool) should still work.
	legacy := &Tool{
		Name:        "legacy",
		Description: "A legacy tool",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
			"required": []any{"text"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return "legacy result", nil
		},
	}

	def := legacy.Definition()
	if def.Parameters["required"] == nil {
		t.Fatal("legacy tool should preserve required fields in Definition()")
	}

	result, err := legacy.Func(context.Background(), json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("legacy tool invocation failed: %v", err)
	}
	if result != "legacy result" {
		t.Fatalf("unexpected legacy result: %v", result)
	}
}

func TestSchemaGeneration_DescriptionTags(t *testing.T) {
	type argsWithDesc struct {
		Path string `json:"path" jsonschema:"required,description=The file path to read"`
	}

	schema := generateSchema[argsWithDesc](false)
	props := schema.Properties
	if props["path"].Description != "The file path to read" {
		t.Fatalf("expected description 'The file path to read', got %q", props["path"].Description)
	}
}

func TestSchemaGeneration_OmitEmptyMarksOptional(t *testing.T) {
	type argsWithOptional struct {
		Required string `json:"required" jsonschema:"required"`
		Optional string `json:"optional,omitempty"`
	}

	schema := generateSchema[argsWithOptional](false)
	if len(schema.Required) != 1 || schema.Required[0] != "required" {
		t.Fatalf("expected required=[required], got %v", schema.Required)
	}
}

func TestCoerceMap_NilSafety(t *testing.T) {
	ci := &coerceInfo{
		intKeys:  map[string]bool{"count": true},
		boolKeys: map[string]bool{},
		jsonKeys: map[string]bool{},
	}

	// nil map should not panic.
	coerceMap(nil, ci)

	// Empty map should not panic.
	coerceMap(map[string]any{}, ci)
}

func TestPatchArgs_EmptyInput(t *testing.T) {
	result, err := patchArgs(json.RawMessage{}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %s", string(result))
	}
}

func TestPatchArgs_NonObjectInput(t *testing.T) {
	// Array input should return as-is.
	result, err := patchArgs(json.RawMessage(`[1,2,3]`), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != `[1,2,3]` {
		t.Fatalf("expected array passthrough, got %s", string(result))
	}
}
