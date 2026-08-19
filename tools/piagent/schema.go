package piagent

import (
	"encoding/json"
	"fmt"

	"github.com/sky-valley/pi/ai"
)

// schemaFromMap 将 Mady agentcore.Tool.Parameters（标准 JSON Schema，map 形态）
// 转换为 pi ai.Schema。pi 的 Schema 覆盖 JSON Schema 2020-12 常用关键字；
// 未知关键字透传到 Extra，不丢失信息。
//
// 降级策略：遇到无法表示的结构（如 $ref）返回错误，由调用方跳过该工具并 WARN，
// 不阻塞子会话派发（03-design.md §1.4）。
func schemaFromMap(raw map[string]any) (*ai.Schema, error) {
	if raw == nil {
		return nil, nil
	}
	// 先走 JSON 序列化保证类型收敛（int/float64/string/[]any/map），
	// 再递归映射，避免 map[string]any 值类型漂移。
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("schema marshal: %w", err)
	}
	var node map[string]any
	if err := json.Unmarshal(b, &node); err != nil {
		return nil, fmt.Errorf("schema unmarshal: %w", err)
	}
	return schemaNode(node)
}

func schemaNode(node map[string]any) (*ai.Schema, error) { //nolint:gocognit // 工具构造/平台后端分支逻辑，拆分收益低，保持豁免
	s := &ai.Schema{}
	for k, v := range node {
		switch k {
		case "type":
			t, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("schema type must be string, got %T", v)
			}
			s.Type = t
		case "description":
			s.Description, _ = v.(string)
		case "properties":
			props, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("schema properties must be object, got %T", v)
			}
			s.Properties = make(map[string]*ai.Schema, len(props))
			for name, pv := range props {
				pm, ok := pv.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("schema property %q must be object, got %T", name, pv)
				}
				child, err := schemaNode(pm)
				if err != nil {
					return nil, fmt.Errorf("property %q: %w", name, err)
				}
				s.Properties[name] = child
				s.PropertyOrder = append(s.PropertyOrder, name)
			}
		case "required":
			reqs, ok := v.([]any)
			if !ok {
				return nil, fmt.Errorf("schema required must be array, got %T", v)
			}
			for _, r := range reqs {
				if rs, ok := r.(string); ok {
					s.Required = append(s.Required, rs)
				}
			}
		case "items":
			im, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("schema items must be object, got %T", v)
			}
			child, err := schemaNode(im)
			if err != nil {
				return nil, fmt.Errorf("items: %w", err)
			}
			s.Items = child
		case "enum":
			evals, ok := v.([]any)
			if !ok {
				return nil, fmt.Errorf("schema enum must be array, got %T", v)
			}
			s.Enum = evals
		case "default":
			s.Default = v
		case "minimum":
			s.Minimum = floatPtrOf(v)
		case "maximum":
			s.Maximum = floatPtrOf(v)
		case "exclusiveMinimum":
			s.ExclusiveMinimum = floatPtrOf(v)
		case "exclusiveMaximum":
			s.ExclusiveMaximum = floatPtrOf(v)
		case "multipleOf":
			s.MultipleOf = floatPtrOf(v)
		case "minLength":
			s.MinLength = intPtrOf(v)
		case "maxLength":
			s.MaxLength = intPtrOf(v)
		case "pattern":
			s.Pattern, _ = v.(string)
		case "minItems":
			s.MinItems = intPtrOf(v)
		case "maxItems":
			s.MaxItems = intPtrOf(v)
		case "format":
			s.Format, _ = v.(string)
		case "const":
			s.Const = v
			s.HasConst = true
		case "anyOf":
			children, err := schemaList(v)
			if err != nil {
				return nil, fmt.Errorf("anyOf: %w", err)
			}
			s.AnyOf = children
		case "oneOf":
			children, err := schemaList(v)
			if err != nil {
				return nil, fmt.Errorf("oneOf: %w", err)
			}
			s.OneOf = children
		case "allOf":
			children, err := schemaList(v)
			if err != nil {
				return nil, fmt.Errorf("allOf: %w", err)
			}
			s.AllOf = children
		case "additionalProperties":
			switch av := v.(type) {
			case bool:
				s.AdditionalAllowed = &av
			case map[string]any:
				child, err := schemaNode(av)
				if err != nil {
					return nil, fmt.Errorf("additionalProperties: %w", err)
				}
				s.AdditionalSchema = child
			default:
				return nil, fmt.Errorf("additionalProperties must be bool or object, got %T", v)
			}
		case "$ref":
			return nil, fmt.Errorf("unsupported schema keyword $ref: %v", v)
		case "nullable":
			if nb, ok := v.(bool); ok {
				s.Nullable = nb
			}
		default:
			if s.Extra == nil {
				s.Extra = map[string]any{}
			}
			s.Extra[k] = v
		}
	}
	return s, nil
}

func schemaList(v any) ([]*ai.Schema, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("must be array, got %T", v)
	}
	out := make([]*ai.Schema, 0, len(arr))
	for i, item := range arr {
		im, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("item %d must be object, got %T", i, item)
		}
		child, err := schemaNode(im)
		if err != nil {
			return nil, err
		}
		out = append(out, child)
	}
	return out, nil
}

func floatPtrOf(v any) *float64 {
	f, ok := toFloat64(v)
	if !ok {
		return nil
	}
	return &f
}

func intPtrOf(v any) *int {
	i, ok := toInt(v)
	if !ok {
		return nil
	}
	return &i
}

func toFloat64(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	}
	return 0, false
}

func toInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case json.Number:
		i, err := t.Int64()
		return int(i), err == nil
	}
	return 0, false
}
