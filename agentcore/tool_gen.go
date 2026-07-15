package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
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
	declSchema := generateSchema[TArgs](false) // strict: required fields preserved
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

// --- Schema generation via reflect (no external dependency) ---

// jsonSchema represents a minimal JSON Schema subset.
type jsonSchema struct {
	Type                 string                `json:"type,omitempty"`
	Description          string                `json:"description,omitempty"`
	Properties           map[string]*jsonSchema `json:"properties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	AdditionalProperties *jsonSchema           `json:"additionalProperties,omitempty"`
	Items                *jsonSchema           `json:"items,omitempty"`
	Enum                 []any                 `json:"enum,omitempty"`
}

// schemaCache avoids re-generating schemas for the same type.
// Protected by schemaCacheMu for concurrent NewTypedTool calls.
var schemaCache = map[reflect.Type]*jsonSchema{}
var schemaCacheMu sync.Mutex

func generateSchema[T any](lenient bool) *jsonSchema {
	t := reflect.TypeOf((*T)(nil)).Elem()
	return generateSchemaForType(t, lenient)
}

// derefType returns the element type if t is a pointer, otherwise t itself.
func derefType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr {
		return t.Elem()
	}
	return t
}

func generateSchemaForType(t reflect.Type, lenient bool) *jsonSchema {
	t = derefType(t)

	// Check cache (only for named types, not slices/maps).
	// Double-checked locking: first read under lock, then write under lock.
	if t.Kind() == reflect.Struct {
		schemaCacheMu.Lock()
		cached, ok := schemaCache[t]
		schemaCacheMu.Unlock()
		if ok {
			s := cloneSchema(cached)
			if lenient {
				relaxSchemaLocal(s)
			}
			return s
		}
	}

	schema := typeToSchema(t, lenient)

	if t.Kind() == reflect.Struct {
		schemaCacheMu.Lock()
		// Double-check: another goroutine may have cached it while we were generating.
		if _, exists := schemaCache[t]; !exists {
			schemaCache[t] = cloneSchema(schema)
		}
		schemaCacheMu.Unlock()
	}

	return schema
}

func typeToSchema(t reflect.Type, lenient bool) *jsonSchema {
	switch t.Kind() {
	case reflect.String:
		return &jsonSchema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return &jsonSchema{Type: "number"}
	case reflect.Bool:
		return &jsonSchema{Type: "boolean"}
	case reflect.Slice, reflect.Array:
		elemType := t.Elem()
		if elemType.Kind() == reflect.Uint8 {
			return &jsonSchema{Type: "string"} // []byte → string
		}
		return &jsonSchema{
			Type:  "array",
			Items: generateSchemaForType(elemType, lenient),
		}
	case reflect.Map:
		valSchema := generateSchemaForType(t.Elem(), lenient)
		return &jsonSchema{
			Type: "object",
			AdditionalProperties: valSchema,
		}
	case reflect.Struct:
		s := &jsonSchema{
			Type:       "object",
			Properties: make(map[string]*jsonSchema),
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name, required, desc := parseJSONTag(f)
			if name == "-" {
				continue
			}

			propSchema := generateSchemaForType(f.Type, lenient)
			if desc != "" {
				propSchema.Description = desc
			}
			s.Properties[name] = propSchema
			if required && !lenient {
				s.Required = append(s.Required, name)
			}
		}
		if !lenient {
			s.AdditionalProperties = &jsonSchema{} // allow extra
		}
		return s
	case reflect.Interface:
		return &jsonSchema{} // open schema
	default:
		return &jsonSchema{Type: "string"}
	}
}

// parseJSONTag extracts JSON field name, required flag, and description from struct tags.
// Supports two tag styles:
//
//	`json:"file_path" jsonschema:"required,description=Path to the file"`
//	`json:"file_path,omitempty"` // omitempty → not required
func parseJSONTag(f reflect.StructField) (name string, required bool, description string) {
	jsonTag := f.Tag.Get("json")
	parts := strings.Split(jsonTag, ",")
	name = parts[0]
	if name == "" {
		name = f.Name // fallback to Go field name
	}

	required = true
	for i := 1; i < len(parts); i++ {
		switch strings.TrimSpace(parts[i]) {
		case "omitempty":
			required = false
		case "-":
			name = "-"
			return
		}
	}

	// Parse jsonschema tag.
	schemaTag := f.Tag.Get("jsonschema")
	for _, part := range strings.Split(schemaTag, ",") {
		part = strings.TrimSpace(part)
		if part == "required" {
			required = true
		} else if strings.HasPrefix(part, "description=") {
			description = strings.TrimPrefix(part, "description=")
		}
	}

	return
}

func relaxSchemaLocal(s *jsonSchema) {
	if s == nil {
		return
	}
	if s.Type == "object" || len(s.Properties) > 0 {
		s.Required = nil
	}
	for _, prop := range s.Properties {
		relaxSchemaLocal(prop)
	}
	if s.Items != nil {
		relaxSchemaLocal(s.Items)
	}
	if s.AdditionalProperties != nil {
		relaxSchemaLocal(s.AdditionalProperties)
	}
}

func cloneSchema(s *jsonSchema) *jsonSchema {
	if s == nil {
		return nil
	}
	data, err := json.Marshal(s)
	if err != nil {
		return &jsonSchema{}
	}
	var cloned jsonSchema
	if err := json.Unmarshal(data, &cloned); err != nil {
		return &jsonSchema{}
	}
	return &cloned
}

func schemaToMap(s *jsonSchema) map[string]any {
	if s == nil {
		return map[string]any{"type": "object"}
	}
	data, err := json.Marshal(s)
	if err != nil {
		return map[string]any{"type": "object"}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]any{"type": "object"}
	}
	return m
}

// --- Type coercion ---

// coerceInfo records which parameter paths need type coercion.
type coerceInfo struct {
	intKeys  map[string]bool // keys whose values should be coerced to number
	boolKeys map[string]bool // keys whose values should be coerced to boolean
	jsonKeys map[string]bool // keys whose values should be parsed from JSON strings
}

func collectCoerceInfo[T any]() *coerceInfo {
	t := reflect.TypeOf((*T)(nil)).Elem()
	ci := &coerceInfo{
		intKeys:  make(map[string]bool),
		boolKeys: make(map[string]bool),
		jsonKeys: make(map[string]bool),
	}
	collectCoerceFromType(t, "", ci)
	return ci
}

func collectCoerceFromType(t reflect.Type, prefix string, ci *coerceInfo) {
	t = derefType(t)
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := parseJSONTag(f)
		if name == "-" {
			continue
		}
		fullName := name
		if prefix != "" {
			fullName = prefix + "." + name
		}
		resolveCoerceKey(f.Type, fullName, ci)
	}
}

func resolveCoerceKey(t reflect.Type, key string, ci *coerceInfo) {
	t = derefType(t)
	// Extract the base name (last component of dot-separated path).
	base := key
	if idx := strings.LastIndex(key, "."); idx >= 0 {
		base = key[idx+1:]
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		ci.intKeys[key] = true
		if base != key {
			ci.intKeys[base] = true
		}
	case reflect.Bool:
		ci.boolKeys[key] = true
		if base != key {
			ci.boolKeys[base] = true
		}
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
		ci.jsonKeys[key] = true
		if base != key {
			ci.jsonKeys[base] = true
		}
		if t.Kind() == reflect.Struct {
			collectCoerceFromType(t, key, ci)
		} else if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Struct {
			collectCoerceFromType(t.Elem(), key, ci)
		}
	}
}

// patchArgs resolves aliases and coerces types in raw JSON arguments.
func patchArgs(raw json.RawMessage, aliases map[string]string, ci *coerceInfo) (json.RawMessage, error) {
	if len(raw) == 0 || (len(aliases) == 0 && ci == nil) {
		return raw, nil
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw, nil // not an object, return as-is
	}

	// Resolve aliases.
	for from, to := range aliases {
		if v, ok := m[from]; ok {
			if _, hasCanonical := m[to]; !hasCanonical {
				m[to] = v
			}
			delete(m, from)
		}
	}

	// Coerce types.
	if ci != nil {
		coerceMap(m, ci)
	}

	data, err := json.Marshal(m)
	if err != nil {
		return raw, err
	}
	return data, nil
}

func coerceMap(m map[string]any, ci *coerceInfo) {
	for k, v := range m {
		if coerced := tryCoerceValue(k, v, ci); coerced != nil {
			m[k] = coerced
			v = coerced
		}
		// Recurse into nested maps.
		if nested, ok := v.(map[string]any); ok {
			coerceMap(nested, ci)
		}
		if arr, ok := v.([]any); ok {
			for i := range arr {
				if nm, ok := arr[i].(map[string]any); ok {
					coerceMap(nm, ci)
				}
			}
		}
	}
}

func tryCoerceValue(key string, val any, ci *coerceInfo) any {
	s, ok := val.(string)
	if !ok {
		return nil
	}

	if ci.intKeys[key] {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return float64(i)
		}
	}

	if ci.boolKeys[key] {
		if b, err := strconv.ParseBool(s); err == nil {
			return b
		}
	}

	if ci.jsonKeys[key] {
		s = strings.TrimSpace(s)
		if (strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) ||
			(strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) {
			var parsed any
			if err := json.Unmarshal([]byte(s), &parsed); err == nil {
				return parsed
			}
		}
	}

	return nil
}

// --- Hook into existing Tool system ---
//
// declarationParams is defined on Tool in tool.go. It carries the strict
// LLM-facing schema for typed tools created via NewTypedTool.
