package doctmpl

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// VarType is the data type of a template variable.
type VarType string

const (
	VarString    VarType = "string"    // 单行字符串（默认）
	VarMultiline VarType = "multiline" // 多行文本
	VarNumber    VarType = "number"    // 数值
	VarBool      VarType = "bool"      // 布尔值
)

// VarDefinition describes the constraints on a single template variable.
type VarDefinition struct {
	Name        string `yaml:"name"`        // 变量名（不含 {{}} 包裹）
	Type        VarType `yaml:"type"`        // 数据类型
	Required    bool   `yaml:"required"`    // 是否必填
	Default     string `yaml:"default"`     // 默认值
	Description string `yaml:"description"` // 变量含义描述
}

// VarSchema is the complete variable constraint set for a template,
// parsed from the frontmatter vars section.
type VarSchema struct {
	Definitions []VarDefinition
	byName      map[string]int // name → Definitions index
}

// NewVarSchema builds a VarSchema from a list of definitions.
// Duplicate names keep the last definition.
func NewVarSchema(defs []VarDefinition) *VarSchema {
	s := &VarSchema{
		Definitions: defs,
		byName:      make(map[string]int, len(defs)),
	}
	for i := range defs {
		s.byName[defs[i].Name] = i
	}
	return s
}

// Empty reports whether the schema has no variable definitions.
func (s *VarSchema) Empty() bool {
	return s == nil || len(s.Definitions) == 0
}

// Names returns all variable names in definition order.
func (s *VarSchema) Names() []string {
	names := make([]string, len(s.Definitions))
	for i, d := range s.Definitions {
		names[i] = d.Name
	}
	return names
}

// RequiredNames returns names of all required variables.
func (s *VarSchema) RequiredNames() []string {
	var names []string
	for _, d := range s.Definitions {
		if d.Required {
			names = append(names, d.Name)
		}
	}
	return names
}

// Get returns the definition for name, if present.
func (s *VarSchema) Get(name string) (VarDefinition, bool) {
	if s == nil {
		return VarDefinition{}, false
	}
	idx, ok := s.byName[name]
	if !ok {
		return VarDefinition{}, false
	}
	return s.Definitions[idx], true
}

// VarError describes a validation issue for a single variable.
type VarError struct {
	Variable string // 变量名
	Code     string // "missing_required" | "invalid_type" | "unresolved"
	Message  string // 人类可读描述
}

func (e VarError) Error() string {
	return fmt.Sprintf("%s: %s", e.Variable, e.Message)
}

// Validate checks the provided variables against the schema.
// Returns warnings (non-blocking) for missing required vars and type mismatches.
func (s *VarSchema) Validate(vars map[string]string) []VarError {
	if s.Empty() {
		return nil
	}
	var errs []VarError
	for _, d := range s.Definitions {
		val, provided := vars[d.Name]
		if d.Required && (!provided || strings.TrimSpace(val) == "") {
			errs = append(errs, VarError{
				Variable: d.Name,
				Code:     "missing_required",
				Message:  "必填变量未提供",
			})
			continue
		}
		if !provided || val == "" {
			continue
		}
		switch d.Type {
		case VarNumber:
			if _, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err != nil {
				errs = append(errs, VarError{
					Variable: d.Name,
					Code:     "invalid_type",
					Message:  fmt.Sprintf("期望 number 类型，实际值: %q", val),
				})
			}
		case VarBool:
			v := strings.TrimSpace(strings.ToLower(val))
			if v != "true" && v != "false" {
				errs = append(errs, VarError{
					Variable: d.Name,
					Code:     "invalid_type",
					Message:  fmt.Sprintf("期望 bool 类型 (true/false)，实际值: %q", val),
				})
			}
		}
	}
	return errs
}

// ApplyDefaults returns a new map with default values applied for
// any variable that has a Default and is missing from vars.
func (s *VarSchema) ApplyDefaults(vars map[string]string) map[string]string {
	if s.Empty() {
		return vars
	}
	result := make(map[string]string, len(vars)+len(s.Definitions))
	for k, v := range vars {
		result[k] = v
	}
	for _, d := range s.Definitions {
		if _, exists := result[d.Name]; !exists && d.Default != "" {
			result[d.Name] = d.Default
		}
	}
	return result
}

var placeholderRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// ExtractPlaceholders returns all unique {{variable}} names found in body.
func ExtractPlaceholders(body string) []string {
	matches := placeholderRe.FindAllStringSubmatch(body, -1)
	seen := make(map[string]bool, len(matches))
	var names []string
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// findResidualPlaceholders returns all {{variable}} names still present
// in the output after variable substitution.
func findResidualPlaceholders(output string) []string {
	return ExtractPlaceholders(output)
}

// ResolveResult contains the output of a validated variable resolution.
type ResolveResult struct {
	Output   string     // 渲染后的文本
	Warnings []VarError // 校验警告（非阻塞）
	Residual []string   // 残留的 {{变量名}} 列表
}

// ValidatedResolve performs validate → apply defaults → resolve → residual
// detection in one pipeline.
func ValidatedResolve(tmpl DocTemplate, vars map[string]string) ResolveResult {
	result := ResolveResult{}

	// 1. Validate and apply defaults.
	if tmpl.VarSchema != nil && !tmpl.VarSchema.Empty() {
		result.Warnings = tmpl.VarSchema.Validate(vars)
		vars = tmpl.VarSchema.ApplyDefaults(vars)
	}

	// 2. Resolve.
	result.Output = ResolveDoc(tmpl, vars)

	// 3. Detect residuals.
	result.Residual = findResidualPlaceholders(result.Output)

	return result
}
