package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
)

// TypedToolFunc is the typed variant of ToolFunc. The function receives
// deserialized TArgs and returns TResults (or error). The agentcore
// framework handles JSON serialization/deserialization automatically.
type TypedToolFunc[TArgs, TResults any] func(ctx context.Context, args TArgs) (TResults, error)

// NewTypedTool creates a Tool from a typed handler function.
//
// Unlike the raw Tool{Func: ToolFunc} path, this constructor:
//  1. Auto-generates JSON Schema from the TArgs struct (via json tags + reflect)
//  2. Produces two schemas:
//     - declaration schema: strict, with required fields — shown to the LLM
//     - runtime schema: lenient, allows extra properties — prevents LLM errors
//  3. Wraps the handler with automatic type coercion (string→int, string→bool, etc.)
//  4. Supports parameter aliases for common LLM naming mistakes
//
// Example:
//
//	type ReadArgs struct {
//	    FilePath string `json:"file_path" jsonschema:"required,description=Path to the file"`
//	    Offset   int    `json:"offset,omitempty" jsonschema:"description=Line offset to start reading from"`
//	}
//	type ReadResult struct {
//	    Content string `json:"content"`
//	}
//
//	tool := NewTypedTool("read_file", "Read a file", func(ctx context.Context, args ReadArgs) (ReadResult, error) {
//	    data, err := os.ReadFile(args.FilePath)
//	    if err != nil {
//	        return ReadResult{}, err
//	    }
//	    return ReadResult{Content: string(data)}, nil
//	})
func NewTypedTool[TArgs, TResults any](
	name, description string,
	fn TypedToolFunc[TArgs, TResults],
	aliases ...map[string]string,
) *Tool {
	// Generate schemas from the TArgs struct.
	declSchema := generateSchema[TArgs](false)   // strict: required fields preserved
	runtimeSchema := generateSchema[TArgs](true) // lenient: no required, extra props allowed

	// Collect properties needing type coercion.
	coercer := collectCoerceInfo[TArgs]()

	// Merge parameter aliases.
	var mergedAliases map[string]string
	for _, a := range aliases {
		if mergedAliases == nil {
			mergedAliases = make(map[string]string, len(a))
		}
		for from, to := range a {
			mergedAliases[from] = to
		}
	}

	// Build the ToolFunc wrapper.
	toolFunc := func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		// Resolve aliases and coerce types in the raw JSON.
		patched, err := patchArgs(rawArgs, mergedAliases, coercer)
		if err != nil {
			return nil, fmt.Errorf("参数处理失败 %s: %w", name, err)
		}

		// Deserialize.
		var args TArgs
		if err := json.Unmarshal(patched, &args); err != nil {
			return nil, fmt.Errorf("参数解析失败 %s: %w", name, err)
		}

		// Call the typed handler.
		result, err := fn(ctx, args)
		if err != nil {
			return nil, err
		}

		// If result is already a *DualToolOutput or DualToolOutput (value type),
		// return as-is to preserve ForLLM/ForUser/Terminate/Silent semantics.
		switch r := any(result).(type) {
		case *DualToolOutput:
			return r, nil
		case DualToolOutput:
			return &r, nil
		}
		if s, ok := any(result).(string); ok {
			return s, nil
		}

		// Otherwise serialize to JSON.
		data, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("结果序列化失败 %s: %w", name, err)
		}
		return string(data), nil
	}

	// Merge schemas: Parameters holds the runtime (lenient) schema;
	// we store the declaration schema separately for Definition().
	params := schemaToMap(runtimeSchema)

	return &Tool{
		Name:        name,
		Description: description,
		Parameters:  params,
		Func:        toolFunc,
		// Store declaration schema for use in Definition() override.
		declarationParams: schemaToMap(declSchema),
	}
}

// --- Hook into existing Tool system ---
//
// declarationParams is defined on Tool in tool.go. It carries the strict
// LLM-facing schema for typed tools created via NewTypedTool.
