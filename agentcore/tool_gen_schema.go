package agentcore

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

// --- Schema generation via reflect (no external dependency) ---

// jsonSchema represents a minimal JSON Schema subset.
type jsonSchema struct {
	Type                 string                 `json:"type,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Properties           map[string]*jsonSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties *jsonSchema            `json:"additionalProperties,omitempty"`
	Items                *jsonSchema            `json:"items,omitempty"`
	Enum                 []any                  `json:"enum,omitempty"`
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
	if t.Kind() == reflect.Pointer {
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

	// 始终生成 strict 版本缓存，保证缓存内容与调用顺序无关。
	// 若首次以 lenient=true 调用就缓存 relax 后的版本，后续 strict 调用
	// 会命中缓存拿到错误的 lenient 版本（缺少 Required 字段）。
	schema := typeToSchema(t, false)

	if t.Kind() == reflect.Struct {
		schemaCacheMu.Lock()
		// Double-check: another goroutine may have cached it while we were generating.
		if _, exists := schemaCache[t]; !exists {
			schemaCache[t] = cloneSchema(schema)
		}
		schemaCacheMu.Unlock()
	}

	// 返回前按需 relax（不污染缓存）。
	if lenient {
		cloned := cloneSchema(schema)
		relaxSchemaLocal(cloned)
		return cloned
	}
	return schema
}

func typeToSchema(t reflect.Type, lenient bool) *jsonSchema {
	switch t.Kind() {
	case reflect.String:
		return &jsonSchema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &jsonSchema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
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
			Type:                 "object",
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
		return &jsonSchema{} // 序列化失败 → 返回空 schema，调用方容忍空结构
	}
	var cloned jsonSchema
	if err := json.Unmarshal(data, &cloned); err != nil {
		return &jsonSchema{} // 反序列化失败 → 同样降级为空 schema
	}
	return &cloned
}

func schemaToMap(s *jsonSchema) map[string]any {
	if s == nil {
		return map[string]any{"type": "object"}
	}
	data, err := json.Marshal(s)
	if err != nil {
		return map[string]any{"type": "object"} // 序列化失败 → 降级为最简 schema map
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]any{"type": "object"} // 反序列化失败 → 同样降级
	}
	return m
}
