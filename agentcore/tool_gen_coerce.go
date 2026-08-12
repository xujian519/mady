package agentcore

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

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
		// 非 object（array/string/null 等）：best-effort 透传，不在此处报错。
		// 后续 json.Unmarshal 到具体 struct 会给出明确的类型错误，
		// 调用方仍能定位失败原因。保持 passthrough 契约以兼容非 object 参数。
		return raw, nil
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
		// 优先按整数解析；仅当 ParseFloat 结果是整数值时才转换，
		// 避免 "1.5" 被强转为 float64 后无法 Unmarshal 到 int 字段。
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return float64(i)
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil && f == float64(int64(f)) {
			return f
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
